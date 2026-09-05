package main

import (
	"fmt"
	"os"
	"sort"

	"go/types"

	"golang.org/x/tools/go/ssa"
)

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
	visitedInterfaceNames[createInterfaceTypeSymbolName(types.NewInterfaceType(nil, nil))] = true
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
