package main

import (
	"fmt"
	"strings"

	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

func (ctx *Context) emitInstruction(instruction ssa.Instruction) {
	fmt.Fprintf(ctx.stream, "\t// %T (%s): %s\n", instruction, instruction.Parent(), instruction)
	fmt.Fprintf(ctx.stream, "\t{\n")
	switch instr := instruction.(type) {
	case *ssa.Alloc:
		if instr.Heap {
			result := createValueRelName(instr)
			ctx.switchFunctionToCallRuntimeApi("gox5_new", "StackFrameNew", createInstructionName(instr), &result, nil,
				paramArgPair{param: "size", arg: fmt.Sprintf("sizeof(%s)", createTypeName(instr.Type().(*types.Pointer).Elem()))},
			)
		} else {
			v := createValueRelName(instr)
			elemType := instr.Type().(*types.Pointer).Elem()
			fmt.Fprintf(ctx.stream, "%s_buf = (%s){};\n", v, createTypeName(elemType))
			fmt.Fprintf(ctx.stream, "%s* raw = &%s_buf;\n", createTypeName(elemType), v)
			fmt.Fprintf(ctx.stream, "%s = %s;\n", v, wrapInObject("raw", instr.Type()))
		}

	case *ssa.BinOp:
		needToCallRuntimeApi := false
		raw := ""
		switch op := instr.Op; op {
		case token.AND_NOT:
			raw = fmt.Sprintf("%s.raw & (~(%s.raw))", createValueRelName(instr.X), createValueRelName(instr.Y))
		case token.EQL, token.NEQ:
			equalFunc := fmt.Sprintf("equal_%s", createTypeName(instr.X.Type()))
			fmt.Fprintf(ctx.stream, "bool raw = %s(&%s, &%s) %s true;", equalFunc, createValueRelName(instr.X), createValueRelName(instr.Y), instr.Op)
			raw = "raw"
		case token.LSS, token.LEQ, token.GTR, token.GEQ:
			if t, ok := instr.X.Type().Underlying().(*types.Basic); ok && t.Kind() == types.String {
				raw = fmt.Sprintf("string_compare(&%s, &%s) %s 0",
					createValueRelName(instr.X), createValueRelName(instr.Y),
					instr.Op.String())
			} else {
				raw = fmt.Sprintf("%s.raw %s %s.raw", createValueRelName(instr.X), instr.Op.String(), createValueRelName(instr.Y))
			}
		case token.ADD:
			if t, ok := instr.Type().Underlying().(*types.Basic); ok && t.Kind() == types.String {
				result := createValueRelName(instr)
				ctx.switchFunctionToCallRuntimeApi("gox5_string_append", "StackFrameStringAppend", createInstructionName(instr), &result, nil,
					paramArgPair{param: "lhs", arg: createValueRelName(instr.X)},
					paramArgPair{param: "rhs", arg: createValueRelName(instr.Y)},
				)
				needToCallRuntimeApi = true
			} else if isSignedIntegerType(instr.Type()) {
				// Go wraps signed integer arithmetic at the type width;
				// compute via unsigned arithmetic so C does not invoke
				// undefined signed-overflow behavior (e.g. under UBSan).
				raw = fmt.Sprintf("(%s)((%s)%s.raw + (%s)%s.raw)",
					createRawTypeName(instr.Type()),
					"u"+createRawTypeName(instr.Type()), createValueRelName(instr.X),
					"u"+createRawTypeName(instr.Type()), createValueRelName(instr.Y))
			} else {
				raw = fmt.Sprintf("%s.raw %s %s.raw", createValueRelName(instr.X), instr.Op.String(), createValueRelName(instr.Y))
			}
		case token.SHL:
			var unsignedRawType string
			switch instr.Type().Underlying().(*types.Basic).Kind() {
			case types.Int, types.Int8, types.Int16, types.Int32, types.Int64:
				unsignedRawType = fmt.Sprintf("u%s", createRawTypeName(instr.X.Type()))
			case types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
				unsignedRawType = createRawTypeName(instr.X.Type())
			default:
				panic(fmt.Sprintf("%s", instr))
			}
			fmt.Fprintf(ctx.stream, "%s unsignedLhs = (%s)(%s.raw);\n", unsignedRawType, unsignedRawType, createValueRelName(instr.X))
			fmt.Fprintf(ctx.stream, "%s rhs = %s.raw;\n", createRawTypeName(instr.Y.Type()), createValueRelName(instr.Y))
			switch instr.Y.Type().Underlying().(*types.Basic).Kind() {
			case types.Int, types.Int8, types.Int16, types.Int32, types.Int64:
				fmt.Fprintln(ctx.stream, "assert(rhs>=0);")
			}
			raw = "(((size_t)rhs) < sizeof(unsignedLhs) * 8) ? (unsignedLhs << rhs) : 0"
		case token.SHR:
			var unsignedRawType string
			var overflowExpr string
			var calcExpr string
			bitLen := "sizeof(unsignedLhs) * 8"
			switch instr.Type().Underlying().(*types.Basic).Kind() {
			case types.Int, types.Int8, types.Int16, types.Int32, types.Int64:
				unsignedRawType = fmt.Sprintf("u%s", createRawTypeName(instr.X.Type()))
				overflowExpr = fmt.Sprintf("%s.raw < 0 ? ((%s)(-1)) : 0", createValueRelName(instr.X), unsignedRawType)
				calcExpr = fmt.Sprintf("rhs == 0 ? unsignedLhs : ((((%s) >> (%s - rhs)) << (%s - rhs)) | (unsignedLhs >> rhs))", overflowExpr, bitLen, bitLen)
			case types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
				unsignedRawType = createRawTypeName(instr.X.Type())
				overflowExpr = "0"
				calcExpr = "unsignedLhs >> rhs"
			default:
				panic(fmt.Sprintf("%s", instr))
			}
			fmt.Fprintf(ctx.stream, "%s unsignedLhs = (%s)(%s.raw);\n", unsignedRawType, unsignedRawType, createValueRelName(instr.X))
			fmt.Fprintf(ctx.stream, "%s rhs = %s.raw;\n", createRawTypeName(instr.Y.Type()), createValueRelName(instr.Y))
			switch instr.Y.Type().Underlying().(*types.Basic).Kind() {
			case types.Int, types.Int8, types.Int16, types.Int32, types.Int64:
				fmt.Fprintln(ctx.stream, "assert(rhs>=0);")
			}
			raw = fmt.Sprintf("((size_t)rhs) < %s ? (%s) : (%s)", bitLen, calcExpr, overflowExpr)
		default:
			if isSignedIntegerType(instr.Type()) && (instr.Op == token.SUB || instr.Op == token.MUL) {
				// See the token.ADD case: wrap signed arithmetic via unsigned.
				raw = fmt.Sprintf("(%s)((%s)%s.raw %s (%s)%s.raw)",
					createRawTypeName(instr.Type()),
					"u"+createRawTypeName(instr.Type()), createValueRelName(instr.X), instr.Op.String(),
					"u"+createRawTypeName(instr.Type()), createValueRelName(instr.Y))
			} else {
				raw = fmt.Sprintf("%s.raw %s %s.raw", createValueRelName(instr.X), instr.Op.String(), createValueRelName(instr.Y))
			}
		}
		if !needToCallRuntimeApi {
			fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInObject(raw, instr.Type()))
		}

	case *ssa.Call:
		callCommon := instr.Common()
		if callCommon.Method != nil {
			signature := callCommon.Method.Type().Underlying().(*types.Signature)
			result_ptr := "NULL"
			if signature.Results().Len() > 0 {
				result_ptr = fmt.Sprintf("&%s", createValueRelName(instr))
			}
			if callCommon.Method.Pkg() != nil && callCommon.Method.Pkg().Path() == "internal/reflectlite" {
				// Method call on the internal/reflectlite.Type interface. There
				// is no reflectlite support in the runtime; the value produced by
				// reflectlite.TypeOf is a zeroed interface, so a concrete
				// dispatch would fail. Provide fixed answers: Comparable() ->
				// true (all WithValue keys are comparable), everything else ->
				// zero. Bodies calling these (e.g. errors.init) are never
				// invoked at runtime; they only need to compile.
				result := createValueRelName(instr)
				switch callCommon.Method.Name() {
				case "Comparable":
					fmt.Fprintf(ctx.stream, "%s.raw = 1;\n", result)
				default:
					fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", result, result)
				}
				fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
				fmt.Fprintf(ctx.stream, "\t}\n")
				return
			}
			if callCommon.Method.Pkg() != nil && callCommon.Method.Pkg().Path() == "reflect" {
				if recvType, ok := signature.Recv().Type().(*types.Named); ok && recvType.Obj().Name() == "Type" &&
					recvType.Obj().Pkg() != nil && recvType.Obj().Pkg().Path() == "reflect" {
					// Method call on the reflect.Type interface. Such an interface value
					// is only ever fabricated by the reflect.TypeOf intercept (in
					// emitSpecialRuntimeCall), which stores the underlying type_id in the
					// receiver field. Route Kind() and String() to the runtime so fmt's
					// spacing logic and the %T verb work; any other reflect.Type method
					// falls back to zero.
					result := createValueRelName(instr)
					typeId := fmt.Sprintf("(uintptr_t)%s.receiver", createValueRelName(callCommon.Value))
					switch callCommon.Method.Name() {
					case "Kind":
						// reflect.Kind is a named unsigned int; the runtime writes the kind as
						// an i32.
						resultPtr := fmt.Sprintf("%s.raw", result)
						ctx.switchFunctionToCallRuntimeApi("gox5_reflect_type_kind", "StackFrameReflectTypeKind", createInstructionName(instr), &resultPtr,
							nil,
							paramArgPair{param: "type_id", arg: typeId})
					case "String":
						ctx.switchFunctionToCallRuntimeApi("gox5_reflect_type_string", "StackFrameReflectTypeString", createInstructionName(instr), &result,
							nil,
							paramArgPair{param: "type_id", arg: typeId})
					default:
						fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", result, result)
						fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
						fmt.Fprintf(ctx.stream, "\t}\n")
						return
					}
					fmt.Fprintf(ctx.stream, "\t}\n")
					return
				}
			}
			ctx.switchFunctionToCallRuntimeApi("gox5_interface_invoke", "StackFrameInterfaceInvoke", createInstructionName(instr), nil,
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
				paramArgPair{param: "result_ptr", arg: result_ptr},
				paramArgPair{param: "interface", arg: fmt.Sprintf("&%s", createValueRelName(callCommon.Value))},
				paramArgPair{param: "method_name", arg: fmt.Sprintf("(StringObject){.raw = \"%s\", .len = sizeof(\"%s\") - 1}", callCommon.Method.Name(), callCommon.Method.Name())},
			)
		} else {
			switch callee := callCommon.Value.(type) {
			case *ssa.Builtin:
				complexNumberBitLength := func(v ssa.Value) uint {
					switch v.Type().Underlying().(*types.Basic).Kind() {
					case types.Complex64:
						return 64
					case types.Complex128:
						return 128
					default:
						panic("unreachable")
					}
				}

				switch callee.Name() {
				case "Add":
					result := createValueRelName(instr)
					ptr := fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))
					offset := fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[1]))
					raw := fmt.Sprintf("(void*)((uintptr_t)%s + (uintptr_t)%s)", ptr, offset)
					fmt.Fprintf(ctx.stream, "%s = %s;\n", result, wrapInObject(raw, instr.Type()))

				case "append":
					result := createValueRelName(instr)
					result += ".raw"
					if t, ok := callCommon.Args[1].Type().Underlying().(*types.Basic); ok && t.Kind() == types.String {
						if instr.Type().Underlying().(*types.Slice).Elem().(*types.Basic).Kind() != types.Byte {
							panic(instr.String())
						}
						ctx.switchFunctionToCallRuntimeApi("gox5_slice_append_string", "StackFrameSliceAppendString", createInstructionName(instr), &result, nil,
							paramArgPair{param: "slice", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
							paramArgPair{param: "string", arg: createValueRelName(callCommon.Args[1])},
						)
					} else {
						ctx.switchFunctionToCallRuntimeApi("gox5_slice_append", "StackFrameSliceAppend", createInstructionName(instr), &result, nil,
							paramArgPair{param: "type_id", arg: wrapInTypeId(callCommon.Args[0].Type().Underlying().(*types.Slice).Elem())},
							paramArgPair{param: "lhs", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
							paramArgPair{param: "rhs", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[1]))},
						)
					}

				case "cap":
					result := createValueRelName(instr)
					ctx.switchFunctionToCallRuntimeApi("gox5_slice_capacity", "StackFrameSliceCapacity", createInstructionName(instr), &result, nil,
						paramArgPair{param: "slice", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
					)

				case "clear":
					switch t := callCommon.Args[0].Type().Underlying().(type) {
					case *types.Map:
						ctx.switchFunctionToCallRuntimeApi("gox5_map_clear", "StackFrameMapClear", createInstructionName(instr), nil, nil,
							paramArgPair{param: "map", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
						)
					case *types.Slice:
						slice := createValueRelName(callCommon.Args[0])
						elemType := t.Elem()
						fmt.Fprintf(ctx.stream, "if (%s.raw.size != 0) { memset(%s.raw.addr, 0, %s.raw.size * sizeof(%s)); }\n", slice, slice, slice, createTypeName(elemType))
					default:
						panic(fmt.Sprintf("unsupported argument for clear: %s (%s)", callCommon.Args[0], t))
					}

				case "close":
					ctx.switchFunctionToCallRuntimeApi("gox5_channel_close", "StackFrameChannelClose", createInstructionName(instr), nil, nil,
						paramArgPair{param: "channel", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
					)

				case "copy":
					result := createValueRelName(instr)
					if t, ok := callCommon.Args[1].Type().Underlying().(*types.Basic); ok && t.Kind() == types.String {
						ctx.switchFunctionToCallRuntimeApi("gox5_slice_copy_string", "StackFrameSliceCopyString", createInstructionName(instr), &result, nil,
							paramArgPair{param: "src", arg: createValueRelName(callCommon.Args[1])},
							paramArgPair{param: "dst", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
						)
					} else {
						ctx.switchFunctionToCallRuntimeApi("gox5_slice_copy", "StackFrameSliceCopy", createInstructionName(instr), &result, nil,
							paramArgPair{param: "type_id", arg: wrapInTypeId(callCommon.Args[0].Type().Underlying().(*types.Slice).Elem())},
							paramArgPair{param: "src", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[1]))},
							paramArgPair{param: "dst", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
						)
					}

				case "complex":
					bitLength := complexNumberBitLength(instr)
					result := createValueRelName(instr)
					ctx.switchFunctionToCallRuntimeApi(
						fmt.Sprintf("gox5_complex%d_new", bitLength),
						fmt.Sprintf("StackFrameComplex%dNew", bitLength),
						createInstructionName(instr), &result, nil,
						paramArgPair{param: "real", arg: createValueRelName(callCommon.Args[0])},
						paramArgPair{param: "imaginary", arg: createValueRelName(callCommon.Args[1])},
					)

				case "delete":
					ctx.switchFunctionToCallRuntimeApi("gox5_map_delete", "StackFrameMapDelete", createInstructionName(instr), nil, nil,
						paramArgPair{param: "map", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
						paramArgPair{param: "key", arg: fmt.Sprintf("&%s", createValueRelName(callCommon.Args[1]))},
					)

				case "imag":
					bitLength := complexNumberBitLength(callCommon.Args[0])
					result := createValueRelName(instr)
					ctx.switchFunctionToCallRuntimeApi(
						fmt.Sprintf("gox5_complex%d_component", bitLength),
						fmt.Sprintf("StackFrameComplex%dComponent", bitLength),
						createInstructionName(instr), &result, nil,
						paramArgPair{param: "value", arg: createValueRelName(callCommon.Args[0])},
						paramArgPair{param: "is_real", arg: "false"},
					)

				case "len":
					switch t := callCommon.Args[0].Type().Underlying().(type) {
					case *types.Basic:
						switch t.Kind() {
						case types.String:
							result := createValueRelName(instr)
							ctx.switchFunctionToCallRuntimeApi("gox5_string_length", "StackFrameStringLength", createInstructionName(instr), &result, nil,
								paramArgPair{param: "string", arg: createValueRelName(callCommon.Args[0])},
							)
						default:
							panic(fmt.Sprintf("unsuported argument for len: %s (%s)", callCommon.Args[0], t))
						}
					case *types.Map:
						result := createValueRelName(instr)
						ctx.switchFunctionToCallRuntimeApi("gox5_map_len", "StackFrameMapLen", createInstructionName(instr), &result, nil,
							paramArgPair{param: "map", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
						)
					case *types.Slice:
						result := createValueRelName(instr)
						ctx.switchFunctionToCallRuntimeApi("gox5_slice_size", "StackFrameSliceSize", createInstructionName(instr), &result, nil,
							paramArgPair{param: "slice", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
						)
					default:
						panic(fmt.Sprintf("unsuported argument for len: %s", callCommon.Args[0]))
					}

				case "max", "min":
					result := createValueRelName(instr)
					arg := callCommon.Args[0]
					basic, ok := arg.Type().Underlying().(*types.Basic)
					isString := ok && basic.Kind() == types.String
					if !ok || (!isNumericKind(basic.Kind()) && !isString) {
						panic(fmt.Sprintf("unsupported argument for %s: %s (%s)", callee.Name(), arg, arg.Type()))
					}
					op := ">"
					if callee.Name() == "min" {
						op = "<"
					}
					expr := createValueRelName(arg)
					if !isString {
						expr = fmt.Sprintf("%s.raw", expr)
					}
					for _, next := range callCommon.Args[1:] {
						if isString {
							rhs := createValueRelName(next)
							expr = fmt.Sprintf("(string_compare(&%s, &%s) %s 0 ? %s : %s)", expr, rhs, op, expr, rhs)
						} else {
							rhs := fmt.Sprintf("%s.raw", createValueRelName(next))
							expr = fmt.Sprintf("(%s %s %s ? %s : %s)", expr, op, rhs, expr, rhs)
						}
					}
					if isString {
						fmt.Fprintf(ctx.stream, "%s = %s;\n", result, expr)
					} else {
						fmt.Fprintf(ctx.stream, "%s = %s;\n", result, wrapInObject(expr, instr.Type()))
					}

				case "print", "println":
					rawExprs := make([]string, len(callCommon.Args))
					lenExprs := make([]string, len(callCommon.Args))
					for i, arg := range callCommon.Args {
						rawExprs[i] = fmt.Sprintf("%s.raw", createValueRelName(arg))
						lenExprs[i] = fmt.Sprintf("%s.len", createValueRelName(arg))
					}
					emitInlinePrintBody(ctx.stream, callee.Name() == "print", callCommon.Args, rawExprs, lenExprs)

				case "real":
					bitLength := complexNumberBitLength(callCommon.Args[0])
					result := createValueRelName(instr)
					ctx.switchFunctionToCallRuntimeApi(
						fmt.Sprintf("gox5_complex%d_component", bitLength),
						fmt.Sprintf("StackFrameComplex%dComponent", bitLength),
						createInstructionName(instr), &result, nil,
						paramArgPair{param: "value", arg: createValueRelName(callCommon.Args[0])},
						paramArgPair{param: "is_real", arg: "true"},
					)

				case "recover":
					result := createValueRelName(instr)
					ctx.switchFunctionToCallRuntimeApi("gox5_panic_recover", "StackFramePanicRecover", createInstructionName(instr), &result, nil)

				case "ssa:wrapnilchk":
					result := createValueRelName(instr)
					ctx.switchFunctionToCallRuntimeApi("gox5_check_non_nil", "StackFrameCheckNonNil", createInstructionName(instr), &result, nil,
						paramArgPair{param: "pointer", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
					)

				case "Sizeof":
					result := createValueRelName(instr)
					expr := fmt.Sprintf("sizeof(%s)", createTypeName(callCommon.Args[0].Type()))
					fmt.Fprintf(ctx.stream, "%s = %s;\n", result, wrapInObject(expr, instr.Type()))

				case "Slice":
					result := createValueRelName(instr)
					ptr := fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))
					length := fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[1]))
					raw := fmt.Sprintf("(SliceObject){.addr = %s, .size = (uintptr_t)%s, .capacity = (uintptr_t)%s}", ptr, length, length)
					fmt.Fprintf(ctx.stream, "%s = %s;\n", result, wrapInObject(raw, instr.Type()))

				case "SliceData":
					result := createValueRelName(instr)
					elemType := instr.Type().Underlying().(*types.Pointer).Elem()
					raw := fmt.Sprintf("(%s*)%s.raw.addr", createTypeName(elemType), createValueRelName(callCommon.Args[0]))
					fmt.Fprintf(ctx.stream, "%s = %s;\n", result, wrapInObject(raw, instr.Type()))

				case "String":
					result := createValueRelName(instr)
					ptr := fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))
					length := fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[1]))
					byteSlice := fmt.Sprintf("(SliceObject){.addr = %s, .size = (uintptr_t)%s, .capacity = (uintptr_t)%s}", ptr, length, length)
					ctx.switchFunctionToCallRuntimeApi("gox5_string_new_from_byte_slice", "StackFrameStringNewFromByteSlice", createInstructionName(instr), &result, nil,
						paramArgPair{param: "byte_slice", arg: byteSlice},
					)

				case "StringData":
					result := createValueRelName(instr)
					elemType := instr.Type().Underlying().(*types.Pointer).Elem()
					raw := fmt.Sprintf("(%s*)%s.raw", createTypeName(elemType), createValueRelName(callCommon.Args[0]))
					fmt.Fprintf(ctx.stream, "%s = %s;\n", result, wrapInObject(raw, instr.Type()))

				default:
					panic(fmt.Sprintf("unsuported builtin function: %s (in %s)", callee.Name(), createFunctionName(instr.Parent())))
				}
				fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))

			default:
				if ctx.emitAtomicCall(instr, callCommon) {
					break
				}
				if function, ok := callee.(*ssa.Function); ok {
					if ctx.emitSpecialRuntimeCall(function, instr, callCommon) {
						break
					}
				}
				nextFunction := createValueRelName(callee)
				if function, ok := callee.(*ssa.Function); ok {
					if target := mathArchImplementation(function); target != nil {
						nextFunction = createValueRelName(target)
					}
				}
				signature := callCommon.Value.Type().Underlying().(*types.Signature)
				signatureName := createSignatureName(signature, false, false)
				ctx.switchFunction(nextFunction, signature, signatureName, createValueRelName(instr), createInstructionName(instr), func() {
					paramBase := 0
					argBase := 0
					if signature.Recv() != nil {
						receiver := createValueRelName(callCommon.Args[0])
						fmt.Fprintf(ctx.stream, "signature->param0 = %s; // receiver: %s\n", receiver, signature.Recv())
						paramBase++
						argBase++
					}
					for i := 0; i < signature.Params().Len(); i++ {
						arg := callCommon.Args[argBase+i]
						fmt.Fprintf(ctx.stream, "signature->param%d = %s; // param: %d\n", paramBase+i, createValueRelName(arg), i)
					}
				})
			}
		}

	case *ssa.ChangeInterface:
		fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), createValueRelName(instr.X))

	case *ssa.ChangeType:
		if t, ok := instr.Type().Underlying().(*types.Basic); ok && (t.Kind() == types.String || t.Kind() == types.UntypedString) {
			fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), createValueRelName(instr.X))
		} else {
			s := wrapInObject(fmt.Sprintf("%s.raw", createValueRelName(instr.X)), instr.Type())
			fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), s)
		}

	case *ssa.Convert:
		switch dstType := instr.Type().Underlying().(type) {
		case *types.Basic:
			switch dstType.Kind() {
			case types.String:
				result := createValueRelName(instr)
				switch srcType := instr.X.Type().Underlying().(type) {
				case *types.Basic:
					arg := fmt.Sprintf("(IntObject){%s.raw}", createValueRelName(instr.X))
					ctx.switchFunctionToCallRuntimeApi("gox5_string_new_from_rune", "StackFrameStringNewFromRune", createInstructionName(instr), &result, nil,
						paramArgPair{param: "rune", arg: arg},
					)
				case *types.Slice:
					if elemType, ok := srcType.Elem().(*types.Basic); ok {
						switch elemType.Kind() {
						case types.Byte:
							arg := fmt.Sprintf("%s.raw", createValueRelName(instr.X))
							ctx.switchFunctionToCallRuntimeApi("gox5_string_new_from_byte_slice", "StackFrameStringNewFromByteSlice", createInstructionName(instr), &result, nil,
								paramArgPair{param: "byte_slice", arg: arg},
							)
						case types.Rune:
							arg := fmt.Sprintf("%s.raw", createValueRelName(instr.X))
							ctx.switchFunctionToCallRuntimeApi("gox5_string_new_from_rune_slice", "StackFrameStringNewFromRuneSlice", createInstructionName(instr), &result, nil,
								paramArgPair{param: "rune_slice", arg: arg},
							)
						default:
							panic(fmt.Sprintf("%s, %s, %s (%T)", instr, srcType, elemType, elemType))
						}
					} else {
						panic(fmt.Sprintf("%s, %s, %s (%T)", instr, srcType, elemType, elemType))
					}
				default:
					panic(fmt.Sprintf("%s, %s (%T)", instr, srcType, srcType))
				}

			case types.Uintptr:
				var raw string
				if srcType, ok := instr.X.Type().Underlying().(*types.Basic); ok && srcType.Kind() == types.UnsafePointer {
					raw = fmt.Sprintf("(uintptr_t)%s.raw", createValueRelName(instr.X))
				} else {
					raw = fmt.Sprintf("%s.raw", createValueRelName(instr.X))
				}
				fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInObject(raw, instr.Type()))

			case types.UnsafePointer:
				var raw string
				if srcType, ok := instr.X.Type().Underlying().(*types.Basic); ok && srcType.Kind() == types.Uintptr {
					raw = fmt.Sprintf("(void*)%s.raw", createValueRelName(instr.X))
				} else {
					raw = fmt.Sprintf("%s.raw", createValueRelName(instr.X))
				}
				fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInObject(raw, instr.Type()))

			default:
				raw := fmt.Sprintf("%s.raw", createValueRelName(instr.X))
				fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInObject(raw, instr.Type()))
			}

		case *types.Slice:
			elemType := dstType.Elem().Underlying().(*types.Basic)
			switch elemType.Kind() {
			case types.Byte, types.Rune:
				// valid conversion
			default:
				panic(instr.String())
			}
			srcType := instr.X.Type().Underlying().(*types.Basic)
			if srcType.Kind() != types.String {
				panic(instr.String())
			}
			result := fmt.Sprintf("%s.raw", createValueRelName(instr))
			ctx.switchFunctionToCallRuntimeApi("gox5_slice_from_string", " StackFrameSliceFromString", createInstructionName(instr), &result, nil,
				paramArgPair{param: "type_id", arg: wrapInTypeId(elemType)},
				paramArgPair{param: "src", arg: createValueRelName(instr.X)},
			)

		default:
			raw := fmt.Sprintf("%s.raw", createValueRelName(instr.X))
			fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInObject(raw, instr.Type()))
		}

	case *ssa.Defer:
		ctx.emitGoOrDefer(instr, instr.Common(), "gox5_defer_register", "StackFrameDeferRegister", "gox5_defer_register_invoke", "StackFrameDeferRegisterInvoke", true)

	case *ssa.Extract:
		fmt.Fprintf(ctx.stream, "%s = %s.raw.e%d;\n", createValueRelName(instr), createValueRelName(instr.Tuple), instr.Index)

	case *ssa.Field:
		index := instr.Field
		name := createFieldName(instr.X.Type().Underlying().(*types.Struct).Field(index), index)
		fmt.Fprintf(ctx.stream, "%s val = %s.%s;\n", createTypeName(instr.Type()), createValueRelName(instr.X), name)
		fmt.Fprintf(ctx.stream, "%s = val;\n", createValueRelName(instr))

	case *ssa.FieldAddr:
		index := instr.Field
		name := createFieldName(instr.X.Type().Underlying().(*types.Pointer).Elem().Underlying().(*types.Struct).Field(index), index)
		fmt.Fprintf(ctx.stream, "%s* raw = &(%s.raw->%s);\n", createTypeName(instr.Type().(*types.Pointer).Elem()), createValueRelName(instr.X), name)
		fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInObject("raw", instr.Type()))

	case *ssa.Index:
		fmt.Fprintf(ctx.stream, "uintptr_t index = %s.raw;\n", createValueRelName(instr.Index))
		switch t := instr.X.Type().Underlying().(type) {
		case *types.Array:
			fmt.Fprintf(ctx.stream, "%s val = %s.raw[index];\n", createTypeName(t.Elem()), createValueRelName(instr.X))
		case *types.Basic:
			if t.Kind() == types.String || t.Kind() == types.UntypedString {
				fmt.Fprintf(ctx.stream, "%s val = { .raw = (uint8_t)%s.raw[index] };\n", createTypeName(instr.Type()), createValueRelName(instr.X))
			} else {
				panic(fmt.Sprintf("%s, %s, %s", instr, instr.X, t))
			}
		default:
			panic(fmt.Sprintf("%s, %s, %s", instr, instr.X, t))
		}
		fmt.Fprintf(ctx.stream, "%s = val;\n", createValueRelName(instr))

	case *ssa.IndexAddr:
		fmt.Fprintf(ctx.stream, "uintptr_t index = %s.raw;\n", createValueRelName(instr.Index))
		switch t := instr.X.Type().Underlying().(type) {
		case *types.Slice:
			fmt.Fprintf(ctx.stream, "%s* raw = &((%s.typed.ptr)[index]);\n", createTypeName(t.Elem()), createValueRelName(instr.X))
		case *types.Pointer:
			fmt.Fprintf(ctx.stream, "%s* raw = &(%s.raw->raw[index]);\n", createTypeName(t.Elem().Underlying().(*types.Array).Elem()), createValueRelName(instr.X))
		default:
			panic(fmt.Sprintf("%s, %s, %s", instr, instr.X, t))
		}
		fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInObject("raw", instr.Type()))

	case *ssa.Go:
		ctx.emitGoOrDefer(instr, instr.Common(), "gox5_lwt_spawn", "StackFrameLwtSpawn", "gox5_lwt_spawn_invoke", "StackFrameLwtSpawnInvoke", false)

	case *ssa.If:
		fmt.Fprintf(ctx.stream, "\treturn %s.raw ? %s : %s;\n", createValueRelName(instr.Cond),
			wrapInFunctionObject(createBasicBlockName(instr.Block().Succs[0])),
			wrapInFunctionObject(createBasicBlockName(instr.Block().Succs[1])))

	case *ssa.Jump:
		fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createBasicBlockName(instr.Block().Succs[0])))

	case *ssa.Lookup:
		switch xt := instr.X.Type().Underlying().(type) {
		case *types.Basic:
			switch xt.Kind() {
			case types.String, types.UntypedString:
				raw := fmt.Sprintf("%s.raw[%s.raw]", createValueRelName(instr.X), createValueRelName(instr.Index))
				fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInObject(raw, instr.Type()))
			default:
				panic(fmt.Sprintf("%s", instr))
			}
		case *types.Map:
			result := createValueRelName(instr)
			key := fmt.Sprintf("&%s", createValueRelName(instr.Index))
			var value, found string
			if instr.CommaOk {
				value = fmt.Sprintf("&%s.raw.e0", result)
				found = fmt.Sprintf("&%s.raw.e1.raw", result)
			} else {
				value = fmt.Sprintf("&%s", result)
				found = "NULL"
			}
			valueType := instr.X.Type().Underlying().(*types.Map).Elem()
			ctx.switchFunctionToCallRuntimeApi("gox5_map_get", "StackFrameMapGet", createInstructionName(instr), nil,
				func() {
					fmt.Fprintf(ctx.stream, "if (next_frame->map.raw == NULL) {\n")
					fmt.Fprintf(ctx.stream, "\tmemset(next_frame->value, 0, sizeof(%s));\n", createTypeName(valueType))
					fmt.Fprintf(ctx.stream, "}\n")
				},
				paramArgPair{param: "map", arg: fmt.Sprintf("%s.raw", createValueRelName(instr.X))},
				paramArgPair{param: "key", arg: key},
				paramArgPair{param: "value", arg: value},
				paramArgPair{param: "found", arg: found},
			)
		default:
			panic(fmt.Sprintf("%s", instr))
		}

	case *ssa.MakeChan:
		result := fmt.Sprintf("%s.raw", createValueRelName(instr))
		ctx.switchFunctionToCallRuntimeApi("gox5_channel_new", "StackFrameChannelNew", createInstructionName(instr), &result, nil,
			paramArgPair{param: "type_id", arg: wrapInTypeId(instr.Type().Underlying().(*types.Chan).Elem())},
			paramArgPair{param: "capacity", arg: createValueRelName(instr.Size)},
		)

	case *ssa.MakeClosure:
		fn := instr.Fn.(*ssa.Function)
		if len(fn.FreeVars) != len(instr.Bindings) {
			panic(fmt.Sprintf("invalid closure invocation: freeVars=%s, bindings=%s", fn, instr.Bindings))
		}
		result := createValueRelName(instr)
		userFunction := fmt.Sprintf("(UserFunction){.func_ptr = %s}", createFunctionName(fn))
		ctx.switchFunctionToCallRuntimeApi("gox5_closure_new", "StackFrameClosureNew", createInstructionName(instr), &result,
			func() {
				fnName := createFunctionName(fn)
				fmt.Fprintf(ctx.stream, "FreeVars_%s* free_vars = (FreeVars_%s*)&next_frame->object_ptrs;\n", fnName, fnName)
				if strings.HasSuffix(fn.Name(), "$bound") {
					if len(fn.FreeVars) != 1 {
						panic(fmt.Sprintf("fn: %s, free_vars: %s", fn, fn.FreeVars))
					}
					fmt.Fprintf(ctx.stream, "free_vars->receiver = %s;\n", createValueRelName(instr.Bindings[0]))
				} else {
					for i, freeVar := range fn.FreeVars {
						val := instr.Bindings[i]
						fmt.Fprintf(ctx.stream, "free_vars->%s = %s;\n", createValueName(freeVar), createValueRelName(val))
					}
				}
				fmt.Fprintf(ctx.stream, "next_frame->num_object_ptrs = sizeof(*free_vars) / sizeof(intptr_t);\n")
			},
			paramArgPair{param: "user_function", arg: userFunction},
		)

	case *ssa.MakeInterface:
		result := createValueRelName(instr)
		receiverArg := fmt.Sprintf("&%s", createValueRelName(instr.X))
		if _, ok := instr.X.(*ssa.Function); ok {
			// See the makeInterfaceReceiver field comment in
			// emitFunctionVariableStructure: keep the function object in a stable
			// frame slot before handing its address to the runtime.
			fmt.Fprintf(ctx.stream, "frame->makeInterfaceReceiver = %s;\n", createValueRelName(instr.X))
			receiverArg = "&frame->makeInterfaceReceiver"
		} else if _, ok := instr.X.(*ssa.Global); ok {
			// A package global is referenced through an inline compound literal,
			// so taking its address would point at a machine-stack slot that is
			// dead by the time gox5_interface_new reads the receiver. Keep a
			// stable copy in the frame instead (see emitFunctionVariableStructure).
			fmt.Fprintf(ctx.stream, "frame->makeInterfaceReceiver_%s = %s;\n", createValueName(instr), createValueRelName(instr.X))
			receiverArg = fmt.Sprintf("&frame->makeInterfaceReceiver_%s", createValueName(instr))
		}
		ctx.switchFunctionToCallRuntimeApi("gox5_interface_new", "StackFrameInterfaceNew", createInstructionName(instr), &result, nil,
			paramArgPair{param: "receiver", arg: receiverArg},
			paramArgPair{param: "type_id", arg: wrapInTypeId(instr.X.Type())},
		)

	case *ssa.MakeMap:
		result := fmt.Sprintf("%s.raw", createValueRelName(instr))
		ctx.switchFunctionToCallRuntimeApi("gox5_map_new", "StackFrameMapNew", createInstructionName(instr), &result, nil,
			paramArgPair{param: "key_type", arg: wrapInTypeId(instr.Type().Underlying().(*types.Map).Key())},
			paramArgPair{param: "value_type", arg: wrapInTypeId(instr.Type().Underlying().(*types.Map).Elem())},
		)

	case *ssa.MakeSlice:
		result := createValueRelName(instr)
		fmt.Fprintf(ctx.stream, "%s.typed.size = %s.raw;\n", result, createValueRelName(instr.Len))
		fmt.Fprintf(ctx.stream, "%s.typed.capacity = %s.raw;\n", result, createValueRelName(instr.Cap))
		ptr := fmt.Sprintf("%s.typed.ptr", result)
		size := fmt.Sprintf("(%s.raw) * sizeof(%s)", createValueRelName(instr.Cap), createTypeName(instr.Type().Underlying().(*types.Slice).Elem()))
		ctx.switchFunctionToCallRuntimeApi("gox5_new", "StackFrameNew", createInstructionName(instr), &ptr, nil,
			paramArgPair{param: "size", arg: size},
		)

	case *ssa.MapUpdate:
		ctx.switchFunctionToCallRuntimeApi("gox5_map_set", "StackFrameMapSet", createInstructionName(instr), nil, nil,
			paramArgPair{param: "map", arg: fmt.Sprintf("%s.raw", createValueRelName(instr.Map))},
			paramArgPair{param: "key", arg: fmt.Sprintf("&%s", createValueRelName(instr.Key))},
			paramArgPair{param: "value", arg: fmt.Sprintf("&%s", createValueRelName(instr.Value))},
		)

	case *ssa.Next:
		result := createValueRelName(instr)
		iter := createValueRelName(instr.Iter)
		rng := fmt.Sprintf("&%s.raw.e1", result)
		if t, ok := instr.Type().(*types.Tuple).At(1).Type().(*types.Basic); ok {
			if t.Kind() == types.Invalid {
				rng = "NULL"
			}
		}
		dom := fmt.Sprintf("&%s.raw.e2", result)
		if t, ok := instr.Type().(*types.Tuple).At(2).Type().(*types.Basic); ok {
			if t.Kind() == types.Invalid {
				dom = "NULL"
			}
		}
		found := fmt.Sprintf("&%s.raw.e0.raw", result)
		count := fmt.Sprintf("&%s.count", iter)
		if instr.IsString {
			mp := fmt.Sprintf("%s.obj.string", iter)
			ctx.switchFunctionToCallRuntimeApi("gox5_string_next", "StackFrameStringNext", createInstructionName(instr), nil, nil,
				paramArgPair{param: "string", arg: mp},
				paramArgPair{param: "index", arg: rng},
				paramArgPair{param: "rune", arg: dom},
				paramArgPair{param: "found", arg: found},
				paramArgPair{param: "count", arg: count},
			)
		} else {
			mp := fmt.Sprintf("%s.obj.map", iter)
			ctx.switchFunctionToCallRuntimeApi("gox5_map_next", "StackFrameMapNext", createInstructionName(instr), nil, nil,
				paramArgPair{param: "map", arg: mp},
				paramArgPair{param: "key", arg: rng},
				paramArgPair{param: "value", arg: dom},
				paramArgPair{param: "found", arg: found},
				paramArgPair{param: "count", arg: count},
			)
		}

	case *ssa.Panic:
		ctx.switchFunctionToCallRuntimeApi("gox5_panic_raise", "StackFramePanicRaise", "NULL", nil, nil,
			paramArgPair{param: "value", arg: createValueRelName(instr.X)},
		)

	case *ssa.Phi:
		panic("unreachable: phis are emitted by emitBlockPhis")

	case *ssa.Range:
		if _, ok := instr.X.Type().(*types.Map); ok {
			fmt.Fprintf(ctx.stream, "%s = (IterObject){.obj = {.map = %s.raw}};\n", createValueRelName(instr), createValueRelName(instr.X))
		} else {
			fmt.Fprintf(ctx.stream, "%s = (IterObject){.obj = {.string = %s}};\n", createValueRelName(instr), createValueRelName(instr.X))
		}

	case *ssa.Return:
		fmt.Fprintf(ctx.stream, "ctx->stack_pointer = frame->common.prev_stack_pointer;\n")
		switch len(instr.Results) {
		case 0:
			// do nothing
		case 1:
			fmt.Fprintf(ctx.stream, "*frame->signature.result_ptr = %s;\n", createValueRelName(instr.Results[0]))
		default:
			for i, v := range instr.Results {
				fmt.Fprintf(ctx.stream, "frame->signature.result_ptr->raw.e%d = %s;\n", i, createValueRelName(v))
			}
		}
		fmt.Fprintf(ctx.stream, "return frame->common.resume_func;\n")

	case *ssa.RunDefers:
		ctx.switchFunctionToCallRuntimeApi("gox5_defer_execute", "StackFrameDeferExecute", createInstructionName(instr), nil, nil)

	case *ssa.Select:
		result := createValueRelName(instr)
		ctx.switchFunctionToCallRuntimeApi("gox5_channel_select", "StackFrameChannelSelect", createInstructionName(instr), nil,
			func() {
				receive_count := 0
				for i, state := range instr.States {
					fmt.Fprintf(ctx.stream, "next_frame->entry_buffer[%d].channel = %s.raw;\n", i, createValueRelName(state.Chan))
					fmt.Fprintf(ctx.stream, "next_frame->entry_buffer[%d].type_id = %s;\n", i, wrapInTypeId(state.Chan.Type().(*types.Chan).Elem()))
					switch state.Dir {
					case types.SendRecv:
						panic("unreachable")
					case types.SendOnly:
						fmt.Fprintf(ctx.stream, "next_frame->entry_buffer[%d].send_data = &%s;\n", i, createValueRelName(state.Send))
						fmt.Fprintf(ctx.stream, "next_frame->entry_buffer[%d].receive_data = NULL;\n", i)
					case types.RecvOnly:
						fmt.Fprintf(ctx.stream, "next_frame->entry_buffer[%d].send_data = NULL;\n", i)
						fmt.Fprintf(ctx.stream, "next_frame->entry_buffer[%d].receive_data = &%s.raw.e%d;\n", i, result, receive_count+2)
						receive_count += 1
					}
				}
			},
			paramArgPair{param: "selected_index", arg: fmt.Sprintf("&%s.raw.e0", result)},
			paramArgPair{param: "receive_available", arg: fmt.Sprintf("&%s.raw.e1", result)},
			paramArgPair{param: "need_block", arg: fmt.Sprintf("%t", instr.Blocking)},
			paramArgPair{param: "entry_count", arg: fmt.Sprintf("%d", len(instr.States))},
		)

	case *ssa.Send:
		ctx.switchFunctionToCallRuntimeApi("gox5_channel_send", "StackFrameChannelSend", createInstructionName(instr), nil, nil,
			paramArgPair{param: "channel", arg: fmt.Sprintf("%s.raw", createValueRelName(instr.Chan))},
			paramArgPair{param: "data", arg: fmt.Sprintf("&%s", createValueRelName(instr.X))},
			paramArgPair{param: "type_id", arg: wrapInTypeId(instr.X.Type())},
		)

	case *ssa.Slice:
		if t, ok := instr.Type().Underlying().(*types.Basic); ok {
			if t.Kind() != types.String {
				panic(fmt.Sprintf("%s (%T)", t, t))
			}
			result := createValueRelName(instr)
			low := "-1"
			if instr.Low != nil {
				low = fmt.Sprintf("%s.raw", createValueRelName(instr.Low))
			}
			high := "-1"
			if instr.High != nil {
				high = fmt.Sprintf("%s.raw", createValueRelName(instr.High))
			}
			ctx.switchFunctionToCallRuntimeApi("gox5_string_substr", "StackFrameStringSubstr", createInstructionName(instr), &result, nil,
				paramArgPair{param: "base", arg: createValueRelName(instr.X)},
				paramArgPair{param: "low", arg: low},
				paramArgPair{param: "high", arg: high},
			)
		} else {
			startIndex := "0"
			if instr.Low != nil {
				startIndex = fmt.Sprintf("%s.raw", createValueRelName(instr.Low))
			}

			ptr := ""
			endIndexDefault := ""
			capacityDefault := ""
			switch t := instr.X.Type().Underlying().(type) {
			case *types.Pointer:
				ptr = "raw->raw"
				elemType := t.Elem().Underlying().(*types.Array)
				length := fmt.Sprintf("%d", elemType.Len())
				endIndexDefault = length
				capacityDefault = length
			case *types.Slice:
				ptr = "typed.ptr"
				endIndexDefault = fmt.Sprintf("%s.typed.size", createValueRelName(instr.X))
				capacityDefault = fmt.Sprintf("%s.typed.capacity", createValueRelName(instr.X))
			default:
				panic(fmt.Sprintf("not implemented: %s (%T)", t, t))
			}

			endIndex := endIndexDefault
			if instr.High != nil {
				endIndex = fmt.Sprintf("%s.raw", createValueRelName(instr.High))
			}

			capacity := capacityDefault
			if instr.Max != nil {
				capacity = fmt.Sprintf("%s.raw", createValueRelName(instr.Max))
			}

			fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInObject("0", instr.Type()))
			fmt.Fprintf(ctx.stream, "%s.typed.ptr = %s.%s + %s;\n", createValueRelName(instr), createValueRelName(instr.X), ptr, startIndex)
			fmt.Fprintf(ctx.stream, "%s.typed.size = %s - %s;\n", createValueRelName(instr), endIndex, startIndex)
			fmt.Fprintf(ctx.stream, "%s.typed.capacity = %s - %s;\n", createValueRelName(instr), capacity, startIndex)
		}

	case *ssa.SliceToArrayPointer:
		// (*[N]T)(x): yields a pointer to the underlying array of slice x.
		// A run-time panic occurs if len(x) is less than the array length.
		arrPtrType := instr.Type().Underlying().(*types.Pointer)
		arrType := arrPtrType.Elem().Underlying().(*types.Array)
		n := arrType.Len()
		if n != 0 {
			fmt.Fprintf(ctx.stream, "if (%s.typed.size < %d) { assert(false && \"slice length too short to convert to array pointer\"); }\n", createValueRelName(instr.X), n)
		}
		fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), wrapInObject(fmt.Sprintf("(%s*)%s.typed.ptr", createTypeName(arrPtrType.Elem()), createValueRelName(instr.X)), instr.Type()))

	case *ssa.Store:
		fmt.Fprintf(ctx.stream, "*(%s.raw) = %s;\n", createValueRelName(instr.Addr), createValueRelName(instr.Val))

	case *ssa.TypeAssert:
		result := createValueRelName(instr)
		var value, success string
		if instr.CommaOk {
			value = fmt.Sprintf("&%s.raw.e0", result)
			success = fmt.Sprintf("&%s.raw.e1.raw", result)
		} else {
			value = fmt.Sprintf("&%s", result)
			success = "NULL"
		}
		var nextFunction, nextFunctionFrame string
		if _, ok := instr.AssertedType.Underlying().(*types.Interface); ok {
			nextFunction = "gox5_interface_convert_to_interface"
			nextFunctionFrame = "StackFrameInterfaceConvertToInterface"
		} else {
			nextFunction = "gox5_interface_convert_to_concrete_type"
			nextFunctionFrame = "StackFrameInterfaceConvertToConcreteType"
		}
		ctx.switchFunctionToCallRuntimeApi(nextFunction, nextFunctionFrame, createInstructionName(instr), nil, nil,
			paramArgPair{param: "interface", arg: fmt.Sprintf("&%s", createValueRelName(instr.X))},
			paramArgPair{param: "to_type", arg: wrapInTypeId(instr.AssertedType)},
			paramArgPair{param: "value", arg: value},
			paramArgPair{param: "success", arg: success},
		)

	case *ssa.UnOp:
		if instr.Op == token.ARROW {
			result := createValueRelName(instr)
			var typeId, data, available string
			if instr.CommaOk {
				typeId = wrapInTypeId(instr.Type().(*types.Tuple).At(0).Type())
				data = fmt.Sprintf("&%s.raw.e0", result)
				available = fmt.Sprintf("&%s.raw.e1.raw", result)
			} else {
				typeId = wrapInTypeId(instr.Type())
				data = fmt.Sprintf("&%s", result)
				available = "NULL"
			}
			ctx.switchFunctionToCallRuntimeApi("gox5_channel_receive", "StackFrameChannelReceive", createInstructionName(instr), nil, nil,
				paramArgPair{param: "channel", arg: fmt.Sprintf("%s.raw", createValueRelName(instr.X))},
				paramArgPair{param: "type_id", arg: typeId},
				paramArgPair{param: "data", arg: data},
				paramArgPair{param: "available", arg: available},
			)
		} else if instr.Op == token.XOR {
			s := wrapInObject(fmt.Sprintf("~(%s.raw)", createValueRelName(instr.X)), instr.Type())
			fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), s)
		} else {
			s := fmt.Sprintf("%s (%s.raw)", instr.Op.String(), createValueRelName(instr.X))
			if instr.Op != token.MUL {
				s = wrapInObject(s, instr.Type())
			}
			fmt.Fprintf(ctx.stream, "%s = %s;\n", createValueRelName(instr), s)
		}

	default:
		panic(fmt.Sprintf("unknown instruction: %s", instruction.String()))
	}
	fmt.Fprintf(ctx.stream, "\t}\n")
}
