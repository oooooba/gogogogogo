package main

import (
	"fmt"
	"strings"

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

// emitBoundFunctionFreeVarsDeclaration declares what a bound method wrapper
// captures, the receiver, for a package that only names the wrapper.
func (ctx *Context) emitBoundFunctionFreeVarsDeclaration(fn *ssa.Function) {
	ctx.emitFreeVarsReceiverStruct(fn, fmt.Sprintf("FreeVars_%s", createFunctionName(fn)))
}

// freeVarsStorage keeps the descriptor of a FreeVars struct, and the enumerator
// that walks it, private to the translation unit that creates the closure. The
// descriptor is only ever read where the closure is built (to fill in
// capture_type), so no file has to agree on a symbol: the file that declares a
// closure function can come from the cache, written by an earlier cgen run,
// while the program that creates the closure is written by this one.
const freeVarsStorage = "static __attribute__((unused)) "

func (ctx *Context) emitFunctionVariableStructure(function *ssa.Function) {
	signature := function.Signature
	if signature.Recv() != nil {
		receiverBoundFuncName := fmt.Sprintf("%s%s", createFunctionName(function), encode("$bound"))
		fmt.Fprintf(ctx.stream, "%sFunctionObject %s (LightWeightThreadContext* ctx);\n", functionStorageClass(function), receiverBoundFuncName)

		ctx.emitFreeVarsReceiverStruct(function, fmt.Sprintf("FreeVars_%s", receiverBoundFuncName))

		receiverBoundSignatureName := createSignatureName(signature, true, false)
		fmt.Fprintf(ctx.stream, "typedef struct {\n")
		fmt.Fprintf(ctx.stream, "\tStackFrameCommon common;\n")
		fmt.Fprintf(ctx.stream, "\t%s signature;\n", receiverBoundSignatureName)
		fmt.Fprintf(ctx.stream, "} StackFrame_%s;\n", receiverBoundFuncName)
		ctx.emitFrameStackMap(function, receiverBoundFuncName, receiverBoundSignatureName, nil)

		receiverThunkFuncName := fmt.Sprintf("%s%s", createFunctionName(function), encode("$thunk"))
		fmt.Fprintf(ctx.stream, "%sFunctionObject %s (LightWeightThreadContext* ctx);\n", functionStorageClass(function), receiverThunkFuncName)
	}

	// A bound method wrapper is named "<method>$bound", which is exactly the
	// name of the struct declared just above for the method it wraps.
	if !isBoundMethodWrapper(function) {
		function := function
		ctx.emitFreeVarsStruct(fmt.Sprintf("FreeVars_%s", createFunctionName(function)), func() {
			for _, freeVar := range function.FreeVars {
				fmt.Fprintf(ctx.stream, "\t%s %s; // %s : %s\n", createTypeName(freeVar.Type()), createValueName(freeVar), freeVar.String(), freeVar.Type())
			}
		})
	}

	concreteSignatureName := createSignatureName(signature, false, false)
	fmt.Fprintf(ctx.stream, "typedef struct {\n")
	fmt.Fprintf(ctx.stream, "\tStackFrameCommon common;\n")
	fmt.Fprintf(ctx.stream, "\t%s signature;\n", concreteSignatureName)

	// The GC stack map below has to walk exactly the members emitted here, so
	// they are collected as they are written out.
	members := []frameMember{}

	if function.Blocks != nil {
		for _, local := range function.Locals {
			if local.Heap {
				panic(fmt.Sprintf("%s", local))
			}
			if ctx.allocBufferEligibleForHostStack(function, local) {
				// Promoted to a host-stack local of the generated C function that
				// owns the Alloc instruction; see functionHostLocalBuffers.
				continue
			}
			id := fmt.Sprintf("%s_buf", createValueName(local))
			elemType := local.Type().(*types.Pointer).Elem()
			fmt.Fprintf(ctx.stream, "\t%s %s;\n", createTypeName(elemType), id)
			members = append(members, frameMember{name: id, typ: elemType})
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
			members = append(members, frameMember{name: id, typ: value.Type()})
		})
	}

	if hasFunctionValueMakeInterface(function) {
		// reflect.ValueOf wraps function values in interfaces via
		// gox5_interface_new, which copies the receiver through a pointer.
		// Passing the address of a block-scoped compound literal is invalid by
		// the time the runtime reads it (the machine-stack slot is dead and may
		// be reused), so keep a stable copy in the frame instead.
		fmt.Fprintf(ctx.stream, "\tFunctionObject makeInterfaceReceiver;\n")
		members = append(members, frameMember{name: "makeInterfaceReceiver", typ: nil})
	}

	for _, makeInterface := range globalValueMakeInterfaces(function) {
		// Same rationale as makeInterfaceReceiver: a MakeInterface whose source
		// is a package global would otherwise hand gox5_interface_new the
		// address of a block-scoped compound literal, which is invalid by the
		// time the runtime reads it. Keep a stable copy in the frame instead.
		id := fmt.Sprintf("makeInterfaceReceiver_%s", createValueName(makeInterface))
		fmt.Fprintf(ctx.stream, "\t%s %s;\n", createTypeName(makeInterface.X.Type()), id)
		members = append(members, frameMember{name: id, typ: makeInterface.X.Type()})
	}

	fmt.Fprintf(ctx.stream, "} StackFrame_%s;\n", createFunctionName(function))
	ctx.emitFrameStackMap(function, createFunctionName(function), concreteSignatureName, members)
}

// frameMember is one pointer-bearing-capable member of a generated stack
// frame: the C designator to offsetof and the Go type stored in it. A nil typ
// means the member is a bare pointer word.
type frameMember struct {
	name string
	typ  types.Type
}

func createFrameTypeInfoName(frameName string) string {
	return fmt.Sprintf("TypeInfo_StackFrame_%s", frameName)
}

func getFrameOffsetRunsName(frameName string) string {
	return fmt.Sprintf("get_member_offset_runs_StackFrame_%s", frameName)
}

// stackMapMemberRun returns the enumeration of one member: the member type's
// own enumerator when it has one, and otherwise a single range covering the
// whole member, reported with the type the collector has to trace it with.
func stackMapMemberRun(ownerCName string, memberName string, typ types.Type) string {
	if isNoPointerType(typ) {
		return ""
	}
	if isStructOrArrayRoot(typ) && hasEnumerablePointerMembers(typ) {
		return fmt.Sprintf("\t%s(visit, base + offsetof(%s, %s), arg); // %s\n", getMemberOffsetRunsName(typ), ownerCName, memberName, typ)
	}
	return stackMapTypedVisit(fmt.Sprintf("base + offsetof(%s, %s)", ownerCName, memberName), createTypeName(typ), typ)
}

// emitFrameStackMap emits the GC stack map of a generated stack frame together
// with the TypeInfo that carries it. A frame layout is not a Go type, so its
// descriptor is synthesized here from the very members the frame struct was
// emitted with, composing the member enumerators of the types that have one.
// Generated frames record their own descriptor on entry, which also covers the
// indirect calls whose callee frame the caller cannot name.
func (ctx *Context) emitFrameStackMap(function *ssa.Function, frameName string, signatureName string, members []frameMember) {
	frameCName := fmt.Sprintf("StackFrame_%s", frameName)
	offsetRunsName := getFrameOffsetRunsName(frameName)
	// Only the frame itself refers to its descriptor, and always from the same
	// translation unit, so both stay local.
	storage := "static __attribute__((unused)) "

	// The common block holds no Go value: a resume function pointer, the link
	// to the previous frame, the free vars and the defer list, none of which
	// has a Go type, so it is scanned word by word.
	body := ""
	body += fmt.Sprintf("	visit(base + offsetof(%s, common.resume_func), sizeof(FunctionObject), (TypeId){.id=0}, GC_SLOT_FUNCTION, arg); // resume_func\n", frameCName)
	body += fmt.Sprintf("	visit(base + offsetof(%s, common.free_vars), sizeof(void*), (TypeId){.id=0}, GC_SLOT_FREE_VARS, arg); // free_vars\n", frameCName)
	body += fmt.Sprintf("	visit(base + offsetof(%s, common.deferred_list), sizeof(void*), (TypeId){.id=0}, GC_SLOT_DEFER_STACK, arg); // deferred_list\n", frameCName)
	body += fmt.Sprintf("\t%s(visit, base + offsetof(%s, signature), arg); // signature\n", getSignatureOffsetRunsName(signatureName), frameCName)
	for _, member := range members {
		if member.typ == nil {
			// A bare FunctionObject word: one word that always can hold a
			// pointer to the object of a function value.
			body += fmt.Sprintf("\tvisit(base + offsetof(%s, %s), sizeof(FunctionObject), (TypeId){.id = 0}, GC_SLOT_FUNCTION, arg); // pointer word\n", frameCName, member.name)
			continue
		}
		body += stackMapMemberRun(frameCName, member.name, member.typ)
	}
	fmt.Fprintf(ctx.stream, "%svoid %s(TypeOffsetVisitor visit, uintptr_t base, void *arg) { // stack map of %s\n%s}\n\n",
		storage, offsetRunsName, frameCName, stackMapFunctionBody(body))

	fmt.Fprintf(ctx.stream, "%sconst TypeInfo %s = {\n", storage, createFrameTypeInfoName(frameName))
	fmt.Fprintf(ctx.stream, "\t.gc_magic = TYPE_INFO_MAGIC,\n")
	fmt.Fprintf(ctx.stream, "\t.name = (StringObject){.raw = \"%s\", .len = sizeof(\"%s\") - 1},\n", frameCName, frameCName)
	fmt.Fprintf(ctx.stream, "\t.num_methods = 0,\n")
	fmt.Fprintf(ctx.stream, "\t.interface_table = NULL,\n")
	fmt.Fprintf(ctx.stream, "\t.is_equal = gox5_frame_type_is_equal,\n")
	fmt.Fprintf(ctx.stream, "\t.hash = gox5_frame_type_hash,\n")
	fmt.Fprintf(ctx.stream, "\t.size = sizeof(%s),\n", frameCName)
	fmt.Fprintf(ctx.stream, "\t.kind = GC_TYPE_STRUCT_ARRAY,\n")
	fmt.Fprintf(ctx.stream, "\t.pointed_to = (TypeId){.id = 0},\n")
	fmt.Fprintf(ctx.stream, "\t.get_member_offset_runs = %s,\n", offsetRunsName)
	fmt.Fprintf(ctx.stream, "};\n\n")
}

// isBoundMethodWrapper reports whether function is the "$bound" wrapper ssa
// synthesizes for a method value: the receiver is the only thing it captures.
func isBoundMethodWrapper(function *ssa.Function) bool {
	return strings.HasSuffix(function.Name(), "$bound")
}

// emitFreeVarsStruct declares the struct holding what a closure captured, if
// the file has not declared it yet.
func (ctx *Context) emitFreeVarsStruct(freeVarsCName string, emitFields func()) {
	if !ctx.markTypeDefinition("freevars-struct", freeVarsCName) {
		return
	}
	fmt.Fprintf(ctx.stream, "typedef struct {\n")
	emitFields()
	fmt.Fprintf(ctx.stream, "} %s;\n", freeVarsCName)
}

// boundMethodReceiverType is the type a bound method wrapper captures, which is
// the receiver of the method it wraps.
func boundMethodReceiverType(function *ssa.Function) types.Type {
	if recv := function.Signature.Recv(); recv != nil {
		return recv.Type()
	}
	if obj, ok := function.Object().(*types.Func); ok {
		if signature, ok := obj.Type().(*types.Signature); ok && signature.Recv() != nil {
			return signature.Recv().Type()
		}
	}
	return nil
}

// emitFreeVarsStructFor declares the struct of what a function captured.
func (ctx *Context) emitFreeVarsStructFor(function *ssa.Function) {
	if isBoundMethodWrapper(function) {
		ctx.emitFreeVarsReceiverStruct(function, fmt.Sprintf("FreeVars_%s", createFunctionName(function)))
		return
	}
	freeVars := function.FreeVars
	ctx.emitFreeVarsStruct(fmt.Sprintf("FreeVars_%s", createFunctionName(function)), func() {
		for _, freeVar := range freeVars {
			fmt.Fprintf(ctx.stream, "\t%s %s; // %s : %s\n", createTypeName(freeVar.Type()), createValueName(freeVar), freeVar.String(), freeVar.Type())
		}
	})
}

// emitFreeVarsReceiverStruct declares the struct of a bound method wrapper,
// which captures nothing but its receiver.
func (ctx *Context) emitFreeVarsReceiverStruct(function *ssa.Function, freeVarsCName string) {
	recvType := boundMethodReceiverType(function)
	if recvType == nil {
		return
	}
	ctx.emitFreeVarsStruct(freeVarsCName, func() {
		fmt.Fprintf(ctx.stream, "\t%s receiver;\n", createTypeName(recvType))
	})
}

// emitClosureCaptureDescriptors emits a private descriptor for every closure the
// given function creates, which is where the collector is told what the closure
// captured. Doing it here rather than where the closure function is declared
// keeps the descriptor next to its only reader, the capture_type assignment.
func (ctx *Context) emitClosureCaptureDescriptors(function *ssa.Function) {
	if function.Blocks == nil {
		return
	}
	for _, block := range function.Blocks {
		for _, instr := range block.Instrs {
			makeClosure, ok := instr.(*ssa.MakeClosure)
			if !ok {
				continue
			}
			fn, ok := makeClosure.Fn.(*ssa.Function)
			if !ok {
				continue
			}
			freeVarsCName := fmt.Sprintf("FreeVars_%s", createFunctionName(fn))
			if !ctx.markTypeDefinition("freevars-descriptor", freeVarsCName) {
				continue
			}
			if isBoundMethodWrapper(fn) {
				ctx.emitFreeVarsReceiverStruct(fn, freeVarsCName)
			} else {
				fn := fn
				ctx.emitFreeVarsStruct(freeVarsCName, func() {
					for _, freeVar := range fn.FreeVars {
						fmt.Fprintf(ctx.stream, "\t%s %s;\n", createTypeName(freeVar.Type()), createValueName(freeVar))
					}
				})
			}
			ctx.emitCapturedValuesTypeInfo(fn, freeVarsCName)
		}
	}
}

// emitCapturedValuesTypeInfo emits the descriptor the collector traces a
// closure's captured values with: the enumerator walks exactly the members of
// the FreeVars struct the closure creation code fills in, and the TypeInfo ties
// the two together.
func (ctx *Context) emitCapturedValuesTypeInfo(function *ssa.Function, freeVarsCName string) {
	typeInfoName := fmt.Sprintf("TypeInfo_%s", freeVarsCName)

	var body string
	if recvType := boundMethodReceiverType(function); isBoundMethodWrapper(function) && recvType != nil {
		body += stackMapMemberRun(freeVarsCName, "receiver", recvType)
	}
	if !isBoundMethodWrapper(function) {
		for _, freeVar := range function.FreeVars {
			body += stackMapMemberRun(freeVarsCName, createValueName(freeVar), freeVar.Type())
		}
	}
	fmt.Fprintf(ctx.stream, "%svoid get_member_offset_runs_%s(TypeOffsetVisitor visit, uintptr_t base, void *arg) { // captured values of %s\n%s}\n\n",
		freeVarsStorage, freeVarsCName, createFunctionName(function), stackMapFunctionBody(body))

	fmt.Fprintf(ctx.stream, "%sconst TypeInfo %s = {\n", freeVarsStorage, typeInfoName)
	fmt.Fprintf(ctx.stream, "\t.gc_magic = TYPE_INFO_MAGIC,\n")
	fmt.Fprintf(ctx.stream, "\t.name = (StringObject){.raw = \"%s\", .len = sizeof(\"%s\") - 1},\n", freeVarsCName, freeVarsCName)
	fmt.Fprintf(ctx.stream, "\t.num_methods = 0,\n")
	fmt.Fprintf(ctx.stream, "\t.interface_table = NULL,\n")
	fmt.Fprintf(ctx.stream, "\t.is_equal = gox5_frame_type_is_equal,\n")
	fmt.Fprintf(ctx.stream, "\t.hash = gox5_frame_type_hash,\n")
	fmt.Fprintf(ctx.stream, "\t.size = sizeof(%s),\n", freeVarsCName)
	fmt.Fprintf(ctx.stream, "\t.kind = GC_TYPE_STRUCT_ARRAY,\n")
	fmt.Fprintf(ctx.stream, "\t.pointed_to = (TypeId){.id = 0},\n")
	fmt.Fprintf(ctx.stream, "\t.get_member_offset_runs = get_member_offset_runs_%s,\n", freeVarsCName)
	fmt.Fprintf(ctx.stream, "};\n\n")
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

// emitFunctionDefinitionPrologue emits the entry of one basic block. isEntry
// marks the blocks a frame can be entered through, which is where the frame
// installs its own GC descriptor: the caller may not know the callee frame at
// all (indirect calls) and no allocation can happen in between.
func (ctx *Context) emitFunctionDefinitionPrologue(storage string, functionName string, frameName string, hasFreeVariables bool, hostBuffers []*ssa.Alloc, isEntry bool) {
	fmt.Fprintf(ctx.stream, "%sFunctionObject %s (LightWeightThreadContext* ctx){\n", storage, functionName)
	freeVarsCompareOp := "=="
	if hasFreeVariables {
		freeVarsCompareOp = "!="
	}
	fmt.Fprintf(ctx.stream, `
	StackFrame_%s* frame = (void*)ctx->stack_pointer;
	assert(frame->common.free_vars %s NULL);
`, frameName, freeVarsCompareOp)
	if isEntry {
		fmt.Fprintf(ctx.stream, "\tframe->common.frame_type = (TypeId){.info = &%s};\n", createFrameTypeInfoName(frameName))
		fmt.Fprintf(ctx.stream, "\tframe->common.frame_size = sizeof(StackFrame_%s);\n", frameName)
	}
	for _, alloc := range hostBuffers {
		id := fmt.Sprintf("%s_buf", createValueName(alloc))
		fmt.Fprintf(ctx.stream, "\t%s %s;\n", createTypeName(alloc.Type().(*types.Pointer).Elem()), id)
	}
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
	// The body reads what it captured through the FreeVars struct, and an
	// instance can be written to a file whose declaration pass never named it.
	if len(function.FreeVars) > 0 {
		ctx.emitFreeVarsStructFor(function)
	}
	ctx.emitClosureCaptureDescriptors(function)
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
	ctx.emitGlobalVariableRegistration(function)
	fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createBasicBlockName(function.Blocks[0])))
	fmt.Fprintf(ctx.stream, "}\n")

	frameName := createFunctionName(function)
	hasFreeVariables := len(function.FreeVars) != 0
	hostBuffers := ctx.functionHostLocalBuffers(function)
	for _, basicBlock := range function.Blocks {
		// Only the first block can be entered from outside, so only it has to
		// install the frame descriptor.
		isEntry := basicBlock == function.Blocks[0]
		ctx.emitFunctionDefinitionPrologue(storage, createBasicBlockName(basicBlock), frameName, hasFreeVariables, hostBuffers[createBasicBlockName(basicBlock)], isEntry)

		ctx.emitBlockPhis(basicBlock)

		for _, instr := range basicBlock.Instrs {
			if _, ok := instr.(*ssa.Phi); ok {
				continue
			}
			ctx.emitInstruction(instr)

			if requireSwitchFunction(instr) {
				ctx.emitFunctionDefinitionEpilogue()
				ctx.emitFunctionDefinitionPrologue(storage, createInstructionName(instr), frameName, hasFreeVariables, hostBuffers[createInstructionName(instr)], false)
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
	ctx.emitFunctionDefinitionPrologue(storage, resumeFuncName, boundFuncName, true, nil, false)
	fmt.Fprintf(ctx.stream, `
	assert(ctx->marker == 0xdeadbeef);
	ctx->stack_pointer = frame->common.prev_stack_pointer;
	return frame->common.resume_func;
`)
	ctx.emitFunctionDefinitionEpilogue()

	ctx.emitFunctionDefinitionPrologue(storage, boundFuncName, boundFuncName, true, nil, true)
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
	ctx.emitFunctionDefinitionPrologue(storage, thunkResumeFuncName, origFuncName, false, nil, false)
	fmt.Fprintf(ctx.stream, `
	assert(ctx->marker == 0xdeadbeef);
	ctx->stack_pointer = frame->common.prev_stack_pointer;
	return frame->common.resume_func;
`)
	ctx.emitFunctionDefinitionEpilogue()

	ctx.emitFunctionDefinitionPrologue(storage, thunkFuncName, origFuncName, false, nil, true)
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
