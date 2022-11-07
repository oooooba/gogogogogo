package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

func main() {
	filename := flag.String("i", "/dev/stdin", "input file")
	flag.Parse()

	cfg := packages.Config{Mode: packages.LoadAllSyntax}
	initPkgs, err := packages.Load(&cfg, *filename)
	if err != nil {
		log.Fatal(err)
	}
	prog, _ := ssautil.AllPackages(initPkgs, ssa.SanityCheckFunctions)
	prog.Build()

	ctx := Context{
		stream:           os.Stdout,
		latestNameMap:    make(map[*ssa.BasicBlock]string),
		signatureNameSet: make(map[string]struct{}),
	}

	if false {
		var keywords []string
		ctx.visitAllFunctions(prog, func(function *ssa.Function) {
			for _, keyword := range keywords {
				if strings.Contains(function.Name(), keyword) {
					function.WriteTo(os.Stderr)
					break
				}
			}
		})
	}

	ctx.emitProgram(prog)
}

type Context struct {
	stream           *os.File
	foundValueSet    map[ssa.Value]struct{}
	latestNameMap    map[*ssa.BasicBlock]string
	signatureNameSet map[string]struct{}
}

func extractTestTargetFunctions(f *ssa.Function) []*ssa.Function {
	targets := make([]*ssa.Function, 0)
	for _, instr := range f.Blocks[0].Instrs {
		if callInstr, ok := instr.(*ssa.Call); ok {
			callCommong := callInstr.Common()
			for _, arg := range callCommong.Args {
				if targetFunction, ok := arg.(*ssa.Function); ok {
					targets = append(targets, targetFunction)
					break
				}
			}
		}
	}
	return targets
}

func encode(str string) string {
	buf := ""
	for _, c := range str {
		s := ""
		switch c {
		case '_':
			s = "___"
		case '$':
			s = "_S_"
		default:
			s = string(c)
		}
		buf += s
	}
	return buf
}

func wrapInFunctionObject(s string) string {
	return fmt.Sprintf("(struct FunctionObject){.func_ptr=%s}", s)
}

func createValueName(value ssa.Value) string {
	if val, ok := value.(*ssa.Const); ok {
		if val.IsNil() {
			switch val.Type().Underlying().(type) {
			case *types.Slice:
				return "(struct Slice){0}"
			default:
				panic(fmt.Sprintf("unimplemented: %s: %s", val.String(), val.Type()))
			}
		} else {
			return val.Value.String()
		}
	} else if val, ok := value.(*ssa.Function); ok {
		return fmt.Sprintf("%s", wrapInFunctionObject(createFunctionName(val)))
	} else if val, ok := value.(*ssa.Parameter); ok {
		for i, param := range val.Parent().Params {
			if val.Name() == param.Name() {
				return fmt.Sprintf("param%d", i)
			}
		}
		panic(fmt.Sprintf("unreachable: val=%s, params=%v", val, val.Parent().Params))
	} else {
		parentName := value.Parent().Name()
		return encode(fmt.Sprintf("v$%s$%s$%p", value.Name(), parentName, value))
	}
}

func createValueRelName(value ssa.Value) string {
	if _, ok := value.(*ssa.Const); ok {
		return createValueName(value)
	} else if _, ok := value.(*ssa.Function); ok {
		return createValueName(value)
	} else if _, ok := value.(*ssa.Parameter); ok {
		return fmt.Sprintf("frame->signature.%s", createValueName(value))
	} else if _, ok := value.(*ssa.FreeVar); ok {
		return fmt.Sprintf("((struct FreeVars_%s*)frame->common.free_vars)->%s",
			createFunctionName(value.Parent()), createValueName(value))
	} else {
		return fmt.Sprintf("frame->%s", createValueName(value))
	}
}

func createTypeName(typ types.Type) string {
	switch t := typ.Underlying().(type) {
	case *types.Array:
		return fmt.Sprintf("%s_arr%d", createTypeName(t.Elem()), t.Len())
	case *types.Basic:
		switch t.Kind() {
		case types.Bool:
			return fmt.Sprintf("bool")
		case types.Int, types.Int8, types.Int16, types.Int32, types.Int64, types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
			return fmt.Sprintf("intptr_t")
		}
	case *types.Chan:
		return fmt.Sprintf("Channel_ptr")
	case *types.Pointer:
		elemType := t.Elem().Underlying()
		if et, ok := elemType.(*types.Array); ok {
			elemType = et.Elem()
		}
		return fmt.Sprintf("%s_ptr", createTypeName(elemType))
	case *types.Signature:
		return fmt.Sprintf("FunctionObject")
	case *types.Slice:
		return fmt.Sprintf("Slice")
	case *types.Struct:
		return encode(fmt.Sprintf("t$%p", t))
	}
	panic(fmt.Sprintf("type not supported: %s", typ.String()))
}

func createType(typ types.Type, id string) string {
	switch t := typ.Underlying().(type) {
	case *types.Array:
		return fmt.Sprintf("%s %s[%d]", createType(t.Elem(), ""), id, t.Len())
	case *types.Basic:
		return fmt.Sprintf("%s %s", createTypeName(t), id)
	case *types.Chan:
		return fmt.Sprintf("struct Channel* %s", id)
	case *types.Pointer:
		elemType := t.Elem().Underlying()
		if et, ok := elemType.(*types.Array); ok {
			elemType = et.Elem()
		}
		return fmt.Sprintf("%s* %s", createType(elemType, ""), id)
	case *types.Signature, *types.Slice, *types.Struct:
		return fmt.Sprintf("struct %s %s", createTypeName(t), id)
	}
	panic(fmt.Sprintf("type not supported: %s", typ.String()))
}

func (ctx *Context) switchFunction(nextFunction string, callCommon *ssa.CallCommon, result string, resumeFunction string) {
	fmt.Fprintf(ctx.stream, "struct StackFrameCommon* next_frame = (struct StackFrameCommon*)(frame + 1);\n")
	fmt.Fprintf(ctx.stream, "assert(((uintptr_t)next_frame) %% sizeof(uintptr_t) == 0);\n")
	fmt.Fprintf(ctx.stream, "next_frame->resume_func = %s;\n", wrapInFunctionObject(resumeFunction))
	fmt.Fprintf(ctx.stream, "next_frame->prev_stack_pointer = ctx->stack_pointer;\n")

	signature := callCommon.Value.Type().(*types.Signature)

	if signature.Results().Len() > 0 || signature.Params().Len() > 0 {
		signatureName := createSignatureName(signature)
		fmt.Fprintf(ctx.stream, "%s* signature = (%s*)(next_frame + 1);\n", signatureName, signatureName)
	}

	if signature.Results().Len() > 0 {
		if signature.Results().Len() != 1 {
			panic("only 0 or 1 return value supported")
		}
		fmt.Fprintf(ctx.stream, "signature->result_ptr = &%s;\n", result)
	}

	base := 0
	if signature.Recv() != nil {
		arg := callCommon.Args[base]
		fmt.Fprintf(ctx.stream, "signature->param%d = %s; // receiver: %s\n",
			base, createValueRelName(arg), signature.Recv())
		base++
	}

	for i := 0; i < signature.Params().Len(); i++ {
		arg := callCommon.Args[base+i]
		fmt.Fprintf(ctx.stream, "signature->param%d = %s; // %s\n",
			base+i, createValueRelName(arg), signature.Params().At(i))
	}

	fmt.Fprintf(ctx.stream, "next_frame->free_vars = NULL;\n")
	fmt.Fprintf(ctx.stream, "ctx->stack_pointer = next_frame;\n")
	fmt.Fprintf(ctx.stream, "return %s;\n", nextFunction)
}

type paramArgPair struct {
	param string
	arg   string
}

func (ctx *Context) switchFunctionToCallRuntimeApi(nextFunction string, nextFunctionFrame string, resumeFunction string,
	resultPtr *string, variableSizeFrameHandler func(), paramArgPairs ...paramArgPair) {
	fmt.Fprintf(ctx.stream, "struct %s* next_frame = (struct %s*)(frame + 1);\n", nextFunctionFrame, nextFunctionFrame)
	fmt.Fprintf(ctx.stream, "assert(((uintptr_t)next_frame) %% sizeof(uintptr_t) == 0);\n")
	fmt.Fprintf(ctx.stream, "next_frame->common.resume_func = %s;\n", wrapInFunctionObject(resumeFunction))
	fmt.Fprintf(ctx.stream, "next_frame->common.prev_stack_pointer = ctx->stack_pointer;\n")

	if resultPtr != nil {
		fmt.Fprintf(ctx.stream, "next_frame->result_ptr = &%s;\n", *resultPtr)
	}
	for i, pair := range paramArgPairs {
		fmt.Fprintf(ctx.stream, "next_frame->%s = %s; // [%d]\n", pair.param, pair.arg, i)
	}

	if variableSizeFrameHandler != nil {
		variableSizeFrameHandler()
	}

	fmt.Fprintf(ctx.stream, "ctx->stack_pointer = (struct StackFrameCommon*)next_frame;\n")
	fmt.Fprintf(ctx.stream, "return %s;\n", wrapInFunctionObject(nextFunction))
}

func (ctx *Context) emitInstruction(instruction ssa.Instruction) {
	fmt.Fprintf(ctx.stream, "\t// %T instruction\n", instruction)
	switch instr := instruction.(type) {
	case *ssa.Alloc:
		if instr.Heap {
			result := createValueRelName(instr)
			ctx.switchFunctionToCallRuntimeApi("gox5_new", "StackFrameNew", createInstructionName(instr), &result, nil,
				paramArgPair{param: "size", arg: fmt.Sprintf("sizeof(%s)", createType(instr.Type().Underlying().(*types.Pointer).Elem(), ""))},
			)
		} else {
			address_of_op := "&"
			if _, ok := instr.Type().Underlying().(*types.Pointer).Elem().(*types.Array); ok {
				address_of_op = ""
			}
			fmt.Fprintf(ctx.stream, `
	%s = %s%s_buf;
	memset(%s, 0, sizeof(%s_buf));
`, createValueRelName(instr), address_of_op, createValueRelName(instr), createValueRelName(instr), createValueRelName(instr))
		}

	case *ssa.BinOp:
		switch op := instr.Op; op {
		case token.EQL, token.NEQ:
			if instr.X.Type().Underlying() != instr.Y.Type().Underlying() {
				panic(fmt.Sprintf("type mismatch: %s (%s) vs %s (%s)", instr.X, instr.X.Type(), instr.Y, instr.Y.Type()))
			}
			switch instr.X.Type().Underlying().(type) {
			case *types.Slice:
				fmt.Fprintf(ctx.stream, "%s = memcmp(&%s, &%s, sizeof(%s)) %s 0;\n", createValueRelName(instr), createValueRelName(instr.X), createValueRelName(instr.Y), createValueRelName(instr.X), instr.Op)
			default:
				fmt.Fprintf(ctx.stream, "%s = %s %s %s;\n", createValueRelName(instr), createValueRelName(instr.X), instr.Op.String(), createValueRelName(instr.Y))
			}
		default:
			fmt.Fprintf(ctx.stream, "%s = %s %s %s;\n", createValueRelName(instr), createValueRelName(instr.X), instr.Op.String(), createValueRelName(instr.Y))
		}

	case *ssa.Call:
		callCommon := instr.Common()
		if callCommon.Method != nil {
			panic("method not supported")
		}

		switch callee := callCommon.Value.(type) {
		case *ssa.Builtin:
			switch callee.Name() {
			case "append":
				result := createValueRelName(instr)
				ctx.switchFunctionToCallRuntimeApi("gox5_append", "StackFrameAppend", createInstructionName(instr), &result, nil,
					paramArgPair{param: "base", arg: fmt.Sprintf("%s", createValueRelName(callCommon.Args[0]))},
					paramArgPair{param: "elements", arg: fmt.Sprintf("%s", createValueRelName(callCommon.Args[1]))},
				)
				return
			case "cap":
				fmt.Fprintf(ctx.stream, "%s = %s.capacity;\n", createValueRelName(instr), createValueRelName(callCommon.Args[0]))
			case "len":
				fmt.Fprintf(ctx.stream, "%s = %s.size;\n", createValueRelName(instr), createValueRelName(callCommon.Args[0]))
			case "ssa:wrapnilchk":
				fmt.Fprintf(ctx.stream, "assert(%s); // ssa:wrapnilchk\n", createValueRelName(callCommon.Args[0]))
				fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), createValueRelName(callCommon.Args[0]))
			default:
				panic(fmt.Sprintf("unsuported builtin function: %s", callee.Name()))
			}
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))

		case *ssa.Function:
			nextFunction := createValueName(callee)
			ctx.switchFunction(nextFunction, callCommon, createValueRelName(instr), createInstructionName(instr))

		case *ssa.MakeClosure, *ssa.Parameter:
			nextFunction := createValueRelName(callee)
			ctx.switchFunction(nextFunction, callCommon, createValueRelName(instr), createInstructionName(instr))

		default:
			panic(fmt.Sprintf("unknown callee: %s, %T", callee, callee))
		}

	case *ssa.FieldAddr:
		fmt.Fprintf(ctx.stream, "%s = &%s->%s;\n", createValueRelName(instr), createValueRelName(instr.X), instr.X.Type().Underlying().(*types.Pointer).Elem().Underlying().(*types.Struct).Field(instr.Field).Name())

	case *ssa.IndexAddr:
		if _, ok := instr.X.Type().Underlying().(*types.Slice); ok {
			fmt.Fprintf(ctx.stream, "%s = &((%s)%s.addr)[%s];\n", createValueRelName(instr), createType(instr.Type(), ""), createValueRelName(instr.X), createValueRelName(instr.Index))
		} else {
			fmt.Fprintf(ctx.stream, "%s = &%s[%s];\n", createValueRelName(instr), createValueRelName(instr.X), createValueRelName(instr.Index))
		}

	case *ssa.Go:
		callCommon := instr.Common()
		if callCommon.Method != nil {
			panic("method not supported")
		}

		var functionObject string
		var signature *types.Signature
		switch callee := callCommon.Value.(type) {
		case *ssa.Function:
			functionObject = createValueName(callee)
			signature = callee.Signature
		case ssa.Value:
			functionObject = createValueRelName(callee)
			signature = callee.Type().(*types.Signature)
		default:
			panic(fmt.Sprintf("unknown callee: %s, %s, %T, %T", instr, callee, instr, callee))
		}

		resultSize := "0"
		if signature.Results().Len() > 0 {
			if signature.Results().Len() != 1 {
				panic("only 0 or 1 return value supported")
			}
			resultSize = fmt.Sprintf("sizeof(%s)", createType(signature.Results().At(0).Type(), ""))
		}

		ctx.switchFunctionToCallRuntimeApi("gox5_spawn", "StackFrameSpawn", createInstructionName(instr), nil,
			func() {
				fmt.Fprintf(ctx.stream, "intptr_t num_arg_buffer_words = 0;\n")
				for i, arg := range callCommon.Args {
					argValue := createValueRelName(arg)
					argType := createType(arg.Type(), "")
					fmt.Fprintf(ctx.stream, "*(%s*)&next_frame->arg_buffer[num_arg_buffer_words] = %s; // param[%d]\n", argType, argValue, i)
					fmt.Fprintf(ctx.stream, "num_arg_buffer_words += sizeof(%s) / sizeof(intptr_t);\n", argType)
				}
				fmt.Fprintf(ctx.stream, "next_frame->num_arg_buffer_words = num_arg_buffer_words;\n")
			},
			paramArgPair{param: "function_object", arg: functionObject},
			paramArgPair{param: "result_size", arg: resultSize},
		)

	case *ssa.If:
		fmt.Fprintf(ctx.stream, "\treturn %s ? %s : %s;\n", createValueRelName(instr.Cond),
			wrapInFunctionObject(createBasicBlockName(instr.Block().Succs[0])),
			wrapInFunctionObject(createBasicBlockName(instr.Block().Succs[1])))

	case *ssa.Jump:
		fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createBasicBlockName(instr.Block().Succs[0])))

	case *ssa.MakeChan:
		result := createValueRelName(instr)
		ctx.switchFunctionToCallRuntimeApi("gox5_make_chan", "StackFrameMakeChan", createInstructionName(instr), &result, nil,
			paramArgPair{param: "size", arg: createValueRelName(instr.Size)},
		)

	case *ssa.MakeClosure:
		fn := instr.Fn.(*ssa.Function)
		if len(fn.FreeVars) != len(instr.Bindings) {
			panic(fmt.Sprintf("invalid closure invocation: freeVars=%s, bindings=%s", fn, instr.Bindings))
		}
		result := createValueRelName(instr)
		userFunction := fmt.Sprintf("(struct UserFunction){.func_ptr = %s}", createFunctionName(fn))
		ctx.switchFunctionToCallRuntimeApi("gox5_make_closure", "StackFrameMakeClosure", createInstructionName(instr), &result,
			func() {
				fnName := createFunctionName(fn)
				fmt.Fprintf(ctx.stream, "struct FreeVars_%s* free_vars = (struct FreeVars_%s*)&next_frame->object_ptrs;\n", fnName, fnName)
				for i, freeVar := range fn.FreeVars {
					val := instr.Bindings[i]
					fmt.Fprintf(ctx.stream, "free_vars->%s = %s;\n", createValueName(freeVar), createValueRelName(val))
				}
				fmt.Fprintf(ctx.stream, "next_frame->num_object_ptrs = sizeof(*free_vars) / sizeof(intptr_t);\n")
			},
			paramArgPair{param: "user_function", arg: userFunction},
		)

	case *ssa.Phi:
		basicBlock := instr.Block()
		for i, edge := range instr.Edges {
			fmt.Fprintf(ctx.stream, "\tif (ctx->prev_func.func_ptr == %s) { %s = %s; } else\n",
				ctx.latestNameMap[basicBlock.Preds[i]], createValueRelName(instr), createValueRelName(edge))
		}
		fmt.Fprintln(ctx.stream, "\t{ assert(false); }")

	case *ssa.Return:
		fmt.Fprintf(ctx.stream, "ctx->stack_pointer = frame->common.prev_stack_pointer;\n")
		if len(instr.Results) > 0 {
			if len(instr.Results) != 1 {
				panic("only 0 or 1 return value supported")
			}
			fmt.Fprintf(ctx.stream, "*frame->signature.result_ptr = %s;\n", createValueRelName(instr.Results[0]))
		}
		fmt.Fprintf(ctx.stream, "return frame->common.resume_func;\n")

	case *ssa.Send:
		ctx.switchFunctionToCallRuntimeApi("gox5_send", "StackFrameSend", createInstructionName(instr), nil, nil,
			paramArgPair{param: "channel", arg: createValueRelName(instr.Chan)},
			paramArgPair{param: "data", arg: createValueRelName(instr.X)},
		)

	case *ssa.Slice:
		fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(struct Slice));\n", createValueRelName(instr))
		startIndex := "0"
		if instr.Low != nil {
			startIndex = createValueRelName(instr.Low)
		}

		length := "0"
		switch elemType := instr.X.Type().(*types.Pointer).Elem().(type) {
		case *types.Array:
			length = fmt.Sprintf("%d", elemType.Len())
		default:
			panic(fmt.Sprintf("not implemented: %s", elemType))
		}

		endIndex := length
		if instr.High != nil {
			endIndex = createValueRelName(instr.High)
		}

		fmt.Fprintf(ctx.stream, "%s.addr = %s + %s;\n", createValueRelName(instr), createValueRelName(instr.X), startIndex)
		fmt.Fprintf(ctx.stream, "%s.size = %s - %s;\n", createValueRelName(instr), endIndex, startIndex)
		fmt.Fprintf(ctx.stream, "%s.capacity = %s - %s;\n", createValueRelName(instr), length, startIndex)

	case *ssa.Store:
		if _, ok := instr.Val.Type().Underlying().(*types.Array); ok {
			fmt.Fprintf(ctx.stream, "memcpy(%s, %s, sizeof(%s));\n", createValueRelName(instr.Addr), createValueRelName(instr.Val), createValueRelName(instr.Val))
		} else {
			fmt.Fprintf(ctx.stream, "*%s = %s;\n", createValueRelName(instr.Addr), createValueRelName(instr.Val))
		}

	case *ssa.UnOp:
		if instr.Op == token.ARROW {
			result := createValueRelName(instr)
			ctx.switchFunctionToCallRuntimeApi("gox5_recv", "StackFrameRecv", createInstructionName(instr), &result, nil,
				paramArgPair{param: "channel", arg: createValueRelName(instr.X)},
			)
		} else if instr.Op == token.MUL {
			fmt.Fprintf(ctx.stream, "memcpy(&%s, %s, sizeof(%s));\n", createValueRelName(instr), createValueRelName(instr.X), createType(instr.Type(), ""))
		} else {
			fmt.Fprintf(ctx.stream, "%s = %s %s;\n", createValueRelName(instr), instr.Op.String(), createValueRelName(instr.X))
		}

	default:
		panic(fmt.Sprintf("unknown instruction: %s", instruction.String()))
	}
}

func createInstructionName(instruction ssa.Instruction) string {
	return encode(fmt.Sprintf("i$%s$%s$%p", instruction.Block().String(), instruction.Parent().Name(), instruction))
}

func createBasicBlockName(basicBlock *ssa.BasicBlock) string {
	return encode(fmt.Sprintf("b$%s$%s$%p", basicBlock.String(), basicBlock.Parent().Name(), basicBlock))
}

func createFunctionName(function *ssa.Function) string {
	methodType := ""
	if function.Signature.Recv() != nil {
		methodType = fmt.Sprintf("$%s", createTypeName(function.Signature.Recv().Type()))
	}
	return encode(fmt.Sprintf("f$%s%s", function.Name(), methodType))
}

func (ctx *Context) emitCallCommonDeclaration(callCommon *ssa.CallCommon) {
	for _, arg := range callCommon.Args {
		ctx.emitValueDeclaration(arg)
	}
}

func (ctx *Context) emitValueDeclaration(value ssa.Value) {
	_, ok := ctx.foundValueSet[value]
	if ok {
		return
	}
	ctx.foundValueSet[value] = struct{}{}

	canEmit := true
	switch val := value.(type) {
	case *ssa.Alloc:
		if val.Heap {
			// do nothing
		} else {
			id := fmt.Sprintf("%s_buf", createValueName(value))
			fmt.Fprintf(ctx.stream, "\t%s;\n", createType(value.Type().(*types.Pointer).Elem(), id))
		}

	case *ssa.BinOp:
		ctx.emitValueDeclaration(val.X)
		ctx.emitValueDeclaration(val.Y)

	case *ssa.Call:
		ctx.emitCallCommonDeclaration(val.Common())

	case *ssa.Const:
		canEmit = false

	case *ssa.FieldAddr:
		ctx.emitValueDeclaration(val.X)

	case *ssa.FreeVar:
		canEmit = false

	case *ssa.Function:
		canEmit = false

	case *ssa.IndexAddr:
		ctx.emitValueDeclaration(val.X)
		ctx.emitValueDeclaration(val.Index)

	case *ssa.MakeChan:
		ctx.emitValueDeclaration(val.Size)

	case *ssa.MakeClosure:
		ctx.emitValueDeclaration(val.Fn)
		for _, freeVar := range val.Bindings {
			ctx.emitValueDeclaration(freeVar)
		}

	case *ssa.Parameter:
		canEmit = false

	case *ssa.Phi:
		for _, edge := range val.Edges {
			ctx.emitValueDeclaration(edge)
		}

	case *ssa.Slice:
		ctx.emitValueDeclaration(val.X)
		if val.Low != nil {
			ctx.emitValueDeclaration(val.Low)
		}
		if val.High != nil {
			ctx.emitValueDeclaration(val.High)
		}

	case *ssa.UnOp:
		ctx.emitValueDeclaration(val.X)

	default:
		panic(fmt.Sprintf("unknown value: %s : %T", value.String(), value))
	}

	if t, ok := value.Type().(*types.Tuple); ok {
		if t.Len() == 0 {
			canEmit = false
		}
	}

	fmt.Fprintf(ctx.stream, "\t// found %T: %s, %s\n", value, createValueName(value), value.String())
	if canEmit {
		id := fmt.Sprintf("%s", createValueName(value))
		fmt.Fprintf(ctx.stream, "\t%s; // %s : %s\n", createType(value.Type(), id), value.String(), value.Type())
	}
}

func requireSwitchFunction(instruction ssa.Instruction) bool {
	switch instruction.(type) {
	case *ssa.Alloc:
		return instruction.(*ssa.Alloc).Heap
	case *ssa.Call, *ssa.Go, *ssa.MakeChan, *ssa.MakeClosure, *ssa.Send:
		return true
	case *ssa.UnOp:
		if instruction.(*ssa.UnOp).Op == token.ARROW {
			return true
		}
	}
	return false
}

func createSignatureItemName(typ types.Type) string {
	switch t := typ.Underlying().(type) {
	case *types.Array:
		return fmt.Sprintf("Array%d%s", t.Len(), createSignatureItemName(t.Elem()))
	case *types.Basic:
		switch t.Kind() {
		case types.Bool:
			return "bool"
		case types.Int:
			return "intptr_t"
		}
	case *types.Chan:
		return "Channel"
	case *types.Pointer:
		return fmt.Sprintf("Pointer%s", createSignatureItemName(t.Elem()))
	case *types.Signature:
		return "FunctionObject"
	case *types.Slice:
		return "Slice"
	case *types.Struct:
		return fmt.Sprintf("Struct%p", t)
	}
	panic(fmt.Sprintf("type not supported: %s", typ.String()))
}

func createSignatureName(signature *types.Signature) string {
	name := "struct Signature$"

	name += "Params$"
	if signature.Recv() != nil {
		name += createSignatureItemName(signature.Recv().Type())
		name += "$"
	}
	for i := 0; i < signature.Params().Len(); i++ {
		name += createSignatureItemName(signature.Params().At(i).Type())
		name += "$"
	}

	name += "Results$"
	if signature.Results().Len() > 0 {
		if signature.Results().Len() != 1 {
			panic("only 0 or 1 return value supported")
		}
		name += createSignatureItemName(signature.Results().At(0).Type())
	}

	return encode(name)
}

func (ctx *Context) tryEmitSignatureDefinition(signature *types.Signature) {
	name := createSignatureName(signature)
	_, ok := ctx.signatureNameSet[name]
	if ok {
		return
	}
	ctx.signatureNameSet[name] = struct{}{}

	fmt.Fprintf(ctx.stream, "%s { /* %p */\n", name, signature)

	if signature.Results().Len() > 0 {
		if signature.Results().Len() != 1 {
			panic("only 0 or 1 return value supported")
		}
		fmt.Fprintf(ctx.stream, "\t%s* result_ptr;\n", createType(signature.Results().At(0).Type(), ""))
	}

	base := 0
	if signature.Recv() != nil {
		id := fmt.Sprintf("param%d", base)
		fmt.Fprintf(ctx.stream, "\t%s; // receiver: %s\n", createType(signature.Recv().Type(), id), signature.Recv().String())
		base++
	}

	for i := 0; i < signature.Params().Len(); i++ {
		id := fmt.Sprintf("param%d", base+i)
		fmt.Fprintf(ctx.stream, "\t%s; // parameter[%d]: %s\n", createType(signature.Params().At(i).Type(), id), i, signature.Params().At(i).String())
	}

	fmt.Fprintln(ctx.stream, "};")
}

func (ctx *Context) emitFunctionHeader(name string, end string) {
	fmt.Fprintf(ctx.stream, "struct FunctionObject %s (struct LightWeightThreadContext* ctx)%s\n", name, end)
}

func (ctx *Context) emitFunctionDeclaration(function *ssa.Function) {
	ctx.emitFunctionHeader(createFunctionName(function), ";")

	ctx.tryEmitSignatureDefinition(function.Signature)

	fmt.Fprintf(ctx.stream, "struct FreeVars_%s {\n", createFunctionName(function))
	for _, freeVar := range function.FreeVars {
		fmt.Fprintf(ctx.stream, "\t// found %T: %s, %s\n", freeVar, createValueName(freeVar), freeVar.String())
		id := fmt.Sprintf("%s", createValueName(freeVar))
		fmt.Fprintf(ctx.stream, "\t%s; // %s : %s\n", createType(freeVar.Type(), id), freeVar.String(), freeVar.Type())
	}
	fmt.Fprintln(ctx.stream, "};")

	fmt.Fprintf(ctx.stream, "struct StackFrame_%s {\n", createFunctionName(function))
	fmt.Fprintf(ctx.stream, "\tstruct StackFrameCommon common;\n")
	fmt.Fprintf(ctx.stream, "\t%s signature;\n", createSignatureName(function.Signature))

	if function.Blocks != nil {
		ctx.foundValueSet = make(map[ssa.Value]struct{})
		for _, basicBlock := range function.DomPreorder() {
			for _, instr := range basicBlock.Instrs {
				if value, ok := instr.(ssa.Value); ok {
					ctx.emitValueDeclaration(value)
				}
			}
		}
	}

	fmt.Fprintln(ctx.stream, "};")

	if function.Blocks == nil {
		return
	}

	for _, basicBlock := range function.DomPreorder() {
		name := createBasicBlockName(basicBlock)
		ctx.emitFunctionHeader(name, ";")
		ctx.latestNameMap[basicBlock] = name
		for _, instr := range basicBlock.Instrs {
			if requireSwitchFunction(instr) {
				continuation_name := createInstructionName(instr)
				ctx.emitFunctionHeader(continuation_name, ";")
				ctx.latestNameMap[basicBlock] = continuation_name
			}
		}
	}
}

func (ctx *Context) emitFunctionDefinitionPrologue(function *ssa.Function, name string) {
	ctx.emitFunctionHeader(name, "{")
	freeVarsCompareOp := "=="
	if len(function.FreeVars) != 0 {
		freeVarsCompareOp = "!="
	}
	fmt.Fprintf(ctx.stream, `
	struct StackFrame_%s* frame = (void*)ctx->stack_pointer;
	assert(frame->common.free_vars %s NULL);
`, createFunctionName(function), freeVarsCompareOp)
}

func (ctx *Context) emitFunctionDefinitionEpilogue(function *ssa.Function) {
	fmt.Fprintln(ctx.stream, "}")
}

func (ctx *Context) emitFunctionDefinition(function *ssa.Function) {
	ctx.emitFunctionHeader(createFunctionName(function), "{")
	fmt.Fprintf(ctx.stream, "\tassert(ctx->marker == 0xdeadbeef);\n")
	fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createBasicBlockName(function.Blocks[0])))
	fmt.Fprintf(ctx.stream, "}\n")

	for _, basicBlock := range function.DomPreorder() {
		ctx.emitFunctionDefinitionPrologue(function, createBasicBlockName(basicBlock))

		for _, instr := range basicBlock.Instrs {
			ctx.emitInstruction(instr)

			if requireSwitchFunction(instr) {
				ctx.emitFunctionDefinitionEpilogue(function)
				ctx.emitFunctionDefinitionPrologue(function, createInstructionName(instr))
			}
		}

		ctx.emitFunctionDefinitionEpilogue(function)
	}
}

func (ctx *Context) emitTypeDefinition(typ *ssa.Type) {
	switch typ := typ.Type().Underlying().(type) {
	case *types.Struct:
		fmt.Fprintf(ctx.stream, "%s { // %s\n", createType(typ, ""), typ)
		for i := 0; i < typ.NumFields(); i++ {
			field := typ.Field(i)
			id := fmt.Sprintf("%s", field.Name())
			fmt.Fprintf(ctx.stream, "\t%s; // %s\n", createType(field.Type(), id), field)
		}
		fmt.Fprintf(ctx.stream, "};\n")

	case *types.Interface:
		// do nothing

	default:
		panic(fmt.Sprintf("not implemented: %s", typ))
	}
}

func findMainPackage(program *ssa.Program) *ssa.Package {
	for _, pkg := range program.AllPackages() {
		if pkg.Pkg.Name() == "main" {
			return pkg
		}
	}
	panic("main package not found")
}

func (ctx *Context) visitAllFunctions(program *ssa.Program, procedure func(function *ssa.Function)) {
	var f func(function *ssa.Function)
	f = func(function *ssa.Function) {
		procedure(function)
		for _, anonFunc := range function.AnonFuncs {
			f(anonFunc)
		}
	}

	mainPkg := findMainPackage(program)
	for symbol := range mainPkg.Members {
		function, ok := mainPkg.Members[symbol].(*ssa.Function)
		if !ok {
			continue
		}

		if symbol == "main" || symbol == "init" {
			continue
		}

		f(function)
	}

	g := func(t types.Type) {
		methodSet := program.MethodSets.MethodSet(t)
		for i := 0; i < methodSet.Len(); i++ {
			function := program.MethodValue(methodSet.At(i))
			if function == nil {
				continue
			}
			f(function)
		}
	}
	for member := range mainPkg.Members {
		typ, ok := mainPkg.Members[member].(*ssa.Type)
		if !ok {
			continue
		}
		g(typ.Type())
		g(types.NewPointer(typ.Type()))
	}
}

func (ctx *Context) emitProgram(program *ssa.Program) {
	fmt.Fprint(ctx.stream, `
#include <stdbool.h>
#include <stdio.h>
#include <stdint.h>
#include <string.h>
#include <assert.h>

#define DECLARE_RUNTIME_API(name, param_type) \
	struct FunctionObject (gox5_##name)(struct LightWeightThreadContext* ctx)

struct GlobalContext;

struct UserFunction {
	const void* func_ptr;
};

struct FunctionObject {
	const void* func_ptr;
};

struct StackFrameCommon {
	struct FunctionObject resume_func;
	struct StackFrameCommon* prev_stack_pointer;
	void* free_vars;
};

struct LightWeightThreadContext {
	struct GlobalContext* global_context;
	struct StackFrameCommon* stack_pointer;
	struct UserFunction prev_func;
	intptr_t marker;
};

struct Channel;

struct Slice {
	void* addr;
	uintptr_t size;
	uintptr_t capacity;
};

struct StackFrameAppend {
	struct StackFrameCommon common;
	struct Slice* result_ptr;
	struct Slice base;
	struct Slice elements;
};
DECLARE_RUNTIME_API(append, StackFrameAppend);

struct StackFrameMakeChan {
	struct StackFrameCommon common;
	struct Channel** result_ptr;
	intptr_t size; // ToDo: correct to proper type
};
DECLARE_RUNTIME_API(make_chan, StackFrameMakeChan);

struct StackFrameMakeClosure {
	struct StackFrameCommon common;
	void* result_ptr;
	struct UserFunction user_function;
	uintptr_t num_object_ptrs;
	void* object_ptrs[0];
};
DECLARE_RUNTIME_API(make_closure, StackFrameMakeClosure);

struct StackFrameNew {
	struct StackFrameCommon common;
	void* result_ptr;
	uintptr_t size;
};
DECLARE_RUNTIME_API(new, StackFrameNew);

struct StackFrameRecv {
	struct StackFrameCommon common;
	intptr_t* result_ptr;
	struct Channel* channel;
};
DECLARE_RUNTIME_API(recv, StackFrameRecv);

struct StackFrameSend {
	struct StackFrameCommon common;
	struct Channel* channel;
	intptr_t data;
};
DECLARE_RUNTIME_API(send, StackFrameSend);

struct StackFrameSpawn {
	struct StackFrameCommon common;
	struct FunctionObject function_object;
	uintptr_t result_size;
	uintptr_t num_arg_buffer_words;
	void* arg_buffer[0];
};
DECLARE_RUNTIME_API(spawn, StackFrameSpawn);
`)

	mainPkg := findMainPackage(program)

	for member := range mainPkg.Members {
		typ, ok := mainPkg.Members[member].(*ssa.Type)
		if !ok {
			continue
		}
		ctx.emitTypeDefinition(typ)
	}

	ctx.visitAllFunctions(program, func(function *ssa.Function) {
		ctx.emitFunctionDeclaration(function)
	})

	ctx.visitAllFunctions(program, func(function *ssa.Function) {
		if function.Blocks != nil {
			ctx.emitFunctionDefinition(function)
		}
	})

	fmt.Fprintln(ctx.stream, "struct { const char* name; void* function; } test_entry_points[] = {")
	for symbol := range mainPkg.Members {
		function, ok := mainPkg.Members[symbol].(*ssa.Function)
		if !ok {
			continue
		}
		if symbol != "main" {
			continue
		}
		testTargetFunctions := extractTestTargetFunctions(function)
		for _, testTargetFunction := range testTargetFunctions {
			fmt.Fprintf(ctx.stream, "{ \"%s\", %s },\n", testTargetFunction.Name(), createFunctionName(testTargetFunction))
		}
	}
	fmt.Fprintln(ctx.stream, "};")

	fmt.Fprint(ctx.stream, `
size_t test_entry_point_num(void) {
	return sizeof(test_entry_points)/sizeof(test_entry_points[0]);
}

const char* test_entry_point_name(size_t i) {
	return test_entry_points[i].name;
}

void* test_entry_point_function(size_t i) {
	return test_entry_points[i].function;
}
`)
}
