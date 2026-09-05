package main

import (
	"fmt"
	"strings"
	"sync"

	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

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

// isInterfaceMethodWrapper reports whether fn is a synthetic $bound/$thunk
// wrapper around an interface method. Such wrappers have no concrete method
// definition to ride along with (the interface method table lives only in
// shared_definition.c), so they must be defined there instead of as glue next
// to a method body.
func isInterfaceMethodWrapper(fn *ssa.Function) bool {
	if !isPlainSyntheticWrapper(fn) {
		return false
	}
	if fn.Blocks == nil || fn.TypeParams().Len() > 0 || len(fn.TypeArgs()) > 0 || hasTypeParamInSignature(fn.Signature) {
		return false
	}
	obj, ok := fn.Object().(*types.Func)
	if !ok {
		return false
	}
	sig, ok := obj.Type().Underlying().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return false
	}
	return types.IsInterface(sig.Recv().Type())
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
