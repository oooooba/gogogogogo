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
		case '#':
			s = "_H_"
		default:
			s = string(c)
		}
		buf += s
	}
	return buf
}

func wrapInFunctionObject(s string) string {
	return fmt.Sprintf("(FunctionObject){.raw=%s}", s)
}

func wrapInObject(s string, t types.Type) string {
	return fmt.Sprintf("(%s){.raw=%s}", createTypeName(t), s)
}

func createValueName(value ssa.Value) string {
	if val, ok := value.(*ssa.Const); ok {
		cst := "0"
		if !val.IsNil() {
			cst = val.Value.String()
			if t, ok := val.Type().Underlying().(*types.Basic); ok {
				switch t.Kind() {
				case types.Float32, types.Float64:
					cst = fmt.Sprintf("%f", val.Float64())
				}
			}
		}
		if t, ok := val.Type().Underlying().(*types.Interface); ok {
			return fmt.Sprintf("(%s){%s}", createTypeName(t), cst)
		}
		return wrapInObject(cst, val.Type())
	} else if val, ok := value.(*ssa.Function); ok {
		return wrapInObject(createFunctionName(val), val.Type())
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
		return wrapInObject(fmt.Sprintf("&%s", createValueName(value)), value.Type())
	} else {
		return fmt.Sprintf("frame->%s", createValueName(value))
	}
}

func createTypeName(typ types.Type) string {
	var f func(typ types.Type) string
	f = func(typ types.Type) string {
		switch t := typ.(type) {
		case *types.Array:
			return fmt.Sprintf("Array<%s$%d>", f(t.Elem()), t.Len())
		case *types.Basic:
			switch t.Kind() {
			case types.Bool, types.UntypedBool:
				return fmt.Sprintf("BoolObject")
			case types.Float32:
				return fmt.Sprintf("Float32Object")
			case types.Float64:
				return fmt.Sprintf("Float64Object")
			case types.Int:
				return fmt.Sprintf("IntObject")
			case types.Int8:
				return fmt.Sprintf("Int8Object")
			case types.Int16:
				return fmt.Sprintf("Int16Object")
			case types.Int32:
				return fmt.Sprintf("Int32Object")
			case types.Int64:
				return fmt.Sprintf("Int64Object")
			case types.Invalid:
				return fmt.Sprintf("InvalidObject")
			case types.String:
				return fmt.Sprintf("StringObject")
			case types.UnsafePointer:
				return fmt.Sprintf("UnsafePointerObject")
			case types.Uint, types.Uintptr:
				return fmt.Sprintf("UintObject")
			case types.Uint8:
				return fmt.Sprintf("Uint8Object")
			case types.Uint16:
				return fmt.Sprintf("Uint16Object")
			case types.Uint32:
				return fmt.Sprintf("Uint32Object")
			case types.Uint64:
				return fmt.Sprintf("Uint64Object")
			}
		case *types.Chan:
			return fmt.Sprintf("ChannelObject")
		case *types.Interface:
			return fmt.Sprintf("Interface")
		case *types.Map:
			return fmt.Sprintf("MapObject")
		case *types.Named:
			// remove "command-line-arguments."
			l := strings.Split(typ.String(), ".")
			return fmt.Sprintf("Named<%s>", l[len(l)-1])
		case *types.Pointer:
			return fmt.Sprintf("Pointer<%s>", f(t.Elem()))
		case *types.Signature:
			return fmt.Sprintf("FunctionObject")
		case *types.Slice:
			return fmt.Sprintf("Slice<%s>", f(t.Elem()))
		case *types.Struct:
			return fmt.Sprintf("Struct%p", t)
		case *types.Tuple:
			name := "Tuple<"
			for i := 0; i < t.Len(); i++ {
				elemType := t.At(i).Type()
				if i != 0 {
					name += "$"
				}
				name += f(elemType)
			}
			name += ">"
			return name
		default:
			if typ.String() == "iter" {
				return "IterObject"
			}
		}
		panic(fmt.Sprintf("type not supported: %s", typ.String()))
	}
	return encode(f(typ))
}

func createRawTypeName(typ types.Type) string {
	switch typ.(*types.Basic).Kind() {
	case types.Bool, types.UntypedBool:
		return "bool"
	case types.Float32:
		return "float"
	case types.Float64:
		return "double"
	case types.Int:
		return "intptr_t"
	case types.Int8:
		return "int8_t"
	case types.Int16:
		return "int16_t"
	case types.Int32:
		return "int32_t"
	case types.Int64:
		return "int64_t"
	case types.Uint:
		return "uintptr_t"
	case types.Uint8:
		return "uint8_t"
	case types.Uint16:
		return "uint16_t"
	case types.Uint32:
		return "uint32_t"
	case types.Uint64:
		return "uint64_t"
	}
	panic(typ)
}

func createTypeIdName(typ types.Type) string {
	return fmt.Sprintf("runtime_info_type_%s", createTypeName(typ))
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

	if signature.Recv() != nil || signature.Results().Len() > 0 || signature.Params().Len() > 0 {
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
	if signature.Recv() != nil {
		var receiver string
		if callCommon.IsInvoke() {
			receiver = fmt.Sprintf("%s.receiver", createValueRelName(callCommon.Value))
		} else {
			receiver = fmt.Sprintf("%s", createValueRelName(callCommon.Args[argBase]))
			argBase++
		}
		fmt.Fprintf(ctx.stream, "signature->param%d = %s; // receiver: %s\n", paramBase, receiver, signature.Recv())
		paramBase++
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

func (ctx *Context) emitPrint(value ssa.Value) {
	switch t := value.Type().(type) {
	case *types.Basic:
		var specifier string
		switch t.Kind() {
		case types.Bool:
			fmt.Fprintf(ctx.stream, `fprintf(stderr, "%%s", %s.raw ? "true" : "false");`+"\n", createValueRelName(value))
			return
		case types.Int:
			specifier = "ld"
		case types.Int8, types.Int16, types.Int32:
			specifier = "d"
		case types.Int64:
			specifier = "ld"
		case types.Uint:
			specifier = "lu"
		case types.Uint8, types.Uint16, types.Uint32:
			specifier = "u"
		case types.Uint64:
			specifier = "lu"
		case types.Float32, types.Float64:
			specifier = "f"
		case types.String:
			specifier = "s"
		default:
			panic(fmt.Sprintf("%s, %s (%T)", value, t, t))
		}
		fmt.Fprintf(ctx.stream, "fprintf(stderr, \"%%%s\", %s.raw);\n", specifier, createValueRelName(value))
	default:
		fmt.Fprintf(ctx.stream, "assert(false); // not supported\n")
	}
}

func (ctx *Context) emitInstruction(instruction ssa.Instruction) {
	fmt.Fprintf(ctx.stream, "\t// %T instruction\n", instruction)
	fmt.Fprintf(ctx.stream, "\t{\n")
	switch instr := instruction.(type) {
	case *ssa.Alloc:
		if instr.Heap {
			result := createValueRelName(instr)
			ctx.switchFunctionToCallRuntimeApi("gox5_new", "StackFrameNew", createInstructionName(instr), &result, nil,
				paramArgPair{param: "size", arg: fmt.Sprintf("sizeof(%s)", createTypeName(instr.Type().(*types.Pointer).Elem()))},
			)
		} else {
			v := createValueRelName(instr)
			elemType := instr.Type().(*types.Pointer).Elem()
			fmt.Fprintf(ctx.stream, "%s_buf = (%s){0};\n", v, createTypeName(elemType))
			fmt.Fprintf(ctx.stream, "%s* raw = &%s_buf;\n", createTypeName(elemType), v)
			fmt.Fprintf(ctx.stream, "%s = %s;\n", v, wrapInObject("raw", instr.Type()))
		}

	case *ssa.BinOp:
		needToCallRuntimeApi := false
		raw := ""
		switch op := instr.Op; op {
		case token.EQL, token.NEQ:
			var equalFunc string
			if t, ok := instr.X.Type().Underlying().(*types.Interface); ok {
				if t.Empty() {
					equalFunc = "equal_InterfaceEmpty"
				} else {
					equalFunc = "equal_InterfaceNonEmpty"
				}
			} else {
				equalFunc = fmt.Sprintf("equal_%s", createTypeName(instr.X.Type()))
			}
			fmt.Fprintf(ctx.stream, "bool raw = %s(&%s, &%s) %s true;", equalFunc, createValueRelName(instr.X), createValueRelName(instr.Y), instr.Op)
			raw = "raw"
		case token.LSS, token.LEQ, token.GTR, token.GEQ:
			raw = fmt.Sprintf("%s.raw %s %s.raw", createValueRelName(instr.X), instr.Op.String(), createValueRelName(instr.Y))
		case token.ADD:
			if t, ok := instr.Type().(*types.Basic); ok && t.Kind() == types.String {
				result := createValueRelName(instr)
				ctx.switchFunctionToCallRuntimeApi("gox5_concat", "StackFrameConcat", createInstructionName(instr), &result, nil,
					paramArgPair{param: "lhs", arg: createValueRelName(instr.X)},
					paramArgPair{param: "rhs", arg: createValueRelName(instr.Y)},
				)
				needToCallRuntimeApi = true
			} else {
				raw = fmt.Sprintf("%s.raw %s %s.raw", createValueRelName(instr.X), instr.Op.String(), createValueRelName(instr.Y))
			}
		case token.SHL:
			var unsignedRawType string
			switch instr.Type().(*types.Basic).Kind() {
			case types.Int, types.Int8, types.Int16, types.Int32, types.Int64:
				unsignedRawType = fmt.Sprintf("u%s", createRawTypeName(instr.X.Type()))
			case types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64:
				unsignedRawType = createRawTypeName(instr.X.Type())
			default:
				panic(fmt.Sprintf("%s", instr))
			}
			fmt.Fprintf(ctx.stream, "%s unsignedLhs = (%s)(%s.raw);\n", unsignedRawType, unsignedRawType, createValueRelName(instr.X))
			fmt.Fprintf(ctx.stream, "%s rhs = %s.raw;\n", createRawTypeName(instr.Y.Type()), createValueRelName(instr.Y))
			raw = "(rhs < sizeof(unsignedLhs) * 8) ? (unsignedLhs << rhs) : 0"
		case token.SHR:
			var unsignedRawType string
			var overflowExpr string
			var calcExpr string
			bitLen := "sizeof(unsignedLhs) * 8"
			switch instr.Type().(*types.Basic).Kind() {
			case types.Int, types.Int8, types.Int16, types.Int32, types.Int64:
				unsignedRawType = fmt.Sprintf("u%s", createRawTypeName(instr.X.Type()))
				overflowExpr = fmt.Sprintf("%s.raw < 0 ? ((%s)(-1)) : 0", createValueRelName(instr.X), unsignedRawType)
				calcExpr = fmt.Sprintf("rhs == 0 ? unsignedLhs : ((((%s) >> (%s - rhs)) << (%s - rhs)) | (unsignedLhs >> rhs))", overflowExpr, bitLen, bitLen)
			case types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64:
				unsignedRawType = createRawTypeName(instr.X.Type())
				overflowExpr = "0"
				calcExpr = "unsignedLhs >> rhs"
			default:
				panic(fmt.Sprintf("%s", instr))
			}
			fmt.Fprintf(ctx.stream, "%s unsignedLhs = (%s)(%s.raw);\n", unsignedRawType, unsignedRawType, createValueRelName(instr.X))
			fmt.Fprintf(ctx.stream, "%s rhs = %s.raw;\n", createRawTypeName(instr.Y.Type()), createValueRelName(instr.Y))
			raw = fmt.Sprintf("rhs < %s ? (%s) : (%s)", bitLen, calcExpr, overflowExpr)
		default:
			raw = fmt.Sprintf("%s.raw %s %s.raw", createValueRelName(instr.X), instr.Op.String(), createValueRelName(instr.Y))
		}
		if !needToCallRuntimeApi {
			fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInObject(raw, instr.Type()))
		}

	case *ssa.Call:
		callCommon := instr.Common()
		if callCommon.Method != nil {
			fmt.Fprintf(ctx.stream,
				"FunctionObject next_function = gox5_search_method(&%s, (StringObject){\"%s\"});\n",
				createValueRelName(callCommon.Value), callCommon.Method.Name())
			fmt.Fprintf(ctx.stream, "assert(next_function.raw != NULL);\n")
			nextFunction := "next_function"
			ctx.switchFunction(nextFunction, callCommon, createValueRelName(instr), createInstructionName(instr))
		} else {
			switch callee := callCommon.Value.(type) {
			case *ssa.Builtin:
				needToCallRuntimeApi := false
				raw := ""
				switch callee.Name() {
				case "append":
					result := createValueRelName(instr)
					result += ".raw"
					ctx.switchFunctionToCallRuntimeApi("gox5_append", "StackFrameAppend", createInstructionName(instr), &result, nil,
						paramArgPair{param: "base", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
						paramArgPair{param: "elements", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[1]))},
					)
					needToCallRuntimeApi = true
				case "cap":
					fmt.Fprintf(ctx.stream, "uintptr_t raw = %s.typed.capacity;", createValueRelName(callCommon.Args[0]))
					raw = "raw"
				case "len":
					switch t := callCommon.Args[0].Type().(type) {
					case *types.Basic:
						switch t.Kind() {
						case types.String:
							raw = fmt.Sprintf("strlen(%s.raw)", createValueRelName(callCommon.Args[0]))
						default:
							panic(fmt.Sprintf("unsuported argument for len: %s (%s)", callCommon.Args[0], t))
						}
					case *types.Map:
						result := createValueRelName(instr)
						ctx.switchFunctionToCallRuntimeApi("gox5_map_len", "StackFrameMapLen", createInstructionName(instr), &result, nil,
							paramArgPair{param: "map", arg: createValueRelName(callCommon.Args[0])},
						)
						needToCallRuntimeApi = true
					case *types.Slice:
						fmt.Fprintf(ctx.stream, "uintptr_t raw = %s.typed.size;", createValueRelName(callCommon.Args[0]))
						raw = "raw"
					default:
						panic(fmt.Sprintf("unsuported argument for len: %s", callCommon.Args[0]))
					}
				case "ssa:wrapnilchk":
					fmt.Fprintf(ctx.stream, "assert(%s.raw); // ssa:wrapnilchk\n", createValueRelName(callCommon.Args[0]))
					raw = fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))
				case "print":
					for _, arg := range callCommon.Args {
						ctx.emitPrint(arg)
					}

				case "println":
					for i, arg := range callCommon.Args {
						if i != 0 {
							fmt.Fprintf(ctx.stream, "fprintf(stderr, \" \");\n")
						}
						ctx.emitPrint(arg)
					}
					fmt.Fprintf(ctx.stream, "fprintf(stderr, \"\\n\");\n")

				default:
					panic(fmt.Sprintf("unsuported builtin function: %s", callee.Name()))
				}
				if !needToCallRuntimeApi {
					if t, ok := instr.Type().(*types.Tuple); !ok || t.Len() > 0 {
						fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInObject(raw, instr.Type()))
					}
					fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
				}

			default:
				if callee.Name() != "init" {
					nextFunction := createValueRelName(callee)
					ctx.switchFunction(nextFunction, callCommon, createValueRelName(instr), createInstructionName(instr))
				}
			}
		}

	case *ssa.ChangeType:
		fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), createValueRelName(instr.X))

	case *ssa.Convert:
		if dstType, ok := instr.Type().(*types.Basic); ok && dstType.Kind() == types.String {
			result := createValueRelName(instr)
			switch srcType := instr.X.Type().(type) {
			case *types.Basic:
				arg := fmt.Sprintf("(IntObject){%s.raw}", createValueRelName(instr.X))
				ctx.switchFunctionToCallRuntimeApi("gox5_make_string_from_rune", "StackFrameMakeStringFromRune", createInstructionName(instr), &result, nil,
					paramArgPair{param: "rune", arg: arg},
				)
			case *types.Slice:
				if elemType, ok := srcType.Elem().(*types.Basic); ok {
					switch elemType.Kind() {
					case types.Byte:
						arg := fmt.Sprintf("%s.raw", createValueRelName(instr.X))
						ctx.switchFunctionToCallRuntimeApi("gox5_make_string_from_byte_slice", "StackFrameMakeStringFromByteSlice", createInstructionName(instr), &result, nil,
							paramArgPair{param: "byte_slice", arg: arg},
						)
					case types.Rune:
						arg := fmt.Sprintf("%s.raw", createValueRelName(instr.X))
						ctx.switchFunctionToCallRuntimeApi("gox5_make_string_from_rune_slice", "StackFrameMakeStringFromRuneSlice", createInstructionName(instr), &result, nil,
							paramArgPair{param: "rune_slice", arg: arg},
						)
					default:
						panic(fmt.Sprintf("%s, %s, %s (%T)", instr, srcType, elemType, elemType))
					}
				} else {
					panic(fmt.Sprintf("%s, %s, %s (%T)", instr, srcType, elemType, elemType))
				}
			default:
				panic(fmt.Sprintf("%s, %s (%T)", instr, srcType, srcType))
			}
		} else {
			raw := fmt.Sprintf("%s.raw", createValueRelName(instr.X))
			fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInObject(raw, instr.Type()))
		}

	case *ssa.Extract:
		fmt.Fprintf(ctx.stream, "%s = %s.raw.e%d;\n", createValueRelName(instr), createValueRelName(instr.Tuple), instr.Index)

	case *ssa.FieldAddr:
		fmt.Fprintf(ctx.stream, "%s* raw = &(%s.raw->%s);\n", createTypeName(instr.Type().(*types.Pointer).Elem()), createValueRelName(instr.X), instr.X.Type().(*types.Pointer).Elem().Underlying().(*types.Struct).Field(instr.Field).Name())
		fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInObject("raw", instr.Type()))

	case *ssa.Index:
		fmt.Fprintf(ctx.stream, "uintptr_t index = %s.raw;\n", createValueRelName(instr.Index))
		switch t := instr.X.Type().(type) {
		case *types.Array:
			fmt.Fprintf(ctx.stream, "%s val = %s.raw[index];\n", createTypeName(t.Elem()), createValueRelName(instr.X))
		default:
			panic(fmt.Sprintf("%s, %s, %s", instr, instr.X, t))
		}
		fmt.Fprintf(ctx.stream, "%s = val;\n", createValueRelName(instr))

	case *ssa.IndexAddr:
		fmt.Fprintf(ctx.stream, "uintptr_t index = %s.raw;\n", createValueRelName(instr.Index))
		switch t := instr.X.Type().(type) {
		case *types.Slice:
			fmt.Fprintf(ctx.stream, "%s* raw = &((%s.typed.ptr)[index]);\n", createTypeName(t.Elem()), createValueRelName(instr.X))
		case *types.Pointer:
			fmt.Fprintf(ctx.stream, "%s* raw = &(%s.raw->raw[index]);\n", createTypeName(t.Elem().(*types.Array).Elem()), createValueRelName(instr.X))
		default:
			panic(fmt.Sprintf("%s, %s, %s", instr, instr.X, t))
		}
		fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInObject("raw", instr.Type()))

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
			resultSize = fmt.Sprintf("sizeof(%s)", createTypeName(signature.Results().At(0).Type()))
		default:
			resultSize = fmt.Sprintf("sizeof(%s)", createTypeName(signature.Results()))
		}

		ctx.switchFunctionToCallRuntimeApi("gox5_spawn", "StackFrameSpawn", createInstructionName(instr), nil,
			func() {
				fmt.Fprintf(ctx.stream, "intptr_t num_arg_buffer_words = 0;\n")
				for i, arg := range callCommon.Args {
					argValue := createValueRelName(arg)
					argType := createTypeName(arg.Type())
					argPtr := fmt.Sprintf("ptr%d", i)
					fmt.Fprintf(ctx.stream, "%s* %s = (void*)&next_frame->arg_buffer[num_arg_buffer_words]; // param[%d]\n", argType, argPtr, i)
					fmt.Fprintf(ctx.stream, "*%s = %s;\n", argPtr, argValue)
					fmt.Fprintf(ctx.stream, "num_arg_buffer_words += sizeof(%s) / sizeof(next_frame->arg_buffer[0]);\n", argType)
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

	case *ssa.Lookup:
		switch xt := instr.X.Type().Underlying().(type) {
		case *types.Basic:
			if xt.Kind() == types.String {
				raw := fmt.Sprintf("%s.raw[%s.raw]", createValueRelName(instr.X), createValueRelName(instr.Index))
				fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInObject(raw, instr.Type()))

			} else {
				panic(fmt.Sprintf("%s", instr))
			}
		case *types.Map:
			result := createValueRelName(instr)
			var keyId string
			if key, ok := instr.Index.(*ssa.Const); ok {
				keyId = fmt.Sprintf("frame->tmp_%p", key)
				fmt.Fprintf(ctx.stream, "%s = %s;\n", keyId, createValueRelName(key))
			} else {
				keyId = createValueRelName(instr.Index)
			}
			key := fmt.Sprintf("&%s", keyId)
			var value, found string
			if instr.CommaOk {
				value = fmt.Sprintf("&%s.raw.e0", result)
				found = fmt.Sprintf("&%s.raw.e1.raw", result)
			} else {
				value = fmt.Sprintf("&%s", result)
				found = "NULL"
			}
			ctx.switchFunctionToCallRuntimeApi("gox5_map_get", "StackFrameMapGet", createInstructionName(instr), nil, nil,
				paramArgPair{param: "map", arg: createValueRelName(instr.X)},
				paramArgPair{param: "key", arg: key},
				paramArgPair{param: "value", arg: value},
				paramArgPair{param: "found", arg: found},
			)
		default:
			panic(fmt.Sprintf("%s", instr))
		}

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
		var receiver string
		if instr.Type().Underlying().(*types.Interface).Empty() {
			switch instrX := instr.X.(type) {
			case *ssa.Const, *ssa.Function:
				id := fmt.Sprintf("tmp_%s", createValueName(instr))
				fmt.Fprintf(ctx.stream, "frame->%s = %s;\n", id, createValueRelName(instrX))
				receiver = fmt.Sprintf("&frame->%s", id)

			default:
				receiver = fmt.Sprintf("&%s", createValueRelName(instr.X))
			}
		} else {
			receiver = fmt.Sprintf("%s.raw", createValueRelName(instr.X))
		}

		typeId := fmt.Sprintf("(TypeId){ .info = &%s }", createTypeIdName(instr.X.Type()))

		result := createValueRelName(instr)
		ctx.switchFunctionToCallRuntimeApi("gox5_make_interface", "StackFrameMakeInterface", createInstructionName(instr), &result, nil,
			paramArgPair{param: "receiver", arg: receiver},
			paramArgPair{param: "type_id", arg: typeId},
		)

	case *ssa.MakeMap:
		result := createValueRelName(instr)
		ctx.switchFunctionToCallRuntimeApi("gox5_make_map", "StackFrameMakeMap", createInstructionName(instr), &result, nil,
			paramArgPair{param: "key_type", arg: fmt.Sprintf("(TypeId){ .info = &%s }", createTypeIdName(instr.Type().Underlying().(*types.Map).Key()))},
			paramArgPair{param: "value_type", arg: fmt.Sprintf("(TypeId){ .info = &%s }", createTypeIdName(instr.Type().Underlying().(*types.Map).Elem()))},
		)

	case *ssa.MapUpdate:
		var keyId string
		if key, ok := instr.Key.(*ssa.Const); ok {
			keyId = fmt.Sprintf("frame->tmp_%p", key)
			fmt.Fprintf(ctx.stream, "%s = %s;\n", keyId, createValueRelName(key))
		} else {
			keyId = createValueRelName(instr.Key)
		}
		var valueId string
		if value, ok := instr.Value.(*ssa.Const); ok {
			valueId = fmt.Sprintf("frame->tmp_%p", value)
			fmt.Fprintf(ctx.stream, "%s = %s;\n", valueId, createValueRelName(value))
		} else {
			valueId = createValueRelName(instr.Value)
		}
		ctx.switchFunctionToCallRuntimeApi("gox5_map_set", "StackFrameMapSet", createInstructionName(instr), nil, nil,
			paramArgPair{param: "map", arg: createValueRelName(instr.Map)},
			paramArgPair{param: "key", arg: fmt.Sprintf("&%s", keyId)},
			paramArgPair{param: "value", arg: fmt.Sprintf("&%s", valueId)},
		)

	case *ssa.Next:
		result := createValueRelName(instr)
		iter := fmt.Sprintf("%s", createValueRelName(instr.Iter))
		mp := fmt.Sprintf("%s.obj", iter)
		key := fmt.Sprintf("&%s.raw.e1", result)
		var value string
		if instr.Type().(*types.Tuple).At(2).Type().(*types.Basic).Kind() == types.Invalid {
			value = "NULL"
		} else {
			value = fmt.Sprintf("&%s.raw.e2", result)
		}
		found := fmt.Sprintf("&%s.raw.e0.raw", result)
		count := fmt.Sprintf("&%s.count", iter)
		ctx.switchFunctionToCallRuntimeApi("gox5_map_next", "StackFrameMapNext", createInstructionName(instr), nil, nil,
			paramArgPair{param: "map", arg: mp},
			paramArgPair{param: "key", arg: key},
			paramArgPair{param: "value", arg: value},
			paramArgPair{param: "found", arg: found},
			paramArgPair{param: "count", arg: count},
		)

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

	case *ssa.Range:
		fmt.Fprintf(ctx.stream, "%s = (IterObject){.obj = %s};\n", createValueRelName(instr), createValueRelName(instr.X))

	case *ssa.Return:
		fmt.Fprintf(ctx.stream, "ctx->stack_pointer = frame->common.prev_stack_pointer;\n")
		switch len(instr.Results) {
		case 0:
			// do nothing
		case 1:
			fmt.Fprintf(ctx.stream, "*frame->signature.result_ptr = %s;\n", createValueRelName(instr.Results[0]))
		default:
			for i, v := range instr.Results {
				fmt.Fprintf(ctx.stream, "frame->signature.result_ptr->raw.e%d = %s;\n", i, createValueRelName(v))
			}
		}
		fmt.Fprintf(ctx.stream, "return frame->common.resume_func;\n")

	case *ssa.Send:
		ctx.switchFunctionToCallRuntimeApi("gox5_send", "StackFrameSend", createInstructionName(instr), nil, nil,
			paramArgPair{param: "channel", arg: createValueRelName(instr.Chan)},
			paramArgPair{param: "data", arg: createValueRelName(instr.X)},
		)

	case *ssa.Slice:
		if t, ok := instr.Type().(*types.Basic); ok {
			if t.Kind() != types.String {
				panic(fmt.Sprintf("%s (%T)", t, t))
			}
			result := createValueRelName(instr)
			low := "-1"
			if instr.Low != nil {
				low = fmt.Sprintf("%s.raw", createValueRelName(instr.Low))
			}
			high := "-1"
			if instr.High != nil {
				high = fmt.Sprintf("%s.raw", createValueRelName(instr.High))
			}
			ctx.switchFunctionToCallRuntimeApi("gox5_strview", "StackFrameStrview", createInstructionName(instr), &result, nil,
				paramArgPair{param: "base", arg: createValueRelName(instr.X)},
				paramArgPair{param: "low", arg: low},
				paramArgPair{param: "high", arg: high},
			)
		} else {
			startIndex := "0"
			if instr.Low != nil {
				startIndex = fmt.Sprintf("%s.raw", createValueRelName(instr.Low))
			}

			ptr := ""
			length := ""
			switch t := instr.X.Type().(type) {
			case *types.Pointer:
				ptr = "raw->raw"
				elemType := t.Elem().(*types.Array)
				length = fmt.Sprintf("%d", elemType.Len())
			case *types.Slice:
				ptr = "typed.ptr"
				length = fmt.Sprintf("%s.typed.capacity", createValueRelName(instr.X))
			default:
				panic(fmt.Sprintf("not implemented: %s (%T)", t, t))
			}

			endIndex := length
			if instr.High != nil {
				endIndex = fmt.Sprintf("%s.raw", createValueRelName(instr.High))
			}

			fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInObject("0", instr.Type()))
			fmt.Fprintf(ctx.stream, "%s.typed.ptr = %s.%s + %s;\n", createValueRelName(instr), createValueRelName(instr.X), ptr, startIndex)
			fmt.Fprintf(ctx.stream, "%s.typed.size = %s - %s;\n", createValueRelName(instr), endIndex, startIndex)
			fmt.Fprintf(ctx.stream, "%s.typed.capacity = %s - %s;\n", createValueRelName(instr), length, startIndex)
		}

	case *ssa.Store:
		fmt.Fprintf(ctx.stream, "*(%s.raw) = %s;\n", createValueRelName(instr.Addr), createValueRelName(instr.Val))

	case *ssa.TypeAssert:
		srcObj := func() string {
			if _, ok := instr.AssertedType.Underlying().(*types.Interface); ok {
				return fmt.Sprintf("%s", createValueRelName(instr.X))
			} else {
				if instr.X.Type().Underlying().(*types.Interface).Empty() {
					return fmt.Sprintf("*((%s*)%s.receiver)", createTypeName(instr.AssertedType), createValueRelName(instr.X))
				} else {
					raw := fmt.Sprintf("%s.receiver", createValueRelName(instr.X))
					return wrapInObject(raw, instr.AssertedType)
				}
			}
		}()
		dstObj := createValueRelName(instr)
		if instr.CommaOk {
			if t, ok := instr.AssertedType.Underlying().(*types.Interface); ok {
				fmt.Fprintf(ctx.stream, "bool can_convert = true;\n")
				fmt.Fprintf(ctx.stream, "Interface* interface = &%s;\n", srcObj)
				for i := 0; i < t.NumExplicitMethods(); i++ {
					fmt.Fprintf(ctx.stream,
						"can_convert = can_convert && (gox5_search_method(interface, (StringObject){\"%s\"}).raw != NULL);\n",
						t.ExplicitMethod(i).Name())
				}
			} else {
				fmt.Fprintf(ctx.stream, "TypeId type_id = (TypeId){ .info = &%s };\n", createTypeIdName(instr.AssertedType))
				fmt.Fprintf(ctx.stream, "bool can_convert = %s.type_id.id == type_id.id;\n", createValueRelName(instr.X))
			}
			fmt.Fprintf(ctx.stream, "%s = %s;\n", dstObj, wrapInObject("0", instr.Type()))
			fmt.Fprintf(ctx.stream, "if (can_convert) {\n")
			fmt.Fprintf(ctx.stream, "%s.raw.e0 = %s;\n", dstObj, srcObj)
			fmt.Fprintf(ctx.stream, "}\n")
			fmt.Fprintf(ctx.stream, "%s.raw.e1 = (BoolObject){.raw=can_convert};\n", dstObj)
		} else {
			fmt.Fprintf(ctx.stream, "%s = %s;\n", dstObj, srcObj)
		}

	case *ssa.UnOp:
		if instr.Op == token.ARROW {
			result := createValueRelName(instr)
			ctx.switchFunctionToCallRuntimeApi("gox5_recv", "StackFrameRecv", createInstructionName(instr), &result, nil,
				paramArgPair{param: "channel", arg: createValueRelName(instr.X)},
			)
		} else {
			s := fmt.Sprintf("%s (%s.raw)", instr.Op.String(), createValueRelName(instr.X))
			if instr.Op != token.MUL {
				s = wrapInObject(s, instr.Type())
			}
			fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), s)
		}

	default:
		panic(fmt.Sprintf("unknown instruction: %s", instruction.String()))
	}
	fmt.Fprintf(ctx.stream, "\t}\n")
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

	case *ssa.Convert:
		ctx.emitValueDeclaration(val.X)

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

	case *ssa.Index:
		ctx.emitValueDeclaration(val.X)
		ctx.emitValueDeclaration(val.Index)

	case *ssa.IndexAddr:
		ctx.emitValueDeclaration(val.X)
		ctx.emitValueDeclaration(val.Index)

	case *ssa.Lookup:
		ctx.emitValueDeclaration(val.X)
		ctx.emitValueDeclaration(val.Index)
		if _, ok := val.X.Type().Underlying().(*types.Map); ok {
			if key, ok := val.Index.(*ssa.Const); ok {
				id := fmt.Sprintf("tmp_%p", key)
				fmt.Fprintf(ctx.stream, "\t%s %s; // %s : %s\n", createTypeName(key.Type()), id, key, key.Type())
			}
		}

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
				fmt.Fprintf(ctx.stream, "\t%s %s; // %s : %s\n", createTypeName(valX.Type()), id, valX.String(), valX.Type())
			}
		}

	case *ssa.MakeMap:
		if val.Reserve != nil {
			ctx.emitValueDeclaration(val.Reserve)
		}

	case *ssa.Next:
		ctx.emitValueDeclaration(val.Iter)

	case *ssa.Parameter:
		canEmit = false

	case *ssa.Phi:
		for _, edge := range val.Edges {
			ctx.emitValueDeclaration(edge)
		}

	case *ssa.Range:
		ctx.emitValueDeclaration(val.X)

	case *ssa.Slice:
		ctx.emitValueDeclaration(val.X)
		if val.Low != nil {
			ctx.emitValueDeclaration(val.Low)
		}
		if val.High != nil {
			ctx.emitValueDeclaration(val.High)
		}

	case *ssa.TypeAssert:
		ctx.emitValueDeclaration(val.X)

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
		fmt.Fprintf(ctx.stream, "\t%s %s; // %s : %s\n", createTypeName(value.Type()), id, value.String(), value.Type())
	}
}

func requireSwitchFunction(instruction ssa.Instruction) bool {
	switch t := instruction.(type) {
	case *ssa.Alloc:
		return instruction.(*ssa.Alloc).Heap
	case *ssa.BinOp:
		if t.Op == token.ADD {
			if tt, ok := t.Type().(*types.Basic); ok && tt.Kind() == types.String {
				return true
			}
		}
		return false
	case *ssa.Call:
		return t.Common().Value.Name() != "init"
	case *ssa.Go, *ssa.MakeChan, *ssa.MakeClosure, *ssa.MakeInterface, *ssa.MakeMap, *ssa.MapUpdate, *ssa.Send:
		return true
	case *ssa.Convert:
		if dstType, ok := t.Type().(*types.Basic); ok && dstType.Kind() == types.String {
			return true
		}
		return false
	case *ssa.Lookup:
		_, ok := t.X.Type().Underlying().(*types.Map)
		return ok
	case *ssa.Next:
		return true
	case *ssa.Slice:
		if dstType, ok := t.Type().(*types.Basic); ok && dstType.Kind() == types.String {
			return true
		}
		return false
	case *ssa.UnOp:
		if t.Op == token.ARROW {
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
		fmt.Fprintf(ctx.stream, "\t%s* result_ptr;\n", createTypeName(signature.Results().At(0).Type()))
	default:
		fmt.Fprintf(ctx.stream, "\t%s* result_ptr;\n", createTypeName(signature.Results()))
	}

	base := 0
	if signature.Recv() != nil {
		id := fmt.Sprintf("param%d", base)
		var typeName string
		if makesReceiverInterface {
			typeName = "void*"
		} else {
			typeName = createTypeName(signature.Recv().Type())
		}
		fmt.Fprintf(ctx.stream, "\t%s %s; // receiver: %s\n", typeName, id, signature.Recv().String())
		base++
	}

	for i := 0; i < signature.Params().Len(); i++ {
		id := fmt.Sprintf("param%d", base+i)
		fmt.Fprintf(ctx.stream, "\t%s %s; // parameter[%d]: %s\n", createTypeName(signature.Params().At(i).Type()), id, i, signature.Params().At(i).String())
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
		fmt.Fprintf(ctx.stream, "\t%s %s; // %s : %s\n", createTypeName(freeVar.Type()), id, freeVar.String(), freeVar.Type())
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
			fmt.Fprintf(ctx.stream, "\t%s %s;\n", createTypeName(local.Type().(*types.Pointer).Elem()), id)
		}

		ctx.foundValueSet = make(map[ssa.Value]struct{})
		for _, basicBlock := range function.DomPreorder() {
			for _, instr := range basicBlock.Instrs {
				if value, ok := instr.(ssa.Value); ok {
					ctx.emitValueDeclaration(value)
				}
				switch instr := instr.(type) {
				case *ssa.MapUpdate:
					if key, ok := instr.Key.(*ssa.Const); ok {
						id := fmt.Sprintf("tmp_%p", key)
						fmt.Fprintf(ctx.stream, "\t%s %s; // %s : %s\n", createTypeName(key.Type()), id, key, key.Type())
					}
					if value, ok := instr.Value.(*ssa.Const); ok {
						id := fmt.Sprintf("tmp_%p", value)
						fmt.Fprintf(ctx.stream, "\t%s %s; // %s : %s\n", createTypeName(value.Type()), id, value, value.Type())
					}
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

func (ctx *Context) emitType() {
	ctx.visitAllTypes(ctx.program, func(typ types.Type) {
		name := createTypeName(typ)
		switch typ := typ.(type) {
		case *types.Array, *types.Pointer, *types.Struct, *types.Tuple:
			fmt.Fprintf(ctx.stream, "typedef struct %s %s; // %s\n", name, name, typ)

		case *types.Basic, *types.Chan, *types.Interface, *types.Map, *types.Signature:
			// do nothing

		case *types.Named:
			underlyingTypeName := createTypeName(typ.Underlying())
			fmt.Fprintf(ctx.stream, "typedef %s %s; // %s\n", underlyingTypeName, name, typ)

		case *types.Slice:
			fmt.Fprintf(ctx.stream, "typedef union %s %s; // %s\n", name, name, typ)

		default:
			if typ.String() == "iter" {
				return
			}

			panic(fmt.Sprintf("not implemented: %s %T", typ, typ))
		}
	})

	// emit Pointer types first to handle self referential structures
	ctx.visitAllTypes(ctx.program, func(typ types.Type) {
		if t, ok := typ.(*types.Pointer); ok {
			name := createTypeName(t)
			elemType := t.Elem()
			fmt.Fprintf(ctx.stream, "struct %s { // %s\n", name, typ)
			fmt.Fprintf(ctx.stream, "\t%s* raw;\n", createTypeName(elemType))
			fmt.Fprintf(ctx.stream, "};\n")
		}
	})

	ctx.visitAllTypes(ctx.program, func(typ types.Type) {
		name := createTypeName(typ)
		switch typ := typ.(type) {
		case *types.Array:
			fmt.Fprintf(ctx.stream, "struct %s { // %s\n", name, typ)
			fmt.Fprintf(ctx.stream, "\t%s raw[%d];\n", createTypeName(typ.Elem()), typ.Len())
			fmt.Fprintf(ctx.stream, "};\n")

		case *types.Basic, *types.Chan, *types.Interface, *types.Map, *types.Named, *types.Pointer, *types.Signature:
			// do nothing

		case *types.Slice:
			fmt.Fprintf(ctx.stream, `
union %s { // %s
	SliceObject raw;
	struct {
		%s* ptr;
		uintptr_t size;
		uintptr_t capacity;
	} typed;
};
`, name, typ, createTypeName(typ.Elem()))

		case *types.Struct:
			fmt.Fprintf(ctx.stream, "struct %s { // %s\n", name, typ)
			for i := 0; i < typ.NumFields(); i++ {
				field := typ.Field(i)
				fmt.Fprintf(ctx.stream, "\t%s %s; // %s\n", createTypeName(field.Type()), field.Name(), field)
			}
			fmt.Fprintf(ctx.stream, "};\n")

		case *types.Tuple:
			fmt.Fprintf(ctx.stream, "struct %s { // %s\n", name, typ)
			fmt.Fprintf(ctx.stream, "struct {\n")
			for i := 0; i < typ.Len(); i++ {
				fmt.Fprintf(ctx.stream, "\t%s e%d; // %s\n", createTypeName(typ.At(i).Type()), i, typ.At(i))
			}
			fmt.Fprintf(ctx.stream, "} raw;\n")
			fmt.Fprintf(ctx.stream, "};\n")

		default:
			if typ.String() == "iter" {
				return
			}

			panic(fmt.Sprintf("not implemented: %s %T", typ, typ))
		}
	})
}

func (ctx *Context) emitTypeInfo() {
	ctx.visitAllTypes(ctx.program, func(typ types.Type) {
		interfaceTableName := fmt.Sprintf("interfaceTable_%s", createTypeName(typ))
		numMethods := fmt.Sprintf("sizeof(%s.entries)/sizeof(%s.entries[0])", interfaceTableName, interfaceTableName)
		interfaceTable := fmt.Sprintf("&%s.entries[0]", interfaceTableName)

		fmt.Fprintf(ctx.stream, "const TypeInfo %s = {\n", createTypeIdName(typ))
		fmt.Fprintf(ctx.stream, ".name = \"%s\",\n", createTypeName(typ))
		fmt.Fprintf(ctx.stream, ".num_methods = %s,\n", numMethods)
		fmt.Fprintf(ctx.stream, ".interface_table = %s,\n", interfaceTable)
		fmt.Fprintf(ctx.stream, ".is_equal = equal_%s,\n", createTypeName(typ))
		fmt.Fprintf(ctx.stream, ".hash = hash_%s,\n", createTypeName(typ))
		fmt.Fprintf(ctx.stream, ".size = sizeof(%s),\n", createTypeName(typ))
		fmt.Fprintf(ctx.stream, "};\n")
	})
}

func (ctx *Context) emitEqualFunctionDeclaration() {
	builtinTypes := []string{
		"Bool", "Float32", "Float64",
		"Int", "Int8", "Int16", "Int32", "Int64",
		"UnsafePointer",
		"Uint", "Uint8", "Uint16", "Uint32", "Uint64",
	}
	for _, bt := range builtinTypes {
		fmt.Fprintf(ctx.stream, "bool equal_%sObject(%sObject* lhs, %sObject* rhs);", bt, bt, bt)
	}
	fmt.Fprintf(ctx.stream, `
bool equal_Value(Value* lhs, Value* rhs);
bool equal_Func(Func* lhs, Func* rhs);
bool equal_InvalidObject(InvalidObject* lhs, InvalidObject* rhs);
bool equal_StringObject(StringObject* lhs, StringObject* rhs);
bool equal_MapObject(MapObject* lhs, MapObject* rhs);
bool equal_Interface(Interface* lhs, Interface* rhs);
`)
	ctx.visitAllTypes(ctx.program, func(typ types.Type) {
		typeName := createTypeName(typ)
		switch typ.(type) {
		case *types.Basic, *types.Interface, *types.Map:
			return
		}
		fmt.Fprintf(ctx.stream, "bool equal_%s(%s* lhs, %s* rhs); // %s\n", typeName, typeName, typeName, typ)
	})
}

func (ctx *Context) emitEqualFunctionDefinition() {
	builtinTypes := []string{
		"Bool", "Float32", "Float64",
		"Int", "Int8", "Int16", "Int32", "Int64",
		"UnsafePointer",
		"Uint", "Uint8", "Uint16", "Uint32", "Uint64",
	}
	for _, bt := range builtinTypes {
		fmt.Fprintf(ctx.stream, `
		bool equal_%sObject(%sObject* lhs, %sObject* rhs) {
			return lhs->raw == rhs->raw;
		}
		`, bt, bt, bt)
	}
	fmt.Fprintf(ctx.stream, `
bool equal_Value(Value* lhs, Value* rhs) {
	assert(lhs != NULL);
	assert(rhs != NULL);
	return memcmp(lhs, rhs, sizeof(*lhs)) == 0;
}

bool equal_Func(Func* lhs, Func* rhs) {
	assert(lhs != NULL);
	assert(rhs != NULL);
	return memcmp(lhs, rhs, sizeof(*lhs)) == 0;
}

bool equal_InvalidObject(InvalidObject* lhs, InvalidObject* rhs) {
	assert(lhs != NULL);
	assert(rhs != NULL);
	return true;
}

bool equal_StringObject(StringObject* lhs, StringObject* rhs) {
	assert(lhs != NULL);
	assert(rhs != NULL);
	return strcmp(lhs->raw, rhs->raw) == 0;
}

bool equal_MapObject(MapObject* lhs, MapObject* rhs) {
	assert(lhs != NULL);
	assert(rhs != NULL);
	if(lhs->raw == rhs->raw) {
		return true;
	}
	if((lhs->raw == NULL) || (rhs->raw == NULL)) {
		return false;
	}
	assert(false); // ToDo: unimplemented
	return false;
}

bool equal_Interface(Interface* lhs, Interface* rhs) {
	(void)lhs;
	(void)rhs;
	assert(false); // dummy
	return false;
}

bool equal_InterfaceEmpty(Interface* lhs, Interface* rhs) {
	assert(lhs!=NULL);
	assert(rhs!=NULL);

	if ((lhs->receiver == NULL) && (rhs->receiver == NULL)) {
		return true;
	}
	if ((lhs->receiver == NULL) || (rhs->receiver == NULL)) {
		return false;
	}

	bool (*f)(void*, void*) = lhs->type_id.info->is_equal;
	return f(lhs->receiver, rhs->receiver);
}

bool equal_InterfaceNonEmpty(Interface* lhs, Interface* rhs) {
	assert(lhs!=NULL);
	assert(rhs!=NULL);

	if ((lhs->receiver == NULL) && (rhs->receiver == NULL)) {
		return true;
	}
	if ((lhs->receiver == NULL) || (rhs->receiver == NULL)) {
		return false;
	}

	bool (*f)(void*, void*) = lhs->type_id.info->is_equal;
	return f(lhs, rhs);
}
`)
	ctx.visitAllTypes(ctx.program, func(typ types.Type) {
		typeName := createTypeName(typ)
		underlyingType := typ.Underlying()
		var body = ""
		body += "\tassert(lhs != NULL);\n"
		body += "\tassert(rhs != NULL);\n"
		if typ == underlyingType {
			switch t := typ.(type) {
			case *types.Basic, *types.Interface, *types.Map:
				return
			case *types.Struct:
				for i := 0; i < t.NumFields(); i++ {
					field := t.Field(i)
					name := field.Name()
					body += fmt.Sprintf("if (!equal_%s(&lhs->%s, &rhs->%s)) { return false; } // %s\n", createTypeName(field.Type()), name, name, field)
				}
				body += "return true;"
			default:
				body += "return memcmp(lhs, rhs, sizeof(*lhs)) == 0;"
			}
		} else {
			if t, ok := underlyingType.(*types.Interface); ok {
				if t.Empty() {
					body = "return equal_InterfaceEmpty(lhs, rhs);\n"
				} else {
					body = "return equal_InterfaceNonEmpty(lhs, rhs);\n"
				}
			} else {
				body += fmt.Sprintf("return equal_%s(lhs, rhs);\n", createTypeName(underlyingType))
			}
		}
		fmt.Fprintf(ctx.stream, "bool equal_%s(%s* lhs, %s* rhs) { // %s\n", typeName, typeName, typeName, typ)
		fmt.Fprintf(ctx.stream, "%s", body)
		fmt.Fprintf(ctx.stream, "}\n")
	})
}

func (ctx *Context) emitHashFunctionDeclaration() {
	builtinTypes := []string{
		"Bool", "Float32", "Float64",
		"Int", "Int8", "Int16", "Int32", "Int64",
		"UnsafePointer",
		"Uint", "Uint8", "Uint16", "Uint32", "Uint64",
	}
	for _, bt := range builtinTypes {
		fmt.Fprintf(ctx.stream, "uintptr_t hash_%sObject(%sObject* obj);", bt, bt)
	}
	fmt.Fprintf(ctx.stream, `
uintptr_t hash_Value(Value* obj);
uintptr_t hash_Func(Func* obj);
uintptr_t hash_InvalidObject(InvalidObject* obj);
uintptr_t hash_StringObject(StringObject* obj);
uintptr_t hash_MapObject(MapObject* obj);
uintptr_t hash_Interface(Interface* obj);
`)
	ctx.visitAllTypes(ctx.program, func(typ types.Type) {
		typeName := createTypeName(typ)
		switch typ.(type) {
		case *types.Basic, *types.Interface, *types.Map:
			return
		}
		fmt.Fprintf(ctx.stream, "uintptr_t hash_%s(%s* obj); // %s\n", typeName, typeName, typ)
	})
}

func (ctx *Context) emitHashFunctionDefinition() {
	builtinTypes := []string{
		"Bool", "Float32", "Float64",
		"Int", "Int8", "Int16", "Int32", "Int64",
		"UnsafePointer",
		"Uint", "Uint8", "Uint16", "Uint32", "Uint64",
	}
	for _, bt := range builtinTypes {
		fmt.Fprintf(ctx.stream, `
		uintptr_t hash_%sObject(%sObject* obj) {
			return (uintptr_t)obj->raw;
		}
		`, bt, bt)
	}
	fmt.Fprintf(ctx.stream, `
uintptr_t hash_Value(Value* obj) {
	assert(obj != NULL);
	assert(false); /// not implemented
	return 0;
}

uintptr_t hash_Func(Func* obj) {
	assert(obj != NULL);
	assert(false); /// not implemented
	return 0;
}

uintptr_t hash_InvalidObject(InvalidObject* obj) {
	assert(obj != NULL);
	assert(false); /// not implemented
	return 0;
}

uintptr_t hash_StringObject(StringObject* obj) {
	assert(obj != NULL);
	assert(false); /// not implemented
	return 0;
}

uintptr_t hash_MapObject(MapObject* obj) {
	assert(obj != NULL);
	assert(false); /// not implemented
	return 0;
}

uintptr_t hash_Interface(Interface* obj) {
	(void)obj;
	assert(false); /// not implemented
	return 0;
}

uintptr_t hash_InterfaceEmpty(Interface* obj) {
	assert(obj!=NULL);
	assert(false); /// not implemented
	return 0;
}

uintptr_t hash_InterfaceNonEmpty(Interface* obj) {
	assert(obj!=NULL);
	assert(false); /// not implemented
	return 0;
}
`)
	ctx.visitAllTypes(ctx.program, func(typ types.Type) {
		typeName := createTypeName(typ)
		underlyingType := typ.Underlying()
		var body = ""
		body += "\tassert(obj != NULL);\n"
		if typ == underlyingType {
			switch t := typ.(type) {
			case *types.Basic, *types.Interface, *types.Map:
				return
			case *types.Struct:
				body += "uintptr_t hash = 0;\n"
				for i := 0; i < t.NumFields(); i++ {
					field := t.Field(i)
					name := field.Name()
					body += fmt.Sprintf("hash += hash_%s(&obj->%s); // %s\n", createTypeName(field.Type()), name, field)
				}
				body += "return hash;\n"
			default:
				body += "assert(false); /// not implemented\n"
				body += "return 0;\n"
			}
		} else {
			if t, ok := underlyingType.(*types.Interface); ok {
				if t.Empty() {
					body = "return hash_InterfaceEmpty(obj);\n"
				} else {
					body = "return hash_InterfaceNonEmpty(obj);\n"
				}
			} else {
				body += fmt.Sprintf("return hash_%s(obj);\n", createTypeName(underlyingType))
			}
		}
		fmt.Fprintf(ctx.stream, "uintptr_t hash_%s(%s* obj) { // %s\n", typeName, typeName, typ)
		fmt.Fprintf(ctx.stream, "%s", body)
		fmt.Fprintf(ctx.stream, "}\n")
	})
}

func (ctx *Context) emitInterfaceTable() {
	mainPkg := findMainPackage(ctx.program)
	allowSet := make(map[string]struct{})
	for member := range mainPkg.Members {
		typ, ok := mainPkg.Members[member].(*ssa.Type)
		if !ok {
			continue
		}
		t := typ.Type()
		allowSet[createTypeName(types.NewPointer(t))] = struct{}{}
	}
	ctx.visitAllTypes(ctx.program, func(typ types.Type) {
		methodSet := ctx.program.MethodSets.MethodSet(typ)
		entryIndexes := make([]int, 0)
		if _, ok := allowSet[createTypeName(typ)]; ok {
			for i := 0; i < methodSet.Len(); i++ {
				function := ctx.program.MethodValue(methodSet.At(i))
				if function != nil {
					entryIndexes = append(entryIndexes, i)
				}
			}
		}
		fmt.Fprintf(ctx.stream, "struct {\n")
		fmt.Fprintf(ctx.stream, "\tInterfaceTableEntry entries[%d];\n", len(entryIndexes))
		fmt.Fprintf(ctx.stream, "} interfaceTable_%s = {{\n", createTypeName(typ))
		for _, index := range entryIndexes {
			function := ctx.program.MethodValue(methodSet.At(index))
			methodName := function.Name()
			method := wrapInFunctionObject(createFunctionName(function))
			fmt.Fprintf(ctx.stream, "\t{\"%s\", %s},\n", methodName, method)
		}
		fmt.Fprintln(ctx.stream, "}};")
	})
}

func (ctx *Context) emitGlobalVariable(gv *ssa.Global) {
	name := createValueName(gv)
	fmt.Fprintf(ctx.stream, "%s %s;\n", createTypeName(gv.Type().(*types.Pointer).Elem()), name)
}

func (ctx *Context) emitRuntimeInfo() {
	fmt.Fprintln(ctx.stream, "Func runtime_info_funcs[] = {")
	ctx.visitAllFunctions(ctx.program, func(function *ssa.Function) {
		fmt.Fprintf(ctx.stream, "{ (StringObject){.raw = \"main.%s\" }, (UserFunction){.func_ptr = %s} },\n", function.Name(), createFunctionName(function))
	})
	fmt.Fprintln(ctx.stream, "};")

	init_func_name := "f_S_init"

	fmt.Fprintf(ctx.stream, `
size_t runtime_info_get_funcs_count(void) {
	return sizeof(runtime_info_funcs)/sizeof(runtime_info_funcs[0]);
}

const Func* runtime_info_refer_func(size_t i) {
	return &runtime_info_funcs[i];
}

UserFunction runtime_info_get_entry_point(void) {
	return (UserFunction) { .func_ptr = f_S_main };
}

FunctionObject dummy_init (LightWeightThreadContext* ctx){
	assert(ctx->marker == 0xdeadbeef);
	StackFrame_f_S_init* frame = (void*)ctx->stack_pointer;
	assert(frame->common.free_vars == NULL);
	ctx->stack_pointer = frame->common.prev_stack_pointer;
	return frame->common.resume_func;
}

UserFunction runtime_info_get_init_point(void) {
	return (UserFunction) { .func_ptr = %s };
}
`, init_func_name)
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

func (ctx *Context) visitAllTypes(program *ssa.Program, procedure func(typ types.Type)) {
	foundTypeSet := make(map[string]struct{})
	var f func(typ types.Type)
	f = func(typ types.Type) {
		name := createTypeName(typ)
		_, ok := foundTypeSet[name]
		if ok {
			return
		}
		foundTypeSet[name] = struct{}{}

		switch typ := typ.(type) {
		case *types.Array:
			f(typ.Elem())

		case *types.Struct:
			for i := 0; i < typ.NumFields(); i++ {
				f(typ.Field(i).Type())
			}

		case *types.Basic:
			// do nothing

		case *types.Chan:
			// do nothing

		case *types.Interface:
			// do nothing

		case *types.Map:
			f(typ.Key())
			f(typ.Elem())

		case *types.Named:
			typeName := createTypeName(typ)
			if typeName == encode("Named<Value>") || typeName == encode("Named<Func>") { // ToDo: ignore standard library definition
				return
			}
			f(typ.Underlying())

		case *types.Pointer:
			f(typ.Elem())

		case *types.Signature:
			// do nothing

		case *types.Slice:
			f(typ.Elem())

		case *types.Tuple:
			for i := 0; i < typ.Len(); i++ {
				f(typ.At(i).Type())
			}

		default:
			if typ.String() == "iter" {
				/// iterator of map or string
				return
			}

			panic(fmt.Sprintf("not implemented: %s %T", typ, typ))
		}

		procedure(typ)
	}

	for _, typ := range types.Typ {
		switch typ.Kind() {
		case types.Complex64, types.Complex128, types.Invalid, types.UntypedComplex, types.UntypedFloat, types.UntypedInt, types.UntypedNil, types.UntypedRune, types.UntypedString:
			continue
		}
		f(typ)
	}

	mainPkg := findMainPackage(ctx.program)

	for member := range mainPkg.Members {
		typ, ok := mainPkg.Members[member].(*ssa.Type)
		if !ok {
			continue
		}
		f(typ.Type())
	}

	for member := range mainPkg.Members {
		gv, ok := mainPkg.Members[member].(*ssa.Global)
		if !ok {
			continue
		}
		f(gv.Type())
	}

	var g func(function *ssa.Function)
	g = func(function *ssa.Function) {
		sig := function.Signature

		for i := 0; i < sig.Results().Len(); i++ {
			f(sig.Results().At(i).Type())
		}

		if sig.Recv() != nil {
			f(sig.Recv().Type())
		}

		for i := 0; i < sig.Params().Len(); i++ {
			f(sig.Params().At(i).Type())
		}
	}

	ctx.visitAllFunctions(ctx.program, func(function *ssa.Function) {
		g(function)
	})

	libraryFunctions := findLibraryFunctions(ctx.program)
	for _, function := range libraryFunctions {
		g(function)
	}

	ctx.visitAllFunctions(ctx.program, func(function *ssa.Function) {
		if function.Blocks == nil {
			return
		}
		for _, basicBlock := range function.DomPreorder() {
			for _, instr := range basicBlock.Instrs {
				if value, ok := instr.(ssa.Value); ok {
					f(value.Type())
				}
			}
		}
	})
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

struct TypeInfo;

typedef union {
	uintptr_t id;
	const struct TypeInfo* info;
} TypeId;

#define DEFINE_BUILTIN_OBJECT_TYPE(name, raw_type) \
	typedef struct { raw_type raw; } name ## Object

DEFINE_BUILTIN_OBJECT_TYPE(Bool, bool);
DEFINE_BUILTIN_OBJECT_TYPE(Float32, float);
DEFINE_BUILTIN_OBJECT_TYPE(Float64, double);
DEFINE_BUILTIN_OBJECT_TYPE(Int, intptr_t);
DEFINE_BUILTIN_OBJECT_TYPE(Int8, int8_t);
DEFINE_BUILTIN_OBJECT_TYPE(Int16, int16_t);
DEFINE_BUILTIN_OBJECT_TYPE(Int32, int32_t);
DEFINE_BUILTIN_OBJECT_TYPE(Int64, int64_t);
DEFINE_BUILTIN_OBJECT_TYPE(String, const char*);
DEFINE_BUILTIN_OBJECT_TYPE(UnsafePointer, void*);
DEFINE_BUILTIN_OBJECT_TYPE(Uint, uintptr_t);
DEFINE_BUILTIN_OBJECT_TYPE(Uint8, uint8_t);
DEFINE_BUILTIN_OBJECT_TYPE(Uint16, uint16_t);
DEFINE_BUILTIN_OBJECT_TYPE(Uint32, uint32_t);
DEFINE_BUILTIN_OBJECT_TYPE(Uint64, uint64_t);

typedef struct {
	void* raw;
} ChannelObject;

typedef struct {
	const void* raw;
} FunctionObject;

typedef struct {
	void* raw;
} MapObject;

typedef struct {
	void* raw;
} InvalidObject;

typedef struct {
	MapObject obj;
	uintptr_t count;
} IterObject;

typedef struct StackFrameCommon {
	FunctionObject resume_func;
	struct StackFrameCommon* prev_stack_pointer;
	void* free_vars;
} StackFrameCommon;

typedef struct {
	GlobalContext* global_context;
	FunctionObject current_func;
	StackFrameCommon* stack_pointer;
	UserFunction prev_func;
	intptr_t marker;
} LightWeightThreadContext;

typedef struct {
	const char* method_name;
	FunctionObject method;
} InterfaceTableEntry;

typedef struct TypeInfo {
	const char* name;
	uintptr_t num_methods;
	const InterfaceTableEntry* interface_table;
	void* is_equal;
	void* hash;
	uintptr_t size;
} TypeInfo;

typedef struct {
	void* receiver;
	TypeId type_id;
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
	StringObject* result_ptr;
	StringObject lhs;
	StringObject rhs;
} StackFrameConcat;
DECLARE_RUNTIME_API(concat, StackFrameConcat);

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
	Interface* result_ptr;
	void* receiver;
	TypeId type_id;
} StackFrameMakeInterface;
DECLARE_RUNTIME_API(make_interface, StackFrameMakeInterface);

typedef struct {
	StackFrameCommon common;
	MapObject* result_ptr;
	TypeId key_type;
	TypeId value_type;
} StackFrameMakeMap;
DECLARE_RUNTIME_API(make_map, StackFrameMakeMap);

typedef struct {
	StackFrameCommon common;
	StringObject* result_ptr;
	SliceObject byte_slice;
} StackFrameMakeStringFromByteSlice;
DECLARE_RUNTIME_API(make_string_from_byte_slice, StackFrameMakeStringFromByteSlice);

typedef struct {
	StackFrameCommon common;
	StringObject* result_ptr;
	IntObject rune;
} StackFrameMakeStringFromRune;
DECLARE_RUNTIME_API(make_string_from_rune, StackFrameMakeStringFromRune);

typedef struct {
	StackFrameCommon common;
	StringObject* result_ptr;
	SliceObject rune_slice;
} StackFrameMakeStringFromRuneSlice;
DECLARE_RUNTIME_API(make_string_from_rune_slice, StackFrameMakeStringFromRuneSlice);

typedef struct {
	StackFrameCommon common;
	MapObject map;
	void* key;
	void* value;
	bool* found;
} StackFrameMapGet;
DECLARE_RUNTIME_API(map_get, StackFrameMapGet);

typedef struct {
	StackFrameCommon common;
	IntObject* result_ptr;
	MapObject map;
} StackFrameMapLen;
DECLARE_RUNTIME_API(map_len, StackFrameMapLen);

typedef struct {
	StackFrameCommon common;
	MapObject map;
	void* key;
	void* value;
	bool* found;
	uintptr_t* count;
} StackFrameMapNext;
DECLARE_RUNTIME_API(map_next, StackFrameMapNext);

typedef struct {
	StackFrameCommon common;
	MapObject map;
	void* key;
	void* value;
} StackFrameMapSet;
DECLARE_RUNTIME_API(map_set, StackFrameMapSet);

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
} StackFrameSchedule;
DECLARE_RUNTIME_API(schedule, StackFrameSchedule);

#define f_S_Gosched gox5_schedule

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

typedef struct {
	StackFrameCommon common;
	StringObject* result_ptr;
	StringObject base;
	intptr_t low;
	intptr_t high;
} StackFrameStrview;
DECLARE_RUNTIME_API(strview, StackFrameStrview);

// ToDo: WA to handle reflect.ValueOf

typedef struct {
	IntObject e0;
} Value;

#define Named_lt_Value_gt_ Value 
#define equal_Named_lt_Value_gt_ equal_Value

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

#define f_S_Pointer_S_Named___lt___Value___gt___ gox5_value_pointer

// ToDo: WA to handle runtime.FuncForPC

typedef struct {
	StringObject name;
	UserFunction function;
} Func;

#define Named_lt_Func_gt_ Func 
#define equal_Named_lt_Func_gt_ equal_Func

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

#define f_S_Name_S_Pointer___lt___Named___lt___Func___gt______gt___ gox5_func_name

// ToDo: WA to handle strings.Split

typedef struct {
	StackFrameCommon common;
	SliceObject* result_ptr;
	StringObject param0;
	StringObject param1;
} StackFrameSplit;
DECLARE_RUNTIME_API(func_for_pc, StackFrameFuncForPc);

#define f_S_Split gox5_split

FunctionObject gox5_search_method(Interface* interface, StringObject method_name);
`)

	ctx.emitType()
	ctx.emitEqualFunctionDeclaration()
	ctx.emitHashFunctionDeclaration()

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

	ctx.emitEqualFunctionDefinition()
	ctx.emitHashFunctionDefinition()
	ctx.emitInterfaceTable()
	ctx.emitTypeInfo()

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
