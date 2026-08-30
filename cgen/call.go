package main

import (
	"fmt"
	"strings"

	"go/types"

	"golang.org/x/tools/go/ssa"
)

func (ctx *Context) switchFunction(nextFunction string, signature *types.Signature, signatureName string, result string, resumeFunction string, paramAndArgsHandler func()) {
	fmt.Fprintf(ctx.stream, "StackFrameCommon* next_frame = (StackFrameCommon*)(frame + 1);\n")
	fmt.Fprintf(ctx.stream, "assert(((uintptr_t)next_frame) %% sizeof(uintptr_t) == 0);\n")
	fmt.Fprintf(ctx.stream, "*next_frame = (StackFrameCommon){ 0 };\n")
	fmt.Fprintf(ctx.stream, "next_frame->resume_func = %s;\n", wrapInFunctionObject(resumeFunction))
	fmt.Fprintf(ctx.stream, "next_frame->prev_stack_pointer = ctx->stack_pointer;\n")

	if signature.Recv() != nil || signature.Results().Len() > 0 || signature.Params().Len() > 0 {
		fmt.Fprintf(ctx.stream, "%s* signature = (%s*)(next_frame + 1);\n", signatureName, signatureName)
	}

	if signature.Results().Len() > 0 {
		fmt.Fprintf(ctx.stream, "signature->result_ptr = &%s;\n", result)
	}

	paramAndArgsHandler()

	fmt.Fprintf(ctx.stream, "next_frame->free_vars = NULL;\n")
	fmt.Fprintf(ctx.stream, "ctx->stack_pointer = next_frame;\n")
	fmt.Fprintf(ctx.stream, "return %s;\n", nextFunction)
}

type paramArgPair struct {
	param string
	arg   string
}

func (ctx *Context) switchFunctionToCallRuntimeApi(nextFunction string, nextFunctionFrame string, resumeFunction string,
	resultPtr *string, variableSizeFrameHandler func(), paramArgPairs ...paramArgPair) {
	fmt.Fprintf(ctx.stream, "%s* next_frame = (%s*)(frame + 1);\n", nextFunctionFrame, nextFunctionFrame)
	fmt.Fprintf(ctx.stream, "assert(((uintptr_t)next_frame) %% sizeof(uintptr_t) == 0);\n")
	fmt.Fprintf(ctx.stream, "*next_frame = (%s){ 0 };\n", nextFunctionFrame)
	fmt.Fprintf(ctx.stream, "next_frame->common.resume_func = %s;\n", wrapInFunctionObject(resumeFunction))
	fmt.Fprintf(ctx.stream, "next_frame->common.prev_stack_pointer = ctx->stack_pointer;\n")

	if resultPtr != nil {
		fmt.Fprintf(ctx.stream, "next_frame->result_ptr = &%s;\n", *resultPtr)
	}
	for i, pair := range paramArgPairs {
		fmt.Fprintf(ctx.stream, "next_frame->%s = %s; // [%d]\n", pair.param, pair.arg, i)
	}

	if variableSizeFrameHandler != nil {
		variableSizeFrameHandler()
	}

	fmt.Fprintf(ctx.stream, "ctx->stack_pointer = (StackFrameCommon*)next_frame;\n")
	fmt.Fprintf(ctx.stream, "return %s;\n", wrapInFunctionObject(nextFunction))
}

func (ctx *Context) emitArgBufferCopies(args []ssa.Value) {
	fmt.Fprintf(ctx.stream, "intptr_t num_arg_buffer_words = 0;\n")
	for i, arg := range args {
		argValue := createValueRelName(arg)
		argType := createTypeName(arg.Type())
		argPtr := fmt.Sprintf("ptr%d", i)
		fmt.Fprintf(ctx.stream, "%s* %s = (void*)&next_frame->arg_buffer[num_arg_buffer_words]; // param[%d]\n", argType, argPtr, i)
		fmt.Fprintf(ctx.stream, "*%s = %s;\n", argPtr, argValue)
		fmt.Fprintf(ctx.stream, "num_arg_buffer_words += (sizeof(%s) + sizeof(next_frame->arg_buffer[0]) - 1) / sizeof(next_frame->arg_buffer[0]);\n", argType)
	}
	fmt.Fprintf(ctx.stream, "next_frame->num_arg_buffer_words = num_arg_buffer_words;\n")
}

func (ctx *Context) emitGoOrDefer(instr ssa.Instruction, callCommon *ssa.CallCommon, registerApi string, registerFrame string, invokeApi string, invokeFrame string, builtinPrintSupported bool) {
	resumeFunction := createInstructionName(instr)
	if callCommon.Method != nil {
		ctx.emitCallCommonForMethod(callCommon, invokeApi, invokeFrame, resumeFunction)
		return
	}
	if builtin, ok := callCommon.Value.(*ssa.Builtin); ok && builtinPrintSupported && (builtin.Name() == "print" || builtin.Name() == "println") {
		wrapperName := ctx.emitBuiltinPrintWrapper(builtin.Name(), callCommon, instr)
		signature := callCommon.Value.Type().Underlying().(*types.Signature)
		resultSize := "0"
		switch signature.Results().Len() {
		case 0:
			// do nothing
		case 1:
			resultSize = fmt.Sprintf("sizeof(%s)", createTypeName(signature.Results().At(0).Type()))
		default:
			resultSize = fmt.Sprintf("sizeof(%s)", createTypeName(signature.Results()))
		}
		ctx.switchFunctionToCallRuntimeApi(registerApi, registerFrame, resumeFunction, nil,
			func() {
				ctx.emitArgBufferCopies(callCommon.Args)
			},
			paramArgPair{param: "function_object", arg: wrapInFunctionObject(wrapperName)},
			paramArgPair{param: "result_size", arg: resultSize},
		)
		return
	}
	if builtin, ok := callCommon.Value.(*ssa.Builtin); ok && builtin.Name() == "close" {
		// The channel argument is copied to the frame's arg_buffer, which is at the
		// same offset as StackFrameChannelClose.channel, so gox5_channel_close can be
		// invoked directly as the entry function.
		ctx.switchFunctionToCallRuntimeApi(registerApi, registerFrame, resumeFunction, nil,
			func() {
				ctx.emitArgBufferCopies(callCommon.Args)
			},
			paramArgPair{param: "function_object", arg: wrapInFunctionObject("gox5_channel_close")},
			paramArgPair{param: "result_size", arg: "0"},
		)
		return
	}
	ctx.emitCallCommon(callCommon, registerApi, registerFrame, resumeFunction)
}

func (ctx *Context) emitCallCommon(callCommon *ssa.CallCommon, nextFunction string, nextFunctionFrame string, resumeFunction string) {
	if callCommon.Method != nil {
		panic("method not supported")
	}

	if function, ok := callCommon.Value.(*ssa.Function); ok {
		callPath := ""
		if function.Pkg != nil {
			callPath = function.Pkg.Pkg.Path()
		}
		if strings.HasPrefix(callPath, "internal/race") || strings.HasPrefix(callPath, "internal/synctest") {
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(resumeFunction))
			return
		}
	}

	var functionObject string
	switch callee := callCommon.Value.(type) {
	case *ssa.Function:
		functionObject = createValueName(callee)
	case ssa.Value:
		functionObject = createValueRelName(callee)
	default:
		panic(fmt.Sprintf("unknown callee: %s, %s, %T, %T", callCommon, callee, callCommon, callee))
	}

	signature := callCommon.Value.Type().Underlying().(*types.Signature)

	resultSize := "0"
	switch signature.Results().Len() {
	case 0:
		// do nothing
	case 1:
		resultSize = fmt.Sprintf("sizeof(%s)", createTypeName(signature.Results().At(0).Type()))
	default:
		resultSize = fmt.Sprintf("sizeof(%s)", createTypeName(signature.Results()))
	}

	ctx.switchFunctionToCallRuntimeApi(nextFunction, nextFunctionFrame, resumeFunction, nil,
		func() {
			ctx.emitArgBufferCopies(callCommon.Args)
		},
		paramArgPair{param: "function_object", arg: functionObject},
		paramArgPair{param: "result_size", arg: resultSize},
	)
}

func (ctx *Context) emitCallCommonForMethod(callCommon *ssa.CallCommon, nextFunction string, nextFunctionFrame string, resumeFunction string) {
	if callCommon.Method == nil {
		panic("only method supported")
	}

	signature := callCommon.Method.Type().Underlying().(*types.Signature)

	resultSize := "0"
	switch signature.Results().Len() {
	case 0:
		// do nothing
	case 1:
		resultSize = fmt.Sprintf("sizeof(%s)", createTypeName(signature.Results().At(0).Type()))
	default:
		resultSize = fmt.Sprintf("sizeof(%s)", createTypeName(signature.Results()))
	}

	ctx.switchFunctionToCallRuntimeApi(nextFunction, nextFunctionFrame, resumeFunction, nil,
		func() {
			receiver := fmt.Sprintf("%s.receiver", createValueRelName(callCommon.Value))
			receiverSize := fmt.Sprintf("%s.type_id.info->size", createValueRelName(callCommon.Value))
			fmt.Fprintf(ctx.stream, "memcpy(&next_frame->arg_buffer[0], %s, %s); // receiver: %s\n", receiver, receiverSize, signature.Recv())
			fmt.Fprintf(ctx.stream, "intptr_t num_arg_buffer_words = (%s + sizeof(next_frame->arg_buffer[0]) - 1) / sizeof(next_frame->arg_buffer[0]);\n", receiverSize)
			for i, arg := range callCommon.Args {
				argValue := createValueRelName(arg)
				argType := createTypeName(arg.Type())
				argPtr := fmt.Sprintf("ptr%d", i)
				fmt.Fprintf(ctx.stream, "%s* %s = (void*)&next_frame->arg_buffer[num_arg_buffer_words]; // param[%d]\n", argType, argPtr, i)
				fmt.Fprintf(ctx.stream, "*%s = %s;\n", argPtr, argValue)
				fmt.Fprintf(ctx.stream, "num_arg_buffer_words += (sizeof(%s) + sizeof(next_frame->arg_buffer[0]) - 1) / sizeof(next_frame->arg_buffer[0]);\n", argType)
			}
			fmt.Fprintf(ctx.stream, "next_frame->num_arg_buffer_words = num_arg_buffer_words;\n")
		},
		paramArgPair{param: "interface", arg: fmt.Sprintf("&%s", createValueRelName(callCommon.Value))},
		paramArgPair{param: "method_name", arg: fmt.Sprintf("(StringObject){.raw = \"%s\", .len = sizeof(\"%s\") - 1}", callCommon.Method.Name(), callCommon.Method.Name())},
		paramArgPair{param: "result_size", arg: resultSize},
	)
}
