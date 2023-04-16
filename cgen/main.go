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
		program:          prog,
		foundTypeSet:     make(map[string]struct{}),
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
	program          *ssa.Program
	foundValueSet    map[ssa.Value]struct{}
	foundTypeSet     map[string]struct{}
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
		case '<':
			s = "_lt_"
		case '>':
			s = "_gt_"
		default:
			s = string(c)
		}
		buf += s
	}
	return buf
}

func wrapInBoolObject(s string) string {
	return fmt.Sprintf("(BoolObject){.raw=%s}", s)
}

func wrapInFunctionObject(s string) string {
	return fmt.Sprintf("(FunctionObject){.func_ptr=%s}", s)
}

func wrapInIntObject(s string) string {
	return fmt.Sprintf("(IntObject){.raw=%s}", s)
}

func wrapInPointerObject(s string, t types.Type) string {
	return fmt.Sprintf("(%s){.raw=%s}", createTypeName(t), s)
}

func createValueName(value ssa.Value) string {
	if val, ok := value.(*ssa.Const); ok {
		if val.IsNil() {
			switch val.Type().(type) {
			case *types.Signature:
				return wrapInFunctionObject("NULL")
			case *types.Slice:
				return fmt.Sprintf("(%s){0}", createTypeName(val.Type()))
			default:
				return wrapInPointerObject("NULL", val.Type())
			}
		} else {
			cst := val.Value.String()
			switch t := val.Type().(type) {
			case *types.Basic:
				if t.Kind() == types.String {
					return fmt.Sprintf("(StringObject){.str_ptr=%s}", cst)
				} else if t.Kind() == types.Bool {
					return wrapInBoolObject(cst)
				} else {
					return wrapInIntObject(cst)
				}

			case *types.Named:
				return fmt.Sprintf("(%s){%s}", createTypeName(val.Type()), cst)

			default:
				panic(fmt.Sprintf("val=%s, %T, %s, %T", val, val, val.Type(), val.Type()))
			}
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
	} else if _, ok := value.(*ssa.Global); ok {
		return encode(fmt.Sprintf("gv$%s$%p", value.Name(), value))
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
		return fmt.Sprintf("((FreeVars_%s*)frame->common.free_vars)->%s",
			createFunctionName(value.Parent()), createValueName(value))
	} else if _, ok := value.(*ssa.Global); ok {
		return wrapInPointerObject(fmt.Sprintf("&%s", createValueName(value)), value.Type())
	} else {
		return fmt.Sprintf("frame->%s", createValueName(value))
	}
}

func createTypeName(typ types.Type) string {
	switch t := typ.(type) {
	case *types.Array:
		return encode(fmt.Sprintf("Array<%s$%d>", createTypeName(t.Elem()), t.Len()))
	case *types.Basic:
		switch t.Kind() {
		case types.Bool:
			return fmt.Sprintf("BoolObject")
		case types.Int, types.Int8, types.Int16, types.Int32, types.Int64, types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
			return fmt.Sprintf("IntObject")
		case types.String:
			return fmt.Sprintf("StringObject")
		}
	case *types.Chan:
		return fmt.Sprintf("ChannelObject")
	case *types.Interface:
		return fmt.Sprintf("Interface")
	case *types.Named:
		// remove "command-line-arguments."
		l := strings.Split(typ.String(), ".")
		return l[len(l)-1]
	case *types.Pointer:
		elemType := t.Elem()
		if et, ok := elemType.(*types.Array); ok {
			elemType = et.Elem()
		}
		return encode(fmt.Sprintf("Pointer<%s>", createTypeName(elemType)))
	case *types.Signature:
		return fmt.Sprintf("FunctionObject")
	case *types.Slice:
		return encode(fmt.Sprintf("Slice<%s>", createTypeName(t.Elem())))
	case *types.Struct:
		return encode(fmt.Sprintf("Struct%p", t))
	case *types.Tuple:
		name := "Tuple<"
		for i := 0; i < t.Len(); i++ {
			elemType := t.At(i).Type()
			if i != 0 {
				name += "$"
			}
			name += createTypeName(elemType)
		}
		name += ">"
		return encode(name)
	}
	panic(fmt.Sprintf("type not supported: %s", typ.String()))
}

func createType(typ types.Type, id string) string {
	switch t := typ.(type) {
	case *types.Array, *types.Interface, *types.Named, *types.Pointer, *types.Signature, *types.Slice, *types.Struct, *types.Tuple:
		return fmt.Sprintf("%s %s", createTypeName(t), id)
	case *types.Basic:
		if t.Kind() == types.String {
			return fmt.Sprintf("%s %s", createTypeName(t), id)
		} else {
			return fmt.Sprintf("%s %s", createTypeName(t), id)
		}
	case *types.Chan:
		return fmt.Sprintf("ChannelObject %s", id)
	}
	panic(fmt.Sprintf("type not supported: %s", typ.String()))
}

func (ctx *Context) switchFunction(nextFunction string, callCommon *ssa.CallCommon, result string, resumeFunction string) {
	fmt.Fprintf(ctx.stream, "StackFrameCommon* next_frame = (StackFrameCommon*)(frame + 1);\n")
	fmt.Fprintf(ctx.stream, "assert(((uintptr_t)next_frame) %% sizeof(uintptr_t) == 0);\n")
	fmt.Fprintf(ctx.stream, "next_frame->resume_func = %s;\n", wrapInFunctionObject(resumeFunction))
	fmt.Fprintf(ctx.stream, "next_frame->prev_stack_pointer = ctx->stack_pointer;\n")

	var signature *types.Signature
	if callCommon.IsInvoke() {
		signature = callCommon.Method.Type().(*types.Signature)
	} else {
		signature = callCommon.Value.Type().(*types.Signature)
	}

	if callCommon.IsInvoke() || signature.Results().Len() > 0 || signature.Params().Len() > 0 {
		var signatureName string
		if callCommon.IsInvoke() {
			signatureName = createSignatureName(signature, true)
		} else {
			signatureName = createSignatureName(signature, false)
		}
		fmt.Fprintf(ctx.stream, "%s* signature = (%s*)(next_frame + 1);\n", signatureName, signatureName)
	}

	if signature.Results().Len() > 0 {
		fmt.Fprintf(ctx.stream, "signature->result_ptr = &%s;\n", result)
	}

	paramBase := 0
	argBase := 0
	if callCommon.IsInvoke() {
		arg := callCommon.Value
		if arg.Type().Underlying().(*types.Interface).Empty() {
			fmt.Fprintf(ctx.stream, "signature->param%d = %s.receiver; // receiver: %s\n",
				paramBase, createValueRelName(arg), signature.Recv())
		} else {
			fmt.Fprintf(ctx.stream, "signature->param%d = %s.receiver; // receiver: %s\n",
				paramBase, createValueRelName(arg), signature.Recv())
		}
		paramBase++
	} else if signature.Recv() != nil {
		arg := callCommon.Args[argBase]
		fmt.Fprintf(ctx.stream, "signature->param%d = %s; // receiver: %s\n",
			paramBase, createValueRelName(arg), signature.Recv())
		paramBase++
		argBase++
	}
	for i := 0; i < signature.Params().Len(); i++ {
		arg := callCommon.Args[argBase+i]
		fmt.Fprintf(ctx.stream, "signature->param%d = %s; // %s\n",
			paramBase+i, createValueRelName(arg), signature.Params().At(i))
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
	fmt.Fprintf(ctx.stream, "%s* next_frame = (%s*)(frame + 1);\n", nextFunctionFrame, nextFunctionFrame)
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

	fmt.Fprintf(ctx.stream, "ctx->stack_pointer = (StackFrameCommon*)next_frame;\n")
	fmt.Fprintf(ctx.stream, "return %s;\n", wrapInFunctionObject(nextFunction))
}

func (ctx *Context) emitInstruction(instruction ssa.Instruction) {
	fmt.Fprintf(ctx.stream, "\t// %T instruction\n", instruction)
	switch instr := instruction.(type) {
	case *ssa.Alloc:
		if instr.Heap {
			result := createValueRelName(instr)
			ctx.switchFunctionToCallRuntimeApi("gox5_new", "StackFrameNew", createInstructionName(instr), &result, nil,
				paramArgPair{param: "size", arg: fmt.Sprintf("sizeof(%s)", createType(instr.Type().(*types.Pointer).Elem(), ""))},
			)
		} else {
			fmt.Fprintf(ctx.stream, "{\n")
			v := createValueRelName(instr)
			elemType := instr.Type().(*types.Pointer).Elem()
			if t, ok := elemType.(*types.Array); ok {
				fmt.Fprintf(ctx.stream, "%s* raw = %s_buf.raw;\n", createTypeName(t.Elem()), v)
			} else {
				fmt.Fprintf(ctx.stream, "%s* raw = &%s_buf;\n", createTypeName(elemType), v)
			}
			fmt.Fprintf(ctx.stream, "memset(raw, 0, sizeof(%s_buf));\n", v)
			fmt.Fprintf(ctx.stream, "%s = %s;\n", v, wrapInPointerObject("raw", instr.Type()))
			fmt.Fprintf(ctx.stream, "}\n")
		}

	case *ssa.BinOp:
		switch op := instr.Op; op {
		case token.EQL, token.NEQ:
			if instr.X.Type() != instr.Y.Type() {
				panic(fmt.Sprintf("type mismatch: %s (%s) vs %s (%s)", instr.X, instr.X.Type(), instr.Y, instr.Y.Type()))
			}
			raw := ""
			switch t := instr.X.Type().(type) {
			case *types.Basic:
				if t.Kind() == types.String {
					raw = fmt.Sprintf("strcmp(%s.str_ptr, %s.str_ptr) %s 0", createValueRelName(instr.X), createValueRelName(instr.Y), instr.Op)
				} else {
					raw = fmt.Sprintf("%s.raw %s %s.raw", createValueRelName(instr.X), instr.Op.String(), createValueRelName(instr.Y))
				}
			case *types.Named, *types.Signature, *types.Slice:
				fmt.Fprintf(ctx.stream, "bool raw = memcmp(&%s, &%s, sizeof(%s)) %s 0;", createValueRelName(instr.X), createValueRelName(instr.Y), createValueRelName(instr.X), instr.Op)
				raw = "raw"
			case *types.Pointer:
				raw = fmt.Sprintf("%s.raw %s %s.raw", createValueRelName(instr.X), instr.Op.String(), createValueRelName(instr.Y))
			default:
				raw = fmt.Sprintf("%s %s %s", createValueRelName(instr.X), instr.Op.String(), createValueRelName(instr.Y))
			}
			fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInBoolObject(raw))
		case token.LSS, token.LEQ, token.GTR, token.GEQ:
			raw := fmt.Sprintf("%s.raw %s %s.raw", createValueRelName(instr.X), instr.Op.String(), createValueRelName(instr.Y))
			fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInBoolObject(raw))
		default:
			raw := fmt.Sprintf("%s.raw %s %s.raw", createValueRelName(instr.X), instr.Op.String(), createValueRelName(instr.Y))
			fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInIntObject(raw))
		}

	case *ssa.Call:
		callCommon := instr.Common()
		if callCommon.Method != nil {
			methodName := callCommon.Method.Name()
			fmt.Fprintf(ctx.stream, `
			FunctionObject next_function = {.func_ptr = NULL};
			Interface* interface = &%s;
			for (uintptr_t i = 0; i < interface->num_methods; ++i) {
				InterfaceTableEntry* entry = &interface->interface_table[i];
				if (strcmp(entry->method_name, "%s") == 0) {
					next_function = entry->method;
					break;
				}
			}
			assert(next_function.func_ptr != NULL);
			`, createValueRelName(callCommon.Value), methodName)
			nextFunction := "next_function"
			ctx.switchFunction(nextFunction, callCommon, createValueRelName(instr), createInstructionName(instr))
			return
		}

		switch callee := callCommon.Value.(type) {
		case *ssa.Builtin:
			switch callee.Name() {
			case "append":
				result := createValueRelName(instr)
				result += ".raw"
				ctx.switchFunctionToCallRuntimeApi("gox5_append", "StackFrameAppend", createInstructionName(instr), &result, nil,
					paramArgPair{param: "base", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
					paramArgPair{param: "elements", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[1]))},
				)
				return
			case "cap":
				fmt.Fprintf(ctx.stream, "intptr_t raw = %s.typed.capacity;", createValueRelName(callCommon.Args[0]))
				fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInIntObject("raw"))
			case "len":
				switch t := callCommon.Args[0].Type().(type) {
				case *types.Basic:
					switch t.Kind() {
					case types.String:
						raw := fmt.Sprintf("strlen(%s.str_ptr)", createValueRelName(callCommon.Args[0]))
						fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInIntObject(raw))
					default:
						panic(fmt.Sprintf("unsuported argument for len: %s (%s)", callCommon.Args[0], t))
					}
				case *types.Slice:
					fmt.Fprintf(ctx.stream, "intptr_t raw = %s.typed.size;", createValueRelName(callCommon.Args[0]))
					fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInIntObject("raw"))
				default:
					panic(fmt.Sprintf("unsuported argument for len: %s", callCommon.Args[0]))
				}
			case "ssa:wrapnilchk":
				fmt.Fprintf(ctx.stream, "assert(%s.raw); // ssa:wrapnilchk\n", createValueRelName(callCommon.Args[0]))
				fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), createValueRelName(callCommon.Args[0]))
			default:
				panic(fmt.Sprintf("unsuported builtin function: %s", callee.Name()))
			}
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))

		case *ssa.Const, *ssa.Function:
			nextFunction := createValueName(callee)
			ctx.switchFunction(nextFunction, callCommon, createValueRelName(instr), createInstructionName(instr))

		case *ssa.MakeClosure, *ssa.Parameter:
			nextFunction := createValueRelName(callee)
			ctx.switchFunction(nextFunction, callCommon, createValueRelName(instr), createInstructionName(instr))

		default:
			panic(fmt.Sprintf("unknown callee: %s, %T", callee, callee))
		}

	case *ssa.ChangeType:
		fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), createValueRelName(instr.X))

	case *ssa.Extract:
		fmt.Fprintf(ctx.stream, "%s = %s.e%d;\n", createValueRelName(instr), createValueRelName(instr.Tuple), instr.Index)

	case *ssa.FieldAddr:
		fmt.Fprintf(ctx.stream, "{\n")
		fmt.Fprintf(ctx.stream, "%s* raw = &(%s.raw->%s);\n", createTypeName(instr.Type().(*types.Pointer).Elem()), createValueRelName(instr.X), instr.X.Type().(*types.Pointer).Elem().Underlying().(*types.Struct).Field(instr.Field).Name())
		fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInPointerObject("raw", instr.Type()))
		fmt.Fprintf(ctx.stream, "}\n")

	case *ssa.IndexAddr:
		fmt.Fprintf(ctx.stream, "{\n")
		if _, ok := instr.X.Type().(*types.Slice); ok {
			fmt.Fprintf(ctx.stream, "%s* raw = &((%s.typed.ptr)[%s.raw]);\n", createTypeName(instr.X.Type().(*types.Slice).Elem()), createValueRelName(instr.X), createValueRelName(instr.Index))
		} else {
			fmt.Fprintf(ctx.stream, "%s* raw = &(%s.raw[%s.raw]);\n", createTypeName(instr.X.Type().(*types.Pointer).Elem().(*types.Array).Elem()), createValueRelName(instr.X), createValueRelName(instr.Index))
		}
		fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInPointerObject("raw", instr.Type()))
		fmt.Fprintf(ctx.stream, "}\n")

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
		switch signature.Results().Len() {
		case 0:
			// do nothing
		case 1:
			resultSize = fmt.Sprintf("sizeof(%s)", createType(signature.Results().At(0).Type(), ""))
		default:
			resultSize = fmt.Sprintf("sizeof(%s)", createType(signature.Results(), ""))
		}

		ctx.switchFunctionToCallRuntimeApi("gox5_spawn", "StackFrameSpawn", createInstructionName(instr), nil,
			func() {
				fmt.Fprintf(ctx.stream, "intptr_t num_arg_buffer_words = 0;\n")
				for i, arg := range callCommon.Args {
					argValue := createValueRelName(arg)
					argType := createType(arg.Type(), "")
					tmpVar := fmt.Sprintf("tmp%d", i)
					fmt.Fprintf(ctx.stream, "%s %s = %s; // param[%d]\n", argType, tmpVar, argValue, i)
					fmt.Fprintf(ctx.stream, "memcpy(&next_frame->arg_buffer[num_arg_buffer_words], &%s, sizeof(%s));\n", tmpVar, tmpVar)
					fmt.Fprintf(ctx.stream, "num_arg_buffer_words += sizeof(%s) / sizeof(intptr_t);\n", tmpVar)
				}
				fmt.Fprintf(ctx.stream, "next_frame->num_arg_buffer_words = num_arg_buffer_words;\n")
			},
			paramArgPair{param: "function_object", arg: functionObject},
			paramArgPair{param: "result_size", arg: resultSize},
		)

	case *ssa.If:
		fmt.Fprintf(ctx.stream, "\treturn %s.raw ? %s : %s;\n", createValueRelName(instr.Cond),
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
		userFunction := fmt.Sprintf("(UserFunction){.func_ptr = %s}", createFunctionName(fn))
		ctx.switchFunctionToCallRuntimeApi("gox5_make_closure", "StackFrameMakeClosure", createInstructionName(instr), &result,
			func() {
				fnName := createFunctionName(fn)
				fmt.Fprintf(ctx.stream, "FreeVars_%s* free_vars = (FreeVars_%s*)&next_frame->object_ptrs;\n", fnName, fnName)
				for i, freeVar := range fn.FreeVars {
					val := instr.Bindings[i]
					fmt.Fprintf(ctx.stream, "free_vars->%s = %s;\n", createValueName(freeVar), createValueRelName(val))
				}
				fmt.Fprintf(ctx.stream, "next_frame->num_object_ptrs = sizeof(*free_vars) / sizeof(intptr_t);\n")
			},
			paramArgPair{param: "user_function", arg: userFunction},
		)

	case *ssa.MakeInterface:
		valueName := createValueRelName(instr)
		interfaceTableName := fmt.Sprintf("interfaceTable_%s", createTypeName(instr.X.Type()))
		if instr.Type().Underlying().(*types.Interface).Empty() {
			switch instrX := instr.X.(type) {
			case *ssa.Const, *ssa.Function:
				id := fmt.Sprintf("tmp_%s", createValueName(instr))
				fmt.Fprintf(ctx.stream, "frame->%s = %s;\n", id, createValueRelName(instrX))
				fmt.Fprintf(ctx.stream, "%s.receiver = &frame->%s;\n", valueName, id)

			default:
				fmt.Fprintf(ctx.stream, "%s.receiver = &%s;\n", valueName, createValueRelName(instr.X))
			}
			fmt.Fprintf(ctx.stream, "%s.num_methods = 0;\n", valueName)
			typ := "NULL"
			switch t := instr.X.Type().(type) {
			case *types.Basic:
				switch t.Kind() {
				case types.Int, types.Int8, types.Int16, types.Int32, types.Int64, types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
					typ = "1"
				case types.String:
					typ = "2"
				}
			}
			fmt.Fprintf(ctx.stream, "%s.interface_table = (void*)%s;\n", valueName, typ)
		} else {
			fmt.Fprintf(ctx.stream, "%s.receiver = %s.raw;\n", valueName, createValueRelName(instr.X))
			fmt.Fprintf(ctx.stream, "%s.num_methods = sizeof(%s.entries)/sizeof(%s.entries[0]);\n", valueName, interfaceTableName, interfaceTableName)
			fmt.Fprintf(ctx.stream, "%s.interface_table = &%s.entries[0];\n", valueName, interfaceTableName)
		}

	case *ssa.Panic:
		fmt.Fprintf(ctx.stream, "fprintf(stderr, \"panic\\n\");\n")
		fmt.Fprintf(ctx.stream, "assert(false);\n")

	case *ssa.Phi:
		basicBlock := instr.Block()
		for i, edge := range instr.Edges {
			fmt.Fprintf(ctx.stream, "\tif (ctx->prev_func.func_ptr == %s) { %s = %s; } else\n",
				ctx.latestNameMap[basicBlock.Preds[i]], createValueRelName(instr), createValueRelName(edge))
		}
		fmt.Fprintln(ctx.stream, "\t{ assert(false); }")

	case *ssa.Return:
		fmt.Fprintf(ctx.stream, "ctx->stack_pointer = frame->common.prev_stack_pointer;\n")
		switch len(instr.Results) {
		case 0:
			// do nothing
		case 1:
			fmt.Fprintf(ctx.stream, "*frame->signature.result_ptr = %s;\n", createValueRelName(instr.Results[0]))
		default:
			for i, v := range instr.Results {
				fmt.Fprintf(ctx.stream, "frame->signature.result_ptr->e%d = %s;\n", i, createValueRelName(v))
			}
		}
		fmt.Fprintf(ctx.stream, "return frame->common.resume_func;\n")

	case *ssa.Send:
		ctx.switchFunctionToCallRuntimeApi("gox5_send", "StackFrameSend", createInstructionName(instr), nil, nil,
			paramArgPair{param: "channel", arg: createValueRelName(instr.Chan)},
			paramArgPair{param: "data", arg: createValueRelName(instr.X)},
		)

	case *ssa.Slice:
		fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", createValueRelName(instr), createTypeName(instr.Type()))
		startIndex := wrapInIntObject("0")
		if instr.Low != nil {
			startIndex = createValueRelName(instr.Low)
		}

		length := wrapInIntObject("0")
		switch elemType := instr.X.Type().(*types.Pointer).Elem().(type) {
		case *types.Array:
			length = wrapInIntObject(fmt.Sprintf("%d", elemType.Len()))
		default:
			panic(fmt.Sprintf("not implemented: %s", elemType))
		}

		endIndex := length
		if instr.High != nil {
			endIndex = createValueRelName(instr.High)
		}

		fmt.Fprintf(ctx.stream, "%s.typed.ptr = %s.raw + %s.raw;\n", createValueRelName(instr), createValueRelName(instr.X), startIndex)
		fmt.Fprintf(ctx.stream, "%s.typed.size = %s.raw - %s.raw;\n", createValueRelName(instr), endIndex, startIndex)
		fmt.Fprintf(ctx.stream, "%s.typed.capacity = %s.raw - %s.raw;\n", createValueRelName(instr), length, startIndex)

	case *ssa.Store:
		if _, ok := instr.Val.Type().(*types.Array); ok {
			fmt.Fprintf(ctx.stream, "memcpy(%s.raw, %s.raw, sizeof(%s));\n", createValueRelName(instr.Addr), createValueRelName(instr.Val), createValueRelName(instr.Val))
		} else {
			fmt.Fprintf(ctx.stream, "*(%s.raw) = %s;\n", createValueRelName(instr.Addr), createValueRelName(instr.Val))
		}

	case *ssa.UnOp:
		if instr.Op == token.ARROW {
			result := createValueRelName(instr)
			ctx.switchFunctionToCallRuntimeApi("gox5_recv", "StackFrameRecv", createInstructionName(instr), &result, nil,
				paramArgPair{param: "channel", arg: createValueRelName(instr.X)},
			)
		} else if instr.Op == token.MUL {
			fmt.Fprintf(ctx.stream, "memcpy(&%s, %s.raw, sizeof(%s));\n", createValueRelName(instr), createValueRelName(instr.X), createType(instr.Type(), ""))
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
		// do nothing

	case *ssa.BinOp:
		ctx.emitValueDeclaration(val.X)
		ctx.emitValueDeclaration(val.Y)

	case *ssa.Call:
		ctx.emitCallCommonDeclaration(val.Common())

	case *ssa.ChangeType:
		ctx.emitValueDeclaration(val.X)

	case *ssa.Const:
		canEmit = false

	case *ssa.Extract:
		ctx.emitValueDeclaration(val.Tuple)

	case *ssa.FieldAddr:
		ctx.emitValueDeclaration(val.X)

	case *ssa.Global:
		canEmit = false

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

	case *ssa.MakeInterface:
		ctx.emitValueDeclaration(val.X)
		if val.Type().Underlying().(*types.Interface).Empty() {
			switch valX := val.X.(type) {
			case *ssa.Const, *ssa.Function:
				id := fmt.Sprintf("tmp_%s", createValueName(val))
				fmt.Fprintf(ctx.stream, "\t%s; // %s : %s\n", createType(valX.Type(), id), valX.String(), valX.Type())
			}
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

func createSignatureName(signature *types.Signature, makesReceiverInterface bool) string {
	name := ""

	if signature.Recv() != nil && makesReceiverInterface {
		name += "Interface"
	}
	name += "Signature$"

	name += "Params$"
	if signature.Recv() != nil && !makesReceiverInterface {
		name += createTypeName(signature.Recv().Type())
		name += "$"
	}
	for i := 0; i < signature.Params().Len(); i++ {
		name += createTypeName(signature.Params().At(i).Type())
		name += "$"
	}

	name += "Results$"
	switch signature.Results().Len() {
	case 0:
		// do nothing
	case 1:
		name += createTypeName(signature.Results().At(0).Type())
	default:
		name += createTypeName(signature.Results())
	}

	return encode(name)
}

func (ctx *Context) tryEmitSignatureDefinition(signature *types.Signature, signatureName string, makesReceiverInterface bool) {
	_, ok := ctx.signatureNameSet[signatureName]
	if ok {
		return
	}
	ctx.signatureNameSet[signatureName] = struct{}{}

	fmt.Fprintf(ctx.stream, "typedef struct { /* %s */\n", signature)

	switch signature.Results().Len() {
	case 0:
		// do nothing
	case 1:
		fmt.Fprintf(ctx.stream, "\t%s* result_ptr;\n", createType(signature.Results().At(0).Type(), ""))
	default:
		fmt.Fprintf(ctx.stream, "\t%s* result_ptr;\n", createType(signature.Results(), ""))
	}

	base := 0
	if signature.Recv() != nil {
		id := fmt.Sprintf("param%d", base)
		var decl string
		if makesReceiverInterface {
			decl = fmt.Sprintf("void* %s", id)
		} else {
			decl = createType(signature.Recv().Type(), id)
		}
		fmt.Fprintf(ctx.stream, "\t%s; // receiver: %s\n", decl, signature.Recv().String())
		base++
	}

	for i := 0; i < signature.Params().Len(); i++ {
		id := fmt.Sprintf("param%d", base+i)
		fmt.Fprintf(ctx.stream, "\t%s; // parameter[%d]: %s\n", createType(signature.Params().At(i).Type(), id), i, signature.Params().At(i).String())
	}

	fmt.Fprintf(ctx.stream, "} %s;\n", signatureName)
}

func (ctx *Context) emitFunctionHeader(name string, end string) {
	fmt.Fprintf(ctx.stream, "FunctionObject %s (LightWeightThreadContext* ctx)%s\n", name, end)
}

func (ctx *Context) emitFunctionDeclaration(function *ssa.Function) {
	ctx.emitFunctionHeader(createFunctionName(function), ";")

	signature := function.Signature
	concreteSignatureName := createSignatureName(signature, false)
	ctx.tryEmitSignatureDefinition(function.Signature, concreteSignatureName, false)
	if function.Signature.Recv() != nil {
		abstructSignatureName := createSignatureName(signature, true)
		ctx.tryEmitSignatureDefinition(function.Signature, abstructSignatureName, true)
	}

	fmt.Fprintf(ctx.stream, "typedef struct {\n")
	for _, freeVar := range function.FreeVars {
		fmt.Fprintf(ctx.stream, "\t// found %T: %s, %s\n", freeVar, createValueName(freeVar), freeVar.String())
		id := fmt.Sprintf("%s", createValueName(freeVar))
		fmt.Fprintf(ctx.stream, "\t%s; // %s : %s\n", createType(freeVar.Type(), id), freeVar.String(), freeVar.Type())
	}
	fmt.Fprintf(ctx.stream, "} FreeVars_%s;\n", createFunctionName(function))

	fmt.Fprintf(ctx.stream, "typedef struct {\n")
	fmt.Fprintf(ctx.stream, "\tStackFrameCommon common;\n")
	fmt.Fprintf(ctx.stream, "\t%s signature;\n", concreteSignatureName)

	if function.Blocks != nil {
		for _, local := range function.Locals {
			if local.Heap {
				panic(fmt.Sprintf("%s", local))
			}
			id := fmt.Sprintf("%s_buf", createValueName(local))
			fmt.Fprintf(ctx.stream, "\t%s;\n", createType(local.Type().(*types.Pointer).Elem(), id))
		}

		ctx.foundValueSet = make(map[ssa.Value]struct{})
		for _, basicBlock := range function.DomPreorder() {
			for _, instr := range basicBlock.Instrs {
				if value, ok := instr.(ssa.Value); ok {
					ctx.emitValueDeclaration(value)
				}
			}
		}
	}

	fmt.Fprintf(ctx.stream, "} StackFrame_%s;\n", createFunctionName(function))

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
	StackFrame_%s* frame = (void*)ctx->stack_pointer;
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

func (ctx *Context) emitTypeDefinition(typ types.Type) {
	name := createTypeName(typ)
	_, ok := ctx.foundTypeSet[name]
	if ok {
		return
	}
	ctx.foundTypeSet[name] = struct{}{}

	switch typ := typ.(type) {
	case *types.Array:
		fmt.Fprintf(ctx.stream, "typedef struct { // %s\n", typ)
		fmt.Fprintf(ctx.stream, "\t%s raw[%d];\n", createType(typ.Elem(), ""), typ.Len())
		fmt.Fprintf(ctx.stream, "} %s;\n", name)

	case *types.Struct:
		fmt.Fprintf(ctx.stream, "typedef struct { // %s\n", typ)
		for i := 0; i < typ.NumFields(); i++ {
			field := typ.Field(i)
			id := fmt.Sprintf("%s", field.Name())
			fmt.Fprintf(ctx.stream, "\t%s; // %s\n", createType(field.Type(), id), field)
		}
		fmt.Fprintf(ctx.stream, "} %s;\n", name)

	case *types.Basic:
		// do nothing

	case *types.Chan:
		// do nothing

	case *types.Interface:
		// do nothing

	case *types.Named:
		typeName := createTypeName(typ)
		if typeName == "Value" || typeName == "Func" { // ToDo: ignore standard library definition
			return
		}
		underlyingType := typ.Underlying()
		ctx.emitTypeDefinition(underlyingType)
		underlyingTypeName := createTypeName(underlyingType)
		fmt.Fprintf(ctx.stream, "typedef %s %s;\n", underlyingTypeName, typeName)

	case *types.Pointer:
		elemType := typ.Elem()
		if et, ok := elemType.(*types.Array); ok {
			elemType = et.Elem()
		}
		ctx.emitTypeDefinition(typ.Elem())
		fmt.Fprintf(ctx.stream, "typedef struct { // %s\n", typ)
		fmt.Fprintf(ctx.stream, "\t%s* raw;\n", createType(elemType, ""))
		fmt.Fprintf(ctx.stream, "} %s;\n", name)

	case *types.Signature:
		// do nothing

	case *types.Slice:
		fmt.Fprintf(ctx.stream, `
typedef union { // %s
	SliceObject raw;
	struct {
		%s* ptr;
		uintptr_t size;
		uintptr_t capacity;
	} typed;
} %s;`, typ, createTypeName(typ.Elem()), name)

	case *types.Tuple:
		// do nothing

	default:
		panic(fmt.Sprintf("not implemented: %s %T", typ, typ))
	}
}

func (ctx *Context) emitType() {
	mainPkg := findMainPackage(ctx.program)

	for member := range mainPkg.Members {
		typ, ok := mainPkg.Members[member].(*ssa.Type)
		if !ok {
			continue
		}
		ctx.emitTypeDefinition(typ.Type())
	}

	for member := range mainPkg.Members {
		gv, ok := mainPkg.Members[member].(*ssa.Global)
		if !ok {
			continue
		}
		ctx.emitTypeDefinition(gv.Type())
	}

	ctx.visitAllFunctions(ctx.program, func(function *ssa.Function) {
		if function.Blocks == nil {
			return
		}
		for _, basicBlock := range function.DomPreorder() {
			for _, instr := range basicBlock.Instrs {
				if value, ok := instr.(ssa.Value); ok {
					ctx.emitTypeDefinition(value.Type())
				}
			}
		}
	})

	ctx.visitAllFunctions(ctx.program, func(function *ssa.Function) {
		if function.Blocks == nil {
			return
		}
		for _, basicBlock := range function.DomPreorder() {
			for _, instr := range basicBlock.Instrs {
				if alloc, ok := instr.(*ssa.Alloc); ok && alloc.Heap {
					t := alloc.Type().(*types.Pointer).Elem()
					ctx.emitTypeDefinition(t)
				}
			}
		}
	})
}

func (ctx *Context) emitInterfaceTable(typ *ssa.Type) {
	t := types.NewPointer(typ.Type())
	methodSet := ctx.program.MethodSets.MethodSet(t)

	if methodSet.Len() == 0 {
		return
	}

	fmt.Fprintf(ctx.stream, "struct {\n")
	fmt.Fprintf(ctx.stream, "\tInterfaceTableEntry entries[%d];\n", methodSet.Len())
	fmt.Fprintf(ctx.stream, "} interfaceTable_%s = {{\n", createTypeName(t))
	for i := 0; i < methodSet.Len(); i++ {
		function := ctx.program.MethodValue(methodSet.At(i))
		methodName := function.Name()
		method := wrapInFunctionObject(createFunctionName(function))
		fmt.Fprintf(ctx.stream, "\t{\"%s\", %s},\n", methodName, method)
	}
	fmt.Fprintln(ctx.stream, "}};")
}

func (ctx *Context) emitGlobalVariable(gv *ssa.Global) {
	name := createValueName(gv)
	fmt.Fprintf(ctx.stream, "%s;\n", createType(gv.Type().(*types.Pointer).Elem(), name))
}

func (ctx *Context) emitRuntimeInfo() {
	fmt.Fprintln(ctx.stream, "Func runtime_info_funcs[] = {")
	ctx.visitAllFunctions(ctx.program, func(function *ssa.Function) {
		fmt.Fprintf(ctx.stream, "{ (StringObject){.str_ptr = \"main.%s\" }, (UserFunction){.func_ptr = %s} },\n", function.Name(), createFunctionName(function))
	})
	fmt.Fprintln(ctx.stream, "};")

	fmt.Fprint(ctx.stream, `
size_t runtime_info_get_funcs_count(void) {
	return sizeof(runtime_info_funcs)/sizeof(runtime_info_funcs[0]);
}

const Func* runtime_info_refer_func(size_t i) {
	return &runtime_info_funcs[i];
}

UserFunction runtime_info_get_entry_point(void) {
	return (UserFunction) { .func_ptr = f_S_main };
}
`)
}

func findMainPackage(program *ssa.Program) *ssa.Package {
	for _, pkg := range program.AllPackages() {
		if pkg.Pkg.Name() == "main" {
			return pkg
		}
	}
	panic("main package not found")
}

func findLibraryFunctions(program *ssa.Program) []*ssa.Function {
	var functions []*ssa.Function
	for _, pkg := range program.AllPackages() {
		if pkg.Pkg.Name() != "fmt" {
			continue
		}
		for symbol := range pkg.Members {
			function, ok := pkg.Members[symbol].(*ssa.Function)
			if !ok {
				continue
			}

			if symbol != "Println" && symbol != "Printf" {
				continue
			}

			functions = append(functions, function)
		}
	}
	for _, pkg := range program.AllPackages() {
		if pkg.Pkg.Name() != "reflect" {
			continue
		}
		for symbol := range pkg.Members {
			function, ok := pkg.Members[symbol].(*ssa.Function)
			if !ok {
				continue
			}

			if symbol != "ValueOf" {
				continue
			}

			functions = append(functions, function)
		}

		typ, ok := pkg.Members["Value"].(*ssa.Type)
		if !ok {
			continue
		}

		methodSet := program.MethodSets.MethodSet(typ.Type())
		for i := 0; i < methodSet.Len(); i++ {
			function := program.MethodValue(methodSet.At(i))
			if function == nil {
				continue
			}
			l := strings.Split(function.String(), ".")
			methodName := l[len(l)-1]
			if methodName != "Pointer" {
				continue
			}
			functions = append(functions, function)
		}
	}
	for _, pkg := range program.AllPackages() {
		if pkg.Pkg.Name() != "runtime" {
			continue
		}
		for symbol := range pkg.Members {
			function, ok := pkg.Members[symbol].(*ssa.Function)
			if !ok {
				continue
			}

			if symbol != "FuncForPC" {
				continue
			}

			functions = append(functions, function)
		}

		typ, ok := pkg.Members["Func"].(*ssa.Type)
		if !ok {
			continue
		}

		methodSet := program.MethodSets.MethodSet(types.NewPointer(typ.Type()))
		for i := 0; i < methodSet.Len(); i++ {
			function := program.MethodValue(methodSet.At(i))
			if function == nil {
				continue
			}
			l := strings.Split(function.String(), ".")
			methodName := l[len(l)-1]
			if methodName != "Name" {
				continue
			}
			functions = append(functions, function)
		}
	}
	for _, pkg := range program.AllPackages() {
		if pkg.Pkg.Name() != "strings" {
			continue
		}
		for symbol := range pkg.Members {
			function, ok := pkg.Members[symbol].(*ssa.Function)
			if !ok {
				continue
			}

			if symbol != "Split" {
				continue
			}

			functions = append(functions, function)
		}
	}
	return functions
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

		if symbol == "init" {
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
	FunctionObject (gox5_##name)(LightWeightThreadContext* ctx)

typedef struct GlobalContext GlobalContext;

typedef struct {
	const void* func_ptr;
} UserFunction;

typedef struct {
	bool raw;
} BoolObject;

typedef struct {
	void* raw;
} ChannelObject;

typedef struct {
	const void* func_ptr;
} FunctionObject;

typedef struct {
	intptr_t raw;
} IntObject;

typedef struct {
	const char* str_ptr;
} StringObject;

typedef struct StackFrameCommon {
	FunctionObject resume_func;
	struct StackFrameCommon* prev_stack_pointer;
	void* free_vars;
} StackFrameCommon;

typedef struct {
	GlobalContext* global_context;
	StackFrameCommon* stack_pointer;
	UserFunction prev_func;
	intptr_t marker;
} LightWeightThreadContext;

typedef struct {
	const char* method_name;
	FunctionObject method;
} InterfaceTableEntry;

typedef struct {
	void* receiver;
	uintptr_t num_methods;
	InterfaceTableEntry* interface_table; // ToDo: use distinguish object type for empty interface (1: int, 2 string)
} Interface;

typedef struct {
	void* addr;
	uintptr_t size;
	uintptr_t capacity;
} SliceObject;

typedef struct {
	StackFrameCommon common;
	SliceObject* result_ptr;
	SliceObject base;
	SliceObject elements;
} StackFrameAppend;
DECLARE_RUNTIME_API(append, StackFrameAppend);

typedef struct {
	StackFrameCommon common;
	ChannelObject* result_ptr;
	IntObject size; // ToDo: correct to proper type
} StackFrameMakeChan;
DECLARE_RUNTIME_API(make_chan, StackFrameMakeChan);

typedef struct {
	StackFrameCommon common;
	FunctionObject* result_ptr;
	UserFunction user_function;
	uintptr_t num_object_ptrs;
	void* object_ptrs[0];
} StackFrameMakeClosure;
DECLARE_RUNTIME_API(make_closure, StackFrameMakeClosure);

typedef struct {
	StackFrameCommon common;
	void* result_ptr;
	uintptr_t size;
} StackFrameNew;
DECLARE_RUNTIME_API(new, StackFrameNew);

typedef struct {
	StackFrameCommon common;
	IntObject* result_ptr;
	ChannelObject channel;
} StackFrameRecv;
DECLARE_RUNTIME_API(recv, StackFrameRecv);

typedef struct {
	StackFrameCommon common;
	ChannelObject channel;
	IntObject data;
} StackFrameSend;
DECLARE_RUNTIME_API(send, StackFrameSend);

typedef struct {
	StackFrameCommon common;
	FunctionObject function_object;
	uintptr_t result_size;
	uintptr_t num_arg_buffer_words;
	void* arg_buffer[0];
} StackFrameSpawn;
DECLARE_RUNTIME_API(spawn, StackFrameSpawn);

// ToDo: generate only when needed
typedef struct {
	IntObject e0;
	IntObject e1;
} Tuple_lt_IntObject_S_IntObject_gt_;

// ToDo: WA to handle fmt.Println

typedef struct {
	IntObject e0;
	Interface e1;
} PrintlnResult;

typedef struct {
	StackFrameCommon common;
	PrintlnResult* result_ptr;
	SliceObject param0;
} StackFramePrintln;
DECLARE_RUNTIME_API(println, StackFramePrintln);

#define f_S_Println gox5_println
#define Tuple_lt_IntObject_S_error_gt_ PrintlnResult

// ToDo: WA to handle fmt.Printf

typedef struct {
	StackFrameCommon common;
	PrintlnResult* result_ptr;
	StringObject param0;
	SliceObject param1;
} StackFramePrintf;
DECLARE_RUNTIME_API(printf, StackFramePrintf);

#define f_S_Printf gox5_printf

// ToDo: WA to handle reflect.ValueOf

typedef struct {
	IntObject e0;
} Value;

typedef struct {
	StackFrameCommon common;
	Value* result_ptr;
	Interface param0;
} StackFrameValueOf;
DECLARE_RUNTIME_API(value_of, StackFrameValueOf);

#define f_S_ValueOf gox5_value_of

// ToDo: WA to handle reflect.Value.Pointer

typedef struct {
	StackFrameCommon common;
	IntObject* result_ptr;
	Value param0;
} StackFrameValuePointer;
DECLARE_RUNTIME_API(value_pointer, StackFrameValuePointer);

#define f_S_Pointer_S_Value gox5_value_pointer

// ToDo: WA to handle runtime.FuncForPC

typedef struct {
	StringObject name;
	UserFunction function;
} Func;

typedef struct {
	StackFrameCommon common;
	const Func** result_ptr;
	IntObject param0;
} StackFrameFuncForPc;
DECLARE_RUNTIME_API(func_for_pc, StackFrameFuncForPc);

#define f_S_FuncForPC gox5_func_for_pc

// ToDo: WA to handle runtime.Func.Name

typedef struct {
	StackFrameCommon common;
	StringObject* result_ptr;
	Func* param0;
} StackFrameFuncName;
DECLARE_RUNTIME_API(func_name, StackFrameFuncName);

#define f_S_Name_S_Pointer___lt___Func___gt___ gox5_func_name

// ToDo: WA to handle strings.Split

typedef struct {
	StackFrameCommon common;
	SliceObject* result_ptr;
	StringObject param0;
	StringObject param1;
} StackFrameSplit;
DECLARE_RUNTIME_API(func_for_pc, StackFrameFuncForPc);

#define f_S_Split gox5_split
`)

	ctx.emitType()

	mainPkg := findMainPackage(program)

	ctx.visitAllFunctions(program, func(function *ssa.Function) {
		ctx.emitFunctionDeclaration(function)
	})

	libraryFunctions := findLibraryFunctions(program)
	for _, function := range libraryFunctions {
		ctx.emitFunctionHeader(createFunctionName(function), ";")

		signature := function.Signature
		concreteSignatureName := createSignatureName(signature, false)
		ctx.tryEmitSignatureDefinition(signature, concreteSignatureName, false)
	}

	for member := range mainPkg.Members {
		typ, ok := mainPkg.Members[member].(*ssa.Type)
		if !ok {
			continue
		}
		ctx.emitInterfaceTable(typ)
	}

	for member := range mainPkg.Members {
		gv, ok := mainPkg.Members[member].(*ssa.Global)
		if !ok {
			continue
		}
		ctx.emitGlobalVariable(gv)
	}

	ctx.visitAllFunctions(program, func(function *ssa.Function) {
		if function.Blocks != nil {
			ctx.emitFunctionDefinition(function)
		}
	})

	ctx.emitRuntimeInfo()
}
