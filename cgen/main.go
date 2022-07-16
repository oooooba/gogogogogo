package main

import (
	"flag"
	"fmt"
	"os"

	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

func main() {
	filename := flag.String("i", "/dev/stdin", "input file")
	flag.Parse()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, *filename, nil, 0)
	if err != nil {
		fmt.Println(err)
		return
	}
	files := []*ast.File{f}

	typesPackage := types.NewPackage("package", "")
	ssaPackage, _, err := ssautil.BuildPackage(
		&types.Config{Importer: importer.Default()}, fset, typesPackage, files, ssa.SanityCheckFunctions)
	if err != nil {
		fmt.Println(err)
		return
	}

	ctx := Context{
		stream: os.Stdout,
	}
	ctx.emitPackage(ssaPackage)
}

type Context struct {
	stream        *os.File
	foundValueSet map[ssa.Value]struct{}
}

func extractTestTargetFunctions(f *ssa.Function) []*ssa.Function {
	targets := make([]*ssa.Function, 0)
	for _, instr := range f.Blocks[0].Instrs {
		if callInstr, ok := instr.(*ssa.Call); ok {
			callCommong := callInstr.Common()
			for _, arg := range callCommong.Args {
				if targetFunction, ok := arg.(*ssa.Function); ok {
					targets = append(targets, targetFunction)
					break
				}
			}
		}
	}
	return targets
}

func createValueName(value ssa.Value) string {
	if val, ok := value.(*ssa.Const); ok {
		if val == nil {
			return "NULL"
		} else {
			return val.Value.String()
		}
	} else {
		parentName := value.Parent().Name()
		return fmt.Sprintf("v$%s$%s$%p", value.Name(), parentName, value)
	}
}

func createValueRelName(value ssa.Value) string {
	if _, ok := value.(*ssa.Const); ok {
		return createValueName(value)
	} else {
		return fmt.Sprintf("frame->%s", createValueName(value))
	}
}

func createType(typ types.Type) string {
	switch t := typ.(type) {
	case *types.Basic:
		switch t.Kind() {
		case types.Bool:
			return "bool"
		case types.Int, types.Int8, types.Int16, types.Int32, types.Int64, types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
			return "intptr_t"
		}
	case *types.Chan:
		return "void*"
	case *types.Pointer:
		return "void*"
	}
	panic("type not supported: " + typ.String())
}

func calculateTypeSize(typ types.Type) int64 {
	switch t := typ.(type) {
	case *types.Basic:
		switch t.Kind() {
		case types.Bool:
			return 1
		case types.Int, types.Int8, types.Int16, types.Int32, types.Int64, types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
			return 8
		}
	case *types.Chan:
		return 8
	case *types.Pointer:
		return 8
	}
	panic("type not supported: " + typ.String())
}

func (ctx *Context) emitInstruction(instruction ssa.Instruction) {
	fmt.Fprintf(ctx.stream, "\t// %T instruction\n", instruction)
	switch instr := instruction.(type) {
	case *ssa.Alloc:
		if instr.Heap {
			size := calculateTypeSize(instr.Type().Underlying().(*types.Pointer).Elem())
			fmt.Fprintf(ctx.stream, `
	struct StackFrameNew* next_frame = (struct StackFrameNew*)(ctx->stack_pointer + sizeof(*frame));
	next_frame->common.resume_func = %s;
	next_frame->common.prev_stack_pointer = ctx->stack_pointer;
	next_frame->result_ptr = &%s;
	next_frame->size = %d;
	ctx->stack_pointer = next_frame;
	return gox5_new;
`, createInstructionName(instr), createValueRelName(instr), size)
		} else {
			panic("unimplemented")
		}

	case *ssa.BinOp:
		fmt.Fprintf(ctx.stream, "%s = %s %s %s;\n", createValueRelName(instr), createValueRelName(instr.X), instr.Op.String(), createValueRelName(instr.Y))

	case *ssa.Call:
		callCommon := instr.Common()
		if callCommon.Method != nil {
			panic("method not supported")
		}

		switch callee := callCommon.Value.(type) {
		case *ssa.Function:
			name := createFunctionName(callee)
			fmt.Fprintf(ctx.stream, `
	struct StackFrame_%s* next_frame = (struct StackFrame_%s*)(ctx->stack_pointer + sizeof(*frame));
	next_frame->common.resume_func = %s;
	next_frame->common.prev_stack_pointer = ctx->stack_pointer;
	next_frame->result_ptr = &%s;
`, name, name, createInstructionName(instr), createValueRelName(instr))
			for i, arg := range callCommon.Args {
				param := callee.Params[i]
				fmt.Fprintf(ctx.stream, "next_frame->%s = %s; // [%d]: param=%s, arg=%s\n", createValueName(param), createValueRelName(arg), i, param.String(), arg.String())
			}
			fmt.Fprintf(ctx.stream, `
	ctx->stack_pointer = next_frame;
	return %s;
`, name)

		default:
			panic("unknown callee")
		}

	case *ssa.Go:
		callCommon := instr.Common()
		if callCommon.Method != nil {
			panic("method not supported")
		}
		switch callee := callCommon.Value.(type) {
		case *ssa.Function:
			name := createFunctionName(callee)
			fmt.Fprintf(ctx.stream, `
	struct StackFrameSpawn* next_frame = (struct StackFrameSpawn*)(ctx->stack_pointer + sizeof(*frame));
	next_frame->common.resume_func = %s;
	next_frame->common.prev_stack_pointer = ctx->stack_pointer;
	next_frame->func = %s;
`, createInstructionName(instr), name)
			if len(callCommon.Args) > 3 {
				panic("currently, support 3 parameters")
			}
			for i, arg := range callCommon.Args {
				param := callee.Params[i]
				fmt.Fprintf(ctx.stream, "next_frame->arg%d = %s; // param=%s, arg=%s\n", i, createValueRelName(arg), param.String(), arg.String())
			}
			for i := len(callCommon.Args); i < 3; i++ {
				fmt.Fprintf(ctx.stream, "next_frame->arg%d = 0; // [padded]\n", i)
			}
			fmt.Fprintf(ctx.stream, `
	ctx->stack_pointer = next_frame;
	return gox5_spawn;
`)
		default:
			panic("unknown callee")
		}

	case *ssa.If:
		fmt.Fprintf(ctx.stream, "\treturn %s ? %s : %s;\n", createValueRelName(instr.Cond), createBasicBlockName(instr.Block().Succs[0]), createBasicBlockName(instr.Block().Succs[1]))

	case *ssa.Jump:
		fmt.Fprintf(ctx.stream, "\treturn %s;\n", createBasicBlockName(instr.Block().Succs[0]))

	case *ssa.MakeChan:
		fmt.Fprintf(ctx.stream, `
	struct StackFrameMakeChan* next_frame = (struct StackFrameMakeChan*)(ctx->stack_pointer + sizeof(*frame));
	next_frame->common.resume_func = %s;
	next_frame->common.prev_stack_pointer = ctx->stack_pointer;
	next_frame->result_ptr = &%s;
	next_frame->size = %s;
	ctx->stack_pointer = next_frame;
	return gox5_make_chan;
`, createInstructionName(instr), createValueRelName(instr), createValueRelName(instr.Size))

	case *ssa.Phi:
		basicBlock := instr.Block()
		for i, edge := range instr.Edges {
			fmt.Fprintf(ctx.stream, "\tif (ctx->prev_func == %s) { %s = %s; } else\n", createBasicBlockName(basicBlock.Preds[i]), createValueRelName(instr), createValueRelName(edge))
		}
		fmt.Fprintln(ctx.stream, "\t{ assert(false); }")

	case *ssa.Return:
		fmt.Fprintf(ctx.stream, `
	void* resume_func = frame->common.resume_func;
	ctx->stack_pointer = frame->common.prev_stack_pointer;
	*frame->result_ptr = %s;
	return resume_func;
`, createValueRelName(instr.Results[0]))

	case *ssa.Send:
		fmt.Fprintf(ctx.stream, `
	struct StackFrameSend* next_frame = (struct StackFrameSend*)(ctx->stack_pointer + sizeof(*frame));
	next_frame->common.resume_func = %s;
	next_frame->common.prev_stack_pointer = ctx->stack_pointer;
	next_frame->channel = %s;
	next_frame->data = %s;
	ctx->stack_pointer = next_frame;
	return gox5_send;
`, createInstructionName(instr), createValueRelName(instr.Chan), createValueRelName(instr.X))

	case *ssa.Store:
		fmt.Fprintf(ctx.stream, "*(%s*)%s = %s;\n", createType(instr.Val.Type()), createValueRelName(instr.Addr), createValueRelName(instr.Val))

	case *ssa.UnOp:
		if instr.Op == token.ARROW {
			fmt.Fprintf(ctx.stream, `
	struct StackFrameRecv* next_frame = (struct StackFrameRecv*)(ctx->stack_pointer + sizeof(*frame));
	next_frame->common.resume_func = %s;
	next_frame->common.prev_stack_pointer = ctx->stack_pointer;
	next_frame->result_ptr = &%s;
	next_frame->channel = %s;
	ctx->stack_pointer = next_frame;
	return gox5_recv;
`, createInstructionName(instr), createValueRelName(instr), createValueRelName(instr.X))
		} else if instr.Op == token.MUL {
			fmt.Fprintf(ctx.stream, "%s = * (%s*)%s;\n", createValueRelName(instr), createType(instr.Type()), createValueRelName(instr.X))
		} else {
			fmt.Fprintf(ctx.stream, "%s = %s %s;\n", createValueRelName(instr), instr.Op.String(), createValueRelName(instr.X))
		}

	default:
		panic("unknown instruction: " + instruction.String())
	}
}

func createInstructionName(instruction ssa.Instruction) string {
	return fmt.Sprintf("i$%s$%s$%p", instruction.Block().String(), instruction.Parent().Name(), instruction)
}

func createBasicBlockName(basicBlock *ssa.BasicBlock) string {
	return fmt.Sprintf("b$%s$%s$%p", basicBlock.String(), basicBlock.Parent().Name(), basicBlock)
}

func createFunctionName(function *ssa.Function) string {
	return fmt.Sprintf("f$%s", function.Name())
}

func (ctx *Context) emitCallCommonDeclaration(callCommon *ssa.CallCommon) {
	for _, arg := range callCommon.Args {
		ctx.emitValueDeclaration(arg)
	}
}

func (ctx *Context) emitValueDeclaration(value ssa.Value) {
	_, ok := ctx.foundValueSet[value]
	if ok {
		return
	}
	ctx.foundValueSet[value] = struct{}{}

	canEmit := true
	switch val := value.(type) {
	case *ssa.Alloc:
		if val.Heap {
			// do nothing
		} else {
			panic("unimplemented")
		}

	case *ssa.BinOp:
		ctx.emitValueDeclaration(val.X)
		ctx.emitValueDeclaration(val.Y)

	case *ssa.Call:
		ctx.emitCallCommonDeclaration(val.Common())

	case *ssa.Const:
		canEmit = false

	case *ssa.MakeChan:
		ctx.emitValueDeclaration(val.Size)

	case *ssa.Parameter:
		canEmit = false

	case *ssa.Phi:
		for _, edge := range val.Edges {
			ctx.emitValueDeclaration(edge)
		}

	case *ssa.UnOp:
		ctx.emitValueDeclaration(val.X)

	default:
		fmt.Printf("type = %T\n", value)
		panic("unknown value: " + value.String())
	}

	fmt.Fprintf(ctx.stream, "\t// found %T: %s, %s\n", value, createValueName(value), value.String())
	if canEmit {
		fmt.Fprintf(ctx.stream, "\t%s %s; // %s : %s\n", createType(value.Type()), createValueName(value), value.String(), value.Type())
	}
}

func requireSwitchFunction(instruction ssa.Instruction) bool {
	switch instruction.(type) {
	case *ssa.Alloc:
		return instruction.(*ssa.Alloc).Heap
	case *ssa.Call, *ssa.Go, *ssa.MakeChan, *ssa.Send:
		return true
	case *ssa.UnOp:
		if instruction.(*ssa.UnOp).Op == token.ARROW {
			return true
		}
	}
	return false
}

func (ctx *Context) emitFunctionDeclaration(function *ssa.Function) {
	fmt.Fprintf(ctx.stream, "void* %s (struct LightWeightThreadContext* ctx);\n", createFunctionName(function))
	fmt.Fprintf(ctx.stream, `
struct StackFrame_%s {
	struct StackFrameCommon common;
	%s* result_ptr;
`, createFunctionName(function), createType(function.Signature.Results().At(0).Type()))
	for i := len(function.Params) - 1; i >= 0; i-- {
		fmt.Fprintf(ctx.stream, "\t%s %s; // parameter[%d]: %s\n", createType(function.Params[i].Type()), createValueName(function.Params[i]), i, function.Params[i].String())
	}

	if function.Blocks != nil {
		ctx.foundValueSet = make(map[ssa.Value]struct{})
		for _, basicBlock := range function.DomPreorder() {
			for _, instr := range basicBlock.Instrs {
				if value, ok := instr.(ssa.Value); ok {
					ctx.emitValueDeclaration(value)
				}
			}
		}
	}

	fmt.Fprintln(ctx.stream, "};")

	if function.Blocks == nil {
		return
	}

	for _, basicBlock := range function.DomPreorder() {
		fmt.Fprintf(ctx.stream, "void* %s (struct LightWeightThreadContext* ctx);\n", createBasicBlockName(basicBlock))
		for _, instr := range basicBlock.Instrs {
			if requireSwitchFunction(instr) {
				fmt.Fprintf(ctx.stream, "void* %s (struct LightWeightThreadContext* ctx);\n", createInstructionName(instr))
			}
		}
	}
}

func (ctx *Context) emitFunctionDefinitionHeader(function *ssa.Function, name string) {
	fmt.Fprintf(ctx.stream, "void* %s (struct LightWeightThreadContext* ctx) {", name)
	fmt.Fprintf(ctx.stream, `
	struct StackFrame_%s* frame = ctx->stack_pointer;
	(void)frame;
`, createFunctionName(function))
}

func (ctx *Context) emitFunctionDefinitionFooter(function *ssa.Function) {
	fmt.Fprintln(ctx.stream, "}")
}

func (ctx *Context) emitFunctionDefinition(function *ssa.Function) {
	fmt.Fprintf(ctx.stream, `
void* %s (struct LightWeightThreadContext* ctx) {
	assert(ctx->marker == 0xdeadbeef);
	return %s;
}
`, createFunctionName(function), createBasicBlockName(function.Blocks[0]))

	ctx.foundValueSet = make(map[ssa.Value]struct{})

	for _, basicBlock := range function.DomPreorder() {
		ctx.emitFunctionDefinitionHeader(function, createBasicBlockName(basicBlock))

		for _, instr := range basicBlock.Instrs {
			ctx.emitInstruction(instr)

			if requireSwitchFunction(instr) {
				ctx.emitFunctionDefinitionFooter(function)
				ctx.emitFunctionDefinitionHeader(function, createInstructionName(instr))
			}
		}

		ctx.emitFunctionDefinitionFooter(function)
	}
}

func (ctx *Context) emitPackage(pkg *ssa.Package) {
	fmt.Fprint(ctx.stream, `
#include <stdbool.h>
#include <stdio.h>
#include <stdint.h>
#include <string.h>
#include <assert.h>

struct LightWeightThreadContext {
	void* global_context;
	void* stack_pointer;
	const void* prev_func;
	intptr_t marker;
};

struct StackFrameCommon {
	void* resume_func;
	void* prev_stack_pointer;
};

struct StackFrameMakeChan {
	struct StackFrameCommon common;
	void** result_ptr;
	intptr_t size;
};
void* gox5_make_chan (struct LightWeightThreadContext* ctx);

struct StackFrameNew {
	struct StackFrameCommon common;
	void** result_ptr;
	intptr_t size;
};
void* gox5_new (struct LightWeightThreadContext* ctx);

struct StackFrameRecv {
	struct StackFrameCommon common;
	intptr_t* result_ptr;
	void* channel; // ATTENTION
};
void* gox5_recv (struct LightWeightThreadContext* ctx);

struct StackFrameSend {
	struct StackFrameCommon common;
	void* channel; // ATTENTION
	intptr_t data;
};
void* gox5_send (struct LightWeightThreadContext* ctx);

struct StackFrameSpawn {
	struct StackFrameCommon common;
    void* func;
	// ATTENTION: number of arguments fixed
	void* arg0; // ATTENTION: arg0 is treated as channel
	intptr_t arg1;
	intptr_t arg2;
};
void* gox5_spawn (struct LightWeightThreadContext* ctx);
`)

	for symbol := range pkg.Members {
		function, ok := pkg.Members[symbol].(*ssa.Function)
		if !ok {
			continue
		}
		if symbol == "main" || symbol == "init" {
			continue
		}
		ctx.emitFunctionDeclaration(function)
	}

	for symbol := range pkg.Members {
		function, ok := pkg.Members[symbol].(*ssa.Function)
		if !ok {
			continue
		}
		if symbol == "main" || symbol == "init" {
			continue
		}
		if function.Blocks == nil {
			continue
		}
		ctx.emitFunctionDefinition(function)
	}

	fmt.Fprintln(ctx.stream, "struct { const char* name; void* function; } test_entry_points[] = {")
	for symbol := range pkg.Members {
		function, ok := pkg.Members[symbol].(*ssa.Function)
		if !ok {
			continue
		}
		if symbol != "main" {
			continue
		}
		testTargetFunctions := extractTestTargetFunctions(function)
		for _, testTargetFunction := range testTargetFunctions {
			fmt.Fprintf(ctx.stream, "{ \"%s\", %s },\n", testTargetFunction.Name(), createFunctionName(testTargetFunction))
		}
	}
	fmt.Fprintln(ctx.stream, "};")

	fmt.Fprint(ctx.stream, `
size_t test_entry_point_num(void) {
	return sizeof(test_entry_points)/sizeof(test_entry_points[0]);
}

const char* test_entry_point_name(size_t i) {
	return test_entry_points[i].name;
}

void* test_entry_point_function(size_t i) {
	return test_entry_points[i].function;
}
`)
}
