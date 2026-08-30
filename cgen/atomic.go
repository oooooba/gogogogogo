package main

import (
	"fmt"

	"go/types"

	"golang.org/x/tools/go/ssa"
)

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
