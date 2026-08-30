package main

import (
	"fmt"

	"go/types"

	"golang.org/x/tools/go/ssa"
)

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
