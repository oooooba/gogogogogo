package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"go/constant"
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
		if c >= 0x80 {
			panic(str)
		}
		if ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') {
			buf += string(c)
		} else {
			buf += fmt.Sprintf("_%02X_", c)
		}
	}
	return buf
}

func wrapInFunctionObject(s string) string {
	return fmt.Sprintf("(FunctionObject){.raw=%s}", s)
}

func wrapInObject(s string, t types.Type) string {
	return fmt.Sprintf("(%s){.raw=%s}", createTypeName(t), s)
}

func wrapInTypeId(typ types.Type) string {
	return fmt.Sprintf("(TypeId){ .info = &%s }", createTypeIdName(typ))
}

func createValueName(value ssa.Value) string {
	if _, ok := value.(*ssa.Const); ok {
		return encode(fmt.Sprintf("c$%s", strconv.QuoteToASCII(value.String())))
	} else if val, ok := value.(*ssa.Function); ok {
		return wrapInObject(createFunctionName(val), val.Type())
	} else if val, ok := value.(*ssa.Parameter); ok {
		for i, param := range val.Parent().Params {
			if val.Name() == param.Name() {
				return fmt.Sprintf("param%d", i)
			}
		}
		panic(fmt.Sprintf("unreachable: val=%s, params=%v", val, val.Parent().Params))
	} else if val, ok := value.(*ssa.Global); ok {
		packageName := createPackageName(val.Package())
		return encode(fmt.Sprintf("gv$%s$%s$%p", value.Name(), packageName, value))
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
			case types.Complex64:
				return fmt.Sprintf("Complex64Object")
			case types.Complex128:
				return fmt.Sprintf("Complex128Object")
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
			case types.String, types.UntypedString:
				return fmt.Sprintf("StringObject")
			case types.UnsafePointer:
				return fmt.Sprintf("UnsafePointerObject")
			case types.Uint:
				return fmt.Sprintf("UintObject")
			case types.Uint8:
				return fmt.Sprintf("Uint8Object")
			case types.Uint16:
				return fmt.Sprintf("Uint16Object")
			case types.Uint32:
				return fmt.Sprintf("Uint32Object")
			case types.Uint64:
				return fmt.Sprintf("Uint64Object")
			case types.Uintptr:
				return fmt.Sprintf("UintptrObject")
			}
		case *types.Chan:
			return fmt.Sprintf("Channel<%s>", f(t.Elem()))
		case *types.Interface:
			return fmt.Sprintf("Interface")
		case *types.Map:
			k := f(t.Key())
			v := f(t.Elem())
			return fmt.Sprintf("Map<%s$%s>", k, v)
		case *types.Named:
			return fmt.Sprintf("Named<%s$%s>", typ.String(), f(typ.Underlying()))
		case *types.Pointer:
			return fmt.Sprintf("Pointer<%s>", f(t.Elem()))
		case *types.Signature:
			return fmt.Sprintf("FunctionObject")
		case *types.Slice:
			return fmt.Sprintf("Slice<%s>", f(t.Elem()))
		case *types.Struct:
			return fmt.Sprintf("Struct<%s>", typ.String())
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
	switch typ.Underlying().(*types.Basic).Kind() {
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
	case types.Uintptr:
		return "uintptr_t"
	}
	panic(typ)
}

func createTypeIdName(typ types.Type) string {
	return fmt.Sprintf("runtime_info_type_%s", createTypeName(typ))
}

func createFieldName(field *types.Var) string {
	rawFieldName := field.Name()
	disallowedWords := []string{"_", "signed"} // ToDo: add C keywords
	for _, disallowedWord := range disallowedWords {
		if rawFieldName == disallowedWord {
			return fmt.Sprintf("%s_%p", rawFieldName, field)
		}
	}
	return rawFieldName
}

func (ctx *Context) switchFunction(nextFunction string, signature *types.Signature, signatureName string, result string, resumeFunction string, paramAndArgsHandler func()) {
	fmt.Fprintf(ctx.stream, "StackFrameCommon* next_frame = (StackFrameCommon*)(frame + 1);\n")
	fmt.Fprintf(ctx.stream, "assert(((uintptr_t)next_frame) %% sizeof(uintptr_t) == 0);\n")
	fmt.Fprintf(ctx.stream, "next_frame->resume_func = %s;\n", wrapInFunctionObject(resumeFunction))
	fmt.Fprintf(ctx.stream, "next_frame->prev_stack_pointer = ctx->stack_pointer;\n")

	if signature.Recv() != nil || signature.Results().Len() > 0 || signature.Params().Len() > 0 {
		fmt.Fprintf(ctx.stream, "%s* signature = (%s*)(next_frame + 1);\n", signatureName, signatureName)
	}

	if signature.Results().Len() > 0 {
		fmt.Fprintf(ctx.stream, "signature->result_ptr = &%s;\n", result)
	}

	paramAndArgsHandler()

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
	fmt.Fprintf(ctx.stream, "*next_frame = (%s){ 0 };\n", nextFunctionFrame)
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
		case types.Complex64, types.Complex128:
			fmt.Fprintln(ctx.stream, `fprintf(stderr, "(");`)
			fmt.Fprintf(ctx.stream, "builtin_print_float(creal(%s.raw));\n", createValueRelName(value))
			fmt.Fprintf(ctx.stream, "builtin_print_float(cimag(%s.raw));\n", createValueRelName(value))
			fmt.Fprintln(ctx.stream, `fprintf(stderr, "i)");`)
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
		case types.Uintptr:
			specifier = "lu"
		case types.Float32, types.Float64:
			fmt.Fprintf(ctx.stream, "builtin_print_float(%s.raw);\n", createValueRelName(value))
			return
		case types.String:
			specifier = "s"
		case types.UnsafePointer:
			specifier = "p"
		default:
			panic(fmt.Sprintf("%s, %s (%T)", value, t, t))
		}
		fmt.Fprintf(ctx.stream, "fprintf(stderr, \"%%%s\", %s.raw);\n", specifier, createValueRelName(value))
	default:
		fmt.Fprintf(ctx.stream, "assert(false); // not supported\n")
	}
}

func (ctx *Context) emitInstruction(instruction ssa.Instruction) {
	fmt.Fprintf(ctx.stream, "\t// %T (%s): %s\n", instruction, instruction.Parent(), instruction)
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
			fmt.Fprintf(ctx.stream, "%s_buf = (%s){};\n", v, createTypeName(elemType))
			fmt.Fprintf(ctx.stream, "%s* raw = &%s_buf;\n", createTypeName(elemType), v)
			fmt.Fprintf(ctx.stream, "%s = %s;\n", v, wrapInObject("raw", instr.Type()))
		}

	case *ssa.BinOp:
		needToCallRuntimeApi := false
		raw := ""
		switch op := instr.Op; op {
		case token.AND_NOT:
			raw = fmt.Sprintf("%s.raw & (~(%s.raw))", createValueRelName(instr.X), createValueRelName(instr.Y))
		case token.EQL, token.NEQ:
			equalFunc := fmt.Sprintf("equal_%s", createTypeName(instr.X.Type()))
			fmt.Fprintf(ctx.stream, "bool raw = %s(&%s, &%s) %s true;", equalFunc, createValueRelName(instr.X), createValueRelName(instr.Y), instr.Op)
			raw = "raw"
		case token.LSS, token.LEQ, token.GTR, token.GEQ:
			raw = fmt.Sprintf("%s.raw %s %s.raw", createValueRelName(instr.X), instr.Op.String(), createValueRelName(instr.Y))
		case token.ADD:
			if t, ok := instr.Type().Underlying().(*types.Basic); ok && t.Kind() == types.String {
				result := createValueRelName(instr)
				ctx.switchFunctionToCallRuntimeApi("gox5_string_append", "StackFrameStringAppend", createInstructionName(instr), &result, nil,
					paramArgPair{param: "lhs", arg: createValueRelName(instr.X)},
					paramArgPair{param: "rhs", arg: createValueRelName(instr.Y)},
				)
				needToCallRuntimeApi = true
			} else {
				raw = fmt.Sprintf("%s.raw %s %s.raw", createValueRelName(instr.X), instr.Op.String(), createValueRelName(instr.Y))
			}
		case token.SHL:
			var unsignedRawType string
			switch instr.Type().Underlying().(*types.Basic).Kind() {
			case types.Int, types.Int8, types.Int16, types.Int32, types.Int64:
				unsignedRawType = fmt.Sprintf("u%s", createRawTypeName(instr.X.Type()))
			case types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
				unsignedRawType = createRawTypeName(instr.X.Type())
			default:
				panic(fmt.Sprintf("%s", instr))
			}
			fmt.Fprintf(ctx.stream, "%s unsignedLhs = (%s)(%s.raw);\n", unsignedRawType, unsignedRawType, createValueRelName(instr.X))
			fmt.Fprintf(ctx.stream, "%s rhs = %s.raw;\n", createRawTypeName(instr.Y.Type()), createValueRelName(instr.Y))
			switch instr.Y.Type().Underlying().(*types.Basic).Kind() {
			case types.Int, types.Int8, types.Int16, types.Int32, types.Int64:
				fmt.Fprintln(ctx.stream, "assert(rhs>=0);")
			}
			raw = "(((size_t)rhs) < sizeof(unsignedLhs) * 8) ? (unsignedLhs << rhs) : 0"
		case token.SHR:
			var unsignedRawType string
			var overflowExpr string
			var calcExpr string
			bitLen := "sizeof(unsignedLhs) * 8"
			switch instr.Type().Underlying().(*types.Basic).Kind() {
			case types.Int, types.Int8, types.Int16, types.Int32, types.Int64:
				unsignedRawType = fmt.Sprintf("u%s", createRawTypeName(instr.X.Type()))
				overflowExpr = fmt.Sprintf("%s.raw < 0 ? ((%s)(-1)) : 0", createValueRelName(instr.X), unsignedRawType)
				calcExpr = fmt.Sprintf("rhs == 0 ? unsignedLhs : ((((%s) >> (%s - rhs)) << (%s - rhs)) | (unsignedLhs >> rhs))", overflowExpr, bitLen, bitLen)
			case types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
				unsignedRawType = createRawTypeName(instr.X.Type())
				overflowExpr = "0"
				calcExpr = "unsignedLhs >> rhs"
			default:
				panic(fmt.Sprintf("%s", instr))
			}
			fmt.Fprintf(ctx.stream, "%s unsignedLhs = (%s)(%s.raw);\n", unsignedRawType, unsignedRawType, createValueRelName(instr.X))
			fmt.Fprintf(ctx.stream, "%s rhs = %s.raw;\n", createRawTypeName(instr.Y.Type()), createValueRelName(instr.Y))
			switch instr.Y.Type().Underlying().(*types.Basic).Kind() {
			case types.Int, types.Int8, types.Int16, types.Int32, types.Int64:
				fmt.Fprintln(ctx.stream, "assert(rhs>=0);")
			}
			raw = fmt.Sprintf("((size_t)rhs) < %s ? (%s) : (%s)", bitLen, calcExpr, overflowExpr)
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
			signature := callCommon.Method.Type().(*types.Signature)
			signatureName := createSignatureName(signature, false, true)
			ctx.switchFunction(nextFunction, signature, signatureName, createValueRelName(instr), createInstructionName(instr), func() {
				paramBase := 0
				if signature.Recv() != nil {
					receiver := fmt.Sprintf("*(void**)(%s.receiver)", createValueRelName(callCommon.Value))
					fmt.Fprintf(ctx.stream, "signature->param0 = %s; // receiver: %s\n", receiver, signature.Recv())
					paramBase++
				}
				for i := 0; i < signature.Params().Len(); i++ {
					arg := callCommon.Args[i]
					fmt.Fprintf(ctx.stream, "signature->param%d = %s; // %s\n",
						paramBase+i, createValueRelName(arg), signature.Params().At(i))
				}
			})
		} else {
			switch callee := callCommon.Value.(type) {
			case *ssa.Builtin:
				complexNumberBitLength := func(v ssa.Value) uint {
					switch v.Type().(*types.Basic).Kind() {
					case types.Complex64:
						return 64
					case types.Complex128:
						return 128
					default:
						panic("unreachable")
					}
				}

				needToCallRuntimeApi := false
				raw := ""
				switch callee.Name() {
				case "append":
					result := createValueRelName(instr)
					result += ".raw"
					if t, ok := callCommon.Args[1].Type().(*types.Basic); ok && t.Kind() == types.String {
						if instr.Type().(*types.Slice).Elem().(*types.Basic).Kind() != types.Byte {
							panic(instr.String())
						}
						ctx.switchFunctionToCallRuntimeApi("gox5_slice_append_string", "StackFrameSliceAppendString", createInstructionName(instr), &result, nil,
							paramArgPair{param: "slice", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
							paramArgPair{param: "string", arg: createValueRelName(callCommon.Args[1])},
						)
					} else {
						ctx.switchFunctionToCallRuntimeApi("gox5_slice_append", "StackFrameSliceAppend", createInstructionName(instr), &result, nil,
							paramArgPair{param: "type_id", arg: wrapInTypeId(callCommon.Args[0].Type().Underlying().(*types.Slice).Elem())},
							paramArgPair{param: "lhs", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
							paramArgPair{param: "rhs", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[1]))},
						)
					}
					needToCallRuntimeApi = true

				case "cap":
					result := createValueRelName(instr)
					ctx.switchFunctionToCallRuntimeApi("gox5_slice_capacity", "StackFrameSliceCapacity", createInstructionName(instr), &result, nil,
						paramArgPair{param: "slice", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
					)
					needToCallRuntimeApi = true

				case "close":
					ctx.switchFunctionToCallRuntimeApi("gox5_channel_close", "StackFrameChannelClose", createInstructionName(instr), nil, nil,
						paramArgPair{param: "channel", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
					)
					needToCallRuntimeApi = true

				case "copy":
					result := createValueRelName(instr)
					if t, ok := callCommon.Args[1].Type().(*types.Basic); ok && t.Kind() == types.String {
						ctx.switchFunctionToCallRuntimeApi("gox5_slice_copy_string", "StackFrameSliceCopyString", createInstructionName(instr), &result, nil,
							paramArgPair{param: "src", arg: createValueRelName(callCommon.Args[1])},
							paramArgPair{param: "dst", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
						)
					} else {
						ctx.switchFunctionToCallRuntimeApi("gox5_slice_copy", "StackFrameSliceCopy", createInstructionName(instr), &result, nil,
							paramArgPair{param: "type_id", arg: wrapInTypeId(callCommon.Args[0].Type().(*types.Slice).Elem())},
							paramArgPair{param: "src", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[1]))},
							paramArgPair{param: "dst", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
						)
					}
					needToCallRuntimeApi = true

				case "complex":
					bitLength := complexNumberBitLength(instr)
					result := createValueRelName(instr)
					ctx.switchFunctionToCallRuntimeApi(
						fmt.Sprintf("gox5_complex%d_new", bitLength),
						fmt.Sprintf("StackFrameComplex%dNew", bitLength),
						createInstructionName(instr), &result, nil,
						paramArgPair{param: "real", arg: createValueRelName(callCommon.Args[0])},
						paramArgPair{param: "imaginary", arg: createValueRelName(callCommon.Args[1])},
					)
					needToCallRuntimeApi = true

				case "imag":
					bitLength := complexNumberBitLength(callCommon.Args[0])
					result := createValueRelName(instr)
					ctx.switchFunctionToCallRuntimeApi(
						fmt.Sprintf("gox5_complex%d_imaginary", bitLength),
						fmt.Sprintf("StackFrameComplex%dImaginary", bitLength),
						createInstructionName(instr), &result, nil,
						paramArgPair{param: "value", arg: createValueRelName(callCommon.Args[0])},
					)
					needToCallRuntimeApi = true

				case "len":
					switch t := callCommon.Args[0].Type().(type) {
					case *types.Basic:
						switch t.Kind() {
						case types.String:
							result := createValueRelName(instr)
							ctx.switchFunctionToCallRuntimeApi("gox5_string_length", "StackFrameStringLength", createInstructionName(instr), &result, nil,
								paramArgPair{param: "string", arg: createValueRelName(callCommon.Args[0])},
							)
							needToCallRuntimeApi = true
						default:
							panic(fmt.Sprintf("unsuported argument for len: %s (%s)", callCommon.Args[0], t))
						}
					case *types.Map:
						result := createValueRelName(instr)
						ctx.switchFunctionToCallRuntimeApi("gox5_map_len", "StackFrameMapLen", createInstructionName(instr), &result, nil,
							paramArgPair{param: "map", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
						)
						needToCallRuntimeApi = true
					case *types.Slice:
						result := createValueRelName(instr)
						ctx.switchFunctionToCallRuntimeApi("gox5_slice_size", "StackFrameSliceSize", createInstructionName(instr), &result, nil,
							paramArgPair{param: "slice", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
						)
						needToCallRuntimeApi = true
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

				case "real":
					bitLength := complexNumberBitLength(callCommon.Args[0])
					result := createValueRelName(instr)
					ctx.switchFunctionToCallRuntimeApi(
						fmt.Sprintf("gox5_complex%d_real", bitLength),
						fmt.Sprintf("StackFrameComplex%dReal", bitLength),
						createInstructionName(instr), &result, nil,
						paramArgPair{param: "value", arg: createValueRelName(callCommon.Args[0])},
					)
					needToCallRuntimeApi = true

				case "recover":
					result := createValueRelName(instr)
					ctx.switchFunctionToCallRuntimeApi("gox5_panic_recover", "StackFramePanicRecover", createInstructionName(instr), &result, nil)
					needToCallRuntimeApi = true

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
					signature := callCommon.Value.Type().(*types.Signature)
					signatureName := createSignatureName(signature, false, false)
					ctx.switchFunction(nextFunction, signature, signatureName, createValueRelName(instr), createInstructionName(instr), func() {
						paramBase := 0
						argBase := 0
						if signature.Recv() != nil {
							receiver := createValueRelName(callCommon.Args[0])
							fmt.Fprintf(ctx.stream, "signature->param0 = %s; // receiver: %s\n", receiver, signature.Recv())
							paramBase++
							argBase++
						}
						for i := 0; i < signature.Params().Len(); i++ {
							arg := callCommon.Args[argBase+i]
							fmt.Fprintf(ctx.stream, "signature->param%d = %s; // %s\n", paramBase+i, createValueRelName(arg), signature.Params().At(i))
						}
					})
				}
			}
		}

	case *ssa.ChangeInterface:
		fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), createValueRelName(instr.X))

	case *ssa.ChangeType:
		s := wrapInObject(fmt.Sprintf("%s.raw", createValueRelName(instr.X)), instr.Type())
		fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), s)

	case *ssa.Convert:
		switch dstType := instr.Type().Underlying().(type) {
		case *types.Basic:
			switch dstType.Kind() {
			case types.String:
				result := createValueRelName(instr)
				switch srcType := instr.X.Type().(type) {
				case *types.Basic:
					arg := fmt.Sprintf("(IntObject){%s.raw}", createValueRelName(instr.X))
					ctx.switchFunctionToCallRuntimeApi("gox5_string_new_from_rune", "StackFrameStringNewFromRune", createInstructionName(instr), &result, nil,
						paramArgPair{param: "rune", arg: arg},
					)
				case *types.Slice:
					if elemType, ok := srcType.Elem().(*types.Basic); ok {
						switch elemType.Kind() {
						case types.Byte:
							arg := fmt.Sprintf("%s.raw", createValueRelName(instr.X))
							ctx.switchFunctionToCallRuntimeApi("gox5_string_new_from_byte_slice", "StackFrameStringNewFromByteSlice", createInstructionName(instr), &result, nil,
								paramArgPair{param: "byte_slice", arg: arg},
							)
						case types.Rune:
							arg := fmt.Sprintf("%s.raw", createValueRelName(instr.X))
							ctx.switchFunctionToCallRuntimeApi("gox5_string_new_from_rune_slice", "StackFrameStringNewFromRuneSlice", createInstructionName(instr), &result, nil,
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

			case types.Uintptr:
				var raw string
				if srcType, ok := instr.X.Type().Underlying().(*types.Basic); ok && srcType.Kind() == types.UnsafePointer {
					raw = fmt.Sprintf("(uintptr_t)%s.raw", createValueRelName(instr.X))
				} else {
					raw = fmt.Sprintf("%s.raw", createValueRelName(instr.X))
				}
				fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInObject(raw, instr.Type()))

			case types.UnsafePointer:
				var raw string
				if srcType, ok := instr.X.Type().Underlying().(*types.Basic); ok && srcType.Kind() == types.Uintptr {
					raw = fmt.Sprintf("(void*)%s.raw", createValueRelName(instr.X))
				} else {
					raw = fmt.Sprintf("%s.raw", createValueRelName(instr.X))
				}
				fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInObject(raw, instr.Type()))

			default:
				raw := fmt.Sprintf("%s.raw", createValueRelName(instr.X))
				fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInObject(raw, instr.Type()))
			}

		case *types.Slice:
			elemType := dstType.Elem().(*types.Basic)
			switch elemType.Kind() {
			case types.Byte, types.Rune:
				// valid conversion
			default:
				panic(instr.String())
			}
			srcType := instr.X.Type().(*types.Basic)
			if srcType.Kind() != types.String {
				panic(instr.String())
			}
			result := fmt.Sprintf("%s.raw", createValueRelName(instr))
			ctx.switchFunctionToCallRuntimeApi("gox5_slice_from_string", " StackFrameSliceFromString", createInstructionName(instr), &result, nil,
				paramArgPair{param: "type_id", arg: wrapInTypeId(elemType)},
				paramArgPair{param: "src", arg: createValueRelName(instr.X)},
			)

		default:
			raw := fmt.Sprintf("%s.raw", createValueRelName(instr.X))
			fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInObject(raw, instr.Type()))
		}

	case *ssa.Defer:
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

		ctx.switchFunctionToCallRuntimeApi("gox5_defer_register", "StackFrameDeferRegister", createInstructionName(instr), nil,
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

	case *ssa.Extract:
		fmt.Fprintf(ctx.stream, "%s = %s.raw.e%d;\n", createValueRelName(instr), createValueRelName(instr.Tuple), instr.Index)

	case *ssa.Field:
		name := createFieldName(instr.X.Type().Underlying().(*types.Struct).Field(instr.Field))
		fmt.Fprintf(ctx.stream, "%s val = %s.%s;\n", createTypeName(instr.Type()), createValueRelName(instr.X), name)
		fmt.Fprintf(ctx.stream, "%s = val;\n", createValueRelName(instr))

	case *ssa.FieldAddr:
		name := createFieldName(instr.X.Type().(*types.Pointer).Elem().Underlying().(*types.Struct).Field(instr.Field))
		fmt.Fprintf(ctx.stream, "%s* raw = &(%s.raw->%s);\n", createTypeName(instr.Type().(*types.Pointer).Elem()), createValueRelName(instr.X), name)
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
		switch t := instr.X.Type().Underlying().(type) {
		case *types.Slice:
			fmt.Fprintf(ctx.stream, "%s* raw = &((%s.typed.ptr)[index]);\n", createTypeName(t.Elem()), createValueRelName(instr.X))
		case *types.Pointer:
			fmt.Fprintf(ctx.stream, "%s* raw = &(%s.raw->raw[index]);\n", createTypeName(t.Elem().Underlying().(*types.Array).Elem()), createValueRelName(instr.X))
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
			switch xt.Kind() {
			case types.String, types.UntypedString:
				raw := fmt.Sprintf("%s.raw[%s.raw]", createValueRelName(instr.X), createValueRelName(instr.Index))
				fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInObject(raw, instr.Type()))
			default:
				panic(fmt.Sprintf("%s", instr))
			}
		case *types.Map:
			result := createValueRelName(instr)
			key := fmt.Sprintf("&%s", createValueRelName(instr.Index))
			var value, found string
			if instr.CommaOk {
				value = fmt.Sprintf("&%s.raw.e0", result)
				found = fmt.Sprintf("&%s.raw.e1.raw", result)
			} else {
				value = fmt.Sprintf("&%s", result)
				found = "NULL"
			}
			ctx.switchFunctionToCallRuntimeApi("gox5_map_get", "StackFrameMapGet", createInstructionName(instr), nil, nil,
				paramArgPair{param: "map", arg: fmt.Sprintf("%s.raw", createValueRelName(instr.X))},
				paramArgPair{param: "key", arg: key},
				paramArgPair{param: "value", arg: value},
				paramArgPair{param: "found", arg: found},
			)
		default:
			panic(fmt.Sprintf("%s", instr))
		}

	case *ssa.MakeChan:
		result := fmt.Sprintf("%s.raw", createValueRelName(instr))
		ctx.switchFunctionToCallRuntimeApi("gox5_channel_new", "StackFrameChannelNew", createInstructionName(instr), &result, nil,
			paramArgPair{param: "type_id", arg: wrapInTypeId(instr.Type().Underlying().(*types.Chan).Elem())},
			paramArgPair{param: "capacity", arg: createValueRelName(instr.Size)},
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
				if strings.HasSuffix(fn.Name(), "$bound") {
					if len(fn.FreeVars) != 1 {
						panic(fmt.Sprintf("fn: %s, free_vars: %s", fn, fn.FreeVars))
					}
					fmt.Fprintf(ctx.stream, "free_vars->receiver = %s;\n", createValueRelName(instr.Bindings[0]))
				} else {
					for i, freeVar := range fn.FreeVars {
						val := instr.Bindings[i]
						fmt.Fprintf(ctx.stream, "free_vars->%s = %s;\n", createValueName(freeVar), createValueRelName(val))
					}
				}
				fmt.Fprintf(ctx.stream, "next_frame->num_object_ptrs = sizeof(*free_vars) / sizeof(intptr_t);\n")
			},
			paramArgPair{param: "user_function", arg: userFunction},
		)

	case *ssa.MakeInterface:
		result := createValueRelName(instr)
		ctx.switchFunctionToCallRuntimeApi("gox5_make_interface", "StackFrameMakeInterface", createInstructionName(instr), &result, nil,
			paramArgPair{param: "receiver", arg: fmt.Sprintf("&%s", createValueRelName(instr.X))},
			paramArgPair{param: "type_id", arg: wrapInTypeId(instr.X.Type())},
		)

	case *ssa.MakeMap:
		result := fmt.Sprintf("%s.raw", createValueRelName(instr))
		ctx.switchFunctionToCallRuntimeApi("gox5_map_new", "StackFrameMapNew", createInstructionName(instr), &result, nil,
			paramArgPair{param: "key_type", arg: wrapInTypeId(instr.Type().Underlying().(*types.Map).Key())},
			paramArgPair{param: "value_type", arg: wrapInTypeId(instr.Type().Underlying().(*types.Map).Elem())},
		)

	case *ssa.MakeSlice:
		result := createValueRelName(instr)
		fmt.Fprintf(ctx.stream, "%s.typed.size = %s.raw;\n", result, createValueRelName(instr.Len))
		fmt.Fprintf(ctx.stream, "%s.typed.capacity = %s.raw;\n", result, createValueRelName(instr.Cap))
		ptr := fmt.Sprintf("%s.typed.ptr", result)
		size := fmt.Sprintf("(%s.raw) * sizeof(%s)", createValueRelName(instr.Cap), createTypeName(instr.Type().(*types.Slice).Elem()))
		ctx.switchFunctionToCallRuntimeApi("gox5_new", "StackFrameNew", createInstructionName(instr), &ptr, nil,
			paramArgPair{param: "size", arg: size},
		)

	case *ssa.MapUpdate:
		ctx.switchFunctionToCallRuntimeApi("gox5_map_set", "StackFrameMapSet", createInstructionName(instr), nil, nil,
			paramArgPair{param: "map", arg: fmt.Sprintf("%s.raw", createValueRelName(instr.Map))},
			paramArgPair{param: "key", arg: fmt.Sprintf("&%s", createValueRelName(instr.Key))},
			paramArgPair{param: "value", arg: fmt.Sprintf("&%s", createValueRelName(instr.Value))},
		)

	case *ssa.Next:
		result := createValueRelName(instr)
		iter := createValueRelName(instr.Iter)
		var rng string
		if instr.Type().(*types.Tuple).At(1).Type().(*types.Basic).Kind() == types.Invalid {
			rng = "NULL"
		} else {
			rng = fmt.Sprintf("&%s.raw.e1", result)
		}
		var dom string
		if instr.Type().(*types.Tuple).At(2).Type().(*types.Basic).Kind() == types.Invalid {
			dom = "NULL"
		} else {
			dom = fmt.Sprintf("&%s.raw.e2", result)
		}
		found := fmt.Sprintf("&%s.raw.e0.raw", result)
		count := fmt.Sprintf("&%s.count", iter)
		if instr.IsString {
			mp := fmt.Sprintf("%s.obj.string", iter)
			ctx.switchFunctionToCallRuntimeApi("gox5_string_next", "StackFrameStringNext", createInstructionName(instr), nil, nil,
				paramArgPair{param: "string", arg: mp},
				paramArgPair{param: "index", arg: rng},
				paramArgPair{param: "rune", arg: dom},
				paramArgPair{param: "found", arg: found},
				paramArgPair{param: "count", arg: count},
			)
		} else {
			mp := fmt.Sprintf("%s.obj.map", iter)
			ctx.switchFunctionToCallRuntimeApi("gox5_map_next", "StackFrameMapNext", createInstructionName(instr), nil, nil,
				paramArgPair{param: "map", arg: mp},
				paramArgPair{param: "key", arg: rng},
				paramArgPair{param: "value", arg: dom},
				paramArgPair{param: "found", arg: found},
				paramArgPair{param: "count", arg: count},
			)
		}

	case *ssa.Panic:
		ctx.switchFunctionToCallRuntimeApi("gox5_panic_raise", "StackFramePanicRaise", "NULL", nil, nil,
			paramArgPair{param: "value", arg: createValueRelName(instr.X)},
		)

	case *ssa.Phi:
		basicBlock := instr.Block()
		for i, edge := range instr.Edges {
			fmt.Fprintf(ctx.stream, "\tif (ctx->prev_func.func_ptr == %s) { %s = %s; } else\n",
				ctx.latestNameMap[basicBlock.Preds[i]], createValueRelName(instr), createValueRelName(edge))
		}
		fmt.Fprintln(ctx.stream, "\t{ assert(false); }")

	case *ssa.Range:
		if _, ok := instr.X.Type().(*types.Map); ok {
			fmt.Fprintf(ctx.stream, "%s = (IterObject){.obj = {.map = %s.raw}};\n", createValueRelName(instr), createValueRelName(instr.X))
		} else {
			fmt.Fprintf(ctx.stream, "%s = (IterObject){.obj = {.string = %s}};\n", createValueRelName(instr), createValueRelName(instr.X))
		}

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

	case *ssa.RunDefers:
		ctx.switchFunctionToCallRuntimeApi("gox5_defer_execute", "StackFrameDeferExecute", createInstructionName(instr), nil, nil)

	case *ssa.Select:
		result := createValueRelName(instr)
		ctx.switchFunctionToCallRuntimeApi("gox5_channel_select", "StackFrameChannelSelect", createInstructionName(instr), nil,
			func() {
				receive_count := 0
				for i, state := range instr.States {
					fmt.Fprintf(ctx.stream, "next_frame->entry_buffer[%d].channel = %s.raw;\n", i, createValueRelName(state.Chan))
					fmt.Fprintf(ctx.stream, "next_frame->entry_buffer[%d].type_id = %s;\n", i, wrapInTypeId(state.Chan.Type().(*types.Chan).Elem()))
					switch state.Dir {
					case types.SendRecv:
						panic("unreachable")
					case types.SendOnly:
						fmt.Fprintf(ctx.stream, "next_frame->entry_buffer[%d].send_data = &%s;\n", i, createValueRelName(state.Send))
						fmt.Fprintf(ctx.stream, "next_frame->entry_buffer[%d].receive_data = NULL;\n", i)
					case types.RecvOnly:
						fmt.Fprintf(ctx.stream, "next_frame->entry_buffer[%d].send_data = NULL;\n", i)
						fmt.Fprintf(ctx.stream, "next_frame->entry_buffer[%d].receive_data = &%s.raw.e%d;\n", i, result, receive_count+2)
						receive_count += 1
					}
				}
			},
			paramArgPair{param: "selected_index", arg: fmt.Sprintf("&%s.raw.e0", result)},
			paramArgPair{param: "receive_available", arg: fmt.Sprintf("&%s.raw.e1", result)},
			paramArgPair{param: "need_block", arg: fmt.Sprintf("%t", instr.Blocking)},
			paramArgPair{param: "entry_count", arg: fmt.Sprintf("%d", len(instr.States))},
		)

	case *ssa.Send:
		ctx.switchFunctionToCallRuntimeApi("gox5_channel_send", "StackFrameChannelSend", createInstructionName(instr), nil, nil,
			paramArgPair{param: "channel", arg: fmt.Sprintf("%s.raw", createValueRelName(instr.Chan))},
			paramArgPair{param: "data", arg: fmt.Sprintf("&%s", createValueRelName(instr.X))},
			paramArgPair{param: "type_id", arg: wrapInTypeId(instr.X.Type())},
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
			ctx.switchFunctionToCallRuntimeApi("gox5_string_substr", "StackFrameStringSubstr", createInstructionName(instr), &result, nil,
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
			switch t := instr.X.Type().Underlying().(type) {
			case *types.Pointer:
				ptr = "raw->raw"
				elemType := t.Elem().Underlying().(*types.Array)
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
				return fmt.Sprintf("*((%s*)%s.receiver)", createTypeName(instr.AssertedType), createValueRelName(instr.X))
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
				fmt.Fprintf(ctx.stream, "TypeId type_id = %s;\n", wrapInTypeId(instr.AssertedType))
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
			var typeId, data, available string
			if instr.CommaOk {
				typeId = wrapInTypeId(instr.Type().(*types.Tuple).At(0).Type())
				data = fmt.Sprintf("&%s.raw.e0", result)
				available = fmt.Sprintf("&%s.raw.e1.raw", result)
			} else {
				typeId = wrapInTypeId(instr.Type())
				data = fmt.Sprintf("&%s", result)
				available = "NULL"
			}
			ctx.switchFunctionToCallRuntimeApi("gox5_channel_receive", "StackFrameChannelReceive", createInstructionName(instr), nil, nil,
				paramArgPair{param: "channel", arg: fmt.Sprintf("%s.raw", createValueRelName(instr.X))},
				paramArgPair{param: "type_id", arg: typeId},
				paramArgPair{param: "data", arg: data},
				paramArgPair{param: "available", arg: available},
			)
		} else if instr.Op == token.XOR {
			s := wrapInObject(fmt.Sprintf("~(%s.raw)", createValueRelName(instr.X)), instr.Type())
			fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), s)
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
	function := instruction.Parent()
	functionName := function.Name()
	packageName := createPackageName(function.Package())
	return encode(fmt.Sprintf("i$%s$%s$%s$%p", instruction.Block().String(), functionName, packageName, instruction))
}

func createBasicBlockName(basicBlock *ssa.BasicBlock) string {
	function := basicBlock.Parent()
	functionName := function.Name()
	packageName := createPackageName(function.Package())
	return encode(fmt.Sprintf("b$%s$%s$%s$%p", basicBlock.String(), functionName, packageName, basicBlock))
}

func createFunctionName(function *ssa.Function) string {
	return encode(fmt.Sprintf("f$%s", function.RelString(nil)))
}

func createPackageName(pkg *ssa.Package) string {
	if pkg == nil {
		return "nil"
	}
	return pkg.Pkg.Name()
}

func requireSwitchFunction(instruction ssa.Instruction) bool {
	switch t := instruction.(type) {
	case *ssa.Alloc:
		return instruction.(*ssa.Alloc).Heap
	case *ssa.BinOp:
		if t.Op == token.ADD {
			if tt, ok := t.Type().Underlying().(*types.Basic); ok && tt.Kind() == types.String {
				return true
			}
		}
		return false
	case *ssa.Call:
		return t.Common().Value.Name() != "init"
	case *ssa.Convert:
		if dstType, ok := t.Type().(*types.Basic); ok && dstType.Kind() == types.String {
			return true
		}
		if _, ok := t.Type().(*types.Slice); ok {
			return true
		}
		return false
	case *ssa.Defer, *ssa.Go, *ssa.MakeChan, *ssa.MakeClosure, *ssa.MakeInterface, *ssa.MakeMap, *ssa.MakeSlice, *ssa.MapUpdate, *ssa.RunDefers, *ssa.Select, *ssa.Send:
		return true
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

func createSignatureName(signature *types.Signature, makesReceiverBound bool, makesReceiverInterface bool) string {
	name := ""

	if signature.Recv() != nil && !makesReceiverBound && makesReceiverInterface {
		name += "Interface"
	}
	name += "Signature$"

	name += "Params$"
	if signature.Recv() != nil && !makesReceiverBound && !makesReceiverInterface {
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

func (ctx *Context) tryEmitSignatureDefinition(signature *types.Signature, signatureName string, makesReceiverBound bool, makesReceiverInterface bool) {
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
	if signature.Recv() != nil && !makesReceiverBound {
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
	if signature.Recv() != nil {
		receiverBoundFuncName := fmt.Sprintf("%s%s", createFunctionName(function), encode("$bound"))
		ctx.emitFunctionHeader(receiverBoundFuncName, ";")

		fmt.Fprintf(ctx.stream, "typedef struct {\n")
		fmt.Fprintf(ctx.stream, "\t%s receiver; // %s\n", createTypeName(signature.Recv().Type()), signature)
		fmt.Fprintf(ctx.stream, "} FreeVars_%s;\n", receiverBoundFuncName)

		receiverBoundSignatureName := createSignatureName(signature, true, false)
		fmt.Fprintf(ctx.stream, "typedef struct {\n")
		fmt.Fprintf(ctx.stream, "\tStackFrameCommon common;\n")
		fmt.Fprintf(ctx.stream, "\t%s signature;\n", receiverBoundSignatureName)
		fmt.Fprintf(ctx.stream, "} StackFrame_%s;\n", receiverBoundFuncName)
	}

	fmt.Fprintf(ctx.stream, "typedef struct {\n")
	for _, freeVar := range function.FreeVars {
		fmt.Fprintf(ctx.stream, "\t// found %T: %s, %s\n", freeVar, createValueName(freeVar), freeVar.String())
		id := fmt.Sprintf("%s", createValueName(freeVar))
		fmt.Fprintf(ctx.stream, "\t%s %s; // %s : %s\n", createTypeName(freeVar.Type()), id, freeVar.String(), freeVar.Type())
	}
	fmt.Fprintf(ctx.stream, "} FreeVars_%s;\n", createFunctionName(function))

	concreteSignatureName := createSignatureName(signature, false, false)
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

		ctx.visitValue(function, func(value ssa.Value) {
			switch value.(type) {
			case *ssa.Builtin, *ssa.Const, *ssa.Global, *ssa.FreeVar, *ssa.Function, *ssa.Parameter:
				return
			}

			if t, ok := value.Type().(*types.Tuple); ok {
				if t.Len() == 0 {
					return
				}
			}

			if value.Parent() == nil {
				panic(fmt.Sprintf("%s, %T", value, value))
			}

			id := createValueName(value)
			fmt.Fprintf(ctx.stream, "\t%s %s; // %s : %s\n", createTypeName(value.Type()), id, value, value.Type())
		})
	}

	fmt.Fprintf(ctx.stream, "} StackFrame_%s;\n", createFunctionName(function))

	if function.Blocks == nil {
		return
	}

	for _, basicBlock := range function.Blocks {
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

func (ctx *Context) emitFunctionDefinitionPrologue(functionName string, frameName string, hasFreeVariables bool) {
	ctx.emitFunctionHeader(functionName, "{")
	freeVarsCompareOp := "=="
	if hasFreeVariables {
		freeVarsCompareOp = "!="
	}
	fmt.Fprintf(ctx.stream, `
	StackFrame_%s* frame = (void*)ctx->stack_pointer;
	assert(frame->common.free_vars %s NULL);
`, frameName, freeVarsCompareOp)
}

func (ctx *Context) emitFunctionDefinitionEpilogue() {
	fmt.Fprintln(ctx.stream, "}")
}

func (ctx *Context) emitFunctionDefinition(function *ssa.Function) {
	ctx.emitFunctionHeader(createFunctionName(function), "{")
	fmt.Fprintf(ctx.stream, "\tassert(ctx->marker == 0xdeadbeef);\n")
	fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createBasicBlockName(function.Blocks[0])))
	fmt.Fprintf(ctx.stream, "}\n")

	frameName := createFunctionName(function)
	hasFreeVariables := len(function.FreeVars) != 0
	for _, basicBlock := range function.Blocks {
		ctx.emitFunctionDefinitionPrologue(createBasicBlockName(basicBlock), frameName, hasFreeVariables)

		for _, instr := range basicBlock.Instrs {
			ctx.emitInstruction(instr)

			if requireSwitchFunction(instr) {
				ctx.emitFunctionDefinitionEpilogue()
				ctx.emitFunctionDefinitionPrologue(createInstructionName(instr), frameName, hasFreeVariables)
			}
		}

		ctx.emitFunctionDefinitionEpilogue()
	}

	signature := function.Signature
	if signature.Recv() == nil {
		return
	}

	origFuncName := createFunctionName(function)
	boundFuncName := fmt.Sprintf("%s%s", origFuncName, encode("$bound"))
	resumeFuncName := fmt.Sprintf("%s_return", boundFuncName)
	ctx.emitFunctionDefinitionPrologue(resumeFuncName, boundFuncName, true)
	fmt.Fprintf(ctx.stream, `
	assert(ctx->marker == 0xdeadbeef);
	ctx->stack_pointer = frame->common.prev_stack_pointer;
	return frame->common.resume_func;
`)
	ctx.emitFunctionDefinitionEpilogue()

	ctx.emitFunctionDefinitionPrologue(boundFuncName, boundFuncName, true)
	nextFuncName := wrapInFunctionObject(origFuncName)
	signatureName := createSignatureName(signature, false, false)
	result := "*frame->signature.result_ptr"
	ctx.switchFunction(nextFuncName, signature, signatureName, result, resumeFuncName, func() {
		for i := 0; i < signature.Params().Len(); i++ {
			fmt.Fprintf(ctx.stream, "signature->param%d = frame->signature.param%d;\n", i+1, i)
		}
		fmt.Fprintf(ctx.stream, "signature->param0 = ((FreeVars_%s*)(frame->common.free_vars))->receiver;\n", boundFuncName)
	})
	ctx.emitFunctionDefinitionEpilogue()
}

func (ctx *Context) retrieveOrderedTypes() []types.Type {
	orderedTypes := make([]types.Type, 0)

	foundTypeSet := make(map[string]struct{})
	var f func(typ types.Type)
	f = func(typ types.Type) {
		name := createTypeName(typ)
		if _, ok := foundTypeSet[name]; ok {
			return
		}

		switch typ := typ.(type) {
		case *types.Array:
			f(typ.Elem())

		case *types.Struct:
			for i := 0; i < typ.NumFields(); i++ {
				f(typ.Field(i).Type())
			}

		case *types.Basic: // do nothing

		case *types.Chan: // do nothing

		case *types.Interface: // do nothing

		case *types.Map:
			f(typ.Key())
			f(typ.Elem())

		case *types.Named:
			f(typ.Underlying())

		case *types.Pointer: // do nothing (should not enter)

		case *types.Signature: // do nothing

		case *types.Slice:
			f(typ.Elem())

		case *types.Tuple:
			for i := 0; i < typ.Len(); i++ {
				f(typ.At(i).Type())
			}

		default:
			if typ.String() == "iter" {
				/// iterator of map or string
			} else {
				panic(fmt.Sprintf("not implemented: %s %T", typ, typ))
			}
		}

		foundTypeSet[name] = struct{}{}
		orderedTypes = append(orderedTypes, typ)
	}

	ctx.visitAllTypes(ctx.program, func(typ types.Type) {
		f(typ)
	})

	return orderedTypes
}

func (ctx *Context) emitType() {
	orderedTypes := ctx.retrieveOrderedTypes()

	for _, typ := range orderedTypes {
		name := createTypeName(typ)
		switch typ := typ.(type) {
		case *types.Array, *types.Chan, *types.Map, *types.Pointer, *types.Struct, *types.Tuple:
			fmt.Fprintf(ctx.stream, "typedef struct %s %s; // %s\n", name, name, typ)

		case *types.Basic, *types.Interface, *types.Signature:
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
	}

	// emit Pointer types first to handle self referential structures
	for _, typ := range orderedTypes {
		if t, ok := typ.(*types.Pointer); ok {
			name := createTypeName(t)
			elemType := t.Elem()
			fmt.Fprintf(ctx.stream, "struct %s { // %s\n", name, typ)
			fmt.Fprintf(ctx.stream, "\t%s* raw;\n", createTypeName(elemType))
			fmt.Fprintf(ctx.stream, "};\n")
		}
	}

	for _, typ := range orderedTypes {
		name := createTypeName(typ)
		switch typ := typ.(type) {
		case *types.Array:
			fmt.Fprintf(ctx.stream, "struct %s { // %s\n", name, typ)
			fmt.Fprintf(ctx.stream, "\t%s raw[%d];\n", createTypeName(typ.Elem()), typ.Len())
			fmt.Fprintf(ctx.stream, "};\n")

		case *types.Basic, *types.Interface, *types.Named, *types.Pointer, *types.Signature:
			// do nothing

		case *types.Chan:
			fmt.Fprintf(ctx.stream, "struct %s { ChannelObject raw; }; // %s\n", name, typ)

		case *types.Map:
			fmt.Fprintf(ctx.stream, "struct %s { MapObject raw; }; // %s\n", name, typ)

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
				fieldName := createFieldName(field)
				fmt.Fprintf(ctx.stream, "\t%s %s; // %s\n", createTypeName(field.Type()), fieldName, field)
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
	}
}

func (ctx *Context) emitSignature() {
	ctx.visitAllFunctions(ctx.program, func(function *ssa.Function) {
		signature := function.Signature
		concreteSignatureName := createSignatureName(signature, false, false)
		ctx.tryEmitSignatureDefinition(signature, concreteSignatureName, false, false)
		if signature.Recv() != nil {
			abstractSignatureName := createSignatureName(signature, false, true)
			ctx.tryEmitSignatureDefinition(signature, abstractSignatureName, false, true)

			receiverBoundSignatureName := createSignatureName(signature, true, false)
			ctx.tryEmitSignatureDefinition(signature, receiverBoundSignatureName, true, true)
		}

		for _, basicBlock := range function.Blocks {
			for _, instruction := range basicBlock.Instrs {
				var callCommon *ssa.CallCommon
				switch instr := instruction.(type) {
				case *ssa.Call:
					callCommon = instr.Common()
				case *ssa.Defer:
					callCommon = instr.Common()
				case *ssa.Go:
					callCommon = instr.Common()
				}
				if callCommon == nil {
					continue
				}
				signature := callCommon.Signature()
				signatureName := createSignatureName(signature, false, false)
				ctx.tryEmitSignatureDefinition(signature, signatureName, false, false)
			}
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

func (ctx *Context) emitConstant() {
	foundConstValueSet := make(map[string]struct{})
	ctx.visitAllFunctions(ctx.program, func(function *ssa.Function) {
		ctx.visitValue(function, func(value ssa.Value) {
			if cst, ok := value.(*ssa.Const); ok {
				valueName := createValueName(cst)
				if _, ok := foundConstValueSet[valueName]; ok {
					return
				}
				foundConstValueSet[valueName] = struct{}{}

				inner := "0"
				if !cst.IsNil() {
					inner = cst.Value.String()
					if t, ok := cst.Type().Underlying().(*types.Basic); ok {
						switch t.Kind() {
						case types.Complex64, types.Complex128, types.Float32, types.Float64:
							inner = fmt.Sprintf("%g", cst.Complex128())
						case types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
							inner = fmt.Sprintf("%su", inner)
						case types.Int, types.Int64:
							inner = fmt.Sprintf("%slu", inner)
						case types.String:
							inner = strconv.Quote(constant.StringVal(cst.Value))
						case types.UnsafePointer:
							inner = fmt.Sprintf("(void*)%su", inner)
						}
					}
				}

				var value string
				if t, ok := cst.Type().Underlying().(*types.Interface); ok {
					value = fmt.Sprintf("(%s){%s}", createTypeName(t), inner)
				} else {
					value = wrapInObject(inner, cst.Type())
				}

				typeName := createTypeName(cst.Type())
				fmt.Fprintf(ctx.stream, "__attribute__((unused)) static const %s %s = %s; // %s\n", typeName, valueName, value, cst)
			}
		})
	})
}

func (ctx *Context) emitEqualFunctionDeclaration() {
	ctx.visitAllTypes(ctx.program, func(typ types.Type) {
		typeName := createTypeName(typ)
		fmt.Fprintf(ctx.stream, "bool equal_%s(const %s* lhs, const %s* rhs); // %s\n", typeName, typeName, typeName, typ)
	})
}

func (ctx *Context) emitEqualFunctionDefinition() {
	fmt.Fprintf(ctx.stream, `
bool equal_MapObject(const MapObject* lhs, const MapObject* rhs) {
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

bool equal_Interface(const Interface* lhs, const Interface* rhs) {
	assert(lhs!=NULL);
	assert(rhs!=NULL);

	if (lhs->type_id.info == NULL && rhs->type_id.info == NULL) {
		return true;
	}

	if ((lhs->type_id.info == NULL) || (rhs->type_id.info == NULL)) {
		return false;
	}

	if ((lhs->receiver == NULL) && (rhs->receiver == NULL)) {
		return true;
	}

	if ((lhs->receiver == NULL) || (rhs->receiver == NULL)) {
		return false;
	}

	bool (*f)(const void*, const void*) = lhs->type_id.info->is_equal;
	return f(lhs->receiver, rhs->receiver);
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
			case *types.Basic:
				switch t.Kind() {
				case types.Invalid:
					body += "return true;\n"
				case types.String:
					body += "return strcmp(lhs->raw, rhs->raw) == 0;\n"
				default:
					body += "return lhs->raw == rhs->raw;\n"
				}
			case *types.Interface:
				return
			case *types.Map:
				body += "return equal_MapObject(&lhs->raw, &rhs->raw);\n"
			case *types.Struct:
				for i := 0; i < t.NumFields(); i++ {
					field := t.Field(i)
					if field.Name() == "_" {
						continue
					}
					name := createFieldName(field)
					body += fmt.Sprintf("if (!equal_%s(&lhs->%s, &rhs->%s)) { return false; } // %s\n", createTypeName(field.Type()), name, name, field)
				}
				body += "return true;"
			default:
				body += "return memcmp(lhs, rhs, sizeof(*lhs)) == 0;"
			}
		} else {
			body += fmt.Sprintf("return equal_%s(lhs, rhs);\n", createTypeName(underlyingType))
		}
		fmt.Fprintf(ctx.stream, "bool equal_%s(const %s* lhs, const %s* rhs) { // %s\n", typeName, typeName, typeName, typ)
		fmt.Fprintf(ctx.stream, "%s", body)
		fmt.Fprintf(ctx.stream, "}\n")
	})
}

func (ctx *Context) emitHashFunctionDeclaration() {
	ctx.visitAllTypes(ctx.program, func(typ types.Type) {
		typeName := createTypeName(typ)
		fmt.Fprintf(ctx.stream, "uintptr_t hash_%s(const %s* obj); // %s\n", typeName, typeName, typ)
	})
}

func (ctx *Context) emitHashFunctionDefinition() {
	fmt.Fprintf(ctx.stream, `
uintptr_t hash_Interface(const Interface* obj) {
	(void)obj;
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
			case *types.Basic:
				switch t.Kind() {
				case types.Invalid, types.String:
					body += "assert(false); /// not implemented\n"
					body += "return 0;\n"
				default:
					body += "return (uintptr_t)obj->raw;\n"
				}
			case *types.Interface:
				return
			case *types.Map:
				body += "assert(false); /// not implemented\n"
				body += "return 0;\n"
			case *types.Struct:
				body += "uintptr_t hash = 0;\n"
				for i := 0; i < t.NumFields(); i++ {
					field := t.Field(i)
					if field.Name() == "_" {
						continue
					}
					name := createFieldName(field)
					body += fmt.Sprintf("hash += hash_%s(&obj->%s); // %s\n", createTypeName(field.Type()), name, field)
				}
				body += "return hash;\n"
			default:
				body += "assert(false); /// not implemented\n"
				body += "return 0;\n"
			}
		} else {
			body += fmt.Sprintf("return hash_%s(obj);\n", createTypeName(underlyingType))
		}
		fmt.Fprintf(ctx.stream, "uintptr_t hash_%s(const %s* obj) { // %s\n", typeName, typeName, typ)
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
		packageName := createPackageName(function.Pkg)
		functionName := function.Name()
		fmt.Fprintf(ctx.stream, "{ (StringObject){.raw = \"%s.%s\" }, (UserFunction){.func_ptr = %s} },\n", packageName, functionName, createFunctionName(function))
	})
	fmt.Fprintln(ctx.stream, "};")

	mainPkg := findMainPackage(ctx.program)
	mainFunctionName := createFunctionName(mainPkg.Members["main"].(*ssa.Function))
	initFunctionName := createFunctionName(mainPkg.Members["init"].(*ssa.Function))

	fmt.Fprintf(ctx.stream, `
size_t runtime_info_get_funcs_count(void) {
	return sizeof(runtime_info_funcs)/sizeof(runtime_info_funcs[0]);
}

const Func* runtime_info_refer_func(size_t i) {
	return &runtime_info_funcs[i];
}

UserFunction runtime_info_get_entry_point(void) {
	return (UserFunction) { .func_ptr = %s };
}

UserFunction runtime_info_get_init_point(void) {
	return (UserFunction) { .func_ptr = %s };
}
`, mainFunctionName, initFunctionName)
}

func (ctx *Context) emitComplexNumberBuiltinFunctions() {
	for _, bitLength := range []int{64, 128} {
		elemBitLength := bitLength / 2

		fmt.Fprintf(ctx.stream, `
typedef struct {
	StackFrameCommon common;
	Complex%dObject* result_ptr;
	Float%dObject real;
	Float%dObject imaginary;
} StackFrameComplex%dNew;

__attribute__((unused)) static
FunctionObject gox5_complex%d_new(LightWeightThreadContext* ctx) {
	StackFrameComplex%dNew* frame = (void*)ctx->stack_pointer;
	*frame->result_ptr = (Complex%dObject){.raw = frame->real.raw + frame->imaginary.raw * I};
	ctx->stack_pointer = frame->common.prev_stack_pointer;
	return frame->common.resume_func;
}
`, bitLength, elemBitLength, elemBitLength, bitLength, bitLength, bitLength, bitLength)

		fmt.Fprintf(ctx.stream, `
typedef struct {
	StackFrameCommon common;
	Float%dObject* result_ptr;
	Complex%dObject value;
} StackFrameComplex%dReal;

__attribute__((unused)) static
FunctionObject gox5_complex%d_real(LightWeightThreadContext* ctx) {
	StackFrameComplex%dReal* frame = (void*)ctx->stack_pointer;
	*frame->result_ptr = (Float%dObject){.raw = creal(frame->value.raw)};
	ctx->stack_pointer = frame->common.prev_stack_pointer;
	return frame->common.resume_func;
}
`, elemBitLength, bitLength, bitLength, bitLength, bitLength, elemBitLength)

		fmt.Fprintf(ctx.stream, `
typedef struct {
	StackFrameCommon common;
	Float%dObject* result_ptr;
	Complex%dObject value;
} StackFrameComplex%dImaginary;

__attribute__((unused)) static
FunctionObject gox5_complex%d_imaginary(LightWeightThreadContext* ctx) {
	StackFrameComplex%dImaginary* frame = (void*)ctx->stack_pointer;
	*frame->result_ptr = (Float%dObject){.raw = cimag(frame->value.raw)};
	ctx->stack_pointer = frame->common.prev_stack_pointer;
	return frame->common.resume_func;
}
`, elemBitLength, bitLength, bitLength, bitLength, bitLength, elemBitLength)
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

	// Todo: replace `[]*ssa.Package{findMainPackage(program)}` to `program.AllPackages()`
	for _, pkg := range []*ssa.Package{findMainPackage(program)} {
		for member := range pkg.Members {
			switch member := pkg.Members[member].(type) {
			case *ssa.Function:
				f(member)
			case *ssa.Type:
				t := member.Type()
				g(t)
				g(types.NewPointer(t))
			}
		}
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
			f(typ.Elem())

		case *types.Interface:
			// do nothing

		case *types.Map:
			f(typ.Key())
			f(typ.Elem())

		case *types.Named:
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
		case types.Invalid, types.UntypedBool, types.UntypedComplex, types.UntypedFloat, types.UntypedInt, types.UntypedNil, types.UntypedRune, types.UntypedString:
			continue
		}
		f(typ)
	}

	for _, pkg := range program.AllPackages() {
		for member := range pkg.Members {
			switch member := pkg.Members[member].(type) {
			case *ssa.Type:
				f(member.Type())

			case *ssa.Global:
				f(member.Type())
			}
		}
	}

	var g func(function *ssa.Function)
	g = func(function *ssa.Function) {
		sig := function.Signature
		if sig.Recv() != nil {
			f(sig.Recv().Type())
		}
		f(sig.Params())
		f(sig.Results())
	}

	ctx.visitAllFunctions(ctx.program, func(function *ssa.Function) {
		g(function)
	})

	ctx.visitAllFunctions(ctx.program, func(function *ssa.Function) {
		if function.Blocks == nil {
			return
		}
		for _, basicBlock := range function.Blocks {
			for _, instr := range basicBlock.Instrs {
				if value, ok := instr.(ssa.Value); ok {
					f(value.Type())
				}
			}
		}
	})
}

func (ctx *Context) visitValue(function *ssa.Function, procedure func(value ssa.Value)) {
	foundValueSet := make(map[ssa.Value]struct{})
	var f func(value ssa.Value)
	g := func(callCommon *ssa.CallCommon) {
		f(callCommon.Value)
		for _, arg := range callCommon.Args {
			f(arg)
		}
	}
	f = func(value ssa.Value) {
		_, ok := foundValueSet[value]
		if ok {
			return
		}
		foundValueSet[value] = struct{}{}

		switch val := value.(type) {
		case *ssa.Alloc, *ssa.Builtin, *ssa.Const, *ssa.FreeVar, *ssa.Function, *ssa.Global, *ssa.Parameter:
			// do nothing

		case *ssa.BinOp:
			f(val.X)
			f(val.Y)

		case *ssa.Call:
			g(val.Common())

		case *ssa.ChangeInterface:
			f(val.X)

		case *ssa.ChangeType:
			f(val.X)

		case *ssa.Convert:
			f(val.X)

		case *ssa.Extract:
			f(val.Tuple)

		case *ssa.Field:
			f(val.X)

		case *ssa.FieldAddr:
			f(val.X)

		case *ssa.Index:
			f(val.X)
			f(val.Index)

		case *ssa.IndexAddr:
			f(val.X)
			f(val.Index)

		case *ssa.Lookup:
			f(val.X)
			f(val.Index)

		case *ssa.MakeChan:
			f(val.Size)

		case *ssa.MakeClosure:
			f(val.Fn)
			for _, freeVar := range val.Bindings {
				f(freeVar)
			}

		case *ssa.MakeInterface:
			f(val.X)

		case *ssa.MakeMap:
			if val.Reserve != nil {
				f(val.Reserve)
			}

		case *ssa.MakeSlice:
			f(val.Len)
			f(val.Cap)

		case *ssa.Next:
			f(val.Iter)

		case *ssa.Phi:
			for _, edge := range val.Edges {
				f(edge)
			}

		case *ssa.Range:
			f(val.X)

		case *ssa.Select:
			for _, state := range val.States {
				f(state.Chan)
				if state.Send != nil {
					f(state.Send)
				}
			}

		case *ssa.Slice:
			f(val.X)
			if val.Low != nil {
				f(val.Low)
			}
			if val.High != nil {
				f(val.High)
			}
			if val.Max != nil {
				f(val.Max)
			}

		case *ssa.TypeAssert:
			f(val.X)

		case *ssa.UnOp:
			f(val.X)

		default:
			value.Parent().WriteTo(os.Stderr)
			panic(fmt.Sprintf("unknown value: %s : %T", value.String(), value))
		}

		procedure(value)
	}

	for _, basicBlock := range function.Blocks {
		for _, instruction := range basicBlock.Instrs {
			switch instr := instruction.(type) {
			case ssa.Value:
				f(instr)

			case *ssa.Defer:
				g(instr.Common())

			case *ssa.Go:
				g(instr.Common())

			case *ssa.If:
				f(instr.Cond)

			case *ssa.Jump, *ssa.RunDefers:
				// do nothing

			case *ssa.MapUpdate:
				f(instr.Map)
				f(instr.Key)
				f(instr.Value)

			case *ssa.Panic:
				f(instr.X)

			case *ssa.Return:
				for _, result := range instr.Results {
					f(result)
				}

			case *ssa.Send:
				f(instr.Chan)
				f(instr.X)

			case *ssa.Store:
				f(instr.Addr)
				f(instr.Val)

			default:
				instr.Parent().WriteTo(os.Stderr)
				panic(fmt.Sprintf("unknown value: %s : %T", instr.String(), instr))
			}
		}
	}
}

func (ctx *Context) emitProgram(program *ssa.Program) {
	fmt.Fprintf(ctx.stream, `
#include <stdbool.h>
#include <stdio.h>
#include <stdint.h>
#include <string.h>
#include <assert.h>
#include <complex.h>

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
DEFINE_BUILTIN_OBJECT_TYPE(Complex64, float complex);
DEFINE_BUILTIN_OBJECT_TYPE(Complex128, double complex);
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
DEFINE_BUILTIN_OBJECT_TYPE(Uintptr, uintptr_t);

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
} InvalidObject;

typedef struct {
	union {
		MapObject map;
		StringObject string;
	} obj;
	uintptr_t count;
} IterObject;

typedef struct StackFrameCommon {
	FunctionObject resume_func;
	struct StackFrameCommon* prev_stack_pointer;
	void* free_vars;
	const void* deferred_list;
} StackFrameCommon;

typedef struct {
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
	TypeId type_id;
	SliceObject lhs;
	SliceObject rhs;
} StackFrameSliceAppend;
DECLARE_RUNTIME_API(slice_append, StackFrameSliceAppend);

typedef struct {
	StackFrameCommon common;
	SliceObject* result_ptr;
	SliceObject slice;
	StringObject string;
} StackFrameSliceAppendString;
DECLARE_RUNTIME_API(slice_append_string, StackFrameSliceAppendString);

typedef struct {
	StackFrameCommon common;
	IntObject* result_ptr;
	SliceObject slice;
} StackFrameSliceCapacity;
DECLARE_RUNTIME_API(slice_capacity, StackFrameSliceCapacity);

typedef struct {
	StackFrameCommon common;
	IntObject* result_ptr;
	TypeId type_id;
	SliceObject src;
	SliceObject dst;
} StackFrameSliceCopy;
DECLARE_RUNTIME_API(slice_copy, StackFrameSliceCopy);

typedef struct {
	StackFrameCommon common;
	IntObject* result_ptr;
	StringObject src;
	SliceObject dst;
} StackFrameSliceCopyString;
DECLARE_RUNTIME_API(slice_copy_string, StackFrameSliceCopy);

typedef struct {
	StackFrameCommon common;
	IntObject* result_ptr;
	SliceObject slice;
} StackFrameSliceSize;
DECLARE_RUNTIME_API(slice_size, StackFrameSliceSize);

typedef struct {
	StackFrameCommon common;
	SliceObject* result_ptr;
	TypeId type_id;
	StringObject src;
} StackFrameSliceFromString;
DECLARE_RUNTIME_API(slice_from_string, StackFrameSliceFromString);

typedef struct {
	StackFrameCommon common;
	StringObject* result_ptr;
	StringObject lhs;
	StringObject rhs;
} StackFrameStringAppend;
DECLARE_RUNTIME_API(string_append, StackFrameStringAppend);

typedef struct {
	StackFrameCommon common;
	StringObject string;
	IntObject* index;
	Int32Object* rune;
	bool* found;
	uintptr_t* count;
} StackFrameStringNext;
DECLARE_RUNTIME_API(string_next, StackFrameStringNext);

typedef struct {
	StackFrameCommon common;
	FunctionObject function_object;
	uintptr_t result_size;
	uintptr_t num_arg_buffer_words;
	void* arg_buffer[0];
} StackFrameDeferRegister;
DECLARE_RUNTIME_API(defer_register, StackFrameDeferRegister);

typedef struct {
	StackFrameCommon common;
	ChannelObject* result_ptr;
	TypeId type_id;
	IntObject capacity; // ToDo: correct to proper type
} StackFrameChannelNew;
DECLARE_RUNTIME_API(channel_new, StackFrameChannelNew);

typedef struct {
	StackFrameCommon common;
	IntObject* selected_index;
	BoolObject* receive_available;
	uintptr_t need_block;
	uintptr_t entry_count;
	struct {
		ChannelObject channel;
		TypeId type_id;
		const void* send_data;
		void* receive_data;
	} entry_buffer[0];
} StackFrameChannelSelect;
DECLARE_RUNTIME_API(channel_select, StackFrameChannelSelect);

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
	const void* receiver;
	TypeId type_id;
} StackFrameMakeInterface;
DECLARE_RUNTIME_API(make_interface, StackFrameMakeInterface);

typedef struct {
	StackFrameCommon common;
	MapObject* result_ptr;
	TypeId key_type;
	TypeId value_type;
} StackFrameMapNew;
DECLARE_RUNTIME_API(map_new, StackFrameMapNew);

typedef struct {
	StackFrameCommon common;
	StringObject* result_ptr;
	SliceObject byte_slice;
} StackFrameStringNewFromByteSlice;
DECLARE_RUNTIME_API(string_new_from_byte_slice, StackFrameStringNewFromByteSlice);

typedef struct {
	StackFrameCommon common;
	StringObject* result_ptr;
	IntObject rune;
} StackFrameStringNewFromRune;
DECLARE_RUNTIME_API(string_new_from_rune, StackFrameStringNewFromRune);

typedef struct {
	StackFrameCommon common;
	StringObject* result_ptr;
	SliceObject rune_slice;
} StackFrameStringNewFromRuneSlice;
DECLARE_RUNTIME_API(string_new_from_rune_slice, StackFrameStringNewFromRuneSlice);

typedef struct {
	StackFrameCommon common;
	MapObject map;
	const void* key;
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
	const void* key;
	void* value;
	bool* found;
	uintptr_t* count;
} StackFrameMapNext;
DECLARE_RUNTIME_API(map_next, StackFrameMapNext);

typedef struct {
	StackFrameCommon common;
	MapObject map;
	const void* key;
	const void* value;
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
	ChannelObject channel;
} StackFrameChannelClose;
DECLARE_RUNTIME_API(channel_close, StackFrameChannelClose);

typedef struct {
	StackFrameCommon common;
	ChannelObject channel;
	TypeId type_id;
	void* data;
	bool* available;
} StackFrameChannelReceive;
DECLARE_RUNTIME_API(channel_receive, StackFrameChannelReceive);

typedef struct {
	StackFrameCommon common;
} StackFrameDeferExecute;
DECLARE_RUNTIME_API(defer_execute, StackFrameDeferExecute);

typedef struct {
	StackFrameCommon common;
} StackFrameSchedule;
DECLARE_RUNTIME_API(schedule, StackFrameSchedule);

#define f_24_runtime_2E_Gosched gox5_schedule

typedef struct {
	StackFrameCommon common;
	ChannelObject channel;
	const void* data;
	TypeId type_id;
} StackFrameChannelSend;
DECLARE_RUNTIME_API(channel_send, StackFrameChannelSend);

typedef struct {
	StackFrameCommon common;
	Interface value;
} StackFramePanicRaise;
DECLARE_RUNTIME_API(panic_raise, StackFramePanicRaise);

typedef struct {
	StackFrameCommon common;
	Interface* result_ptr;
} StackFramePanicRecover;
DECLARE_RUNTIME_API(panic_recover, StackFramePanicRecover);

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
	IntObject* result_ptr;
	StringObject string;
} StackFrameStringLength;
DECLARE_RUNTIME_API(string_length, StackFrameStringLength);

typedef struct {
	StackFrameCommon common;
	StringObject* result_ptr;
	StringObject base;
	intptr_t low;
	intptr_t high;
} StackFrameStringSubstr;
DECLARE_RUNTIME_API(string_substr, StackFrameStringSubstr);

typedef struct {
	StringObject name;
	UserFunction function;
} Func;

FunctionObject gox5_search_method(Interface* interface, StringObject method_name);

__attribute__((unused)) static void builtin_print_float(double val) {
	char buf[20];
	int len = snprintf(buf, sizeof(buf) / sizeof(buf[0]), "%%+.6e", val);
	int len_e = 0;
	for(int i = len - 1; i > 0; --i) {
		char c = buf[i];
		if(c == '+' || c == '-') break;
		++len_e;
	}
	if(len_e < 3) {
		for(; len > 0; --len) {
			char c = buf[len];
			if(c == '+' || c == '-') break;
			buf[len + 1] = c;
		}
		assert(len > 0);
		buf[len + 1] = '0';
	}
	fprintf(stderr, "%%s", buf);
}
`)

	ctx.emitComplexNumberBuiltinFunctions()

	ctx.emitType()
	ctx.emitSignature()
	ctx.emitEqualFunctionDeclaration()
	ctx.emitHashFunctionDeclaration()

	ctx.visitAllFunctions(program, func(function *ssa.Function) {
		ctx.emitFunctionDeclaration(function)
	})

	ctx.emitEqualFunctionDefinition()
	ctx.emitHashFunctionDefinition()
	ctx.emitInterfaceTable()
	ctx.emitTypeInfo()
	ctx.emitConstant()

	for _, pkg := range program.AllPackages() {
		for member := range pkg.Members {
			gv, ok := pkg.Members[member].(*ssa.Global)
			if !ok {
				continue
			}
			ctx.emitGlobalVariable(gv)
		}
	}

	ctx.visitAllFunctions(program, func(function *ssa.Function) {
		if function.Blocks != nil {
			ctx.emitFunctionDefinition(function)
		}
	})

	ctx.emitRuntimeInfo()
}
