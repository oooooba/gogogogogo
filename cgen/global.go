package main

import (
	"fmt"

	"go/types"

	"golang.org/x/tools/go/ssa"
)

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
