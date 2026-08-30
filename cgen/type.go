package main

import (
	"fmt"
	"strings"

	"go/constant"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

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
