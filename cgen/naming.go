package main

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	"go/constant"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

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
