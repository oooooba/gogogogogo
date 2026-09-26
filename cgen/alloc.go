package main

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// allocEscapeInfo caches, per SSA function, the reverse use map and the name of
// the generated C function that owns each instruction. Both maps are pure,
// deterministic functions of the function, so frame-struct emission and
// instruction emission always reach identical decisions about a buffer.
type allocEscapeInfo struct {
	// consumers maps every function-local ssa.Value to the instructions that
	// read it. Consumer edges come from instr.Operands(nil), which also yields
	// Phi edges and MakeClosure bindings.
	consumers map[ssa.Value][]ssa.Instruction
	// instructionFunction maps every instruction to the name of the generated C
	// function it is emitted within. The CPS model closes one C function before
	// every switch-requiring instruction and opens its continuation.
	instructionFunction map[ssa.Instruction]string
}

func (ctx *Context) allocEscapeInfoFor(function *ssa.Function) *allocEscapeInfo {
	if ctx.functionEscapeInfo == nil {
		ctx.functionEscapeInfo = make(map[*ssa.Function]*allocEscapeInfo)
	}
	if info, ok := ctx.functionEscapeInfo[function]; ok {
		return info
	}
	result := &allocEscapeInfo{
		consumers:           map[ssa.Value][]ssa.Instruction{},
		instructionFunction: map[ssa.Instruction]string{},
	}
	if function.Blocks != nil {
		for _, block := range function.Blocks {
			name := createBasicBlockName(block)
			for _, instr := range block.Instrs {
				result.instructionFunction[instr] = name
				if requireSwitchFunction(instr) {
					name = createInstructionName(instr)
				}
			}
		}
		for _, block := range function.Blocks {
			for _, instr := range block.Instrs {
				for _, operand := range instr.Operands(nil) {
					value, ok := (*operand).(ssa.Value)
					if !ok || value == nil || value.Parent() != function {
						continue
					}
					result.consumers[value] = append(result.consumers[value], instr)
				}
			}
		}
	}
	ctx.functionEscapeInfo[function] = result
	return result
}

// typeMayReference reports whether a value of the given type may hold a pointer
// to memory. Only such values can carry a buffer address beyond the generated C
// function that owns it; scalar-only values cannot escape it.
func typeMayReference(typ types.Type) bool {
	switch t := typ.(type) {
	case *types.Alias:
		return typeMayReference(t.Underlying())
	case *types.Array:
		return typeMayReference(t.Elem())
	case *types.Basic:
		switch t.Kind() {
		case types.String, types.UntypedString, types.UnsafePointer, types.Uintptr:
			// Strings carry a .raw pointer; uintptr and unsafe.Pointer may
			// hold an address obtained from the buffer.
			return true
		}
		return false
	case *types.Chan, *types.Interface, *types.Map, *types.Pointer, *types.Signature, *types.Slice, *types.TypeParam:
		return true
	case *types.Named:
		return typeMayReference(t.Underlying())
	case *types.Struct:
		for i := 0; i < t.NumFields(); i++ {
			if typeMayReference(t.Field(i).Type()) {
				return true
			}
		}
		return false
	case *types.Tuple:
		for i := 0; i < t.Len(); i++ {
			if typeMayReference(t.At(i).Type()) {
				return true
			}
		}
		return false
	default:
		// Unknown extension types (e.g. iter objects) may reference memory.
		return true
	}
}

// allocEscapePoint reports whether instr copies operand values into storage
// that the runtime reads after the current generated C function has returned:
// the goroutine stack frame, the deferred list, a channel, a map, or a spawned
// goroutine. A pointer into a host-stack buffer handed to such an instruction
// would dangle the moment the current C function returns.
func allocEscapePoint(instr ssa.Instruction) bool {
	if requireSwitchFunction(instr) {
		// Every switch-requiring instruction hands its operands to a runtime
		// API through next_frame fields, which outlive the current C function.
		return true
	}
	switch instr.(type) {
	case *ssa.Panic, *ssa.Return:
		// A Panic value persists until unwinding; a Return value is copied into
		// the caller's signature frame and read by a later C function.
		return true
	}
	return false
}

// allocBufferEligibleForHostStack reports whether the storage of a non-heap
// Alloc can become a host-stack local of the generated C function owning the
// Alloc instruction instead of a StackFrame field. This is sound only when
// every value that may hold an address into the buffer is read exclusively
// within that single generated C function execution: nothing may flow into a
// different C function (the CPS model switches C functions at every runtime API
// call and at every basic-block edge), into a Phi, or into an escape point.
//
// The result is a pure function of (function, alloc), and the same predicate
// guards both the StackFrame struct emission and the Alloc instruction
// emission, so the two files generated for one function always agree.
func (ctx *Context) allocBufferEligibleForHostStack(function *ssa.Function, alloc *ssa.Alloc) bool {
	if alloc.Heap {
		return false
	}
	if function.Blocks == nil {
		return false
	}
	info := ctx.allocEscapeInfoFor(function)
	owner := info.instructionFunction[alloc]

	inP := map[ssa.Value]bool{alloc: true}
	queue := []ssa.Value{alloc}
	for len(queue) > 0 {
		v := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		for _, consumer := range info.consumers[v] {
			if info.instructionFunction[consumer] != owner {
				return false
			}
			if _, ok := consumer.(*ssa.Phi); ok {
				return false
			}
			if store, ok := consumer.(*ssa.Store); ok {
				// Storing a value that may reference the buffer into arbitrary
				// memory leaks the address; storing into the buffer itself (the
				// address operand) only changes bytes that stay local.
				if store.Val == v {
					return false
				}
				continue
			}
			if allocEscapePoint(consumer) {
				return false
			}
			if value, ok := consumer.(ssa.Value); ok && typeMayReference(value.Type()) && !inP[value] {
				inP[value] = true
				queue = append(queue, value)
			}
		}
	}
	return true
}

// functionHostLocalBuffers returns, keyed by generated C function name, the
// non-heap Alloc buffers that must be declared as host-stack locals of that C
// function. The label progression mirrors emitFunctionDefinition exactly.
func (ctx *Context) functionHostLocalBuffers(function *ssa.Function) map[string][]*ssa.Alloc {
	result := map[string][]*ssa.Alloc{}
	if function.Blocks == nil {
		return result
	}
	for _, block := range function.Blocks {
		name := createBasicBlockName(block)
		for _, instr := range block.Instrs {
			if alloc, ok := instr.(*ssa.Alloc); ok {
				if ctx.allocBufferEligibleForHostStack(function, alloc) {
					result[name] = append(result[name], alloc)
				}
			}
			if requireSwitchFunction(instr) {
				name = createInstructionName(instr)
			}
		}
	}
	return result
}
