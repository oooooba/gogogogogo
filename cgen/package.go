package main

import (
	"fmt"
	"strings"

	"go/types"

	"golang.org/x/tools/go/ssa"
)

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

	emptyInterface := types.NewInterfaceType(nil, nil)
	ctx.emitInterfaceTableDeclaration(emptyInterface, allowSet)
	ctx.emitInterfaceTableDefinition(emptyInterface, allowSet)
	ctx.emitTypeInfoDefinition(emptyInterface)

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
			if functionPkg(f) == pkg.Pkg && !isInterfaceMethodWrapper(f) {
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
