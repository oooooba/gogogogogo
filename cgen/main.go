package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

func main() {
	filename := flag.String("i", "/dev/stdin", "input file")
	buildDirname := flag.String("b", "/tmp", "build directory")
	cacheDirname := flag.String("cache", "cache", "cache directory")
	flag.Parse()

	cfg := packages.Config{Mode: packages.LoadAllSyntax}
	initPkgs, err := packages.Load(&cfg, *filename)
	if err != nil {
		log.Fatal(err)
	}
	prog, _ := ssautil.AllPackages(initPkgs, ssa.SanityCheckFunctions|ssa.InstantiateGenerics)
	prog.Build()

	if false {
		var keywords []string
		ctx := Context{
			stream:        nil,
			program:       prog,
			latestNameMap: make(map[*ssa.BasicBlock]string),
		}
		for _, pkg := range allPackagesSorted(prog) {
			ctx.traverseFunction(pkg, func(function *ssa.Function) {
				for _, keyword := range keywords {
					if strings.Contains(function.Name(), keyword) {
						function.WriteTo(os.Stderr)
					}
				}
			})
		}
	}

	emitProgram(prog, *buildDirname, *cacheDirname)
}

type Context struct {
	stream                   *os.File
	program                  *ssa.Program
	latestNameMap            map[*ssa.BasicBlock]string
	orderedPackageMembers    []ssa.Member
	builtinPrintWrapperBuf   strings.Builder
	builtinPrintWrapperNames map[*ssa.CallCommon]string
	extraFunctions           []*ssa.Function
	cachedFunctions          []*ssa.Function
	assertedInterfaceTypes   map[string]types.Type
	instantiatedNamedTypes   map[string]types.Type
	visitedInterfaceNames    map[string]bool
	emittedTypeDefinitions   map[string]struct{}
	instanceOrderedTypes     []types.Type
}

func (ctx *Context) markTypeDefinition(kind, name string) bool {
	if ctx.emittedTypeDefinitions == nil {
		ctx.emittedTypeDefinitions = make(map[string]struct{})
	}
	key := kind + ":" + name
	if _, ok := ctx.emittedTypeDefinitions[key]; ok {
		return false
	}
	ctx.emittedTypeDefinitions[key] = struct{}{}
	return true
}

func encode(str string) string {
	var buf strings.Builder
	for _, c := range str {
		if c >= 0x80 {
			panic(str)
		}
		if ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') {
			buf.WriteByte(byte(c))
		} else {
			fmt.Fprintf(&buf, "_%02X_", c)
		}
	}
	return buf.String()
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

func isNumericKind(kind types.BasicKind) bool {
	switch kind {
	case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
		types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr,
		types.Float32, types.Float64:
		return true
	default:
		return false
	}
}

func (ctx *Context) emitAtomicCall(instruction *ssa.Call, callCommon *ssa.CallCommon) bool {
	atomicFunctionOperations := map[string]string{
		"AddInt32":              "add",
		"AddInt64":              "add",
		"AddUint32":             "add",
		"AddUint64":             "add",
		"AddUintptr":            "add",
		"AndInt32":              "and",
		"AndInt64":              "and",
		"AndUint32":             "and",
		"AndUint64":             "and",
		"AndUintptr":            "and",
		"CompareAndSwapInt32":   "cas",
		"CompareAndSwapInt64":   "cas",
		"CompareAndSwapUint32":  "cas",
		"CompareAndSwapUint64":  "cas",
		"CompareAndSwapUintptr": "cas",
		"LoadInt32":             "load",
		"LoadInt64":             "load",
		"LoadUint32":            "load",
		"LoadUint64":            "load",
		"LoadUintptr":           "load",
		"OrInt32":               "or",
		"OrInt64":               "or",
		"OrUint32":              "or",
		"OrUint64":              "or",
		"OrUintptr":             "or",
		"StoreInt32":            "store",
		"StoreInt64":            "store",
		"StoreUint32":           "store",
		"StoreUint64":           "store",
		"StoreUintptr":          "store",
		"SwapInt32":             "swap",
		"SwapInt64":             "swap",
		"SwapUint32":            "swap",
		"SwapUint64":            "swap",
		"SwapUintptr":           "swap",
	}

	function, ok := callCommon.Value.(*ssa.Function)
	if !ok || function.Pkg == nil || function.Pkg.Pkg.Path() != "sync/atomic" {
		return false
	}
	switch function.Name() {
	case "LoadPointer", "StorePointer", "SwapPointer", "CompareAndSwapPointer":
		// These are linkname/asm without Go bodies; emit inline C11 atomics
		// over the unsafe.Pointer field.
		addr := fmt.Sprintf("(_Atomic void**)&(%s.raw->raw)", createValueRelName(callCommon.Args[0]))
		result := createValueRelName(instruction)
		switch function.Name() {
		case "LoadPointer":
			expr := fmt.Sprintf("atomic_load_explicit(%s, memory_order_seq_cst)", addr)
			fmt.Fprintf(ctx.stream, "%s = %s;\n", result, wrapInObject(expr, instruction.Type()))
		case "StorePointer":
			fmt.Fprintf(ctx.stream, "atomic_store_explicit(%s, %s.raw, memory_order_seq_cst);\n", addr, createValueRelName(callCommon.Args[1]))
		case "SwapPointer":
			expr := fmt.Sprintf("atomic_exchange_explicit(%s, %s.raw, memory_order_seq_cst)", addr, createValueRelName(callCommon.Args[1]))
			fmt.Fprintf(ctx.stream, "%s = %s;\n", result, wrapInObject(expr, instruction.Type()))
		case "CompareAndSwapPointer":
			fmt.Fprintf(ctx.stream, "void* expected = %s.raw;\n", createValueRelName(callCommon.Args[1]))
			fmt.Fprintf(ctx.stream, "bool swapped = atomic_compare_exchange_strong_explicit(%s, &expected, %s.raw, memory_order_seq_cst, memory_order_seq_cst);\n", addr, createValueRelName(callCommon.Args[2]))
			fmt.Fprintf(ctx.stream, "%s = %s;\n", result, wrapInObject("swapped", instruction.Type()))
		}
		fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instruction)))
		return true
	}
	op, ok := atomicFunctionOperations[function.Name()]
	if !ok {
		return false
	}
	pointerType, ok := callCommon.Args[0].Type().(*types.Pointer)
	if !ok {
		panic(instruction)
	}
	rawType := createRawTypeName(pointerType.Elem())
	addr := fmt.Sprintf("(_Atomic %s*)&(%s.raw->raw)", rawType, createValueRelName(callCommon.Args[0]))
	result := createValueRelName(instruction)
	switch op {
	case "load":
		expr := fmt.Sprintf("atomic_load_explicit(%s, memory_order_seq_cst)", addr)
		fmt.Fprintf(ctx.stream, "%s = %s;\n", result, wrapInObject(expr, instruction.Type()))
	case "store":
		fmt.Fprintf(ctx.stream, "atomic_store_explicit(%s, %s.raw, memory_order_seq_cst);\n", addr, createValueRelName(callCommon.Args[1]))
	case "add":
		operand := createValueRelName(callCommon.Args[1])
		expr := fmt.Sprintf("atomic_fetch_add_explicit(%s, %s.raw, memory_order_seq_cst) + %s.raw", addr, operand, operand)
		fmt.Fprintf(ctx.stream, "%s = %s;\n", result, wrapInObject(expr, instruction.Type()))
	case "swap":
		expr := fmt.Sprintf("atomic_exchange_explicit(%s, %s.raw, memory_order_seq_cst)", addr, createValueRelName(callCommon.Args[1]))
		fmt.Fprintf(ctx.stream, "%s = %s;\n", result, wrapInObject(expr, instruction.Type()))
	case "cas":
		fmt.Fprintf(ctx.stream, "%s expected = %s.raw;\n", rawType, createValueRelName(callCommon.Args[1]))
		fmt.Fprintf(ctx.stream, "bool swapped = atomic_compare_exchange_strong_explicit(%s, &expected, %s.raw, memory_order_seq_cst, memory_order_seq_cst);\n", addr, createValueRelName(callCommon.Args[2]))
		fmt.Fprintf(ctx.stream, "%s = %s;\n", result, wrapInObject("swapped", instruction.Type()))
	case "and":
		expr := fmt.Sprintf("atomic_fetch_and_explicit(%s, %s.raw, memory_order_seq_cst)", addr, createValueRelName(callCommon.Args[1]))
		fmt.Fprintf(ctx.stream, "%s = %s;\n", result, wrapInObject(expr, instruction.Type()))
	case "or":
		expr := fmt.Sprintf("atomic_fetch_or_explicit(%s, %s.raw, memory_order_seq_cst)", addr, createValueRelName(callCommon.Args[1]))
		fmt.Fprintf(ctx.stream, "%s = %s;\n", result, wrapInObject(expr, instruction.Type()))
	default:
		panic(function.Name())
	}
	fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instruction)))
	return true
}

func mathArchImplementation(function *ssa.Function) *ssa.Function {
	var mathArchToPure = map[string]string{
		"archFloor": "floor",
		"archCeil":  "ceil",
		"archTrunc": "trunc",
		"archMax":   "max",
		"archMin":   "min",
		"archExp":   "exp",
		"archExp2":  "exp2",
		"archLog":   "log",
		"archHypot": "hypot",
	}

	if function.Pkg == nil || function.Pkg.Pkg.Path() != "math" {
		return nil
	}
	pureName, ok := mathArchToPure[function.Name()]
	if !ok {
		return nil
	}
	return function.Pkg.Func(pureName)
}

func createValueName(value ssa.Value) string {
	if _, ok := value.(*ssa.Const); ok {
		constVal := value.(*ssa.Const)
		var full string
		switch {
		case constVal.Value != nil && constVal.Value.Kind() == constant.String:
			full = strconv.QuoteToASCII(constant.StringVal(constVal.Value))
		case constVal.Value != nil && constVal.Value.Kind() == constant.Float:
			if t, ok := constVal.Type().Underlying().(*types.Basic); ok {
				switch t.Kind() {
				case types.Float32:
					f := float32(constVal.Float64())
					full = fmt.Sprintf("%s(0x%x)", constVal.Type().String(), math.Float32bits(f))
				case types.Float64:
					f := constVal.Float64()
					full = fmt.Sprintf("%s(0x%x)", constVal.Type().String(), math.Float64bits(f))
				default:
					full = strconv.QuoteToASCII(value.String())
				}
			} else {
				full = strconv.QuoteToASCII(value.String())
			}
		default:
			full = strconv.QuoteToASCII(value.String())
		}
		return encode(fmt.Sprintf("c$%s", full))
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
		packageName := createPackageName(val.Package().Pkg)
		return encode(fmt.Sprintf("gv$%s$%s", value.Name(), packageName))
	} else {
		parentName := value.Parent().Name()
		return encode(fmt.Sprintf("v$%s$%s", value.Name(), parentName))
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

// ToDo: refactor to avoid using a global variable
var typeNameCache sync.Map

func createTypeName(typ types.Type) string {
	if cached, ok := typeNameCache.Load(typ); ok {
		return cached.(string)
	}

	var f func(typ types.Type) string
	f = func(typ types.Type) string {
		switch t := typ.(type) {
		case *types.Alias:
			return f(t.Underlying())
		case *types.Array:
			return fmt.Sprintf("Array<%s$%d>", f(t.Elem()), t.Len())
		case *types.Basic:
			switch t.Kind() {
			case types.Bool, types.UntypedBool:
				return "BoolObject"
			case types.Complex64:
				return "Complex64Object"
			case types.Complex128:
				return "Complex128Object"
			case types.Float32:
				return "Float32Object"
			case types.Float64:
				return "Float64Object"
			case types.Int:
				return "IntObject"
			case types.Int8:
				return "Int8Object"
			case types.Int16:
				return "Int16Object"
			case types.Int32:
				return "Int32Object"
			case types.Int64:
				return "Int64Object"
			case types.Invalid:
				return "InvalidObject"
			case types.String, types.UntypedString:
				return "StringObject"
			case types.UnsafePointer:
				return "UnsafePointerObject"
			case types.Uint:
				return "UintObject"
			case types.Uint8:
				return "Uint8Object"
			case types.Uint16:
				return "Uint16Object"
			case types.Uint32:
				return "Uint32Object"
			case types.Uint64:
				return "Uint64Object"
			case types.Uintptr:
				return "UintptrObject"
			}
		case *types.Chan:
			return fmt.Sprintf("Channel<%s>", f(t.Elem()))
		case *types.Interface:
			return "Interface"
		case *types.Map:
			k := f(t.Key())
			if n, ok := t.Key().(*types.Named); ok {
				if _, ok := n.Underlying().(*types.Interface); ok {
					k = "Interface"
				}
			}
			v := f(t.Elem())
			if n, ok := t.Elem().(*types.Named); ok {
				if _, ok := n.Underlying().(*types.Interface); ok {
					v = "Interface"
				}
			}
			return fmt.Sprintf("Map<%s$%s>", k, v)
		case *types.Named:
			return fmt.Sprintf("Named<%s$%s>", typ.String(), f(typ.Underlying()))
		case *types.Pointer:
			return fmt.Sprintf("Pointer<%s>", f(t.Elem()))
		case *types.Signature:
			return "FunctionObject"
		case *types.Slice:
			var en string
			if n, ok := t.Elem().(*types.Named); ok {
				if _, ok := n.Underlying().(*types.Interface); ok {
					en = "Interface"
				} else {
					en = f(t.Elem())
				}
			} else {
				en = f(t.Elem())
			}
			return fmt.Sprintf("Slice<%s>", en)
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
		case *types.TypeParam:
			return fmt.Sprintf("TypeParam<%s>", t.String())
		default:
			if typ.String() == "iter" {
				return "IterObject"
			}
		}
		panic(fmt.Sprintf("type not supported: %s (%T)", typ.String(), typ))
	}
	name := encode(f(typ))
	actual, _ := typeNameCache.LoadOrStore(typ, name)
	return actual.(string)
}

func isSignedIntegerType(typ types.Type) bool {
	b, ok := typ.Underlying().(*types.Basic)
	if !ok {
		return false
	}
	switch b.Kind() {
	case types.Int, types.Int8, types.Int16, types.Int32, types.Int64:
		return true
	}
	return false
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
	return fmt.Sprintf("runtime_info_type_%s", createInterfaceTypeSymbolName(typ))
}

func createInterfaceTypeSymbolName(typ types.Type) string {
	iface, ok := typ.(*types.Interface)
	if !ok || iface.NumMethods() == 0 {
		return createTypeName(typ)
	}
	methods := make([]string, 0, iface.NumMethods())
	for i := 0; i < iface.NumMethods(); i++ {
		methods = append(methods, iface.Method(i).Type().String())
	}
	sort.Strings(methods)
	return encode(fmt.Sprintf("Interface<%s>", strings.Join(methods, "$")))
}

func createFieldName(field *types.Var, index int) string {
	rawFieldName := field.Name()
	disallowedWords := []string{"_", "signed"} // ToDo: add C keywords
	for _, disallowedWord := range disallowedWords {
		if rawFieldName == disallowedWord {
			return fmt.Sprintf("%s_%d", rawFieldName, index)
		}
	}
	return rawFieldName
}

func (ctx *Context) switchFunction(nextFunction string, signature *types.Signature, signatureName string, result string, resumeFunction string, paramAndArgsHandler func()) {
	fmt.Fprintf(ctx.stream, "StackFrameCommon* next_frame = (StackFrameCommon*)(frame + 1);\n")
	fmt.Fprintf(ctx.stream, "assert(((uintptr_t)next_frame) %% sizeof(uintptr_t) == 0);\n")
	fmt.Fprintf(ctx.stream, "*next_frame = (StackFrameCommon){ 0 };\n")
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

func (ctx *Context) emitArgBufferCopies(args []ssa.Value) {
	fmt.Fprintf(ctx.stream, "intptr_t num_arg_buffer_words = 0;\n")
	for i, arg := range args {
		argValue := createValueRelName(arg)
		argType := createTypeName(arg.Type())
		argPtr := fmt.Sprintf("ptr%d", i)
		fmt.Fprintf(ctx.stream, "%s* %s = (void*)&next_frame->arg_buffer[num_arg_buffer_words]; // param[%d]\n", argType, argPtr, i)
		fmt.Fprintf(ctx.stream, "*%s = %s;\n", argPtr, argValue)
		fmt.Fprintf(ctx.stream, "num_arg_buffer_words += (sizeof(%s) + sizeof(next_frame->arg_buffer[0]) - 1) / sizeof(next_frame->arg_buffer[0]);\n", argType)
	}
	fmt.Fprintf(ctx.stream, "next_frame->num_arg_buffer_words = num_arg_buffer_words;\n")
}

func (ctx *Context) emitGoOrDefer(instr ssa.Instruction, callCommon *ssa.CallCommon, registerApi string, registerFrame string, invokeApi string, invokeFrame string, builtinPrintSupported bool) {
	resumeFunction := createInstructionName(instr)
	if callCommon.Method != nil {
		ctx.emitCallCommonForMethod(callCommon, invokeApi, invokeFrame, resumeFunction)
		return
	}
	if builtin, ok := callCommon.Value.(*ssa.Builtin); ok && builtinPrintSupported && (builtin.Name() == "print" || builtin.Name() == "println") {
		wrapperName := ctx.emitBuiltinPrintWrapper(builtin.Name(), callCommon, instr)
		signature := callCommon.Value.Type().Underlying().(*types.Signature)
		resultSize := "0"
		switch signature.Results().Len() {
		case 0:
			// do nothing
		case 1:
			resultSize = fmt.Sprintf("sizeof(%s)", createTypeName(signature.Results().At(0).Type()))
		default:
			resultSize = fmt.Sprintf("sizeof(%s)", createTypeName(signature.Results()))
		}
		ctx.switchFunctionToCallRuntimeApi(registerApi, registerFrame, resumeFunction, nil,
			func() {
				ctx.emitArgBufferCopies(callCommon.Args)
			},
			paramArgPair{param: "function_object", arg: wrapInFunctionObject(wrapperName)},
			paramArgPair{param: "result_size", arg: resultSize},
		)
		return
	}
	if builtin, ok := callCommon.Value.(*ssa.Builtin); ok && builtin.Name() == "close" {
		// The channel argument is copied to the frame's arg_buffer, which is at the
		// same offset as StackFrameChannelClose.channel, so gox5_channel_close can be
		// invoked directly as the entry function.
		ctx.switchFunctionToCallRuntimeApi(registerApi, registerFrame, resumeFunction, nil,
			func() {
				ctx.emitArgBufferCopies(callCommon.Args)
			},
			paramArgPair{param: "function_object", arg: wrapInFunctionObject("gox5_channel_close")},
			paramArgPair{param: "result_size", arg: "0"},
		)
		return
	}
	ctx.emitCallCommon(callCommon, registerApi, registerFrame, resumeFunction)
}

func (ctx *Context) emitCallCommon(callCommon *ssa.CallCommon, nextFunction string, nextFunctionFrame string, resumeFunction string) {
	if callCommon.Method != nil {
		panic("method not supported")
	}

	if function, ok := callCommon.Value.(*ssa.Function); ok {
		callPath := ""
		if function.Pkg != nil {
			callPath = function.Pkg.Pkg.Path()
		}
		if strings.HasPrefix(callPath, "internal/race") || strings.HasPrefix(callPath, "internal/synctest") {
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(resumeFunction))
			return
		}
	}

	var functionObject string
	switch callee := callCommon.Value.(type) {
	case *ssa.Function:
		functionObject = createValueName(callee)
	case ssa.Value:
		functionObject = createValueRelName(callee)
	default:
		panic(fmt.Sprintf("unknown callee: %s, %s, %T, %T", callCommon, callee, callCommon, callee))
	}

	signature := callCommon.Value.Type().Underlying().(*types.Signature)

	resultSize := "0"
	switch signature.Results().Len() {
	case 0:
		// do nothing
	case 1:
		resultSize = fmt.Sprintf("sizeof(%s)", createTypeName(signature.Results().At(0).Type()))
	default:
		resultSize = fmt.Sprintf("sizeof(%s)", createTypeName(signature.Results()))
	}

	ctx.switchFunctionToCallRuntimeApi(nextFunction, nextFunctionFrame, resumeFunction, nil,
		func() {
			ctx.emitArgBufferCopies(callCommon.Args)
		},
		paramArgPair{param: "function_object", arg: functionObject},
		paramArgPair{param: "result_size", arg: resultSize},
	)
}

func (ctx *Context) emitCallCommonForMethod(callCommon *ssa.CallCommon, nextFunction string, nextFunctionFrame string, resumeFunction string) {
	if callCommon.Method == nil {
		panic("only method supported")
	}

	signature := callCommon.Method.Type().Underlying().(*types.Signature)

	resultSize := "0"
	switch signature.Results().Len() {
	case 0:
		// do nothing
	case 1:
		resultSize = fmt.Sprintf("sizeof(%s)", createTypeName(signature.Results().At(0).Type()))
	default:
		resultSize = fmt.Sprintf("sizeof(%s)", createTypeName(signature.Results()))
	}

	ctx.switchFunctionToCallRuntimeApi(nextFunction, nextFunctionFrame, resumeFunction, nil,
		func() {
			receiver := fmt.Sprintf("%s.receiver", createValueRelName(callCommon.Value))
			receiverSize := fmt.Sprintf("%s.type_id.info->size", createValueRelName(callCommon.Value))
			fmt.Fprintf(ctx.stream, "memcpy(&next_frame->arg_buffer[0], %s, %s); // receiver: %s\n", receiver, receiverSize, signature.Recv())
			fmt.Fprintf(ctx.stream, "intptr_t num_arg_buffer_words = (%s + sizeof(next_frame->arg_buffer[0]) - 1) / sizeof(next_frame->arg_buffer[0]);\n", receiverSize)
			for i, arg := range callCommon.Args {
				argValue := createValueRelName(arg)
				argType := createTypeName(arg.Type())
				argPtr := fmt.Sprintf("ptr%d", i)
				fmt.Fprintf(ctx.stream, "%s* %s = (void*)&next_frame->arg_buffer[num_arg_buffer_words]; // param[%d]\n", argType, argPtr, i)
				fmt.Fprintf(ctx.stream, "*%s = %s;\n", argPtr, argValue)
				fmt.Fprintf(ctx.stream, "num_arg_buffer_words += (sizeof(%s) + sizeof(next_frame->arg_buffer[0]) - 1) / sizeof(next_frame->arg_buffer[0]);\n", argType)
			}
			fmt.Fprintf(ctx.stream, "next_frame->num_arg_buffer_words = num_arg_buffer_words;\n")
		},
		paramArgPair{param: "interface", arg: fmt.Sprintf("&%s", createValueRelName(callCommon.Value))},
		paramArgPair{param: "method_name", arg: fmt.Sprintf("(StringObject){.raw = \"%s\", .len = sizeof(\"%s\") - 1}", callCommon.Method.Name(), callCommon.Method.Name())},
		paramArgPair{param: "result_size", arg: resultSize},
	)
}

func isFunctionBodySkippedPackage(pkg *ssa.Package) bool {
	return isFunctionBodySkippedPackagePath(pkg.Pkg.Path())
}

func isFunctionBodySkippedPackagePath(path string) bool {
	if path == "runtime" || path == "internal/race" || path == "internal/abi" {
		return true
	}
	if path == "internal/cpu" || path == "internal/strconv" || path == "internal/stringslite" || path == "internal/bytealg" || path == "internal/sync" || path == "internal/oserror" {
		return false
	}
	if path == "internal/poll" || path == "internal/intern" || path == "internal/unsafeheader" {
		return false
	}
	if path == "internal/syscall/unix" {
		return false
	}
	if strings.HasPrefix(path, "internal/") || strings.HasPrefix(path, "runtime/internal/") {
		return true
	}
	return false
}

func isFunctionBodySkipped(fn *ssa.Function) bool {
	if origin := fn.Origin(); origin != nil {
		fn = origin
	}
	if fn.Pkg != nil && isFunctionBodySkippedPackagePath(fn.Pkg.Pkg.Path()) {
		return true
	}
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			var common *ssa.CallCommon
			switch instr := instr.(type) {
			case *ssa.Call:
				common = instr.Common()
			case *ssa.Go:
				common = instr.Common()
			case *ssa.Defer:
				common = instr.Common()
			}
			if common == nil {
				continue
			}
			if f, ok := common.Value.(*ssa.Function); ok {
				if origin := f.Origin(); origin != nil {
					f = origin
				}
				if f.Pkg == nil || !isFunctionBodySkippedPackagePath(f.Pkg.Pkg.Path()) {
					continue
				}
				if f.Name() == "init" {
					continue
				}
				switch f.Pkg.Pkg.Path() {
				case "runtime":
					// These are handled by emitSpecialRuntimeCall, so don't skip
					// their callers (Goexit/Gosched for sync.Pool via pinSlow,
					// GOMAXPROCS for sync.Pool, FuncForPC/Name for the function
					// identity tests in xtests/reflect.go).
					if f.Name() == "Goexit" || f.Name() == "Gosched" || f.Name() == "GOMAXPROCS" || f.Name() == "FuncForPC" || f.Name() == "Name" || f.Name() == "SetFinalizer" || f.Name() == "fcntl" || f.Name() == "beforeExit" || f.Name() == "KeepAlive" {
						continue
					}
				case "syscall":
					// Exit is handled by emitSpecialRuntimeCall (terminates the
					// process directly), so don't skip its callers (os.Exit).
					// write/read/openat are handled by emitSpecialRuntimeCall
					// (map to the C write(2)/read(2)/openat(2) calls), so don't
					// skip their callers (os.File.Write/os.File.Read/os.Open).
					// Open and close forward to openat/close via generated
					// bodies, so they must not be skipped either.
					if f.Name() == "Exit" || f.Name() == "write" || f.Name() == "read" || f.Name() == "openat" || f.Name() == "Open" || f.Name() == "close" || f.Name() == "Close" || f.Name() == "fcntl" || f.Name() == "runtime_entersyscall" || f.Name() == "runtime_exitsyscall" {
						continue
					}
				case "internal/testlog":
					// PanicOnExit0 is intercepted by emitSpecialRuntimeCall (returns
					// false for a non-test binary); don't skip its callers (os.Exit).
					// Open records file opens for the test log; in a non-test
					// binary it is a no-op, so don't skip os.Open's callers.
					if f.Name() == "PanicOnExit0" || f.Name() == "Open" {
						continue
					}
				case "reflect":
					// ValueOf/Pointer/rtypeOf/TypeOf are handled by
					// emitSpecialRuntimeCall (function identity tests and the
					// reflect.Type fabrication used by fmt), so don't skip
					// callers of them.
					switch f.Name() {
					case "ValueOf", "Pointer", "rtypeOf", "TypeOf":
						continue
					}
				}
				if strings.HasPrefix(f.Pkg.Pkg.Path(), "internal/race") ||
					strings.HasPrefix(f.Pkg.Pkg.Path(), "internal/synctest") ||
					strings.HasPrefix(f.Pkg.Pkg.Path(), "internal/msan") ||
					strings.HasPrefix(f.Pkg.Pkg.Path(), "internal/asan") {
					continue
				}
				if f.Pkg.Pkg.Path() == "internal/godebug" {
					// godebug.New/Value/IncNonDefault are intercepted by
					// emitSpecialRuntimeCall (see below), so don't skip their
					// callers (time.init, time.syncTimer).
					switch f.Name() {
					case "New", "Value", "IncNonDefault", "init":
						continue
					}
				}
				if f.Pkg.Pkg.Path() == "internal/reflectlite" && f.Name() == "TypeOf" {
					// reflectlite.TypeOf is intercepted by emitSpecialRuntimeCall
					// (returns a zeroed Type interface); its interface method
					// calls are intercepted during *ssa.Call emission. Keep
					// callers such as context.WithValue emitted.
					continue
				}
				if f.Pkg.Pkg.Path() == "internal/abi" && f.Name() == "NoEscape" {
					continue
				}
				return true
			}
		}
	}
	return false
}

func (ctx *Context) emitSpecialRuntimeCall(callee *ssa.Function, instr *ssa.Call, callCommon *ssa.CallCommon) bool {
	pkgPath := ""
	if callee.Pkg != nil {
		pkgPath = callee.Pkg.Pkg.Path()
	}
	funcName := callee.Name()

	if pkgPath == "runtime" {
		switch funcName {
		case "SetFinalizer":
			// The runtime has no finalizer support; this is a no-op so that
			// callers such as os.newFile are emitted instead of being skipped.
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "beforeExit":
			// runtime_beforeExit is invoked by os.Exit; it has no effect on the
			// C runtime (no race detector / coverage). Treat as a no-op.
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "KeepAlive":
			// runtime.KeepAlive is a compiler hint with no runtime effect.
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		}
	}
	if pkgPath == "internal/syscall/unix" {
		switch funcName {
		case "fcntl":
			// internal/syscall/unix.fcntl is the Go wrapper around the runtime
			// fcntl syscall. fcntl is not supported, so return a zeroed
			// (val, errno) tuple; callers treat val != -1 as success.
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "%s.raw.e0 = (Int32Object){.raw = 0};\n", result)
			fmt.Fprintf(ctx.stream, "%s.raw.e1 = (Int32Object){.raw = 0};\n", result)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		}
	}
	if pkgPath == "os" {
		switch funcName {
		case "runtime_args":
			// os.runtime_args forwards to runtime.args, which is not supported.
			// Return an empty []string; the os.Stdin/Stdout/Stderr tests do not
			// depend on os.Args.
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "%s.raw = (SliceObject){0};\n", result)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "runtime_beforeExit":
			// os.runtime_beforeExit forwards to runtime.beforeExit (race/coverage
			// hooks). There is no effect on the C runtime; treat as a no-op.
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		}
	}
	if pkgPath == "syscall" {
		switch funcName {
		case "Exit":
			// syscall.Exit forwards to the (unsupported) runtime exit. Exit the
			// whole process directly; os.Exit is expected to terminate.
			arg := createValueRelName(callCommon.Args[0])
			fmt.Fprintf(ctx.stream, "exit(%s.raw);\n", arg)
			return true
		case "write":
			// syscall.write(fd int, p []byte) (n int, err error) is the low-level
			// wrapper around the write(2) syscall. Map it directly to the C
			// write(2) call so that os.File.Write on real file descriptors works.
			fd := createValueRelName(callCommon.Args[0])
			p := createValueRelName(callCommon.Args[1])
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "ssize_t written = write(%s.raw, %s.raw.addr, %s.raw.size);\n", fd, p, p)
			fmt.Fprintf(ctx.stream, "%s.raw.e0 = (IntObject){.raw = written < 0 ? -1 : (long)written};\n", result)
			fmt.Fprintf(ctx.stream, "%s.raw.e1 = (Interface){0};\n", result)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "read":
			// syscall.read(fd int, p []byte) (n int, err error) is the low-level
			// wrapper around the read(2) syscall. Map it directly to the C
			// read(2) call so that os.File.Read on real file descriptors works.
			// A zero-length buffer reads nothing (and avoids feeding a NULL
			// pointer with length 0, which some platforms treat as an error).
			fd := createValueRelName(callCommon.Args[0])
			p := createValueRelName(callCommon.Args[1])
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "ssize_t got = %s.raw.size == 0 ? 0 : read(%s.raw, %s.raw.addr, %s.raw.size);\n", p, fd, p, p)
			fmt.Fprintf(ctx.stream, "%s.raw.e0 = (IntObject){.raw = got < 0 ? -1 : (long)got};\n", result)
			fmt.Fprintf(ctx.stream, "%s.raw.e1 = (Interface){0};\n", result)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "openat":
			// syscall.openat(dirfd int, path string, flags int, mode uint32)
			// (fd int, err error) is the low-level wrapper around the openat(2)
			// syscall. Map it directly to the C openat(2) call so that os.Open on
			// real file descriptors works. Strings used as paths come from
			// literals, whose .raw is NUL-terminated.
			fd := createValueRelName(callCommon.Args[0])
			path := createValueRelName(callCommon.Args[1])
			flags := createValueRelName(callCommon.Args[2])
			mode := createValueRelName(callCommon.Args[3])
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "int opened_fd = openat((int)%s.raw, %s.raw, (int)%s.raw, (unsigned int)%s.raw);\n", fd, path, flags, mode)
			fmt.Fprintf(ctx.stream, "%s.raw.e0 = (IntObject){.raw = opened_fd};\n", result)
			fmt.Fprintf(ctx.stream, "%s.raw.e1 = (Interface){0};\n", result)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "close", "Close":
			// syscall.close(fd int) (err error) / syscall.Close(fd int) error map to
			// the C close(2) call; the error is not propagated (nil) for simplicity.
			fd := createValueRelName(callCommon.Args[0])
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "close((int)%s.raw);\n", fd)
			fmt.Fprintf(ctx.stream, "%s = (Interface){0};\n", result)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "pipe2":
			// syscall.pipe2(p *[2]_C_int, flags int) (err error) maps to the C
			// pipe2(2) call, which writes the read and write ends of a new pipe
			// into p[0] and p[1]. The error is not propagated (nil) for
			// simplicity.
			p0 := createValueRelName(callCommon.Args[0])
			flags := createValueRelName(callCommon.Args[1])
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "int pipe_rc = pipe2((int*)%s.raw, (int)%s.raw); (void)pipe_rc;\n", p0, flags)
			fmt.Fprintf(ctx.stream, "%s = (Interface){0};\n", result)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "runtime_entersyscall", "runtime_exitsyscall":
			// syscall.runtime_entersyscall/runtime_exitsyscall are linknames to
			// runtime.entersyscall/exitsyscall, the M/P state bookkeeping around
			// a system call. The C runtime is effectively single-threaded with no
			// such state, so treat both as no-ops so raw-syscall wrappers can run.
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "Syscall":
			// syscall.Syscall(trap, a1, a2, a3) (r1, r2 uintptr, err syscall.Errno)
			// is the 4-argument raw syscall wrapper. os.Open/Read/Close passes
			// through here only for syscalls not already intercepted by name
			// (openat/read/fcntl above). Handle the syscalls the standard
			// library needs here; abort on anything else so we notice it.
			if cst, ok := callCommon.Args[0].(*ssa.Const); ok {
				trap, _ := constant.Int64Val(cst.Value)
				switch trap {
				case 3: // SYS_CLOSE
					// close(fd) (r1=0, r2=0, err=nil)
					a1 := createValueRelName(callCommon.Args[1])
					result := createValueRelName(instr)
					fmt.Fprintf(ctx.stream, "close((int)%s.raw);\n", a1)
					fmt.Fprintf(ctx.stream, "%s.raw.e0 = (UintptrObject){.raw=0};\n", result)
					fmt.Fprintf(ctx.stream, "%s.raw.e1 = (UintptrObject){.raw=0};\n", result)
					fmt.Fprintf(ctx.stream, "%s.raw.e2 = (UintptrObject){.raw=0};\n", result)
					fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
					return true
				case 263: // SYS_UNLINKAT (used by os.Remove)
					// unlinkat(dirfd, path, flags) (r1=0, r2=0, err=nil)
					a1 := createValueRelName(callCommon.Args[1])
					a2 := createValueRelName(callCommon.Args[2])
					a3 := createValueRelName(callCommon.Args[3])
					result := createValueRelName(instr)
					fmt.Fprintf(ctx.stream, "unlinkat((int)%s.raw, (const char*)%s.raw, (int)%s.raw);\n", a1, a2, a3)
					fmt.Fprintf(ctx.stream, "%s.raw.e0 = (UintptrObject){.raw=0};\n", result)
					fmt.Fprintf(ctx.stream, "%s.raw.e1 = (UintptrObject){.raw=0};\n", result)
					fmt.Fprintf(ctx.stream, "%s.raw.e2 = (UintptrObject){.raw=0};\n", result)
					fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
					return true
				}
			}
			// Unknown/unsupported raw syscall: abort so it is detected.
			fmt.Fprintf(ctx.stream, "assert(false && \"unsupported syscall.Syscall trap\");\n")
			return true
		case "fcntl":
			// syscall.fcntl(fd int, cmd int, arg int) (val int, err error) is the
			// low-level wrapper around fcntl(2). It is used by the poll runtime to
			// query/clear the non-blocking flag when a file is opened. Map it to
			// the C fcntl(2) call; the error is not propagated (nil) for simplicity.
			fd := createValueRelName(callCommon.Args[0])
			cmd := createValueRelName(callCommon.Args[1])
			arg := createValueRelName(callCommon.Args[2])
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "int fcntl_val = fcntl((int)%s.raw, %s.raw, (long)%s.raw);\n", fd, cmd, arg)
			fmt.Fprintf(ctx.stream, "%s.raw.e0 = (IntObject){.raw = fcntl_val};\n", result)
			fmt.Fprintf(ctx.stream, "%s.raw.e1 = (Interface){0};\n", result)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		}
	}

	if pkgPath == "internal/poll" {
		// (*poll.FD).Init sets up the netpoller binding for freshly-opened file
		// descriptors. The C runtime has no netpoller. Rather than emulate its
		// runtime_poll* entry points, skip the binding entirely: leave
		// fd.pd.runtimeCtx at 0 so pollDesc.prepare()/pollable() treat the fd as
		// a plain blocking descriptor. A read on a regular file then proceeds
		// directly through the intercepted syscall.read.
		switch funcName {
		case "Init":
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "%s = (Interface){0};\n", result)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "runtime_Semacquire", "runtime_Semrelease":
			// These are linknames to runtime.semacquire/semrelease, used by the
			// fdMutex to coordinate fd operations (e.g. during FD.Close). The C
			// runtime is single-threaded and there are no concurrent fd ops, so
			// both are no-ops.
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		}
	}

	if pkgPath == "internal/testlog" {
		switch funcName {
		case "PanicOnExit0":
			// For a non-test binary there is no registered test log, so this
			// returns false (never panic on os.Exit(0)).
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "%s.raw = false;\n", result)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "Open":
			// testlog.Open records the file being opened for the test log.
			// In a non-test binary there is no log; it has no return value, so
			// treat it as a no-op to keep os.Open generated.
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		}
	}
	if pkgPath == "iter" {
		switch funcName {
		case "newcoro":
			result := createValueRelName(instr)
			ctx.switchFunctionToCallRuntimeApi("gox5_coro_new", "StackFrameCoroNew", createInstructionName(instr), &result, nil,
				paramArgPair{param: "function_object", arg: createValueRelName(callCommon.Args[0])},
			)
			return true
		case "coroswitch":
			ctx.switchFunctionToCallRuntimeApi("gox5_coro_switch", "StackFrameCoroSwitch", createInstructionName(instr), nil, nil,
				paramArgPair{param: "coro", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
			)
			return true
		}
	}

	if pkgPath == "maps" {
		switch funcName {
		case "clone":
			result := createValueRelName(instr)
			ctx.switchFunctionToCallRuntimeApi("gox5_map_clone", "StackFrameMapClone", createInstructionName(instr), &result, nil,
				paramArgPair{param: "map", arg: createValueRelName(callCommon.Args[0])},
			)
			return true
		}
	}

	if pkgPath == "internal/abi" {
		switch funcName {
		case "NoEscape":
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "%s = %s;\n", result, createValueRelName(callCommon.Args[0]))
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		}
	}

	if pkgPath == "internal/bytealg" {
		switch funcName {
		case "Compare":
			result := createValueRelName(instr)
			ctx.switchFunctionToCallRuntimeApi("gox5_slice_compare", "StackFrameSliceCompare", createInstructionName(instr), &result, nil,
				paramArgPair{param: "lhs", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
				paramArgPair{param: "rhs", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[1]))},
			)
			return true
		case "CompareString":
			// bytealg.CompareString has a Go body whose abigen linkname
			// (abigen_runtime_cmpstring) is a stub, so compare the strings'
			// bytes via gox5_slice_compare.
			result := createValueRelName(instr)
			a := createValueRelName(callCommon.Args[0])
			b := createValueRelName(callCommon.Args[1])
			ctx.switchFunctionToCallRuntimeApi("gox5_slice_compare", "StackFrameSliceCompare", createInstructionName(instr), &result, nil,
				paramArgPair{param: "lhs", arg: fmt.Sprintf("(SliceObject){.addr = (void*)%s.raw, .size = %s.len, .capacity = %s.len}", a, a, a)},
				paramArgPair{param: "rhs", arg: fmt.Sprintf("(SliceObject){.addr = (void*)%s.raw, .size = %s.len, .capacity = %s.len}", b, b, b)},
			)
			return true
		case "Count":
			result := createValueRelName(instr)
			ctx.switchFunctionToCallRuntimeApi("gox5_slice_count", "StackFrameSliceCount", createInstructionName(instr), &result, nil,
				paramArgPair{param: "b", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
				paramArgPair{param: "c", arg: createValueRelName(callCommon.Args[1])},
			)
			return true
		case "CountString":
			// bytealg.CountString has no Go body on amd64 (assembly-backed),
			// so route it to gox5_slice_count over the string's bytes.
			result := createValueRelName(instr)
			s := createValueRelName(callCommon.Args[0])
			ctx.switchFunctionToCallRuntimeApi("gox5_slice_count", "StackFrameSliceCount", createInstructionName(instr), &result, nil,
				paramArgPair{param: "b", arg: fmt.Sprintf("(SliceObject){.addr = (void*)%s.raw, .size = %s.len, .capacity = %s.len}", s, s, s)},
				paramArgPair{param: "c", arg: createValueRelName(callCommon.Args[1])},
			)
			return true
		case "Index":
			result := createValueRelName(instr)
			ctx.switchFunctionToCallRuntimeApi("gox5_slice_search_slice", "StackFrameSliceSearchSlice", createInstructionName(instr), &result, nil,
				paramArgPair{param: "lhs", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
				paramArgPair{param: "rhs", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[1]))},
			)
			return true
		case "IndexByte":
			result := createValueRelName(instr)
			ctx.switchFunctionToCallRuntimeApi("gox5_slice_search_byte", "StackFrameSliceSearchByte", createInstructionName(instr), &result, nil,
				paramArgPair{param: "b", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
				paramArgPair{param: "c", arg: createValueRelName(callCommon.Args[1])},
			)
			return true
		case "IndexByteString":
			result := createValueRelName(instr)
			ctx.switchFunctionToCallRuntimeApi("gox5_string_search_byte", "StackFrameStringSearchByte", createInstructionName(instr), &result, nil,
				paramArgPair{param: "string", arg: createValueRelName(callCommon.Args[0])},
				paramArgPair{param: "byte", arg: createValueRelName(callCommon.Args[1])},
			)
			return true
		case "IndexString":
			result := createValueRelName(instr)
			ctx.switchFunctionToCallRuntimeApi("gox5_string_search_string", "StackFrameStringSearchString", createInstructionName(instr), &result, nil,
				paramArgPair{param: "lhs", arg: createValueRelName(callCommon.Args[0])},
				paramArgPair{param: "rhs", arg: createValueRelName(callCommon.Args[1])},
			)
			return true
		case "MakeNoZero":
			result := fmt.Sprintf("%s.raw", createValueRelName(instr))
			ctx.switchFunctionToCallRuntimeApi("gox5_slice_new_uninitialized", "StackFrameSliceNewUninitialized", createInstructionName(instr), &result, nil,
				paramArgPair{param: "n", arg: createValueRelName(callCommon.Args[0])},
			)
			return true
		}
	}

	if pkgPath == "errors" && funcName == "init" {
		fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
		return true
	}
	if pkgPath == "sync" && funcName == "init" {
		fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
		return true
	}
	if pkgPath == "internal/cpu" && funcName == "init" {
		fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
		return true
	}
	if pkgPath == "syscall" && funcName == "init" {
		// syscall.init would run runtime_envs() and rlimit setup that call into
		// the skipped runtime package (runtime_envs, Getrlimit via Syscall) and
		// abort. No code in the supported tests uses the syscall environment or
		// rlimit state, so skip the whole initializer.
		fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
		return true
	}

	if pkgPath == "internal/godebug" {
		switch funcName {
		case "New":
			// godebug.New is called by time.init (asynctimerchan) and other
			// package initializers to register a GODEBUG setting. The runtime
			// has no GODEBUG support, so return a fresh zeroed *Setting; the
			// (*Setting).Value accessor is intercepted below to return "".
			result := createValueRelName(instr)
			ctx.switchFunctionToCallRuntimeApi("gox5_new", "StackFrameNew", createInstructionName(instr), &result, nil,
				paramArgPair{param: "size", arg: fmt.Sprintf("sizeof(%s)", createTypeName(instr.Type().Underlying().(*types.Pointer).Elem()))},
			)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "Value":
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", result, result)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "IncNonDefault":
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "init":
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		default:
			panic(fmt.Sprintf("unexpected call to internal/godebug.%s (called from %s)", funcName, createFunctionName(instr.Parent())))
		}
	}

	if pkgPath == "internal/reflectlite" && funcName == "TypeOf" {
		// context.WithValue checks reflectlite.TypeOf(key).Comparable(); there is
		// no reflectlite support in the runtime, so return a zeroed Type
		// interface. Interface method invocations on reflectlite.Type are
		// intercepted in the *ssa.Call emission below.
		result := createValueRelName(instr)
		fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", result, result)
		fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
		return true
	}

	if pkgPath == "reflect" || pkgPath == "runtime" {
		switch {
		case pkgPath == "reflect" && funcName == "TypeOf":
			// fmt's pp.doPrint calls reflect.TypeOf(arg).Kind() on every
			// argument to decide whether to insert a space, and %T calls
			// reflect.TypeOf(arg).String(). reflect.Type is not supported in the
			// runtime, so fabricate a reflect.Type interface whose receiver field
			// carries the argument's type_id. The Kind()/String() interface
			// methods are intercepted during *ssa.Call emission below.
			result := createValueRelName(instr)
			arg := createValueRelName(callCommon.Args[0])
			fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", result, result)
			fmt.Fprintf(ctx.stream, "%s.receiver = (void*)(uintptr_t)%s.type_id.id;\n", result, arg)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case pkgPath == "reflect" && funcName == "ValueOf":
			// reflect.ValueOf inspects the interface's type word and data pointer
			// through the unsafe abi.EmptyInterface layout. The runtime stores a
			// FunctionObject for function-valued interfaces, so build a
			// reflect.Value whose ptr field carries the function's raw address.
			result := createValueRelName(instr)
			arg := createValueRelName(callCommon.Args[0])
			fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", result, result)
			fmt.Fprintf(ctx.stream, "if (%s.receiver != NULL) {\n", arg)
			fmt.Fprintf(ctx.stream, "\t%s.ptr.raw = (void*)((FunctionObject*)%s.receiver)->raw;\n", result, arg)
			fmt.Fprintf(ctx.stream, "}\n")
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case pkgPath == "reflect" && funcName == "Pointer":
			// (reflect.Value).Pointer returns the data pointer stored in the Value.
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "%s.raw = (uintptr_t)%s.ptr.raw;\n", result, createValueRelName(callCommon.Args[0]))
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case pkgPath == "reflect" && funcName == "rtypeOf":
			// reflect.rtypeOf is called from reflect.init to initialize package
			// variables (stringType, bytesType); its body calls internal/abi.TypeOf
			// which is a skipped stub. Return a zeroed *abi.Type.
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", result, result)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case pkgPath == "runtime" && funcName == "FuncForPC":
			// runtime.FuncForPC looks the code address up in the function name
			// registry emitted into shared_definition.c and returns a
			// *runtime.Func pointing into it (or NULL when unknown).
			result := createValueRelName(instr)
			pc := fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))
			fmt.Fprintf(ctx.stream, "%s.raw = (void*)gox5_runtime_func_for_pc(%s);\n", result, pc)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case pkgPath == "runtime" && funcName == "Name":
			// (runtime.Func).Name returns the registered name for the function
			// address that the *runtime.Func points at.
			result := createValueRelName(instr)
			recv := fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))
			fmt.Fprintf(ctx.stream, "%s = gox5_runtime_func_name((const UserFunctionInfo*)%s);\n", result, recv)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		}
	}

	// sync and internal/sync declare their blocking/waiting primitives as
	// linkname stubs (runtime_Semacquire*, runtime_Semrelease, runtime_canSpin,
	// runtime_doSpin, runtime_nanotime, runtime_rand, throw, fatal). Route calls
	// to them into the Rust runtime instead of the assert(false) stubs.
	{
		origin := callee
		if o := callee.Origin(); o != nil {
			origin = o
		}
		originPkgPath := ""
		if origin.Pkg != nil {
			originPkgPath = origin.Pkg.Pkg.Path()
		}
		originFuncName := origin.Name()

		if originPkgPath == "internal/synctest" {
			switch originFuncName {
			case "IsInBubble", "IsAssociated", "Associate":
				result := createValueRelName(instr)
				fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", result, result)
			case "Disassociate", "init":
				// no-op
			default:
				panic(fmt.Sprintf("unexpected call to internal/synctest.%s (called from %s)", originFuncName, createFunctionName(instr.Parent())))
			}
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		}

		if originPkgPath == "sync/atomic" && (originFuncName == "runtime_procPin" || originFuncName == "runtime_procUnpin") {
			// sync/atomic.Value (value.go) uses these linkname stubs to disable
			// preemption around the first Store; single-threaded runtime: no-op.
			if callCommon.Signature().Results().Len() > 0 {
				result := createValueRelName(instr)
				fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", result, result)
			}
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		}

		if originPkgPath == "sync" || originPkgPath == "internal/sync" {
			switch originFuncName {
			case "runtime_Semacquire", "runtime_SemacquireWaitGroup", "runtime_SemacquireMutex", "runtime_SemacquireRWMutex", "runtime_SemacquireRWMutexR":
				ctx.switchFunctionToCallRuntimeApi("gox5_semaphore_acquire", "StackFrameSemaphoreAcquire", createInstructionName(instr), nil, nil,
					paramArgPair{param: "s", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
				)
				return true
			case "runtime_Semrelease":
				ctx.switchFunctionToCallRuntimeApi("gox5_semaphore_release", "StackFrameSemaphoreRelease", createInstructionName(instr), nil, nil,
					paramArgPair{param: "s", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
				)
				return true
			case "runtime_canSpin", "runtime_nanotime", "runtime_rand":
				result := createValueRelName(instr)
				fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", result, result)
				fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
				return true
			case "runtime_doSpin":
				fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
				return true
			case "runtime_procPin":
				// Single-threaded runtime: pin to P 0 (pool local index 0).
				result := createValueRelName(instr)
				fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", result, result)
				fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
				return true
			case "runtime_procUnpin":
				fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
				return true
			case "runtime_LoadAcquintptr":
				// Acquire load of a uintptr through the given pointer (used by sync.Pool).
				result := createValueRelName(instr)
				fmt.Fprintf(ctx.stream, "%s.raw = atomic_load_explicit((_Atomic uintptr_t*)%s.raw, memory_order_acquire);\n", result, createValueRelName(callCommon.Args[0]))
				fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
				return true
			case "runtime_StoreReluintptr":
				// Release store of a uintptr through the given pointer (used by sync.Pool).
				fmt.Fprintf(ctx.stream, "atomic_store_explicit((_Atomic uintptr_t*)%s.raw, %s.raw, memory_order_release);\n", createValueRelName(callCommon.Args[0]), createValueRelName(callCommon.Args[1]))
				fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
				return true
			case "throw", "fatal":
				fmt.Fprintf(ctx.stream, "assert(false);\n")
				fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
				return true
			}
		}
	}

	if pkgPath == "math/rand" && funcName == "runtime_rand" {
		// math/rand.runtime_rand is a //go:linkname to runtime.rand with no Go
		// body, used to seed the global rand source. Route it into the Rust
		// runtime which returns a deterministic pseudo-random uint64.
		result := createValueRelName(instr)
		ctx.switchFunctionToCallRuntimeApi("gox5_runtime_rand", "StackFrameRuntimeRand", createInstructionName(instr), &result, nil)
		return true
	}

	if pkgPath == "time" && funcName == "runtimeNano" {
		// time.runtimeNano is a //go:linkname to runtime.nanotime with no Go
		// body; return 0 (single-threaded runtime has no real clock source).
		result := createValueRelName(instr)
		fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", result, result)
		fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
		return true
	}

	if isFunctionBodySkippedPackagePath(pkgPath) {
		if funcName == "init" {
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		}

		if pkgPath == "runtime" && funcName == "Goexit" {
			ctx.switchFunctionToCallRuntimeApi("gox5_lwt_exit", "StackFrameLwtExit", "NULL", nil, nil)
			return true
		}
		if pkgPath == "runtime" && funcName == "Gosched" {
			ctx.switchFunctionToCallRuntimeApi("gox5_lwt_yield", "StackFrameLwtYield", createInstructionName(instr), nil, nil)
			return true
		}
		if pkgPath == "runtime" && funcName == "GOMAXPROCS" {
			// sync.Pool.pinSlow asks for the number of Ps; report 1.
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", result, result)
			fmt.Fprintf(ctx.stream, "%s.raw = 1;\n", result)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		}

		if strings.HasPrefix(pkgPath, "internal/race") ||
			strings.HasPrefix(pkgPath, "internal/msan") ||
			strings.HasPrefix(pkgPath, "internal/asan") {
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		}

		panic(fmt.Sprintf("unexpected call to function in skipped package: %s.%s (called from %s)", pkgPath, funcName, createFunctionName(instr.Parent())))
	}

	return false
}

func (ctx *Context) emitBuiltinPrintWrapper(name string, callCommon *ssa.CallCommon, instr ssa.Instruction) string {
	if ctx.builtinPrintWrapperNames == nil {
		ctx.builtinPrintWrapperNames = make(map[*ssa.CallCommon]string)
	}
	if cached, ok := ctx.builtinPrintWrapperNames[callCommon]; ok {
		return cached
	}

	wrapperName := fmt.Sprintf("builtin_print_wrapper_%s", createInstructionName(instr))
	ctx.builtinPrintWrapperNames[callCommon] = wrapperName

	fmt.Fprintf(&ctx.builtinPrintWrapperBuf, "__attribute__((unused)) static FunctionObject %s(LightWeightThreadContext* ctx) {\n", wrapperName)
	fmt.Fprintf(&ctx.builtinPrintWrapperBuf, "\tassert(ctx->marker == 0xdeadbeef);\n")
	fmt.Fprintf(&ctx.builtinPrintWrapperBuf, "\tStackFrameCommon* args_frame = (StackFrameCommon*)ctx->stack_pointer;\n")
	fmt.Fprintf(&ctx.builtinPrintWrapperBuf, "\tuintptr_t* arg_words = (uintptr_t*)(args_frame + 1);\n")
	fmt.Fprintf(&ctx.builtinPrintWrapperBuf, "\tStackFrameCommon* prev = (StackFrameCommon*)args_frame->prev_stack_pointer;\n")
	fmt.Fprintf(&ctx.builtinPrintWrapperBuf, "\tuintptr_t arg_word_index = 0;\n")

	for i, arg := range callCommon.Args {
		argType := createTypeName(arg.Type())
		fmt.Fprintf(&ctx.builtinPrintWrapperBuf, "\t%s arg%d_val = *(%s*)&arg_words[arg_word_index];\n", argType, i, argType)
		fmt.Fprintf(&ctx.builtinPrintWrapperBuf, "\targ_word_index += sizeof(%s) / sizeof(arg_words[0]);\n", argType)
	}

	rawExprs := make([]string, len(callCommon.Args))
	lenExprs := make([]string, len(callCommon.Args))
	for i := range callCommon.Args {
		rawExprs[i] = fmt.Sprintf("arg%d_val.raw", i)
		lenExprs[i] = fmt.Sprintf("arg%d_val.len", i)
	}
	emitInlinePrintBody(&ctx.builtinPrintWrapperBuf, name == "print", callCommon.Args, rawExprs, lenExprs)

	fmt.Fprintf(&ctx.builtinPrintWrapperBuf, "\tctx->stack_pointer = prev;\n")
	fmt.Fprintf(&ctx.builtinPrintWrapperBuf, "\treturn (FunctionObject){.raw = gox5_defer_execute};\n")
	fmt.Fprintf(&ctx.builtinPrintWrapperBuf, "}\n")

	return wrapperName
}

func emitInlinePrintBody(w io.Writer, packs bool, args []ssa.Value, rawExprs []string, lenExprs []string) {
	var emitInlinePrintArgByType func(w io.Writer, t types.Type, rawExpr string, lenExpr string)
	emitInlinePrintArgByType = func(w io.Writer, t types.Type, rawExpr string, lenExpr string) {
		switch t := t.(type) {
		case *types.Basic:
			switch t.Kind() {
			case types.Bool:
				fmt.Fprintf(w, "\tfprintf(stderr, \"%%s\", %s ? \"true\" : \"false\");\n", rawExpr)
			case types.Complex64, types.Complex128:
				fmt.Fprintf(w, "\tfprintf(stderr, \"(\");\n")
				fmt.Fprintf(w, "\tfprintf(stderr, \"%%.15g\", creal(%s) == 0.0 && signbit(creal(%s)) ? 0.0 : creal(%s));\n", rawExpr, rawExpr, rawExpr)
				fmt.Fprintf(w, "\tif (!signbit(cimag(%s))) fprintf(stderr, \"+\");\n", rawExpr)
				fmt.Fprintf(w, "\tfprintf(stderr, \"%%.15g\", cimag(%s) == 0.0 && signbit(cimag(%s)) ? 0.0 : cimag(%s));\n", rawExpr, rawExpr, rawExpr)
				fmt.Fprintf(w, "\tfprintf(stderr, \"i)\");\n")
			case types.Int, types.Int64:
				fmt.Fprintf(w, "\tfprintf(stderr, \"%%ld\", (long)(%s));\n", rawExpr)
			case types.Int8, types.Int16, types.Int32:
				fmt.Fprintf(w, "\tfprintf(stderr, \"%%d\", (int)(%s));\n", rawExpr)
			case types.Uint, types.Uint64, types.Uintptr:
				fmt.Fprintf(w, "\tfprintf(stderr, \"%%lu\", (unsigned long)(%s));\n", rawExpr)
			case types.Uint8, types.Uint16, types.Uint32:
				fmt.Fprintf(w, "\tfprintf(stderr, \"%%u\", (unsigned int)(%s));\n", rawExpr)
			case types.Float32, types.Float64:
				fmt.Fprintf(w, "\tfprintf(stderr, \"%%.15g\", (%s) == 0.0 && signbit((%s)) ? 0.0 : (%s));\n", rawExpr, rawExpr, rawExpr)
			case types.String:
				fmt.Fprintf(w, "\tif (%s) { fprintf(stderr, \"%%.*s\", (int)(%s), %s); }\n", rawExpr, lenExpr, rawExpr)
			case types.UnsafePointer:
				fmt.Fprintf(w, "\tfprintf(stderr, \"%%p\", %s);\n", rawExpr)
			default:
				panic(fmt.Sprintf("unsupported type for print/println: %s (%T)", t, t))
			}
		case *types.Named:
			emitInlinePrintArgByType(w, t.Underlying(), rawExpr, lenExpr)
		case *types.Pointer:
			fmt.Fprintf(w, "\tfprintf(stderr, \"%%p\", (void*)(%s));\n", rawExpr)
		default:
			panic(fmt.Sprintf("unsupported type for print/println: %s (%T)", t, t))
		}
	}
	for i, arg := range args {
		if !packs && i != 0 {
			fmt.Fprintf(w, "\tfprintf(stderr, \" \");\n")
		}
		emitInlinePrintArgByType(w, arg.Type(), rawExprs[i], lenExprs[i])
	}
	if !packs {
		fmt.Fprintf(w, "\tfprintf(stderr, \"\\n\");\n")
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
			if t, ok := instr.X.Type().Underlying().(*types.Basic); ok && t.Kind() == types.String {
				raw = fmt.Sprintf("string_compare(&%s, &%s) %s 0",
					createValueRelName(instr.X), createValueRelName(instr.Y),
					instr.Op.String())
			} else {
				raw = fmt.Sprintf("%s.raw %s %s.raw", createValueRelName(instr.X), instr.Op.String(), createValueRelName(instr.Y))
			}
		case token.ADD:
			if t, ok := instr.Type().Underlying().(*types.Basic); ok && t.Kind() == types.String {
				result := createValueRelName(instr)
				ctx.switchFunctionToCallRuntimeApi("gox5_string_append", "StackFrameStringAppend", createInstructionName(instr), &result, nil,
					paramArgPair{param: "lhs", arg: createValueRelName(instr.X)},
					paramArgPair{param: "rhs", arg: createValueRelName(instr.Y)},
				)
				needToCallRuntimeApi = true
			} else if isSignedIntegerType(instr.Type()) {
				// Go wraps signed integer arithmetic at the type width;
				// compute via unsigned arithmetic so C does not invoke
				// undefined signed-overflow behavior (e.g. under UBSan).
				raw = fmt.Sprintf("(%s)((%s)%s.raw + (%s)%s.raw)",
					createRawTypeName(instr.Type()),
					"u"+createRawTypeName(instr.Type()), createValueRelName(instr.X),
					"u"+createRawTypeName(instr.Type()), createValueRelName(instr.Y))
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
			if isSignedIntegerType(instr.Type()) && (instr.Op == token.SUB || instr.Op == token.MUL) {
				// See the token.ADD case: wrap signed arithmetic via unsigned.
				raw = fmt.Sprintf("(%s)((%s)%s.raw %s (%s)%s.raw)",
					createRawTypeName(instr.Type()),
					"u"+createRawTypeName(instr.Type()), createValueRelName(instr.X), instr.Op.String(),
					"u"+createRawTypeName(instr.Type()), createValueRelName(instr.Y))
			} else {
				raw = fmt.Sprintf("%s.raw %s %s.raw", createValueRelName(instr.X), instr.Op.String(), createValueRelName(instr.Y))
			}
		}
		if !needToCallRuntimeApi {
			fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInObject(raw, instr.Type()))
		}

	case *ssa.Call:
		callCommon := instr.Common()
		if callCommon.Method != nil {
			signature := callCommon.Method.Type().Underlying().(*types.Signature)
			result_ptr := "NULL"
			if signature.Results().Len() > 0 {
				result_ptr = fmt.Sprintf("&%s", createValueRelName(instr))
			}
			if callCommon.Method.Pkg() != nil && callCommon.Method.Pkg().Path() == "internal/reflectlite" {
				// Method call on the internal/reflectlite.Type interface. There
				// is no reflectlite support in the runtime; the value produced by
				// reflectlite.TypeOf is a zeroed interface, so a concrete
				// dispatch would fail. Provide fixed answers: Comparable() ->
				// true (all WithValue keys are comparable), everything else ->
				// zero. Bodies calling these (e.g. errors.init) are never
				// invoked at runtime; they only need to compile.
				result := createValueRelName(instr)
				switch callCommon.Method.Name() {
				case "Comparable":
					fmt.Fprintf(ctx.stream, "%s.raw = 1;\n", result)
				default:
					fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", result, result)
				}
				fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
				fmt.Fprintf(ctx.stream, "\t}\n")
				return
			}
			if callCommon.Method.Pkg() != nil && callCommon.Method.Pkg().Path() == "reflect" {
				if recvType, ok := signature.Recv().Type().(*types.Named); ok && recvType.Obj().Name() == "Type" &&
					recvType.Obj().Pkg() != nil && recvType.Obj().Pkg().Path() == "reflect" {
					// Method call on the reflect.Type interface. Such an interface value
					// is only ever fabricated by the reflect.TypeOf intercept (in
					// emitSpecialRuntimeCall), which stores the underlying type_id in the
					// receiver field. Route Kind() and String() to the runtime so fmt's
					// spacing logic and the %T verb work; any other reflect.Type method
					// falls back to zero.
					result := createValueRelName(instr)
					typeId := fmt.Sprintf("(uintptr_t)%s.receiver", createValueRelName(callCommon.Value))
					switch callCommon.Method.Name() {
					case "Kind":
						// reflect.Kind is a named unsigned int; the runtime writes the kind as
						// an i32.
						resultPtr := fmt.Sprintf("%s.raw", result)
						ctx.switchFunctionToCallRuntimeApi("gox5_reflect_type_kind", "StackFrameReflectTypeKind", createInstructionName(instr), &resultPtr,
							nil,
							paramArgPair{param: "type_id", arg: typeId})
					case "String":
						ctx.switchFunctionToCallRuntimeApi("gox5_reflect_type_string", "StackFrameReflectTypeString", createInstructionName(instr), &result,
							nil,
							paramArgPair{param: "type_id", arg: typeId})
					default:
						fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", result, result)
						fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
						fmt.Fprintf(ctx.stream, "\t}\n")
						return
					}
					fmt.Fprintf(ctx.stream, "\t}\n")
					return
				}
			}
			ctx.switchFunctionToCallRuntimeApi("gox5_interface_invoke", "StackFrameInterfaceInvoke", createInstructionName(instr), nil,
				func() {
					receiver := fmt.Sprintf("%s.receiver", createValueRelName(callCommon.Value))
					receiverSize := fmt.Sprintf("%s.type_id.info->size", createValueRelName(callCommon.Value))
					fmt.Fprintf(ctx.stream, "memcpy(&next_frame->arg_buffer[0], %s, %s); // receiver: %s\n", receiver, receiverSize, signature.Recv())
					fmt.Fprintf(ctx.stream, "intptr_t num_arg_buffer_words = (%s + sizeof(next_frame->arg_buffer[0]) - 1) / sizeof(next_frame->arg_buffer[0]);\n", receiverSize)
					for i, arg := range callCommon.Args {
						argValue := createValueRelName(arg)
						argType := createTypeName(arg.Type())
						argPtr := fmt.Sprintf("ptr%d", i)
						fmt.Fprintf(ctx.stream, "%s* %s = (void*)&next_frame->arg_buffer[num_arg_buffer_words]; // param[%d]\n", argType, argPtr, i)
						fmt.Fprintf(ctx.stream, "*%s = %s;\n", argPtr, argValue)
						fmt.Fprintf(ctx.stream, "num_arg_buffer_words += (sizeof(%s) + sizeof(next_frame->arg_buffer[0]) - 1) / sizeof(next_frame->arg_buffer[0]);\n", argType)
					}
					fmt.Fprintf(ctx.stream, "next_frame->num_arg_buffer_words = num_arg_buffer_words;\n")
				},
				paramArgPair{param: "result_ptr", arg: result_ptr},
				paramArgPair{param: "interface", arg: fmt.Sprintf("&%s", createValueRelName(callCommon.Value))},
				paramArgPair{param: "method_name", arg: fmt.Sprintf("(StringObject){.raw = \"%s\", .len = sizeof(\"%s\") - 1}", callCommon.Method.Name(), callCommon.Method.Name())},
			)
		} else {
			switch callee := callCommon.Value.(type) {
			case *ssa.Builtin:
				complexNumberBitLength := func(v ssa.Value) uint {
					switch v.Type().Underlying().(*types.Basic).Kind() {
					case types.Complex64:
						return 64
					case types.Complex128:
						return 128
					default:
						panic("unreachable")
					}
				}

				switch callee.Name() {
				case "Add":
					result := createValueRelName(instr)
					ptr := fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))
					offset := fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[1]))
					raw := fmt.Sprintf("(void*)((uintptr_t)%s + (uintptr_t)%s)", ptr, offset)
					fmt.Fprintf(ctx.stream, "%s = %s;\n", result, wrapInObject(raw, instr.Type()))

				case "append":
					result := createValueRelName(instr)
					result += ".raw"
					if t, ok := callCommon.Args[1].Type().Underlying().(*types.Basic); ok && t.Kind() == types.String {
						if instr.Type().Underlying().(*types.Slice).Elem().(*types.Basic).Kind() != types.Byte {
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

				case "cap":
					result := createValueRelName(instr)
					ctx.switchFunctionToCallRuntimeApi("gox5_slice_capacity", "StackFrameSliceCapacity", createInstructionName(instr), &result, nil,
						paramArgPair{param: "slice", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
					)

				case "clear":
					switch t := callCommon.Args[0].Type().Underlying().(type) {
					case *types.Map:
						ctx.switchFunctionToCallRuntimeApi("gox5_map_clear", "StackFrameMapClear", createInstructionName(instr), nil, nil,
							paramArgPair{param: "map", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
						)
					case *types.Slice:
						slice := createValueRelName(callCommon.Args[0])
						elemType := t.Elem()
						fmt.Fprintf(ctx.stream, "if (%s.raw.size != 0) { memset(%s.raw.addr, 0, %s.raw.size * sizeof(%s)); }\n", slice, slice, slice, createTypeName(elemType))
					default:
						panic(fmt.Sprintf("unsupported argument for clear: %s (%s)", callCommon.Args[0], t))
					}

				case "close":
					ctx.switchFunctionToCallRuntimeApi("gox5_channel_close", "StackFrameChannelClose", createInstructionName(instr), nil, nil,
						paramArgPair{param: "channel", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
					)

				case "copy":
					result := createValueRelName(instr)
					if t, ok := callCommon.Args[1].Type().Underlying().(*types.Basic); ok && t.Kind() == types.String {
						ctx.switchFunctionToCallRuntimeApi("gox5_slice_copy_string", "StackFrameSliceCopyString", createInstructionName(instr), &result, nil,
							paramArgPair{param: "src", arg: createValueRelName(callCommon.Args[1])},
							paramArgPair{param: "dst", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
						)
					} else {
						ctx.switchFunctionToCallRuntimeApi("gox5_slice_copy", "StackFrameSliceCopy", createInstructionName(instr), &result, nil,
							paramArgPair{param: "type_id", arg: wrapInTypeId(callCommon.Args[0].Type().Underlying().(*types.Slice).Elem())},
							paramArgPair{param: "src", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[1]))},
							paramArgPair{param: "dst", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
						)
					}

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

				case "delete":
					ctx.switchFunctionToCallRuntimeApi("gox5_map_delete", "StackFrameMapDelete", createInstructionName(instr), nil, nil,
						paramArgPair{param: "map", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
						paramArgPair{param: "key", arg: fmt.Sprintf("&%s", createValueRelName(callCommon.Args[1]))},
					)

				case "imag":
					bitLength := complexNumberBitLength(callCommon.Args[0])
					result := createValueRelName(instr)
					ctx.switchFunctionToCallRuntimeApi(
						fmt.Sprintf("gox5_complex%d_component", bitLength),
						fmt.Sprintf("StackFrameComplex%dComponent", bitLength),
						createInstructionName(instr), &result, nil,
						paramArgPair{param: "value", arg: createValueRelName(callCommon.Args[0])},
						paramArgPair{param: "is_real", arg: "false"},
					)

				case "len":
					switch t := callCommon.Args[0].Type().Underlying().(type) {
					case *types.Basic:
						switch t.Kind() {
						case types.String:
							result := createValueRelName(instr)
							ctx.switchFunctionToCallRuntimeApi("gox5_string_length", "StackFrameStringLength", createInstructionName(instr), &result, nil,
								paramArgPair{param: "string", arg: createValueRelName(callCommon.Args[0])},
							)
						default:
							panic(fmt.Sprintf("unsuported argument for len: %s (%s)", callCommon.Args[0], t))
						}
					case *types.Map:
						result := createValueRelName(instr)
						ctx.switchFunctionToCallRuntimeApi("gox5_map_len", "StackFrameMapLen", createInstructionName(instr), &result, nil,
							paramArgPair{param: "map", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
						)
					case *types.Slice:
						result := createValueRelName(instr)
						ctx.switchFunctionToCallRuntimeApi("gox5_slice_size", "StackFrameSliceSize", createInstructionName(instr), &result, nil,
							paramArgPair{param: "slice", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
						)
					default:
						panic(fmt.Sprintf("unsuported argument for len: %s", callCommon.Args[0]))
					}

				case "max", "min":
					result := createValueRelName(instr)
					arg := callCommon.Args[0]
					basic, ok := arg.Type().Underlying().(*types.Basic)
					isString := ok && basic.Kind() == types.String
					if !ok || (!isNumericKind(basic.Kind()) && !isString) {
						panic(fmt.Sprintf("unsupported argument for %s: %s (%s)", callee.Name(), arg, arg.Type()))
					}
					op := ">"
					if callee.Name() == "min" {
						op = "<"
					}
					expr := createValueRelName(arg)
					if !isString {
						expr = fmt.Sprintf("%s.raw", expr)
					}
					for _, next := range callCommon.Args[1:] {
						if isString {
							rhs := createValueRelName(next)
							expr = fmt.Sprintf("(string_compare(&%s, &%s) %s 0 ? %s : %s)", expr, rhs, op, expr, rhs)
						} else {
							rhs := fmt.Sprintf("%s.raw", createValueRelName(next))
							expr = fmt.Sprintf("(%s %s %s ? %s : %s)", expr, op, rhs, expr, rhs)
						}
					}
					if isString {
						fmt.Fprintf(ctx.stream, "%s = %s;\n", result, expr)
					} else {
						fmt.Fprintf(ctx.stream, "%s = %s;\n", result, wrapInObject(expr, instr.Type()))
					}

				case "print", "println":
					rawExprs := make([]string, len(callCommon.Args))
					lenExprs := make([]string, len(callCommon.Args))
					for i, arg := range callCommon.Args {
						rawExprs[i] = fmt.Sprintf("%s.raw", createValueRelName(arg))
						lenExprs[i] = fmt.Sprintf("%s.len", createValueRelName(arg))
					}
					emitInlinePrintBody(ctx.stream, callee.Name() == "print", callCommon.Args, rawExprs, lenExprs)

				case "real":
					bitLength := complexNumberBitLength(callCommon.Args[0])
					result := createValueRelName(instr)
					ctx.switchFunctionToCallRuntimeApi(
						fmt.Sprintf("gox5_complex%d_component", bitLength),
						fmt.Sprintf("StackFrameComplex%dComponent", bitLength),
						createInstructionName(instr), &result, nil,
						paramArgPair{param: "value", arg: createValueRelName(callCommon.Args[0])},
						paramArgPair{param: "is_real", arg: "true"},
					)

				case "recover":
					result := createValueRelName(instr)
					ctx.switchFunctionToCallRuntimeApi("gox5_panic_recover", "StackFramePanicRecover", createInstructionName(instr), &result, nil)

				case "ssa:wrapnilchk":
					result := createValueRelName(instr)
					ctx.switchFunctionToCallRuntimeApi("gox5_check_non_nil", "StackFrameCheckNonNil", createInstructionName(instr), &result, nil,
						paramArgPair{param: "pointer", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
					)

				case "Sizeof":
					result := createValueRelName(instr)
					expr := fmt.Sprintf("sizeof(%s)", createTypeName(callCommon.Args[0].Type()))
					fmt.Fprintf(ctx.stream, "%s = %s;\n", result, wrapInObject(expr, instr.Type()))

				case "Slice":
					result := createValueRelName(instr)
					ptr := fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))
					length := fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[1]))
					raw := fmt.Sprintf("(SliceObject){.addr = %s, .size = (uintptr_t)%s, .capacity = (uintptr_t)%s}", ptr, length, length)
					fmt.Fprintf(ctx.stream, "%s = %s;\n", result, wrapInObject(raw, instr.Type()))

				case "SliceData":
					result := createValueRelName(instr)
					elemType := instr.Type().Underlying().(*types.Pointer).Elem()
					raw := fmt.Sprintf("(%s*)%s.raw.addr", createTypeName(elemType), createValueRelName(callCommon.Args[0]))
					fmt.Fprintf(ctx.stream, "%s = %s;\n", result, wrapInObject(raw, instr.Type()))

				case "String":
					result := createValueRelName(instr)
					ptr := fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))
					length := fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[1]))
					byteSlice := fmt.Sprintf("(SliceObject){.addr = %s, .size = (uintptr_t)%s, .capacity = (uintptr_t)%s}", ptr, length, length)
					ctx.switchFunctionToCallRuntimeApi("gox5_string_new_from_byte_slice", "StackFrameStringNewFromByteSlice", createInstructionName(instr), &result, nil,
						paramArgPair{param: "byte_slice", arg: byteSlice},
					)

				case "StringData":
					result := createValueRelName(instr)
					elemType := instr.Type().Underlying().(*types.Pointer).Elem()
					raw := fmt.Sprintf("(%s*)%s.raw", createTypeName(elemType), createValueRelName(callCommon.Args[0]))
					fmt.Fprintf(ctx.stream, "%s = %s;\n", result, wrapInObject(raw, instr.Type()))

				default:
					panic(fmt.Sprintf("unsuported builtin function: %s (in %s)", callee.Name(), createFunctionName(instr.Parent())))
				}
				fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))

			default:
				if ctx.emitAtomicCall(instr, callCommon) {
					break
				}
				if function, ok := callee.(*ssa.Function); ok {
					if ctx.emitSpecialRuntimeCall(function, instr, callCommon) {
						break
					}
				}
				nextFunction := createValueRelName(callee)
				if function, ok := callee.(*ssa.Function); ok {
					if target := mathArchImplementation(function); target != nil {
						nextFunction = createValueRelName(target)
					}
				}
				signature := callCommon.Value.Type().Underlying().(*types.Signature)
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
						fmt.Fprintf(ctx.stream, "signature->param%d = %s; // param: %d\n", paramBase+i, createValueRelName(arg), i)
					}
				})
			}
		}

	case *ssa.ChangeInterface:
		fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), createValueRelName(instr.X))

	case *ssa.ChangeType:
		if t, ok := instr.Type().Underlying().(*types.Basic); ok && (t.Kind() == types.String || t.Kind() == types.UntypedString) {
			fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), createValueRelName(instr.X))
		} else {
			s := wrapInObject(fmt.Sprintf("%s.raw", createValueRelName(instr.X)), instr.Type())
			fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), s)
		}

	case *ssa.Convert:
		switch dstType := instr.Type().Underlying().(type) {
		case *types.Basic:
			switch dstType.Kind() {
			case types.String:
				result := createValueRelName(instr)
				switch srcType := instr.X.Type().Underlying().(type) {
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
			elemType := dstType.Elem().Underlying().(*types.Basic)
			switch elemType.Kind() {
			case types.Byte, types.Rune:
				// valid conversion
			default:
				panic(instr.String())
			}
			srcType := instr.X.Type().Underlying().(*types.Basic)
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
		ctx.emitGoOrDefer(instr, instr.Common(), "gox5_defer_register", "StackFrameDeferRegister", "gox5_defer_register_invoke", "StackFrameDeferRegisterInvoke", true)

	case *ssa.Extract:
		fmt.Fprintf(ctx.stream, "%s = %s.raw.e%d;\n", createValueRelName(instr), createValueRelName(instr.Tuple), instr.Index)

	case *ssa.Field:
		index := instr.Field
		name := createFieldName(instr.X.Type().Underlying().(*types.Struct).Field(index), index)
		fmt.Fprintf(ctx.stream, "%s val = %s.%s;\n", createTypeName(instr.Type()), createValueRelName(instr.X), name)
		fmt.Fprintf(ctx.stream, "%s = val;\n", createValueRelName(instr))

	case *ssa.FieldAddr:
		index := instr.Field
		name := createFieldName(instr.X.Type().Underlying().(*types.Pointer).Elem().Underlying().(*types.Struct).Field(index), index)
		fmt.Fprintf(ctx.stream, "%s* raw = &(%s.raw->%s);\n", createTypeName(instr.Type().(*types.Pointer).Elem()), createValueRelName(instr.X), name)
		fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInObject("raw", instr.Type()))

	case *ssa.Index:
		fmt.Fprintf(ctx.stream, "uintptr_t index = %s.raw;\n", createValueRelName(instr.Index))
		switch t := instr.X.Type().Underlying().(type) {
		case *types.Array:
			fmt.Fprintf(ctx.stream, "%s val = %s.raw[index];\n", createTypeName(t.Elem()), createValueRelName(instr.X))
		case *types.Basic:
			if t.Kind() == types.String || t.Kind() == types.UntypedString {
				fmt.Fprintf(ctx.stream, "%s val = { .raw = (uint8_t)%s.raw[index] };\n", createTypeName(instr.Type()), createValueRelName(instr.X))
			} else {
				panic(fmt.Sprintf("%s, %s, %s", instr, instr.X, t))
			}
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
		ctx.emitGoOrDefer(instr, instr.Common(), "gox5_lwt_spawn", "StackFrameLwtSpawn", "gox5_lwt_spawn_invoke", "StackFrameLwtSpawnInvoke", false)

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
			valueType := instr.X.Type().Underlying().(*types.Map).Elem()
			ctx.switchFunctionToCallRuntimeApi("gox5_map_get", "StackFrameMapGet", createInstructionName(instr), nil,
				func() {
					fmt.Fprintf(ctx.stream, "if (next_frame->map.raw == NULL) {\n")
					fmt.Fprintf(ctx.stream, "\tmemset(next_frame->value, 0, sizeof(%s));\n", createTypeName(valueType))
					fmt.Fprintf(ctx.stream, "}\n")
				},
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
		ctx.switchFunctionToCallRuntimeApi("gox5_closure_new", "StackFrameClosureNew", createInstructionName(instr), &result,
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
		receiverArg := fmt.Sprintf("&%s", createValueRelName(instr.X))
		if _, ok := instr.X.(*ssa.Function); ok {
			// See the makeInterfaceReceiver field comment in
			// emitFunctionVariableStructure: keep the function object in a stable
			// frame slot before handing its address to the runtime.
			fmt.Fprintf(ctx.stream, "frame->makeInterfaceReceiver = %s;\n", createValueRelName(instr.X))
			receiverArg = "&frame->makeInterfaceReceiver"
		} else if _, ok := instr.X.(*ssa.Global); ok {
			// A package global is referenced through an inline compound literal,
			// so taking its address would point at a machine-stack slot that is
			// dead by the time gox5_interface_new reads the receiver. Keep a
			// stable copy in the frame instead (see emitFunctionVariableStructure).
			fmt.Fprintf(ctx.stream, "frame->makeInterfaceReceiver_%s = %s;\n", createValueName(instr), createValueRelName(instr.X))
			receiverArg = fmt.Sprintf("&frame->makeInterfaceReceiver_%s", createValueName(instr))
		}
		ctx.switchFunctionToCallRuntimeApi("gox5_interface_new", "StackFrameInterfaceNew", createInstructionName(instr), &result, nil,
			paramArgPair{param: "receiver", arg: receiverArg},
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
		size := fmt.Sprintf("(%s.raw) * sizeof(%s)", createValueRelName(instr.Cap), createTypeName(instr.Type().Underlying().(*types.Slice).Elem()))
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
		rng := fmt.Sprintf("&%s.raw.e1", result)
		if t, ok := instr.Type().(*types.Tuple).At(1).Type().(*types.Basic); ok {
			if t.Kind() == types.Invalid {
				rng = "NULL"
			}
		}
		dom := fmt.Sprintf("&%s.raw.e2", result)
		if t, ok := instr.Type().(*types.Tuple).At(2).Type().(*types.Basic); ok {
			if t.Kind() == types.Invalid {
				dom = "NULL"
			}
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
		panic("unreachable: phis are emitted by emitBlockPhis")

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
		if t, ok := instr.Type().Underlying().(*types.Basic); ok {
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
			endIndexDefault := ""
			capacityDefault := ""
			switch t := instr.X.Type().Underlying().(type) {
			case *types.Pointer:
				ptr = "raw->raw"
				elemType := t.Elem().Underlying().(*types.Array)
				length := fmt.Sprintf("%d", elemType.Len())
				endIndexDefault = length
				capacityDefault = length
			case *types.Slice:
				ptr = "typed.ptr"
				endIndexDefault = fmt.Sprintf("%s.typed.size", createValueRelName(instr.X))
				capacityDefault = fmt.Sprintf("%s.typed.capacity", createValueRelName(instr.X))
			default:
				panic(fmt.Sprintf("not implemented: %s (%T)", t, t))
			}

			endIndex := endIndexDefault
			if instr.High != nil {
				endIndex = fmt.Sprintf("%s.raw", createValueRelName(instr.High))
			}

			capacity := capacityDefault
			if instr.Max != nil {
				capacity = fmt.Sprintf("%s.raw", createValueRelName(instr.Max))
			}

			fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInObject("0", instr.Type()))
			fmt.Fprintf(ctx.stream, "%s.typed.ptr = %s.%s + %s;\n", createValueRelName(instr), createValueRelName(instr.X), ptr, startIndex)
			fmt.Fprintf(ctx.stream, "%s.typed.size = %s - %s;\n", createValueRelName(instr), endIndex, startIndex)
			fmt.Fprintf(ctx.stream, "%s.typed.capacity = %s - %s;\n", createValueRelName(instr), capacity, startIndex)
		}

	case *ssa.Store:
		fmt.Fprintf(ctx.stream, "*(%s.raw) = %s;\n", createValueRelName(instr.Addr), createValueRelName(instr.Val))

	case *ssa.TypeAssert:
		result := createValueRelName(instr)
		var value, success string
		if instr.CommaOk {
			value = fmt.Sprintf("&%s.raw.e0", result)
			success = fmt.Sprintf("&%s.raw.e1.raw", result)
		} else {
			value = fmt.Sprintf("&%s", result)
			success = "NULL"
		}
		var nextFunction, nextFunctionFrame string
		if _, ok := instr.AssertedType.Underlying().(*types.Interface); ok {
			nextFunction = "gox5_interface_convert_to_interface"
			nextFunctionFrame = "StackFrameInterfaceConvertToInterface"
		} else {
			nextFunction = "gox5_interface_convert_to_concrete_type"
			nextFunctionFrame = "StackFrameInterfaceConvertToConcreteType"
		}
		ctx.switchFunctionToCallRuntimeApi(nextFunction, nextFunctionFrame, createInstructionName(instr), nil, nil,
			paramArgPair{param: "interface", arg: fmt.Sprintf("&%s", createValueRelName(instr.X))},
			paramArgPair{param: "to_type", arg: wrapInTypeId(instr.AssertedType)},
			paramArgPair{param: "value", arg: value},
			paramArgPair{param: "success", arg: success},
		)

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
	block := instruction.Block()
	blockName := block.String()
	index := func() int {
		for i, instr := range block.Instrs {
			if instr == instruction {
				return i
			}
		}
		panic(instruction)
	}()
	function := instruction.Parent()
	functionName := function.RelString(nil)
	return encode(fmt.Sprintf("i$%d$%s$%s", index, blockName, functionName))
}

func createBasicBlockName(basicBlock *ssa.BasicBlock) string {
	function := basicBlock.Parent()
	functionName := function.RelString(nil)
	return encode(fmt.Sprintf("b$%s$%s", basicBlock.String(), functionName))
}

func createFunctionName(function *ssa.Function) string {
	return encode(fmt.Sprintf("f$%s", function.RelString(nil)))
}

func isGeneratedGenericInstance(fn *ssa.Function) bool {
	return fn.Pkg == nil && len(fn.TypeArgs()) > 0 && fn.Parent() == nil
}

func isInGenericInstanceSubtree(fn *ssa.Function) bool {
	for f := fn; f != nil; f = f.Parent() {
		if isGeneratedGenericInstance(f) {
			return true
		}
	}
	return false
}

// isGenericInstanceSubtreeMember reports whether fn belongs to some generic
// instance's subtree, including the synthetic $bound/$thunk wrappers that
// go/ssa creates for method values on instantiated receivers.
func isGenericInstanceSubtreeMember(fn *ssa.Function) bool {
	if isInGenericInstanceSubtree(fn) {
		return true
	}
	if fn == nil || fn.Pkg != nil || fn.Parent() != nil {
		return false
	}
	name := fn.Name()
	return strings.HasSuffix(name, "$bound") || strings.HasSuffix(name, "$thunk")
}

// wrapperTargetFunction returns the function that a synthetic $bound/$thunk
// wrapper forwards to.
func wrapperTargetFunction(fn *ssa.Function) *ssa.Function {
	if fn.Blocks == nil {
		return nil
	}
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			for _, operand := range instr.Operands(nil) {
				if ref, ok := (*operand).(*ssa.Function); ok && ref != fn {
					return ref
				}
			}
		}
	}
	return nil
}

// isPlainSyntheticWrapper reports whether fn is a synthetic $bound/$thunk
// wrapper around a non-generic method. Such wrappers belong to no generic
// instance subtree, so shared_definition.c defines them once with external
// linkage instead of every user file carrying a private copy.
func isPlainSyntheticWrapper(fn *ssa.Function) bool {
	if isInGenericInstanceSubtree(fn) {
		return false
	}
	if fn == nil || fn.Pkg != nil || fn.Parent() != nil {
		return false
	}
	name := fn.Name()
	return strings.HasSuffix(name, "$bound") || strings.HasSuffix(name, "$thunk")
}

func hasTypeParamInSignature(sig *types.Signature) bool {
	if sig.Recv() != nil && hasTypeParameter(sig.Recv().Type()) {
		return true
	}
	for i := 0; i < sig.Params().Len(); i++ {
		if hasTypeParameter(sig.Params().At(i).Type()) {
			return true
		}
	}
	for i := 0; i < sig.Results().Len(); i++ {
		if hasTypeParameter(sig.Results().At(i).Type()) {
			return true
		}
	}
	return false
}

// ToDo: refactor to avoid using a global variable
var hasTypeParamCache sync.Map

func hasTypeParameter(typ types.Type) bool {
	if cached, ok := hasTypeParamCache.Load(typ); ok {
		return cached.(bool)
	}

	seen := map[types.Type]bool{}
	var f func(typ types.Type) bool
	f = func(typ types.Type) bool {
		if seen[typ] {
			return false
		}
		seen[typ] = true
		switch t := typ.(type) {
		case *types.Alias:
			return f(t.Underlying())
		case *types.Array:
			return f(t.Elem())
		case *types.Chan:
			return f(t.Elem())
		case *types.Map:
			return f(t.Key()) || f(t.Elem())
		case *types.Named:
			if t.TypeParams().Len() > 0 && t.TypeArgs().Len() == 0 {
				return true
			}
			return f(t.Underlying())
		case *types.Pointer:
			return f(t.Elem())
		case *types.Slice:
			return f(t.Elem())
		case *types.Struct:
			for i := 0; i < t.NumFields(); i++ {
				if f(t.Field(i).Type()) {
					return true
				}
			}
			return false
		case *types.Tuple:
			for i := 0; i < t.Len(); i++ {
				if f(t.At(i).Type()) {
					return true
				}
			}
			return false
		case *types.Signature:
			if t.Recv() != nil && f(t.Recv().Type()) {
				return true
			}
			return f(t.Params()) || f(t.Results())
		case *types.TypeParam:
			return true
		default:
			return false
		}
	}
	result := f(typ)
	hasTypeParamCache.Store(typ, result)
	return result
}

func createPackageName(pkg *types.Package) string {
	return encode(pkg.Path())
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
		return true
	case *ssa.Convert:
		if dstType, ok := t.Type().Underlying().(*types.Basic); ok && dstType.Kind() == types.String {
			return true
		}
		if _, ok := t.Type().Underlying().(*types.Slice); ok {
			return true
		}
		return false
	case *ssa.Defer, *ssa.Go, *ssa.MakeChan, *ssa.MakeClosure, *ssa.MakeInterface, *ssa.MakeMap, *ssa.MakeSlice, *ssa.MapUpdate, *ssa.RunDefers, *ssa.Select, *ssa.Send, *ssa.TypeAssert:
		return true
	case *ssa.Lookup:
		_, ok := t.X.Type().Underlying().(*types.Map)
		return ok
	case *ssa.Next:
		return true
	case *ssa.Slice:
		if dstType, ok := t.Type().Underlying().(*types.Basic); ok && dstType.Kind() == types.String {
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

func (ctx *Context) emitFunctionHeader(name string, end string) {
	fmt.Fprintf(ctx.stream, "FunctionObject %s (LightWeightThreadContext* ctx)%s\n", name, end)
}

// functionStorageClass returns the C storage-class keyword for the given
// function. Generic instances (and their closures/bound wrappers) get a
// private copy per translation unit under monomorphization, so they must be
// static to allow several packages to embed the same instantiation. The
// unused attribute keeps -Werror happy when a copy ends up referenced only
// through eliminated code paths.
func functionStorageClass(function *ssa.Function) string {
	if isInGenericInstanceSubtree(function) {
		return "static __attribute__((unused)) "
	}
	return ""
}

// emitFunctionDeclarationHeader emits a prototype for the given function,
// matching the storage class used by its definition.
func (ctx *Context) emitFunctionDeclarationHeader(function *ssa.Function, end string) {
	fmt.Fprintf(ctx.stream, "%sFunctionObject %s (LightWeightThreadContext* ctx)%s\n", functionStorageClass(function), createFunctionName(function), end)
}

// declarationStorageClass returns the storage class for a bare prototype.
// Static is only sound when this file also defines the function; references
// discovered inside stubbed bodies point at functions no file defines, and a
// static declaration for those would trip -Wunused-function.
func (ctx *Context) declarationStorageClass(function *ssa.Function) string {
	if functionStorageClass(function) == "" {
		return ""
	}
	for _, cached := range ctx.cachedFunctions {
		if cached == function {
			return functionStorageClass(function)
		}
	}
	return ""
}

func (ctx *Context) emitBoundFunctionFreeVarsDeclaration(fn *ssa.Function) {
	obj := fn.Object().(*types.Func)
	recvType := obj.Type().(*types.Signature).Recv().Type()
	fnName := createFunctionName(fn)
	fmt.Fprintf(ctx.stream, "typedef struct {\n")
	fmt.Fprintf(ctx.stream, "\t%s receiver; // %s\n", createTypeName(recvType), fn)
	fmt.Fprintf(ctx.stream, "} FreeVars_%s;\n", fnName)
}

func (ctx *Context) emitFunctionVariableStructure(function *ssa.Function) {
	signature := function.Signature
	if signature.Recv() != nil {
		receiverBoundFuncName := fmt.Sprintf("%s%s", createFunctionName(function), encode("$bound"))
		fmt.Fprintf(ctx.stream, "%sFunctionObject %s (LightWeightThreadContext* ctx);\n", functionStorageClass(function), receiverBoundFuncName)

		fmt.Fprintf(ctx.stream, "typedef struct {\n")
		fmt.Fprintf(ctx.stream, "\t%s receiver; // %s\n", createTypeName(signature.Recv().Type()), signature)
		fmt.Fprintf(ctx.stream, "} FreeVars_%s;\n", receiverBoundFuncName)

		receiverBoundSignatureName := createSignatureName(signature, true, false)
		fmt.Fprintf(ctx.stream, "typedef struct {\n")
		fmt.Fprintf(ctx.stream, "\tStackFrameCommon common;\n")
		fmt.Fprintf(ctx.stream, "\t%s signature;\n", receiverBoundSignatureName)
		fmt.Fprintf(ctx.stream, "} StackFrame_%s;\n", receiverBoundFuncName)

		receiverThunkFuncName := fmt.Sprintf("%s%s", createFunctionName(function), encode("$thunk"))
		fmt.Fprintf(ctx.stream, "%sFunctionObject %s (LightWeightThreadContext* ctx);\n", functionStorageClass(function), receiverThunkFuncName)
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

		ctx.traverseValue(function, func(value ssa.Value) {
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

	if hasFunctionValueMakeInterface(function) {
		// reflect.ValueOf wraps function values in interfaces via
		// gox5_interface_new, which copies the receiver through a pointer.
		// Passing the address of a block-scoped compound literal is invalid by
		// the time the runtime reads it (the machine-stack slot is dead and may
		// be reused), so keep a stable copy in the frame instead.
		fmt.Fprintf(ctx.stream, "\tFunctionObject makeInterfaceReceiver;\n")
	}

	for _, makeInterface := range globalValueMakeInterfaces(function) {
		// Same rationale as makeInterfaceReceiver: a MakeInterface whose source
		// is a package global would otherwise hand gox5_interface_new the
		// address of a block-scoped compound literal, which is invalid by the
		// time the runtime reads it. Keep a stable copy in the frame instead.
		fmt.Fprintf(ctx.stream, "\t%s makeInterfaceReceiver_%s;\n", createTypeName(makeInterface.X.Type()), createValueName(makeInterface))
	}

	fmt.Fprintf(ctx.stream, "} StackFrame_%s;\n", createFunctionName(function))
}

func hasFunctionValueMakeInterface(function *ssa.Function) bool {
	if function.Blocks == nil {
		return false
	}
	for _, basicBlock := range function.Blocks {
		for _, instr := range basicBlock.Instrs {
			if makeInterface, ok := instr.(*ssa.MakeInterface); ok {
				if _, ok := makeInterface.X.(*ssa.Function); ok {
					return true
				}
			}
		}
	}
	return false
}

func globalValueMakeInterfaces(function *ssa.Function) []*ssa.MakeInterface {
	var result []*ssa.MakeInterface
	if function.Blocks == nil {
		return result
	}
	for _, basicBlock := range function.Blocks {
		for _, instr := range basicBlock.Instrs {
			if makeInterface, ok := instr.(*ssa.MakeInterface); ok {
				if _, ok := makeInterface.X.(*ssa.Global); ok {
					result = append(result, makeInterface)
				}
			}
		}
	}
	return result
}

func (ctx *Context) emitFunctionDefinitionPrologue(storage string, functionName string, frameName string, hasFreeVariables bool) {
	fmt.Fprintf(ctx.stream, "%sFunctionObject %s (LightWeightThreadContext* ctx){\n", storage, functionName)
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

func (ctx *Context) emitPhiAssign(dest string, instr *ssa.Phi) {
	basicBlock := instr.Block()
	for i, edge := range instr.Edges {
		fmt.Fprintf(ctx.stream, "\tif (ctx->prev_func.func_ptr == %s) { %s = %s; } else\n",
			ctx.latestNameMap[basicBlock.Preds[i]], dest, createValueRelName(edge))
	}
	fmt.Fprintln(ctx.stream, "\t{ assert(false); }")
}

func (ctx *Context) emitBlockPhis(basicBlock *ssa.BasicBlock) {
	var phis []*ssa.Phi
	for _, instr := range basicBlock.Instrs {
		phi, ok := instr.(*ssa.Phi)
		if !ok {
			break
		}
		phis = append(phis, phi)
	}
	if len(phis) == 0 {
		return
	}
	for _, phi := range phis {
		tempName := createPhiTempName(phi)
		fmt.Fprintf(ctx.stream, "\t%s %s;\n", createTypeName(phi.Type()), tempName)
		ctx.emitPhiAssign(tempName, phi)
	}
	for _, phi := range phis {
		fmt.Fprintf(ctx.stream, "\t%s = %s;\n", createValueRelName(phi), createPhiTempName(phi))
	}
}

func createPhiTempName(phi *ssa.Phi) string {
	return encode(fmt.Sprintf("%s$phi_temp", createValueName(phi)))
}

func (ctx *Context) emitFunctionDefinition(function *ssa.Function) {
	if function.Pkg != nil && function.Pkg.Pkg.Name() == "runtime" && function.Name() == "init" { // ToDo
		ctx.emitFunctionHeader(createFunctionName(function), "{")
		fmt.Fprintf(ctx.stream, "\tassert(ctx->marker == 0xdeadbeef);\n")
		fmt.Fprintf(ctx.stream, "\tStackFrame_%s* frame = (void*)ctx->stack_pointer;\n", createFunctionName(function))
		fmt.Fprintf(ctx.stream, "\tctx->stack_pointer = frame->common.prev_stack_pointer;\n")
		fmt.Fprintf(ctx.stream, "\treturn frame->common.resume_func;\n")
		fmt.Fprintf(ctx.stream, "}\n")
		return
	}
	storage := functionStorageClass(function)
	fmt.Fprintf(ctx.stream, "%sFunctionObject %s (LightWeightThreadContext* ctx){\n", storage, createFunctionName(function))
	fmt.Fprintf(ctx.stream, "\tassert(ctx->marker == 0xdeadbeef);\n")
	fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createBasicBlockName(function.Blocks[0])))
	fmt.Fprintf(ctx.stream, "}\n")

	frameName := createFunctionName(function)
	hasFreeVariables := len(function.FreeVars) != 0
	for _, basicBlock := range function.Blocks {
		ctx.emitFunctionDefinitionPrologue(storage, createBasicBlockName(basicBlock), frameName, hasFreeVariables)

		ctx.emitBlockPhis(basicBlock)

		for _, instr := range basicBlock.Instrs {
			if _, ok := instr.(*ssa.Phi); ok {
				continue
			}
			ctx.emitInstruction(instr)

			if requireSwitchFunction(instr) {
				ctx.emitFunctionDefinitionEpilogue()
				ctx.emitFunctionDefinitionPrologue(storage, createInstructionName(instr), frameName, hasFreeVariables)
			}
		}

		ctx.emitFunctionDefinitionEpilogue()
	}

	ctx.emitReceiverBoundThunkGlue(function, storage)
}

// emitReceiverBoundThunkGlue emits the bodies of the synthetic $bound and
// $thunk wrappers that pair with a method definition. It must run for every
// emitted method, including ones whose own body is reduced to a stub.
func (ctx *Context) emitReceiverBoundThunkGlue(function *ssa.Function, storage string) {
	signature := function.Signature
	if signature.Recv() == nil {
		return
	}

	origFuncName := createFunctionName(function)
	boundFuncName := fmt.Sprintf("%s%s", origFuncName, encode("$bound"))
	resumeFuncName := fmt.Sprintf("%s_return", boundFuncName)
	ctx.emitFunctionDefinitionPrologue(storage, resumeFuncName, boundFuncName, true)
	fmt.Fprintf(ctx.stream, `
	assert(ctx->marker == 0xdeadbeef);
	ctx->stack_pointer = frame->common.prev_stack_pointer;
	return frame->common.resume_func;
`)
	ctx.emitFunctionDefinitionEpilogue()

	ctx.emitFunctionDefinitionPrologue(storage, boundFuncName, boundFuncName, true)
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

	thunkFuncName := fmt.Sprintf("%s%s", origFuncName, encode("$thunk"))
	thunkResumeFuncName := fmt.Sprintf("%s_return", thunkFuncName)
	ctx.emitFunctionDefinitionPrologue(storage, thunkResumeFuncName, origFuncName, false)
	fmt.Fprintf(ctx.stream, `
	assert(ctx->marker == 0xdeadbeef);
	ctx->stack_pointer = frame->common.prev_stack_pointer;
	return frame->common.resume_func;
`)
	ctx.emitFunctionDefinitionEpilogue()

	ctx.emitFunctionDefinitionPrologue(storage, thunkFuncName, origFuncName, false)
	nextFuncName = wrapInFunctionObject(origFuncName)
	signatureName = createSignatureName(signature, false, false)
	result = "*frame->signature.result_ptr"
	ctx.switchFunction(nextFuncName, signature, signatureName, result, thunkResumeFuncName, func() {
		for i := 0; i <= signature.Params().Len(); i++ {
			fmt.Fprintf(ctx.stream, "signature->param%d = frame->signature.param%d;\n", i, i)
		}
	})
	ctx.emitFunctionDefinitionEpilogue()
}

func (ctx *Context) retrieveOrderedTypes(pkg *ssa.Package, extraSeeds []types.Type) []types.Type {
	return ctx.orderTypes(func(procedure func(types.Type)) {
		ctx.traverseType(pkg, procedure)
		for _, typ := range extraSeeds {
			procedure(typ)
		}
	})
}

func (ctx *Context) orderTypes(seeds func(procedure func(types.Type))) []types.Type {
	pointerTypes := make([]types.Type, 0)
	nonPointerTypes := make([]types.Type, 0)

	foundTypeSet := make(map[string]struct{})
	var f func(typ types.Type)
	f = func(typ types.Type) {
		name := createTypeName(typ)
		if _, ok := foundTypeSet[name]; ok {
			return
		}

		isPointerType := false
		switch typ := typ.(type) {
		case *types.Alias:
			f(typ.Underlying())
			return

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
			isPointerType = true

		case *types.Signature: // do nothing

		case *types.Slice:
			f(typ.Elem())

		case *types.Tuple:
			for i := 0; i < typ.Len(); i++ {
				f(typ.At(i).Type())
			}

		case *types.TypeParam: // do nothing

		default:
			if typ.String() == "iter" {
				/// iterator of map or string
			} else {
				panic(fmt.Sprintf("not implemented: %s %T", typ, typ))
			}
		}

		foundTypeSet[name] = struct{}{}
		if isPointerType {
			pointerTypes = append(pointerTypes, typ)
		} else {
			nonPointerTypes = append(nonPointerTypes, typ)
		}
	}

	seeds(f)

	return append(pointerTypes, nonPointerTypes...)
}

func (ctx *Context) emitTypeDeclaration(typ types.Type) {
	name := createTypeName(typ)
	switch typ := typ.(type) {
	case *types.Array, *types.Chan, *types.Map, *types.Pointer, *types.Struct, *types.Tuple:
		fmt.Fprintf(ctx.stream, "typedef struct %s %s; // %s\n", name, name, typ)

	case *types.Basic, *types.Interface, *types.Signature, *types.TypeParam:
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

func (ctx *Context) emitTypeDefinition(typ types.Type) {
	name := createTypeName(typ)
	switch typ := typ.(type) {
	case *types.Array:
		fmt.Fprintf(ctx.stream, "struct %s { // %s\n", name, typ)
		fmt.Fprintf(ctx.stream, "\t%s raw[%d];\n", createTypeName(typ.Elem()), typ.Len())
		fmt.Fprintf(ctx.stream, "};\n")

	case *types.Basic, *types.Interface, *types.Named, *types.Signature, *types.TypeParam:
		// do nothing

	case *types.Chan:
		fmt.Fprintf(ctx.stream, "struct %s { ChannelObject raw; }; // %s\n", name, typ)

	case *types.Map:
		fmt.Fprintf(ctx.stream, "struct %s { MapObject raw; }; // %s\n", name, typ)

	case *types.Pointer:
		fmt.Fprintf(ctx.stream, "struct %s { // %s\n", name, typ)
		fmt.Fprintf(ctx.stream, "\t%s* raw;\n", createTypeName(typ.Elem()))
		fmt.Fprintf(ctx.stream, "};\n")

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
			fieldName := createFieldName(field, i)
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

func (ctx *Context) emitSignature(pkg *ssa.Package) {
	signatureNameSet := make(map[string]struct{})
	tryEmitSignatureDefinition := func(signature *types.Signature, signatureName string, makesReceiverBound bool, makesReceiverInterface bool) {
		_, ok := signatureNameSet[signatureName]
		if ok {
			return
		}
		signatureNameSet[signatureName] = struct{}{}

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

	ctx.traverseFunction(pkg, func(function *ssa.Function) {
		signature := function.Signature

		hasTypeParam := false
		for i := 0; i < signature.Params().Len(); i++ {
			if _, ok := signature.Params().At(i).Type().(*types.TypeParam); ok {
				hasTypeParam = true
				break
			}
		}
		for i := 0; i < signature.Results().Len(); i++ {
			if _, ok := signature.Results().At(i).Type().(*types.TypeParam); ok {
				hasTypeParam = true
				break
			}
		}
		if hasTypeParam {
			return
		}

		concreteSignatureName := createSignatureName(signature, false, false)
		tryEmitSignatureDefinition(signature, concreteSignatureName, false, false)
		if signature.Recv() != nil {
			abstractSignatureName := createSignatureName(signature, false, true)
			tryEmitSignatureDefinition(signature, abstractSignatureName, false, true)

			receiverBoundSignatureName := createSignatureName(signature, true, false)
			tryEmitSignatureDefinition(signature, receiverBoundSignatureName, true, true)
		}

		ctx.traverseCallCommon(function, func(callCommon *ssa.CallCommon) {
			signature := callCommon.Signature()
			signatureName := createSignatureName(signature, false, false)
			tryEmitSignatureDefinition(signature, signatureName, false, false)
		})
	})
}

func (ctx *Context) emitTypeInfoDeclaration(typ types.Type) {
	fmt.Fprintf(ctx.stream, "extern const TypeInfo %s;\n", createTypeIdName(typ))
}

func (ctx *Context) emitTypeInfoDefinition(typ types.Type) {
	if !ctx.markTypeDefinition("typeinfo", createTypeIdName(typ)) {
		return
	}
	interfaceTableName := fmt.Sprintf("interfaceTable_%s", createInterfaceTypeSymbolName(typ))
	numMethods := fmt.Sprintf("sizeof(%s.entries)/sizeof(%s.entries[0])", interfaceTableName, interfaceTableName)
	interfaceTable := fmt.Sprintf("&%s.entries[0]", interfaceTableName)

	fmt.Fprintf(ctx.stream, "const TypeInfo %s = {\n", createTypeIdName(typ))
	fmt.Fprintf(ctx.stream, ".name = (StringObject){.raw = \"%s\", .len = sizeof(\"%s\") - 1},\n", createTypeName(typ), createTypeName(typ))
	fmt.Fprintf(ctx.stream, ".num_methods = %s,\n", numMethods)
	fmt.Fprintf(ctx.stream, ".interface_table = %s,\n", interfaceTable)
	fmt.Fprintf(ctx.stream, ".is_equal = equal_%s,\n", createTypeName(typ))
	fmt.Fprintf(ctx.stream, ".hash = hash_%s,\n", createTypeName(typ))
	fmt.Fprintf(ctx.stream, ".size = sizeof(%s),\n", createTypeName(typ))
	fmt.Fprintf(ctx.stream, "};\n")
}

func (ctx *Context) emitConstant(cst *ssa.Const) {
	inner := "0"
	if !cst.IsNil() && cst.Value != nil {
		inner = cst.Value.String()
		if t, ok := cst.Type().Underlying().(*types.Basic); ok {
			switch t.Kind() {
			case types.Complex64, types.Complex128:
				inner = fmt.Sprintf("%g", cst.Complex128())
			case types.Float32, types.Float64:
				inner = fmt.Sprintf("%g", cst.Float64())
			case types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
				inner = fmt.Sprintf("%su", inner)
			case types.Int, types.Int64:
				inner = fmt.Sprintf("%slu", inner)
			case types.String, types.UntypedString:
				inner = "\""
				fullString := constant.StringVal(cst.Value)
				for i := 0; i < len(fullString); i++ {
					inner += fmt.Sprintf("\\x%02x", fullString[i])
				}
				inner += "\""
			case types.UnsafePointer:
				inner = fmt.Sprintf("(void*)%su", inner)
			}
		}
	}

	var value string
	if t, ok := cst.Type().Underlying().(*types.Interface); ok {
		value = fmt.Sprintf("(%s){%s}", createTypeName(t), inner)
	} else if cst.Value == nil {
		typeName := createTypeName(cst.Type())
		valueName := createValueName(cst)
		fmt.Fprintf(ctx.stream, "__attribute__((unused)) static const %s %s; // %s\n", typeName, valueName, cst)
		return
	} else {
		if basic, ok := cst.Type().Underlying().(*types.Basic); ok && (basic.Kind() == types.String || basic.Kind() == types.UntypedString) {
			value = fmt.Sprintf("(StringObject){.raw = %s, .len = sizeof(%s) - 1}", inner, inner)
		} else {
			value = wrapInObject(inner, cst.Type())
		}
	}

	typeName := createTypeName(cst.Type())
	valueName := createValueName(cst)
	fmt.Fprintf(ctx.stream, "__attribute__((unused)) static const %s %s = %s; // %s\n", typeName, valueName, value, cst)
}

func (ctx *Context) emitEqualFunctionDeclaration(typ types.Type) {
	typeName := createTypeName(typ)
	fmt.Fprintf(ctx.stream, "bool equal_%s(const %s* lhs, const %s* rhs); // %s\n", typeName, typeName, typeName, typ)
}

func (ctx *Context) emitEqualFunctionDefinition(typ types.Type) {
	typeName := createTypeName(typ)
	if !ctx.markTypeDefinition("equal", typeName) {
		return
	}
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
				body += "if (lhs->raw == rhs->raw) { return true; }\n"
				body += "if (lhs->len != rhs->len) { return false; }\n"
				body += "if (lhs->len == 0) { return true; }\n"
				body += "if (lhs->raw == NULL || rhs->raw == NULL) { return false; }\n"
				body += "return memcmp(lhs->raw, rhs->raw, lhs->len) == 0;\n"
			default:
				body += "return lhs->raw == rhs->raw;\n"
			}
		case *types.Interface:
			panic(typ)
		case *types.Map:
			body += "return equal_MapObject(&lhs->raw, &rhs->raw);\n"
		case *types.Struct:
			for i := 0; i < t.NumFields(); i++ {
				field := t.Field(i)
				if field.Name() == "_" {
					continue
				}
				name := createFieldName(field, i)
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
}

func (ctx *Context) emitHashFunctionDeclaration(typ types.Type) {
	typeName := createTypeName(typ)
	fmt.Fprintf(ctx.stream, "uintptr_t hash_%s(const %s* obj); // %s\n", typeName, typeName, typ)
}

func (ctx *Context) emitHashFunctionDefinition(typ types.Type) {
	typeName := createTypeName(typ)
	if !ctx.markTypeDefinition("hash", typeName) {
		return
	}
	underlyingType := typ.Underlying()
	var body = ""
	body += "\tassert(obj != NULL);\n"
	if typ == underlyingType {
		switch t := typ.(type) {
		case *types.Basic:
			switch t.Kind() {
			case types.Invalid:
				body += "assert(false); /// not implemented\n"
				body += "return 0;\n"
			case types.String:
				body += "uint64_t hash = UINT64_C(14695981039346656037); // FNV-1a 64-bit\n"
				body += "\n"
				body += "for (uintptr_t i = 0; i < obj->len; ++i) { hash ^= (unsigned char)obj->raw[i]; hash *= UINT64_C(1099511628211); }\n"
				body += "return hash;\n"
			default:
				body += "return (uintptr_t)obj->raw;\n"
			}
		case *types.Chan:
			body += "return (uintptr_t)obj->raw.raw;\n"
		case *types.Interface:
			panic(typ)
		case *types.Map:
			body += "assert(false); /// not implemented\n"
			body += "return 0;\n"
		case *types.Pointer:
			body += "return (uintptr_t)obj->raw;\n"
		case *types.Struct:
			body += "uintptr_t hash = 0;\n"
			for i := 0; i < t.NumFields(); i++ {
				field := t.Field(i)
				if field.Name() == "_" {
					continue
				}
				name := createFieldName(field, i)
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
}

func (ctx *Context) interfaceTableEntryIndexes(typ types.Type, allowSet map[string]struct{}) []int {
	methodSet := ctx.program.MethodSets.MethodSet(typ)
	entryIndexes := make([]int, 0)
	if _, ok := typ.Underlying().(*types.Interface); ok {
		for i := 0; i < methodSet.Len(); i++ {
			entryIndexes = append(entryIndexes, i)
		}
		return entryIndexes
	}
	if _, ok := allowSet[createTypeName(typ)]; ok {
		for i := 0; i < methodSet.Len(); i++ {
			function := ctx.program.MethodValue(methodSet.At(i))
			if function != nil && !isMethodFromSkippedPackage(function) {
				entryIndexes = append(entryIndexes, i)
			}
		}
	}
	return entryIndexes
}

func (ctx *Context) emitInterfaceTableDeclaration(typ types.Type, allowSet map[string]struct{}) {
	if !ctx.markTypeDefinition("itabledecl", createInterfaceTypeSymbolName(typ)) {
		return
	}
	entryIndexes := ctx.interfaceTableEntryIndexes(typ, allowSet)
	name := createInterfaceTypeSymbolName(typ)
	fmt.Fprintf(ctx.stream, "struct InterfaceTable_%s { InterfaceTableEntry entries[%d]; };\n", name, len(entryIndexes))
	fmt.Fprintf(ctx.stream, "extern struct InterfaceTable_%s interfaceTable_%s;\n", name, name)
}

func (ctx *Context) emitInterfaceTableDefinition(typ types.Type, allowSet map[string]struct{}) {
	if !ctx.markTypeDefinition("itable", createInterfaceTypeSymbolName(typ)) {
		return
	}
	entryIndexes := ctx.interfaceTableEntryIndexes(typ, allowSet)
	methodSet := ctx.program.MethodSets.MethodSet(typ)
	name := createInterfaceTypeSymbolName(typ)
	fmt.Fprintf(ctx.stream, "struct InterfaceTable_%s interfaceTable_%s = {{\n", name, name)
	for _, index := range entryIndexes {
		sel := methodSet.At(index)
		var methodName, method string
		if _, ok := typ.Underlying().(*types.Interface); ok {
			methodName = sel.Obj().Name()
			method = "(FunctionObject){.raw=NULL}"
		} else {
			function := ctx.program.MethodValue(sel)
			methodName = function.Name()
			method = wrapInFunctionObject(createFunctionName(function))
		}
		methodSignature := methodSignatureString(sel)
		fmt.Fprintf(ctx.stream, "\t{ .method_name = (StringObject){.raw = \"%s\", .len = sizeof(\"%s\") - 1}, .method = %s, .method_signature = (StringObject){.raw = \"%s\", .len = sizeof(\"%s\") - 1} },\n", methodName, methodName, method, methodSignature, methodSignature)
	}
	fmt.Fprintln(ctx.stream, "}};")
}

func methodSignatureString(sel *types.Selection) string {
	return signatureTypeString(sel.Obj().(*types.Func).Type())
}

func signatureTypeString(t types.Type) string {
	switch t := t.(type) {
	case *types.Alias:
		return signatureTypeString(t.Rhs())
	case *types.Named:
		if t.Obj().Pkg() == nil {
			return t.Obj().Name()
		}
		return fmt.Sprintf("%s.%s", t.Obj().Pkg().Path(), t.Obj().Name())
	case *types.Basic:
		return t.Name()
	case *types.Array:
		return fmt.Sprintf("[%d]%s", t.Len(), signatureTypeString(t.Elem()))
	case *types.Slice:
		return "[]" + signatureTypeString(t.Elem())
	case *types.Struct:
		var b strings.Builder
		b.WriteString("struct{")
		for i := 0; i < t.NumFields(); i++ {
			if i > 0 {
				b.WriteString("; ")
			}
			field := t.Field(i)
			if !field.Embedded() {
				b.WriteString(field.Name())
				b.WriteString(" ")
			}
			b.WriteString(signatureTypeString(field.Type()))
		}
		b.WriteString("}")
		return b.String()
	case *types.Pointer:
		return "*" + signatureTypeString(t.Elem())
	case *types.Tuple:
		var b strings.Builder
		b.WriteString("(")
		for i := 0; i < t.Len(); i++ {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(signatureTypeString(t.At(i).Type()))
		}
		b.WriteString(")")
		return b.String()
	case *types.Signature:
		var b strings.Builder
		b.WriteString("func(")
		if t.Variadic() {
			for i := 0; i < t.Params().Len()-1; i++ {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(signatureTypeString(t.Params().At(i).Type()))
			}
			if t.Params().Len() > 0 {
				b.WriteString(", ")
			}
			last := t.Params().At(t.Params().Len() - 1).Type()
			lastElem := last.Underlying().(*types.Slice).Elem()
			b.WriteString("...")
			b.WriteString(signatureTypeString(lastElem))
		} else {
			for i := 0; i < t.Params().Len(); i++ {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(signatureTypeString(t.Params().At(i).Type()))
			}
		}
		b.WriteString(")")
		if t.Results().Len() > 0 {
			b.WriteString(" (")
			for i := 0; i < t.Results().Len(); i++ {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(signatureTypeString(t.Results().At(i).Type()))
			}
			b.WriteString(")")
		}
		return b.String()
	case *types.Interface:
		return t.String()
	case *types.Map:
		return fmt.Sprintf("map[%s]%s", signatureTypeString(t.Key()), signatureTypeString(t.Elem()))
	case *types.Chan:
		switch t.Dir() {
		case types.SendOnly:
			return "chan<- " + signatureTypeString(t.Elem())
		case types.RecvOnly:
			return "<-chan " + signatureTypeString(t.Elem())
		default:
			return "chan " + signatureTypeString(t.Elem())
		}
	case *types.TypeParam:
		return t.String()
	default:
		return t.String()
	}
}

func isMethodFromSkippedPackage(function *ssa.Function) bool {
	if isFunctionBodySkipped(function) {
		return true
	}
	if function.Pkg != nil && isFunctionBodySkippedPackage(function.Pkg) {
		return true
	}
	if function.Signature != nil && function.Signature.Recv() != nil {
		if pkgPath := packagePathOfType(function.Signature.Recv().Type()); isFunctionBodySkippedPackagePath(pkgPath) {
			return true
		}
	}
	return false
}

func packagePathOfType(typ types.Type) string {
	switch typ := typ.(type) {
	case *types.Alias:
		return packagePathOfType(typ.Rhs())
	case *types.Array:
		return packagePathOfType(typ.Elem())
	case *types.Named:
		if obj := typ.Obj(); obj != nil && obj.Pkg() != nil {
			return obj.Pkg().Path()
		}
	case *types.Pointer:
		return packagePathOfType(typ.Elem())
	case *types.Slice:
		return packagePathOfType(typ.Elem())
	}
	return ""
}

func (ctx *Context) emitGlobalVariableDeclaration(gv *ssa.Global) {
	name := createValueName(gv)
	fmt.Fprintf(ctx.stream, "extern %s %s;\n", createTypeName(gv.Type().(*types.Pointer).Elem()), name)
}

func (ctx *Context) emitGlobalVariableDefinition(gv *ssa.Global) {
	name := createValueName(gv)
	fmt.Fprintf(ctx.stream, "%s %s;\n", createTypeName(gv.Type().(*types.Pointer).Elem()), name)
}

func (ctx *Context) emitRuntimeInfo() {
	mainPkg := findMainPackage(ctx.program)
	if mainPkg == nil {
		return
	}
	mainFunctionName := createFunctionName(mainPkg.Members["main"].(*ssa.Function))
	initFunctionName := createFunctionName(mainPkg.Members["init"].(*ssa.Function))

	ctx.emitFunctionHeader(mainFunctionName, ";")
	ctx.emitFunctionHeader(initFunctionName, ";")
	fmt.Fprintf(ctx.stream, `
UserFunction runtime_info_get_entry_point(void) {
	return (UserFunction) { .func_ptr = %s };
}

UserFunction runtime_info_get_init_point(void) {
	return (UserFunction) { .func_ptr = %s };
}
`, mainFunctionName, initFunctionName)
}

func findMainPackage(program *ssa.Program) *ssa.Package {
	for _, pkg := range allPackagesSorted(program) {
		if pkg.Pkg.Name() == "main" {
			return pkg
		}
	}
	return nil
}

func (ctx *Context) traverseValue(function *ssa.Function, procedure func(value ssa.Value)) {
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

func (ctx *Context) traverseCallCommon(function *ssa.Function, procedure func(callCommon *ssa.CallCommon)) {
	for _, basicBlock := range function.Blocks {
		for _, instruction := range basicBlock.Instrs {
			switch instr := instruction.(type) {
			case *ssa.Call:
				procedure(instr.Common())
			case *ssa.Defer:
				procedure(instr.Common())
			case *ssa.Go:
				procedure(instr.Common())
			}
		}
	}
}

func isAnyGenericInstance(fn *ssa.Function) bool {
	return fn.Pkg == nil && len(fn.TypeArgs()) > 0
}

// hasUnboundTypeArgs reports whether fn is a partially instantiated instance
// whose type arguments still contain type parameters. Such functions only
// appear inside generic origin bodies and must never be emitted.
func hasUnboundTypeArgs(fn *ssa.Function) bool {
	for _, typ := range fn.TypeArgs() {
		if hasTypeParameter(typ) {
			return true
		}
	}
	return false
}

func getCallCallee(common *ssa.CallCommon) *ssa.Function {
	if fn := common.StaticCallee(); fn != nil {
		return fn
	}
	if fn, ok := common.Value.(*ssa.Function); ok {
		return fn
	}
	return nil
}

// instanceCollector gathers generic instances reachable through function
// bodies. With monomorphization every file that uses an instance emits its
// own private copy, so instances are attributed by usage instead of origin.
type instanceCollector struct {
	seen                map[*ssa.Function]struct{}
	result              []*ssa.Function
	recordPlainWrappers bool
}

// walk records generic instances reachable from fn. Recursion only descends
// into instance subtrees and their synthetic $bound/$thunk wrappers; entering
// ordinary foreign functions is skipped so their private instance usage stays
// attributed to the file that defines them. Starters passed with force=true
// (the collecting file's own members) are entered unconditionally.
func (collector *instanceCollector) walk(fn *ssa.Function, force bool) {
	if fn == nil {
		return
	}
	if _, ok := collector.seen[fn]; ok {
		return
	}
	// Gate BEFORE remembering: a function rejected here must stay eligible
	// for a later forced visit, otherwise bodies reached first through
	// ordinary references would never be scanned.
	if !force && !isGenericInstanceSubtreeMember(fn) {
		return
	}
	if fn.Blocks != nil && isFunctionBodySkipped(fn) && !isAnyGenericInstance(fn) {
		return
	}
	collector.seen[fn] = struct{}{}

	// Bound-method and thunk synthetics of an instance ride along with the
	// instance definition itself, so only genuine roots are recorded.
	if isAnyGenericInstance(fn) && !strings.HasSuffix(fn.Name(), "$bound") && !strings.HasSuffix(fn.Name(), "$thunk") && !hasUnboundTypeArgs(fn) {
		collector.result = append(collector.result, fn)
	}
	// Wrappers of generic-origin methods keep parameterized signatures and
	// cannot be emitted anywhere. Methods from emitted packages define their
	// own wrappers, so only wrappers of body-skipped targets belong here.
	if collector.recordPlainWrappers && isPlainSyntheticWrapper(fn) &&
		fn.TypeParams().Len() == 0 && !hasTypeParamInSignature(fn.Signature) {
		target := wrapperTargetFunction(fn)
		if target != nil && isFunctionBodySkipped(target) {
			collector.result = append(collector.result, fn)
		}
	}

	if fn.Blocks == nil {
		return
	}

	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			for _, operand := range instr.Operands(nil) {
				if ref, ok := (*operand).(*ssa.Function); ok {
					collector.walk(ref, false)
				}
			}
			var callee *ssa.Function
			switch instr := instr.(type) {
			case *ssa.Call:
				callee = getCallCallee(instr.Common())
			case *ssa.Defer:
				callee = getCallCallee(instr.Common())
			case *ssa.Go:
				callee = getCallCallee(instr.Common())
			}
			if callee != nil {
				collector.walk(callee, false)
			}
		}
	}

	for _, anon := range fn.AnonFuncs {
		collector.walk(anon, force)
	}
}

func (collector *instanceCollector) sortedResult() []*ssa.Function {
	sort.Slice(collector.result, func(i, j int) bool {
		return collector.result[i].RelString(nil) < collector.result[j].RelString(nil)
	})
	return collector.result
}

// collectInstances appends to ctx.extraFunctions every generic instance used
// by pkg's own code, directly or transitively.
func (ctx *Context) collectInstances(pkg *ssa.Package) {
	collector := &instanceCollector{seen: map[*ssa.Function]struct{}{}}
	ctx.appendReachableInstances(pkg, collector)
	ctx.extraFunctions = append(ctx.extraFunctions, collector.sortedResult()...)
}

func (ctx *Context) appendReachableInstances(pkg *ssa.Package, collector *instanceCollector) {
	for _, member := range sortedPackageMembers(pkg) {
		switch member := member.(type) {
		case *ssa.Function:
			collector.walk(member, true)
		case *ssa.Type:
			walkTypeMethods(ctx.program, member.Type(), func(fn *ssa.Function) { collector.walk(fn, true) })
			walkTypeMethods(ctx.program, types.NewPointer(member.Type()), func(fn *ssa.Function) { collector.walk(fn, true) })
		}
	}
}

// collectSharedInstances appends to ctx.extraFunctions every generic instance
// referenced from shared_definition.c itself. Interface method tables are
// defined only there, so their concrete methods seed the set; the rest comes
// from transitive usage inside the seeded instances.
func (ctx *Context) collectSharedInstances() {
	collector := &instanceCollector{seen: map[*ssa.Function]struct{}{}, recordPlainWrappers: true}
	allowSet := ctx.buildAllowSet(nil)

	seed := func(typ types.Type) {
		methodSet := ctx.program.MethodSets.MethodSet(typ)
		for _, index := range ctx.interfaceTableEntryIndexes(typ, allowSet) {
			function := ctx.program.MethodValue(methodSet.At(index))
			if function != nil {
				collector.walk(function, isGenericInstanceSubtreeMember(function))
			}
		}
	}

	ctx.traverseBasicType(seed)
	for _, pkg := range allPackagesSorted(ctx.program) {
		for _, member := range sortedPackageMembers(pkg) {
			switch member := member.(type) {
			case *ssa.Type:
				seed(member.Type())
				seed(types.NewPointer(member.Type()))
			case *ssa.Function:
				collector.walk(member, true)
			}
		}
	}

	ctx.extraFunctions = append(ctx.extraFunctions, collector.sortedResult()...)
}

func receiverPackage(function *ssa.Function) *types.Package {
	if function.Signature == nil || function.Signature.Recv() == nil {
		return nil
	}
	t := function.Signature.Recv().Type()
	for {
		switch u := t.(type) {
		case *types.Pointer:
			t = u.Elem()
		case *types.Alias:
			t = u.Rhs()
		case *types.Named:
			return t.(*types.Named).Obj().Pkg()
		default:
			return nil
		}
	}
}

func walkTypeMethods(program *ssa.Program, t types.Type, walk func(fn *ssa.Function)) {
	methodSet := program.MethodSets.MethodSet(t)
	for i := 0; i < methodSet.Len(); i++ {
		if function := program.MethodValue(methodSet.At(i)); function != nil {
			walk(function)
		}
	}
}

func sortedPackageMembers(pkg *ssa.Package) []ssa.Member {
	mp := map[string]ssa.Member{}
	for _, member := range pkg.Members {
		mp[member.RelString(nil)] = member
	}
	keys := []string{}
	for key := range mp {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	members := []ssa.Member{}
	for _, key := range keys {
		members = append(members, mp[key])
	}
	return members
}

func (ctx *Context) traverseFunction(pkg *ssa.Package, procedure func(function *ssa.Function)) {
	if ctx.cachedFunctions == nil {
		visited := make(map[*ssa.Function]struct{})
		seenNames := make(map[string]struct{})
		var f func(function *ssa.Function, force bool)
		f = func(function *ssa.Function, force bool) {
			if function == nil {
				return
			}
			if _, ok := visited[function]; ok {
				return
			}
			visited[function] = struct{}{}
			if pkg != nil && !force {
				owner := ""
				if function.Pkg != nil {
					owner = function.Pkg.Pkg.Path()
				}
				if rp := receiverPackage(function); rp != nil {
					owner = rp.Path()
				}
				if owner != "" && owner != pkg.Pkg.Path() {
					return
				}
			}
			if function.TypeParams().Len() > 0 && len(function.TypeArgs()) == 0 {
				return
			}
			if hasTypeParamInSignature(function.Signature) {
				return
			}
			name := createFunctionName(function)
			if _, ok := seenNames[name]; ok {
				return
			}
			seenNames[name] = struct{}{}
			ctx.cachedFunctions = append(ctx.cachedFunctions, function)
			for _, anonFunc := range function.AnonFuncs {
				f(anonFunc, force)
			}
		}

		g := func(t types.Type) {
			methodSet := ctx.program.MethodSets.MethodSet(t)
			for i := 0; i < methodSet.Len(); i++ {
				function := ctx.program.MethodValue(methodSet.At(i))
				if function == nil {
					continue
				}
				f(function, false)
			}
		}

		ctx.traversePackageMember(pkg, func(member ssa.Member) {
			switch member := member.(type) {
			case *ssa.Function:
				f(member, false)
			case *ssa.Type:
				t := member.Type()
				g(t)
				g(types.NewPointer(t))
			}
		})

		for _, fn := range ctx.extraFunctions {
			// Instances are placed in this file by design; never let the
			// owner filter reassign them to their origin package.
			f(fn, true)
		}
	}

	for _, function := range ctx.cachedFunctions {
		procedure(function)
	}
}

func (ctx *Context) traverseBasicType(procedure func(typ types.Type)) {
	for _, typ := range types.Typ {
		switch typ.Kind() {
		case types.Invalid, types.UntypedBool, types.UntypedComplex, types.UntypedFloat, types.UntypedInt, types.UntypedNil, types.UntypedRune, types.UntypedString:
			continue
		}
		procedure(typ)
	}
}

func (ctx *Context) traverseType(pkg *ssa.Package, procedure func(typ types.Type)) {
	foundTypeSet := make(map[string]struct{})
	var f func(typ types.Type)
	f = func(typ types.Type) {
		if _, ok := typ.(*types.Alias); ok {
			f(typ.Underlying())
			return
		}
		if hasTypeParameter(typ) {
			return
		}
		name := createTypeName(typ)
		_, ok := foundTypeSet[name]
		if ok {
			return
		}
		foundTypeSet[name] = struct{}{}

		switch typ := typ.(type) {
		case *types.Array:
			f(typ.Elem())

		case *types.Basic:
			// handled in traverseBasicType
			return

		case *types.Chan:
			f(typ.Elem())

		case *types.Interface, *types.Signature, *types.TypeParam:
			// do nothing

		case *types.Map:
			f(typ.Key())
			f(typ.Elem())

		case *types.Named:
			f(typ.Underlying())

		case *types.Pointer:
			f(typ.Elem())

		case *types.Slice:
			f(typ.Elem())

		case *types.Struct:
			for i := 0; i < typ.NumFields(); i++ {
				f(typ.Field(i).Type())
			}

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

	ctx.traversePackageMember(pkg, func(member ssa.Member) {
		switch member := member.(type) {
		case *ssa.Global:
			f(member.Type())
		case *ssa.Type:
			f(member.Type())
		}
	})

	ctx.traverseFunction(pkg, func(function *ssa.Function) {
		sig := function.Signature

		hasTypeParam := false
		for i := 0; i < sig.Params().Len(); i++ {
			if _, ok := sig.Params().At(i).Type().(*types.TypeParam); ok {
				hasTypeParam = true
				break
			}
		}
		for i := 0; i < sig.Results().Len(); i++ {
			if _, ok := sig.Results().At(i).Type().(*types.TypeParam); ok {
				hasTypeParam = true
				break
			}
		}
		if hasTypeParam {
			return
		}

		if sig.Recv() != nil {
			f(sig.Recv().Type())
		}
		f(sig.Params())
		f(sig.Results())

		if function.Blocks == nil {
			return
		}

		ctx.traverseValue(function, func(value ssa.Value) {
			typ := value.Type()
			if _, ok := typ.(*types.TypeParam); ok {
				return
			}
			if sig, ok := typ.(*types.Signature); ok {
				hasTypeParam := false
				for i := 0; i < sig.Params().Len(); i++ {
					if _, ok := sig.Params().At(i).Type().(*types.TypeParam); ok {
						hasTypeParam = true
						break
					}
				}
				if hasTypeParam {
					return
				}
			}
			f(typ)
		})

		ctx.traverseCallCommon(function, func(callCommon *ssa.CallCommon) {
			sig := callCommon.Signature()
			if sig == nil {
				return
			}
			if sig.Recv() != nil {
				f(sig.Recv().Type())
			}
			f(sig.Params())
			f(sig.Results())
		})
	})
}

func (ctx *Context) traversePackageMember(pkg *ssa.Package, procedure func(member ssa.Member)) {
	if ctx.orderedPackageMembers == nil {
		mp := map[string]ssa.Member{}
		for _, pkg2 := range allPackagesSorted(ctx.program) {
			if pkg != nil && pkg != pkg2 {
				continue
			}
			for _, member := range pkg2.Members {
				mp[member.RelString(nil)] = member
			}
		}

		keys := []string{}
		for key := range mp {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		members := []ssa.Member{}
		for _, key := range keys {
			members = append(members, mp[key])
		}

		ctx.orderedPackageMembers = members
	}

	for _, member := range ctx.orderedPackageMembers {
		procedure(member)
	}
}

func (ctx *Context) emitCommon() {
	fmt.Fprintln(ctx.stream, `#include "predefined.h"`)
}

func (ctx *Context) emitTypeDeclarationAndDefinition(pkg *ssa.Package, extraTypeSeeds []types.Type) {
	orderedTypes := ctx.retrieveOrderedTypes(pkg, extraTypeSeeds)
	if len(extraTypeSeeds) > 0 {
		ctx.instanceOrderedTypes = ctx.orderTypes(func(procedure func(types.Type)) {
			for _, typ := range extraTypeSeeds {
				procedure(typ)
			}
		})
	} else {
		ctx.instanceOrderedTypes = nil
	}
	for _, typ := range orderedTypes {
		ctx.emitTypeDeclaration(typ)
	}
	for _, typ := range orderedTypes {
		ctx.emitTypeDefinition(typ)
	}
}

func (ctx *Context) buildAllowSet(pkg *ssa.Package) map[string]struct{} {
	allowSet := make(map[string]struct{})
	ctx.traverseBasicType(func(typ types.Type) {
		allowSet[createTypeName(typ)] = struct{}{}
	})
	ctx.traversePackageMember(pkg, func(member ssa.Member) {
		if typ, ok := member.(*ssa.Type); ok {
			t := typ.Type()
			allowSet[createTypeName(t)] = struct{}{}
			allowSet[createTypeName(types.NewPointer(t))] = struct{}{}
		}
	})
	return allowSet
}

func (ctx *Context) emitInterfaceDataDeclaration(pkg *ssa.Package) {
	allowSet := ctx.buildAllowSet(pkg)

	fmt.Fprintf(ctx.stream, `
bool equal_MapObject(const MapObject* lhs, const MapObject* rhs);
bool equal_Interface(const Interface* lhs, const Interface* rhs);
uintptr_t hash_Interface(const Interface* obj);
extern struct InterfaceTable_Interface interfaceTable_Interface;
extern const TypeInfo runtime_info_type_Interface;
`)

	ctx.traverseBasicType(func(typ types.Type) {
		ctx.emitEqualFunctionDeclaration(typ)
		ctx.emitHashFunctionDeclaration(typ)
		ctx.emitInterfaceTableDeclaration(typ, allowSet)
		ctx.emitTypeInfoDeclaration(typ)
	})
	ctx.traverseType(pkg, func(typ types.Type) {
		if _, ok := typ.(*types.Interface); !ok {
			ctx.emitEqualFunctionDeclaration(typ)
			ctx.emitHashFunctionDeclaration(typ)
		}
		ctx.emitInterfaceTableDeclaration(typ, allowSet)
		ctx.emitTypeInfoDeclaration(typ)
	})
	for _, typ := range sortedAssertedInterfaceTypes(ctx.assertedInterfaceTypes) {
		if ctx.visitedInterfaceNames != nil && ctx.visitedInterfaceNames[createInterfaceTypeSymbolName(typ)] {
			continue
		}
		ctx.emitInterfaceTableDeclaration(typ, allowSet)
		ctx.emitTypeInfoDeclaration(typ)
	}
	for _, typ := range sortedInstantiatedNamedTypes(ctx.instantiatedNamedTypes) {
		ctx.emitTypeInfoDeclaration(typ)
	}
	for _, typ := range ctx.instanceOrderedTypes {
		if _, ok := typ.(*types.Interface); !ok {
			ctx.emitEqualFunctionDeclaration(typ)
			ctx.emitHashFunctionDeclaration(typ)
		}
		ctx.emitInterfaceTableDeclaration(typ, allowSet)
		ctx.emitTypeInfoDeclaration(typ)
	}
}

func (ctx *Context) emitInterfaceDataDefinition() {
	allowSet := ctx.buildAllowSet(nil)

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

uintptr_t hash_Interface(const Interface* obj) {
	if (obj->type_id.info == NULL) {
		return 0;
	}
	if (obj->receiver == NULL) {
		return 0;
	}
	uintptr_t (*f)(const void*) = obj->type_id.info->hash;
	return f(obj->receiver);
}
`)

	ctx.traverseBasicType(func(typ types.Type) {
		ctx.emitEqualFunctionDefinition(typ)
		ctx.emitHashFunctionDefinition(typ)
		ctx.emitInterfaceTableDefinition(typ, allowSet)
		ctx.emitTypeInfoDefinition(typ)
	})
	ctx.traverseType(nil, func(typ types.Type) {
		if _, ok := typ.(*types.Interface); !ok {
			ctx.emitEqualFunctionDefinition(typ)
			ctx.emitHashFunctionDefinition(typ)
		}
		ctx.emitInterfaceTableDefinition(typ, allowSet)
		ctx.emitTypeInfoDefinition(typ)
	})
	for _, typ := range sortedAssertedInterfaceTypes(ctx.assertedInterfaceTypes) {
		if ctx.visitedInterfaceNames != nil && ctx.visitedInterfaceNames[createInterfaceTypeSymbolName(typ)] {
			continue
		}
		ctx.emitInterfaceTableDefinition(typ, allowSet)
		ctx.emitTypeInfoDefinition(typ)
	}
	for _, typ := range ctx.instanceOrderedTypes {
		if _, ok := typ.(*types.Interface); !ok {
			ctx.emitEqualFunctionDefinition(typ)
			ctx.emitHashFunctionDefinition(typ)
		}
		ctx.emitInterfaceTableDefinition(typ, allowSet)
		ctx.emitTypeInfoDefinition(typ)
	}
}

func (ctx *Context) emitPackage(pkg *ssa.Package) {
	ctx.emitCommon()

	ctx.emitTypeDeclarationAndDefinition(pkg, nil)
	ctx.emitInterfaceDataDeclaration(pkg)

	ctx.emitSignature(pkg)

	ctx.traverseFunction(pkg, func(function *ssa.Function) {
		ctx.emitFunctionDeclarationHeader(function, ";")
		ctx.emitFunctionVariableStructure(function)
	})

	ctx.traverseFunction(pkg, func(function *ssa.Function) {
		ctx.traverseCallCommon(function, func(callCommon *ssa.CallCommon) {
			if f, ok := callCommon.Value.(*ssa.Function); ok {
				fmt.Fprintf(ctx.stream, "%sFunctionObject %s (LightWeightThreadContext* ctx);\n", ctx.declarationStorageClass(f), createFunctionName(f))
			}
		})
	})

	functionPkg := func(f *ssa.Function) *types.Package {
		if f.Pkg != nil {
			return f.Pkg.Pkg
		}
		if obj := f.Object(); obj != nil {
			return obj.Pkg()
		}
		return nil
	}

	emittedBoundFunctionFreeVars := make(map[string]struct{})
	ctx.traverseFunction(pkg, func(function *ssa.Function) {
		ctx.traverseValue(function, func(value ssa.Value) {
			f, ok := value.(*ssa.Function)
			if !ok {
				return
			}
			if isInGenericInstanceSubtree(f) {
				return
			}
			if strings.HasSuffix(f.Name(), "$bound") || strings.HasSuffix(f.Name(), "$thunk") {
				if target := wrapperTargetFunction(f); target != nil && len(target.TypeArgs()) > 0 {
					return
				}
			}
			if functionPkg(f) == pkg.Pkg {
				return
			}
			ctx.emitFunctionDeclarationHeader(f, ";")
			if strings.HasSuffix(f.Name(), "$bound") {
				fnName := createFunctionName(f)
				if _, ok := emittedBoundFunctionFreeVars[fnName]; ok {
					return
				}
				emittedBoundFunctionFreeVars[fnName] = struct{}{}
				ctx.emitBoundFunctionFreeVarsDeclaration(f)
			}
		})
	})

	ctx.traverseFunction(pkg, func(function *ssa.Function) {
		ctx.traverseValue(function, func(value ssa.Value) {
			if gv, ok := value.(*ssa.Global); ok {
				ctx.emitGlobalVariableDeclaration(gv)
			}
		})
	})

	ctx.traversePackageMember(pkg, func(member ssa.Member) {
		if global, ok := member.(*ssa.Global); ok {
			ctx.emitGlobalVariableDefinition(global)
		}
	})

	ctx.traverseFunction(pkg, func(function *ssa.Function) {
		if function.Blocks == nil {
			return
		}
		for _, basicBlock := range function.Blocks {
			for _, instr := range basicBlock.Instrs {
				if deferInstr, ok := instr.(*ssa.Defer); ok {
					callCommon := deferInstr.Common()
					if callCommon.Method == nil {
						if builtin, ok := callCommon.Value.(*ssa.Builtin); ok && (builtin.Name() == "print" || builtin.Name() == "println") {
							ctx.emitBuiltinPrintWrapper(builtin.Name(), callCommon, deferInstr)
						}
					}
				}
			}
		}
	})

	if ctx.builtinPrintWrapperBuf.Len() > 0 {
		fmt.Fprintln(ctx.stream)
		fmt.Fprintln(ctx.stream, "// Deferred builtin wrappers")
		fmt.Fprintln(ctx.stream, ctx.builtinPrintWrapperBuf.String())
	}

	foundConstValueSet := make(map[string]struct{})
	ctx.traverseFunction(pkg, func(function *ssa.Function) {
		if function.Blocks == nil {
			if function.Pkg == nil {
				fmt.Fprintf(ctx.stream, "%sFunctionObject %s(LightWeightThreadContext* ctx){ (void)ctx; assert(false); return (FunctionObject){NULL}; }\n", functionStorageClass(function), createFunctionName(function))
			}
			return
		}
		if isFunctionBodySkipped(function) {
			fmt.Fprintf(ctx.stream, "%sFunctionObject %s(LightWeightThreadContext* ctx){ (void)ctx; assert(false); return (FunctionObject){NULL}; }\n", functionStorageClass(function), createFunctionName(function))
			ctx.emitReceiverBoundThunkGlue(function, functionStorageClass(function))
			return
		}
		ctx.traverseValue(function, func(value ssa.Value) {
			if cst, ok := value.(*ssa.Const); ok {
				valueName := createValueName(cst)
				if _, ok := foundConstValueSet[valueName]; ok {
					return
				}
				foundConstValueSet[valueName] = struct{}{}
				ctx.emitConstant(cst)
			}
		})
		for _, basicBlock := range function.Blocks {
			name := createBasicBlockName(basicBlock)
			fmt.Fprintf(ctx.stream, "%sFunctionObject %s (LightWeightThreadContext* ctx);\n", functionStorageClass(function), name)
			ctx.latestNameMap[basicBlock] = name
			for _, instr := range basicBlock.Instrs {
				if requireSwitchFunction(instr) {
					continuationName := createInstructionName(instr)
					fmt.Fprintf(ctx.stream, "%sFunctionObject %s (LightWeightThreadContext* ctx);\n", functionStorageClass(function), continuationName)
					ctx.latestNameMap[basicBlock] = continuationName
				}
			}
		}
		ctx.emitFunctionDefinition(function)
	})

	if pkg.Pkg.Path() == "os" {
		if fileObj := pkg.Pkg.Scope().Lookup("File"); fileObj != nil {
			if fileInnerObj := pkg.Pkg.Scope().Lookup("file"); fileInnerObj != nil {
				fileStructName := createTypeName(fileObj.Type())
				fileInnerStructName := createTypeName(fileInnerObj.Type())
				streams := []struct {
					global string
					name   string
					fd     int
				}{
					{"gv_24_Stdin_24_os", "/dev/stdin", 0},
					{"gv_24_Stdout_24_os", "/dev/stdout", 1},
					{"gv_24_Stderr_24_os", "/dev/stderr", 2},
				}
				fmt.Fprintf(ctx.stream, "\n// Custom C initialization of os.Stdin/Stdout/Stderr at program start\n")
				for i := range streams {
					inner := fmt.Sprintf("gox5_os_inner_std_%d", i)
					fileVar := fmt.Sprintf("gox5_os_file_std_%d", i)
					fmt.Fprintf(ctx.stream, "static %s %s;\n", fileInnerStructName, inner)
					fmt.Fprintf(ctx.stream, "static %s %s;\n", fileStructName, fileVar)
				}
				fmt.Fprintf(ctx.stream, "__attribute__((constructor)) static void gox5_init_os_std_streams(void) {\n")
				fmt.Fprintf(ctx.stream, "\tgv_24_Stdin_24_syscall.raw = 0;\n")
				fmt.Fprintf(ctx.stream, "\tgv_24_Stdout_24_syscall.raw = 1;\n")
				fmt.Fprintf(ctx.stream, "\tgv_24_Stderr_24_syscall.raw = 2;\n")

				for i, st := range streams {
					inner := fmt.Sprintf("gox5_os_inner_std_%d", i)
					fileVar := fmt.Sprintf("gox5_os_file_std_%d", i)
					fmt.Fprintf(ctx.stream, "\t%s.file.raw = &%s;\n", fileVar, inner)
					fmt.Fprintf(ctx.stream, "\t%s.pfd.Sysfd = (IntObject){.raw = %d};\n", inner, st.fd)
					fmt.Fprintf(ctx.stream, "\t%s.name = (StringObject){.raw = \"%s\", .len = sizeof(\"%s\") - 1};\n", inner, st.name, st.name)
					fmt.Fprintf(ctx.stream, "\t%s.raw = &%s;\n", st.global, fileVar)
				}
				fmt.Fprintf(ctx.stream, "}\n")
			}
		}
	}
}

func allPackagesSorted(program *ssa.Program) []*ssa.Package {
	pkgs := program.AllPackages()
	sort.Slice(pkgs, func(i, j int) bool {
		return createPackageName(pkgs[i].Pkg) < createPackageName(pkgs[j].Pkg)
	})
	return pkgs
}

func generateMakefile(makefile *os.File, program *ssa.Program, buildDirname string, cacheDirname string, cachedPackages map[string]bool) {
	cgenDir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	buildDirAbs, err := filepath.Abs(buildDirname)
	if err != nil {
		panic(err)
	}
	relToCgen, err := filepath.Rel(buildDirAbs, cgenDir)
	if err != nil {
		panic(err)
	}
	relToTarget, err := filepath.Rel(buildDirAbs, filepath.Join(cgenDir, "..", "target"))
	if err != nil {
		panic(err)
	}
	cacheDirAbs, err := filepath.Abs(cacheDirname)
	if err != nil {
		panic(err)
	}
	relToCache, err := filepath.Rel(buildDirAbs, cacheDirAbs)
	if err != nil {
		panic(err)
	}

	cCompiler := "gcc"
	cCompilerWrapper := "ccache"
	cc := fmt.Sprintf("$(shell command -v %s >/dev/null 2>&1 && echo %s %s || echo %s)", cCompilerWrapper, cCompilerWrapper, cCompiler, cCompiler)

	cflags := []string{
		"-Wall", "-Wextra", "-Werror", "-std=c11", "-O2",
		"-fstrict-aliasing", "-Wstrict-aliasing",
	}
	cflagsDebug := []string{
		"-Wall", "-Wextra", "-Werror", "-std=c11",
		"-g", "-O1", "-fno-var-tracking-assignments",
		"-fsanitize=undefined", "-fsanitize=address",
		"-fno-omit-frame-pointer", "-fno-sanitize-recover=all",
		"-fstrict-aliasing", "-Wstrict-aliasing",
	}
	ldflags := []string{
		"-lpthread", "-ldl", "-lm",
	}
	ldflagsDebug := []string{
		"-fsanitize=undefined", "-fsanitize=address", "-fno-sanitize-recover=all",
		"-lpthread", "-ldl", "-lm",
	}
	libsRelease := filepath.Join(relToTarget, "release", "libgogogogogo.a")
	libsDebug := filepath.Join(relToTarget, "debug", "libgogogogogo.a")

	fmt.Fprintf(makefile, "CC = %s\n", cc)
	fmt.Fprintf(makefile, "CFLAGS = %s\n", strings.Join(cflags, " "))
	fmt.Fprintf(makefile, "CFLAGS_DEBUG = %s\n", strings.Join(cflagsDebug, " "))
	fmt.Fprintf(makefile, "LDFLAGS = %s\n", strings.Join(ldflags, " "))
	fmt.Fprintf(makefile, "LDFLAGS_DEBUG = %s\n", strings.Join(ldflagsDebug, " "))
	fmt.Fprintf(makefile, "LIBS_RELEASE = %s\n", libsRelease)
	fmt.Fprintf(makefile, "LIBS_DEBUG = %s\n", libsDebug)

	objsRelease := []string{}
	objsDebug := []string{}

	cFileRule := func(outputName string, extraDeps ...string) {
		objRelease := fmt.Sprintf("%s.o", outputName)
		objDebug := fmt.Sprintf("%s.debug.o", outputName)
		deps := "predefined.h " + outputName
		for _, d := range extraDeps {
			deps += " " + d
		}
		fmt.Fprintf(makefile, "%s: %s\n", objRelease, deps)
		fmt.Fprintf(makefile, "\t@$(CC) $(CFLAGS) -c -o %s %s\n", objRelease, outputName)
		fmt.Fprintf(makefile, "%s: %s\n", objDebug, deps)
		fmt.Fprintf(makefile, "\t@$(CC) $(CFLAGS_DEBUG) -c -o %s %s\n", objDebug, outputName)
		objsRelease = append(objsRelease, objRelease)
		objsDebug = append(objsDebug, objDebug)
	}

	cFileRule("shared_definition.c")
	for _, pkg := range allPackagesSorted(program) {
		if isFunctionBodySkippedPackage(pkg) {
			continue
		}
		outputName := fmt.Sprintf("package_%s.c", createPackageName(pkg.Pkg))
		if cachedPackages[createPackageName(pkg.Pkg)] {
			fmt.Fprintf(makefile, "%s:\n", outputName)
			fmt.Fprintf(makefile, "\t@ln -sf %s/%s %s\n", relToCache, outputName, outputName)
		}
		cFileRule(outputName, "shared_definition.c")
	}

	fmt.Fprintf(makefile, "\n")
	fmt.Fprintf(makefile, "predefined.h:\n")
	fmt.Fprintf(makefile, "\t@ln -sf %s/predefined.h predefined.h\n", relToCgen)
	fmt.Fprintf(makefile, "\n")
	fmt.Fprintf(makefile, "all: bin.exe\n")
	fmt.Fprintf(makefile, "\n")
	fmt.Fprintf(makefile, "bin.exe: %s\n", strings.Join(objsRelease, " "))
	fmt.Fprintf(makefile, "\t@$(CC) -o bin.exe %s $(LIBS_RELEASE) $(LDFLAGS)\n", strings.Join(objsRelease, " "))
	fmt.Fprintf(makefile, "\n")
	fmt.Fprintf(makefile, "bin-debug-user.exe: %s\n", strings.Join(objsDebug, " "))
	fmt.Fprintf(makefile, "\t@$(CC) -o bin-debug-user.exe %s $(LIBS_RELEASE) $(LDFLAGS_DEBUG)\n", strings.Join(objsDebug, " "))
	fmt.Fprintf(makefile, "\n")
	fmt.Fprintf(makefile, "bin-debug-runtime.exe: %s\n", strings.Join(objsRelease, " "))
	fmt.Fprintf(makefile, "\t@$(CC) -o bin-debug-runtime.exe %s $(LIBS_DEBUG) $(LDFLAGS)\n", strings.Join(objsRelease, " "))
	fmt.Fprintf(makefile, "\n")
	fmt.Fprintf(makefile, "bin-debug-user-debug-runtime.exe: %s\n", strings.Join(objsDebug, " "))
	fmt.Fprintf(makefile, "\t@$(CC) -o bin-debug-user-debug-runtime.exe %s $(LIBS_DEBUG) $(LDFLAGS_DEBUG)\n", strings.Join(objsDebug, " "))
}

func sortedAssertedInterfaceTypes(types_ map[string]types.Type) []types.Type {
	names := make([]string, 0, len(types_))
	for name := range types_ {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]types.Type, 0, len(names))
	for _, name := range names {
		result = append(result, types_[name])
	}
	return result
}

// collectInstantiatedNamedTypes gathers every instantiated named type that is
// reachable anywhere in the program. Their equal/hash helpers and TypeInfo are
// defined once in shared_definition.c with external linkage.
func collectInstantiatedNamedTypes(program *ssa.Program) map[string]types.Type {
	result := map[string]types.Type{}
	seenFunctions := map[*ssa.Function]struct{}{}
	seenTypes := map[string]struct{}{}

	var harvestType func(typ types.Type)
	harvestType = func(typ types.Type) {
		name := createTypeName(typ)
		if _, ok := seenTypes[name]; ok {
			return
		}
		seenTypes[name] = struct{}{}
		if !hasTypeParameter(typ) {
			switch typ.(type) {
			case *types.Alias, *types.Basic, *types.Interface, *types.TypeParam:
				// only their components are harvested
			default:
				result[name] = typ
			}
		}
		switch typ := typ.(type) {
		case *types.Alias:
			harvestType(typ.Underlying())
		case *types.Array:
			harvestType(typ.Elem())
		case *types.Chan:
			harvestType(typ.Elem())
		case *types.Map:
			harvestType(typ.Key())
			harvestType(typ.Elem())
		case *types.Named:
			instantiated := typ.TypeArgs().Len() > 0 && !hasTypeParameter(typ)
			if instantiated {
				result[name] = typ
				for i := 0; i < typ.TypeArgs().Len(); i++ {
					harvestType(typ.TypeArgs().At(i))
				}
			}
			harvestType(typ.Underlying())
		case *types.Pointer:
			harvestType(typ.Elem())
		case *types.Slice:
			harvestType(typ.Elem())
		case *types.Signature:
			if typ.Recv() != nil {
				harvestType(typ.Recv().Type())
			}
			harvestType(typ.Params())
			harvestType(typ.Results())
		case *types.Struct:
			for i := 0; i < typ.NumFields(); i++ {
				harvestType(typ.Field(i).Type())
			}
		case *types.Tuple:
			for i := 0; i < typ.Len(); i++ {
				harvestType(typ.At(i).Type())
			}
		}
	}

	var walkFunction func(fn *ssa.Function)
	walkFunction = func(fn *ssa.Function) {
		if fn == nil {
			return
		}
		if _, ok := seenFunctions[fn]; ok {
			return
		}
		seenFunctions[fn] = struct{}{}
		harvestType(fn.Signature)
		if fn.Blocks == nil {
			return
		}
		for _, block := range fn.Blocks {
			for _, instr := range block.Instrs {
				for _, operand := range instr.Operands(nil) {
					if value, ok := (*operand).(ssa.Value); ok {
						harvestType(value.Type())
					}
					if ref, ok := (*operand).(*ssa.Function); ok {
						walkFunction(ref)
					}
				}
			}
		}
		for _, anon := range fn.AnonFuncs {
			walkFunction(anon)
		}
	}

	collector := &instanceCollector{seen: map[*ssa.Function]struct{}{}}
	helperCtx := &Context{program: program}
	for _, pkg := range allPackagesSorted(program) {
		for _, member := range sortedPackageMembers(pkg) {
			switch member := member.(type) {
			case *ssa.Function:
				walkFunction(member)
			case *ssa.Global:
				harvestType(member.Type())
			case *ssa.Type:
				harvestType(member.Type())
				harvestType(types.NewPointer(member.Type()))
			}
		}
		helperCtx.appendReachableInstances(pkg, collector)
	}
	for _, fn := range collector.sortedResult() {
		walkFunction(fn)
	}

	delete(result, "error")
	return result
}

func sortedInstantiatedNamedTypes(types_ map[string]types.Type) []types.Type {
	names := make([]string, 0, len(types_))
	for name := range types_ {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]types.Type, 0, len(names))
	for _, name := range names {
		result = append(result, types_[name])
	}
	return result
}

func collectAssertedInterfaceTypesOfFunctions(ctx *Context, pkg *ssa.Package) map[string]types.Type {
	result := map[string]types.Type{}
	ctx.traverseFunction(pkg, func(function *ssa.Function) {
		for _, block := range function.Blocks {
			for _, instr := range block.Instrs {
				if ta, ok := instr.(*ssa.TypeAssert); ok {
					iface, ok := ta.AssertedType.(*types.Interface)
					if ok && iface.NumMethods() > 0 {
						result[createInterfaceTypeSymbolName(iface)] = iface
					}
				}
			}
		}
	})
	return result
}

func collectAssertedInterfaceTypes(program *ssa.Program) map[string]types.Type {
	ctx := Context{
		program:       program,
		latestNameMap: make(map[*ssa.BasicBlock]string),
	}
	collector := &instanceCollector{seen: map[*ssa.Function]struct{}{}}
	for _, pkg := range allPackagesSorted(program) {
		ctx.appendReachableInstances(pkg, collector)
	}
	ctx.extraFunctions = append(ctx.extraFunctions, collector.sortedResult()...)
	return collectAssertedInterfaceTypesOfFunctions(&ctx, nil)
}

func handleSharedDefinition(program *ssa.Program, assertedInterfaceTypes map[string]types.Type, instantiatedNamedTypes map[string]types.Type, outputPath string) {
	f, err := os.Create(outputPath)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	ctx := Context{
		stream:                 f,
		program:                program,
		latestNameMap:          make(map[*ssa.BasicBlock]string),
		assertedInterfaceTypes: assertedInterfaceTypes,
		instantiatedNamedTypes: instantiatedNamedTypes,
	}

	ctx.collectSharedInstances()

	ctx.emitCommon()

	visitedInterfaceNames := map[string]bool{}
	ctx.traverseType(nil, func(typ types.Type) {
		if _, ok := typ.(*types.Interface); ok {
			visitedInterfaceNames[createInterfaceTypeSymbolName(typ)] = true
		}
	})
	ctx.visitedInterfaceNames = visitedInterfaceNames

	ctx.emitTypeDeclarationAndDefinition(nil, sortedInstantiatedNamedTypes(ctx.instantiatedNamedTypes))
	ctx.emitInterfaceDataDeclaration(nil)
	ctx.traverseFunction(nil, func(function *ssa.Function) {
		ctx.emitFunctionDeclarationHeader(function, ";")
	})

	ctx.emitInterfaceDataDefinition()

	ctx.traverseFunction(nil, func(function *ssa.Function) {
		if function.Blocks != nil {
			return
		}
		if createFunctionName(function) == "f_24_runtime_2E_mcall" {
			return
		}
		fmt.Fprintf(ctx.stream, "FunctionObject %s(LightWeightThreadContext* ctx){ (void)ctx; assert(false); return (FunctionObject){NULL}; }", createFunctionName(function))
	})

	ctx.emitSignature(nil)

	ctx.traverseFunction(nil, func(function *ssa.Function) {
		if !isInGenericInstanceSubtree(function) && !isPlainSyntheticWrapper(function) {
			return
		}
		ctx.emitFunctionDeclarationHeader(function, ";")
		ctx.emitFunctionVariableStructure(function)
	})

	ctx.traverseFunction(nil, func(function *ssa.Function) {
		if !isInGenericInstanceSubtree(function) && !isPlainSyntheticWrapper(function) || function.Blocks == nil {
			return
		}
		for _, basicBlock := range function.Blocks {
			for _, instr := range basicBlock.Instrs {
				if deferInstr, ok := instr.(*ssa.Defer); ok {
					callCommon := deferInstr.Common()
					if callCommon.Method == nil {
						if builtin, ok := callCommon.Value.(*ssa.Builtin); ok && (builtin.Name() == "print" || builtin.Name() == "println") {
							fmt.Fprintln(ctx.stream, ctx.emitBuiltinPrintWrapper(builtin.Name(), callCommon, deferInstr))
						}
					}
				}
			}
		}
	})

	foundInstanceConstValueSet := make(map[string]struct{})
	ctx.traverseFunction(nil, func(function *ssa.Function) {
		if !isInGenericInstanceSubtree(function) && !isPlainSyntheticWrapper(function) {
			return
		}
		if function.Blocks == nil {
			return
		}
		if isFunctionBodySkipped(function) {
			fmt.Fprintf(ctx.stream, "%sFunctionObject %s(LightWeightThreadContext* ctx){ (void)ctx; assert(false); return (FunctionObject){NULL}; }\n", functionStorageClass(function), createFunctionName(function))
			ctx.emitReceiverBoundThunkGlue(function, functionStorageClass(function))
			return
		}
		ctx.traverseValue(function, func(value ssa.Value) {
			if gv, ok := value.(*ssa.Global); ok {
				ctx.emitGlobalVariableDeclaration(gv)
			}
		})
		ctx.traverseValue(function, func(value ssa.Value) {
			if cst, ok := value.(*ssa.Const); ok {
				valueName := createValueName(cst)
				if _, ok := foundInstanceConstValueSet[valueName]; ok {
					return
				}
				foundInstanceConstValueSet[valueName] = struct{}{}
				ctx.emitConstant(cst)
			}
		})
		for _, basicBlock := range function.Blocks {
			name := createBasicBlockName(basicBlock)
			fmt.Fprintf(ctx.stream, "%sFunctionObject %s (LightWeightThreadContext* ctx);\n", functionStorageClass(function), name)
			ctx.latestNameMap[basicBlock] = name
			for _, instr := range basicBlock.Instrs {
				if requireSwitchFunction(instr) {
					continuationName := createInstructionName(instr)
					fmt.Fprintf(ctx.stream, "%sFunctionObject %s (LightWeightThreadContext* ctx);\n", functionStorageClass(function), continuationName)
					ctx.latestNameMap[basicBlock] = continuationName
				}
			}
		}
		ctx.emitFunctionDefinition(function)
	})

	ctx.emitRuntimeInfo()
	ctx.emitFunctionNameRegistry()

	fmt.Fprintf(ctx.stream, `
const UserFunctionInfo* gox5_runtime_func_for_pc(uintptr_t pc) {
	for (uintptr_t i = 0; i < sizeof(userFunctionInfoTable) / sizeof(userFunctionInfoTable[0]); i++) {
		if (userFunctionInfoTable[i].pc == pc) {
			return &userFunctionInfoTable[i];
		}
	}
	return NULL;
}

StringObject gox5_runtime_func_name(const UserFunctionInfo* func) {
	if (func == NULL) {
		return (StringObject){.raw = NULL, .len = 0};
	}
	return func->name;	
}
`)
}

func (ctx *Context) emitFunctionNameRegistry() {
	fmt.Fprintln(ctx.stream, "static const UserFunctionInfo userFunctionInfoTable[] = {")
	for _, pkg := range allPackagesSorted(ctx.program) {
		if isFunctionBodySkippedPackage(pkg) {
			continue
		}
		for _, member := range sortedPackageMembers(pkg) {
			fn, ok := member.(*ssa.Function)
			if !ok {
				continue
			}
			if fn.TypeParams().Len() > 0 {
				continue
			}
			name := functionRuntimeName(fn)
			fmt.Fprintf(ctx.stream, "\t{ (uintptr_t)%s, (StringObject){.raw = \"%s\", .len = sizeof(\"%s\") - 1} }, // %s\n",
				createFunctionName(fn), name, name, fn.RelString(nil))
		}
	}
	fmt.Fprintln(ctx.stream, "};")
}

func functionRuntimeName(fn *ssa.Function) string {
	if fn.Pkg == nil {
		return fn.Name()
	}
	return fn.Pkg.Pkg.Name() + "." + fn.Name()
}

func handlePackage(program *ssa.Program, pkg *ssa.Package, outputPath string) {
	f, err := os.Create(outputPath)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	ctx := Context{
		stream:        f,
		program:       program,
		latestNameMap: make(map[*ssa.BasicBlock]string),
	}

	// The package file defines static copies of every generic instance its
	// own code uses (monomorphization), so keep them in ctx.extraFunctions.
	ctx.collectInstances(pkg)
	ctx.assertedInterfaceTypes = collectAssertedInterfaceTypesOfFunctions(&ctx, pkg)
	ctx.emitPackage(pkg)
}

func handleMakefile(program *ssa.Program, outputPath string, buildDirname string, cacheDirname string, cachedPackages map[string]bool) {
	makefile, err := os.Create(outputPath)
	if err != nil {
		panic(err)
	}
	defer makefile.Close()
	generateMakefile(makefile, program, buildDirname, cacheDirname, cachedPackages)
}

func loadCachedPackages(cacheDirname string) map[string]bool {
	result := map[string]bool{}
	entries, err := os.ReadDir(cacheDirname)
	if err != nil {
		return result
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "package_") || !strings.HasSuffix(name, ".c") {
			continue
		}
		pkgName := strings.TrimSuffix(strings.TrimPrefix(name, "package_"), ".c")
		result[pkgName] = true
	}
	return result
}

func emitProgram(program *ssa.Program, buildDirname string, cacheDirname string) {
	waitGroup := sync.WaitGroup{}

	cachedPackages := loadCachedPackages(cacheDirname)
	assertedInterfaceTypes := collectAssertedInterfaceTypes(program)
	instantiatedNamedTypes := collectInstantiatedNamedTypes(program)

	waitGroup.Add(1)
	go func() {
		definitionName := "shared_definition.c"
		handleSharedDefinition(program, assertedInterfaceTypes, instantiatedNamedTypes, fmt.Sprintf("%s/%s", buildDirname, definitionName))
		waitGroup.Done()
	}()

	for _, pkg := range allPackagesSorted(program) {
		if isFunctionBodySkippedPackage(pkg) {
			continue
		}
		if cachedPackages[createPackageName(pkg.Pkg)] {
			continue
		}
		waitGroup.Add(1)
		go func(pkg *ssa.Package) {
			outputName := fmt.Sprintf("package_%s.c", createPackageName(pkg.Pkg))
			handlePackage(program, pkg, fmt.Sprintf("%s/%s", buildDirname, outputName))
			waitGroup.Done()
		}(pkg)
	}

	waitGroup.Add(1)
	go func() {
		makefileName := "Makefile"
		handleMakefile(program, fmt.Sprintf("%s/%s", buildDirname, makefileName), buildDirname, cacheDirname, cachedPackages)
		waitGroup.Done()
	}()

	waitGroup.Wait()
}
