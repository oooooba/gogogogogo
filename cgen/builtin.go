package main

import (
	"fmt"
	"io"
	"strings"

	"go/constant"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

func (ctx *Context) emitSpecialRuntimeCall(callee *ssa.Function, instr *ssa.Call, callCommon *ssa.CallCommon) bool {
	pkgPath := ""
	if callee.Pkg != nil {
		pkgPath = callee.Pkg.Pkg.Path()
	}
	funcName := callee.Name()

	if pkgPath == "runtime" {
		switch funcName {
		case "SetFinalizer":
			// The runtime has no finalizer support; this is a no-op so that
			// callers such as os.newFile are emitted instead of being skipped.
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "beforeExit":
			// runtime_beforeExit is invoked by os.Exit; it has no effect on the
			// C runtime (no race detector / coverage). Treat as a no-op.
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "KeepAlive":
			// runtime.KeepAlive is a compiler hint with no runtime effect.
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		}
	}
	if pkgPath == "internal/syscall/unix" {
		switch funcName {
		case "fcntl":
			// internal/syscall/unix.fcntl is the Go wrapper around the runtime
			// fcntl syscall. fcntl is not supported, so return a zeroed
			// (val, errno) tuple; callers treat val != -1 as success.
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "%s.raw.e0 = (Int32Object){.raw = 0};\n", result)
			fmt.Fprintf(ctx.stream, "%s.raw.e1 = (Int32Object){.raw = 0};\n", result)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		}
	}
	if pkgPath == "os" {
		switch funcName {
		case "runtime_args":
			// os.runtime_args forwards to runtime.args, which is not supported.
			// Return an empty []string; the os.Stdin/Stdout/Stderr tests do not
			// depend on os.Args.
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "%s.raw = (SliceObject){0};\n", result)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "runtime_beforeExit":
			// os.runtime_beforeExit forwards to runtime.beforeExit (race/coverage
			// hooks). There is no effect on the C runtime; treat as a no-op.
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		}
	}
	if pkgPath == "syscall" {
		switch funcName {
		case "Exit":
			// syscall.Exit forwards to the (unsupported) runtime exit. Exit the
			// whole process directly; os.Exit is expected to terminate.
			arg := createValueRelName(callCommon.Args[0])
			fmt.Fprintf(ctx.stream, "exit(%s.raw);\n", arg)
			return true
		case "write":
			// syscall.write(fd int, p []byte) (n int, err error) is the low-level
			// wrapper around the write(2) syscall. Map it directly to the C
			// write(2) call so that os.File.Write on real file descriptors works.
			fd := createValueRelName(callCommon.Args[0])
			p := createValueRelName(callCommon.Args[1])
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "ssize_t written = write(%s.raw, %s.raw.addr, %s.raw.size);\n", fd, p, p)
			fmt.Fprintf(ctx.stream, "%s.raw.e0 = (IntObject){.raw = written < 0 ? -1 : (long)written};\n", result)
			fmt.Fprintf(ctx.stream, "%s.raw.e1 = (Interface){0};\n", result)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "read":
			// syscall.read(fd int, p []byte) (n int, err error) is the low-level
			// wrapper around the read(2) syscall. Map it directly to the C
			// read(2) call so that os.File.Read on real file descriptors works.
			// A zero-length buffer reads nothing (and avoids feeding a NULL
			// pointer with length 0, which some platforms treat as an error).
			fd := createValueRelName(callCommon.Args[0])
			p := createValueRelName(callCommon.Args[1])
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "ssize_t got = %s.raw.size == 0 ? 0 : read(%s.raw, %s.raw.addr, %s.raw.size);\n", p, fd, p, p)
			fmt.Fprintf(ctx.stream, "%s.raw.e0 = (IntObject){.raw = got < 0 ? -1 : (long)got};\n", result)
			fmt.Fprintf(ctx.stream, "%s.raw.e1 = (Interface){0};\n", result)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "openat":
			// syscall.openat(dirfd int, path string, flags int, mode uint32)
			// (fd int, err error) is the low-level wrapper around the openat(2)
			// syscall. Map it directly to the C openat(2) call so that os.Open on
			// real file descriptors works. Strings used as paths come from
			// literals, whose .raw is NUL-terminated.
			fd := createValueRelName(callCommon.Args[0])
			path := createValueRelName(callCommon.Args[1])
			flags := createValueRelName(callCommon.Args[2])
			mode := createValueRelName(callCommon.Args[3])
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "int opened_fd = openat((int)%s.raw, %s.raw, (int)%s.raw, (unsigned int)%s.raw);\n", fd, path, flags, mode)
			fmt.Fprintf(ctx.stream, "%s.raw.e0 = (IntObject){.raw = opened_fd};\n", result)
			fmt.Fprintf(ctx.stream, "%s.raw.e1 = (Interface){0};\n", result)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "close", "Close":
			// syscall.close(fd int) (err error) / syscall.Close(fd int) error map to
			// the C close(2) call; the error is not propagated (nil) for simplicity.
			fd := createValueRelName(callCommon.Args[0])
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "close((int)%s.raw);\n", fd)
			fmt.Fprintf(ctx.stream, "%s = (Interface){0};\n", result)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "pipe2":
			// syscall.pipe2(p *[2]_C_int, flags int) (err error) maps to the C
			// pipe2(2) call, which writes the read and write ends of a new pipe
			// into p[0] and p[1]. The error is not propagated (nil) for
			// simplicity.
			p0 := createValueRelName(callCommon.Args[0])
			flags := createValueRelName(callCommon.Args[1])
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "int pipe_rc = pipe2((int*)%s.raw, (int)%s.raw); (void)pipe_rc;\n", p0, flags)
			fmt.Fprintf(ctx.stream, "%s = (Interface){0};\n", result)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "runtime_entersyscall", "runtime_exitsyscall":
			// syscall.runtime_entersyscall/runtime_exitsyscall are linknames to
			// runtime.entersyscall/exitsyscall, the M/P state bookkeeping around
			// a system call. The C runtime is effectively single-threaded with no
			// such state, so treat both as no-ops so raw-syscall wrappers can run.
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "Syscall":
			// syscall.Syscall(trap, a1, a2, a3) (r1, r2 uintptr, err syscall.Errno)
			// is the 4-argument raw syscall wrapper. os.Open/Read/Close passes
			// through here only for syscalls not already intercepted by name
			// (openat/read/fcntl above). Handle the syscalls the standard
			// library needs here; abort on anything else so we notice it.
			if cst, ok := callCommon.Args[0].(*ssa.Const); ok {
				trap, _ := constant.Int64Val(cst.Value)
				switch trap {
				case 3: // SYS_CLOSE
					// close(fd) (r1=0, r2=0, err=nil)
					a1 := createValueRelName(callCommon.Args[1])
					result := createValueRelName(instr)
					fmt.Fprintf(ctx.stream, "close((int)%s.raw);\n", a1)
					fmt.Fprintf(ctx.stream, "%s.raw.e0 = (UintptrObject){.raw=0};\n", result)
					fmt.Fprintf(ctx.stream, "%s.raw.e1 = (UintptrObject){.raw=0};\n", result)
					fmt.Fprintf(ctx.stream, "%s.raw.e2 = (UintptrObject){.raw=0};\n", result)
					fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
					return true
				case 263: // SYS_UNLINKAT (used by os.Remove)
					// unlinkat(dirfd, path, flags) (r1=0, r2=0, err=nil)
					a1 := createValueRelName(callCommon.Args[1])
					a2 := createValueRelName(callCommon.Args[2])
					a3 := createValueRelName(callCommon.Args[3])
					result := createValueRelName(instr)
					fmt.Fprintf(ctx.stream, "unlinkat((int)%s.raw, (const char*)%s.raw, (int)%s.raw);\n", a1, a2, a3)
					fmt.Fprintf(ctx.stream, "%s.raw.e0 = (UintptrObject){.raw=0};\n", result)
					fmt.Fprintf(ctx.stream, "%s.raw.e1 = (UintptrObject){.raw=0};\n", result)
					fmt.Fprintf(ctx.stream, "%s.raw.e2 = (UintptrObject){.raw=0};\n", result)
					fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
					return true
				}
			}
			// Unknown/unsupported raw syscall: abort so it is detected.
			fmt.Fprintf(ctx.stream, "assert(false && \"unsupported syscall.Syscall trap\");\n")
			return true
		case "fcntl":
			// syscall.fcntl(fd int, cmd int, arg int) (val int, err error) is the
			// low-level wrapper around fcntl(2). It is used by the poll runtime to
			// query/clear the non-blocking flag when a file is opened. Map it to
			// the C fcntl(2) call; the error is not propagated (nil) for simplicity.
			fd := createValueRelName(callCommon.Args[0])
			cmd := createValueRelName(callCommon.Args[1])
			arg := createValueRelName(callCommon.Args[2])
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "int fcntl_val = fcntl((int)%s.raw, %s.raw, (long)%s.raw);\n", fd, cmd, arg)
			fmt.Fprintf(ctx.stream, "%s.raw.e0 = (IntObject){.raw = fcntl_val};\n", result)
			fmt.Fprintf(ctx.stream, "%s.raw.e1 = (Interface){0};\n", result)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		}
	}

	if pkgPath == "internal/poll" {
		// (*poll.FD).Init sets up the netpoller binding for freshly-opened file
		// descriptors. The C runtime has no netpoller. Rather than emulate its
		// runtime_poll* entry points, skip the binding entirely: leave
		// fd.pd.runtimeCtx at 0 so pollDesc.prepare()/pollable() treat the fd as
		// a plain blocking descriptor. A read on a regular file then proceeds
		// directly through the intercepted syscall.read.
		switch funcName {
		case "Init":
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "%s = (Interface){0};\n", result)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "runtime_Semacquire", "runtime_Semrelease":
			// These are linknames to runtime.semacquire/semrelease, used by the
			// fdMutex to coordinate fd operations (e.g. during FD.Close). The C
			// runtime is single-threaded and there are no concurrent fd ops, so
			// both are no-ops.
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		}
	}

	if pkgPath == "internal/testlog" {
		switch funcName {
		case "PanicOnExit0":
			// For a non-test binary there is no registered test log, so this
			// returns false (never panic on os.Exit(0)).
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "%s.raw = false;\n", result)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "Open":
			// testlog.Open records the file being opened for the test log.
			// In a non-test binary there is no log; it has no return value, so
			// treat it as a no-op to keep os.Open generated.
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		}
	}
	if pkgPath == "iter" {
		switch funcName {
		case "newcoro":
			result := createValueRelName(instr)
			ctx.switchFunctionToCallRuntimeApi("gox5_coro_new", "StackFrameCoroNew", createInstructionName(instr), &result, nil,
				paramArgPair{param: "function_object", arg: createValueRelName(callCommon.Args[0])},
			)
			return true
		case "coroswitch":
			ctx.switchFunctionToCallRuntimeApi("gox5_coro_switch", "StackFrameCoroSwitch", createInstructionName(instr), nil, nil,
				paramArgPair{param: "coro", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
			)
			return true
		}
	}

	if pkgPath == "maps" {
		switch funcName {
		case "clone":
			result := createValueRelName(instr)
			ctx.switchFunctionToCallRuntimeApi("gox5_map_clone", "StackFrameMapClone", createInstructionName(instr), &result, nil,
				paramArgPair{param: "map", arg: createValueRelName(callCommon.Args[0])},
			)
			return true
		}
	}

	if pkgPath == "internal/abi" {
		switch funcName {
		case "NoEscape":
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "%s = %s;\n", result, createValueRelName(callCommon.Args[0]))
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		}
	}

	if pkgPath == "internal/bytealg" {
		switch funcName {
		case "Compare":
			result := createValueRelName(instr)
			ctx.switchFunctionToCallRuntimeApi("gox5_slice_compare", "StackFrameSliceCompare", createInstructionName(instr), &result, nil,
				paramArgPair{param: "lhs", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
				paramArgPair{param: "rhs", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[1]))},
			)
			return true
		case "CompareString":
			// bytealg.CompareString has a Go body whose abigen linkname
			// (abigen_runtime_cmpstring) is a stub, so compare the strings'
			// bytes via gox5_slice_compare.
			result := createValueRelName(instr)
			a := createValueRelName(callCommon.Args[0])
			b := createValueRelName(callCommon.Args[1])
			ctx.switchFunctionToCallRuntimeApi("gox5_slice_compare", "StackFrameSliceCompare", createInstructionName(instr), &result, nil,
				paramArgPair{param: "lhs", arg: fmt.Sprintf("(SliceObject){.addr = (void*)%s.raw, .size = %s.len, .capacity = %s.len}", a, a, a)},
				paramArgPair{param: "rhs", arg: fmt.Sprintf("(SliceObject){.addr = (void*)%s.raw, .size = %s.len, .capacity = %s.len}", b, b, b)},
			)
			return true
		case "Count":
			result := createValueRelName(instr)
			ctx.switchFunctionToCallRuntimeApi("gox5_slice_count", "StackFrameSliceCount", createInstructionName(instr), &result, nil,
				paramArgPair{param: "b", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
				paramArgPair{param: "c", arg: createValueRelName(callCommon.Args[1])},
			)
			return true
		case "CountString":
			// bytealg.CountString has no Go body on amd64 (assembly-backed),
			// so route it to gox5_slice_count over the string's bytes.
			result := createValueRelName(instr)
			s := createValueRelName(callCommon.Args[0])
			ctx.switchFunctionToCallRuntimeApi("gox5_slice_count", "StackFrameSliceCount", createInstructionName(instr), &result, nil,
				paramArgPair{param: "b", arg: fmt.Sprintf("(SliceObject){.addr = (void*)%s.raw, .size = %s.len, .capacity = %s.len}", s, s, s)},
				paramArgPair{param: "c", arg: createValueRelName(callCommon.Args[1])},
			)
			return true
		case "Index":
			result := createValueRelName(instr)
			ctx.switchFunctionToCallRuntimeApi("gox5_slice_search_slice", "StackFrameSliceSearchSlice", createInstructionName(instr), &result, nil,
				paramArgPair{param: "lhs", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
				paramArgPair{param: "rhs", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[1]))},
			)
			return true
		case "IndexByte":
			result := createValueRelName(instr)
			ctx.switchFunctionToCallRuntimeApi("gox5_slice_search_byte", "StackFrameSliceSearchByte", createInstructionName(instr), &result, nil,
				paramArgPair{param: "b", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
				paramArgPair{param: "c", arg: createValueRelName(callCommon.Args[1])},
			)
			return true
		case "IndexByteString":
			result := createValueRelName(instr)
			ctx.switchFunctionToCallRuntimeApi("gox5_string_search_byte", "StackFrameStringSearchByte", createInstructionName(instr), &result, nil,
				paramArgPair{param: "string", arg: createValueRelName(callCommon.Args[0])},
				paramArgPair{param: "byte", arg: createValueRelName(callCommon.Args[1])},
			)
			return true
		case "IndexString":
			result := createValueRelName(instr)
			ctx.switchFunctionToCallRuntimeApi("gox5_string_search_string", "StackFrameStringSearchString", createInstructionName(instr), &result, nil,
				paramArgPair{param: "lhs", arg: createValueRelName(callCommon.Args[0])},
				paramArgPair{param: "rhs", arg: createValueRelName(callCommon.Args[1])},
			)
			return true
		case "MakeNoZero":
			result := fmt.Sprintf("%s.raw", createValueRelName(instr))
			ctx.switchFunctionToCallRuntimeApi("gox5_slice_new_uninitialized", "StackFrameSliceNewUninitialized", createInstructionName(instr), &result, nil,
				paramArgPair{param: "n", arg: createValueRelName(callCommon.Args[0])},
			)
			return true
		}
	}

	if pkgPath == "errors" && funcName == "init" {
		fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
		return true
	}
	if pkgPath == "sync" && funcName == "init" {
		fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
		return true
	}
	if pkgPath == "internal/cpu" && funcName == "init" {
		fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
		return true
	}
	if pkgPath == "syscall" && funcName == "init" {
		// syscall.init would run runtime_envs() and rlimit setup that call into
		// the skipped runtime package (runtime_envs, Getrlimit via Syscall) and
		// abort. No code in the supported tests uses the syscall environment or
		// rlimit state, so skip the whole initializer.
		fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
		return true
	}

	if pkgPath == "internal/godebug" {
		switch funcName {
		case "New":
			// godebug.New is called by time.init (asynctimerchan) and other
			// package initializers to register a GODEBUG setting. The runtime
			// has no GODEBUG support, so return a fresh zeroed *Setting; the
			// (*Setting).Value accessor is intercepted below to return "".
			result := createValueRelName(instr)
			ctx.switchFunctionToCallRuntimeApi("gox5_new", "StackFrameNew", createInstructionName(instr), &result, nil,
				paramArgPair{param: "size", arg: fmt.Sprintf("sizeof(%s)", createTypeName(instr.Type().Underlying().(*types.Pointer).Elem()))},
			)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "Value":
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", result, result)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "IncNonDefault":
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case "init":
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		default:
			panic(fmt.Sprintf("unexpected call to internal/godebug.%s (called from %s)", funcName, createFunctionName(instr.Parent())))
		}
	}

	if pkgPath == "internal/reflectlite" && funcName == "TypeOf" {
		// context.WithValue checks reflectlite.TypeOf(key).Comparable(); there is
		// no reflectlite support in the runtime, so return a zeroed Type
		// interface. Interface method invocations on reflectlite.Type are
		// intercepted in the *ssa.Call emission below.
		result := createValueRelName(instr)
		fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", result, result)
		fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
		return true
	}

	if pkgPath == "reflect" || pkgPath == "runtime" {
		switch {
		case pkgPath == "reflect" && funcName == "TypeOf":
			// fmt's pp.doPrint calls reflect.TypeOf(arg).Kind() on every
			// argument to decide whether to insert a space, and %T calls
			// reflect.TypeOf(arg).String(). reflect.Type is not supported in the
			// runtime, so fabricate a reflect.Type interface whose receiver field
			// carries the argument's type_id. The Kind()/String() interface
			// methods are intercepted during *ssa.Call emission below.
			result := createValueRelName(instr)
			arg := createValueRelName(callCommon.Args[0])
			fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", result, result)
			fmt.Fprintf(ctx.stream, "%s.receiver = (void*)(uintptr_t)%s.type_id.id;\n", result, arg)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case pkgPath == "reflect" && funcName == "ValueOf":
			// reflect.ValueOf inspects the interface's type word and data pointer
			// through the unsafe abi.EmptyInterface layout. The runtime stores a
			// FunctionObject for function-valued interfaces, so build a
			// reflect.Value whose ptr field carries the function's raw address.
			result := createValueRelName(instr)
			arg := createValueRelName(callCommon.Args[0])
			fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", result, result)
			fmt.Fprintf(ctx.stream, "if (%s.receiver != NULL) {\n", arg)
			fmt.Fprintf(ctx.stream, "\t%s.ptr.raw = (void*)((FunctionObject*)%s.receiver)->raw;\n", result, arg)
			fmt.Fprintf(ctx.stream, "}\n")
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case pkgPath == "reflect" && funcName == "Pointer":
			// (reflect.Value).Pointer returns the data pointer stored in the Value.
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "%s.raw = (uintptr_t)%s.ptr.raw;\n", result, createValueRelName(callCommon.Args[0]))
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case pkgPath == "reflect" && funcName == "rtypeOf":
			// reflect.rtypeOf is called from reflect.init to initialize package
			// variables (stringType, bytesType); its body calls internal/abi.TypeOf
			// which is a skipped stub. Return a zeroed *abi.Type.
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", result, result)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case pkgPath == "runtime" && funcName == "FuncForPC":
			// runtime.FuncForPC looks the code address up in the function name
			// registry emitted into shared_definition.c and returns a
			// *runtime.Func pointing into it (or NULL when unknown).
			result := createValueRelName(instr)
			pc := fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))
			fmt.Fprintf(ctx.stream, "%s.raw = (void*)gox5_runtime_func_for_pc(%s);\n", result, pc)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		case pkgPath == "runtime" && funcName == "Name":
			// (runtime.Func).Name returns the registered name for the function
			// address that the *runtime.Func points at.
			result := createValueRelName(instr)
			recv := fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))
			fmt.Fprintf(ctx.stream, "%s = gox5_runtime_func_name((const UserFunctionInfo*)%s);\n", result, recv)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		}
	}

	// sync and internal/sync declare their blocking/waiting primitives as
	// linkname stubs (runtime_Semacquire*, runtime_Semrelease, runtime_canSpin,
	// runtime_doSpin, runtime_nanotime, runtime_rand, throw, fatal). Route calls
	// to them into the Rust runtime instead of the assert(false) stubs.
	{
		origin := callee
		if o := callee.Origin(); o != nil {
			origin = o
		}
		originPkgPath := ""
		if origin.Pkg != nil {
			originPkgPath = origin.Pkg.Pkg.Path()
		}
		originFuncName := origin.Name()

		if originPkgPath == "internal/synctest" {
			switch originFuncName {
			case "IsInBubble", "IsAssociated", "Associate":
				result := createValueRelName(instr)
				fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", result, result)
			case "Disassociate", "init":
				// no-op
			default:
				panic(fmt.Sprintf("unexpected call to internal/synctest.%s (called from %s)", originFuncName, createFunctionName(instr.Parent())))
			}
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		}

		if originPkgPath == "sync/atomic" && (originFuncName == "runtime_procPin" || originFuncName == "runtime_procUnpin") {
			// sync/atomic.Value (value.go) uses these linkname stubs to disable
			// preemption around the first Store; single-threaded runtime: no-op.
			if callCommon.Signature().Results().Len() > 0 {
				result := createValueRelName(instr)
				fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", result, result)
			}
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		}

		if originPkgPath == "sync" || originPkgPath == "internal/sync" {
			switch originFuncName {
			case "runtime_Semacquire", "runtime_SemacquireWaitGroup", "runtime_SemacquireMutex", "runtime_SemacquireRWMutex", "runtime_SemacquireRWMutexR":
				ctx.switchFunctionToCallRuntimeApi("gox5_semaphore_acquire", "StackFrameSemaphoreAcquire", createInstructionName(instr), nil, nil,
					paramArgPair{param: "s", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
				)
				return true
			case "runtime_Semrelease":
				ctx.switchFunctionToCallRuntimeApi("gox5_semaphore_release", "StackFrameSemaphoreRelease", createInstructionName(instr), nil, nil,
					paramArgPair{param: "s", arg: fmt.Sprintf("%s.raw", createValueRelName(callCommon.Args[0]))},
				)
				return true
			case "runtime_canSpin", "runtime_nanotime", "runtime_rand":
				result := createValueRelName(instr)
				fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", result, result)
				fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
				return true
			case "runtime_doSpin":
				fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
				return true
			case "runtime_procPin":
				// Single-threaded runtime: pin to P 0 (pool local index 0).
				result := createValueRelName(instr)
				fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", result, result)
				fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
				return true
			case "runtime_procUnpin":
				fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
				return true
			case "runtime_LoadAcquintptr":
				// Acquire load of a uintptr through the given pointer (used by sync.Pool).
				result := createValueRelName(instr)
				fmt.Fprintf(ctx.stream, "%s.raw = atomic_load_explicit((_Atomic uintptr_t*)%s.raw, memory_order_acquire);\n", result, createValueRelName(callCommon.Args[0]))
				fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
				return true
			case "runtime_StoreReluintptr":
				// Release store of a uintptr through the given pointer (used by sync.Pool).
				fmt.Fprintf(ctx.stream, "atomic_store_explicit((_Atomic uintptr_t*)%s.raw, %s.raw, memory_order_release);\n", createValueRelName(callCommon.Args[0]), createValueRelName(callCommon.Args[1]))
				fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
				return true
			case "throw", "fatal":
				fmt.Fprintf(ctx.stream, "assert(false);\n")
				fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
				return true
			}
		}
	}

	if pkgPath == "math/rand" && funcName == "runtime_rand" {
		// math/rand.runtime_rand is a //go:linkname to runtime.rand with no Go
		// body, used to seed the global rand source. Route it into the Rust
		// runtime which returns a deterministic pseudo-random uint64.
		result := createValueRelName(instr)
		ctx.switchFunctionToCallRuntimeApi("gox5_runtime_rand", "StackFrameRuntimeRand", createInstructionName(instr), &result, nil)
		return true
	}

	if pkgPath == "time" && funcName == "runtimeNano" {
		// time.runtimeNano is a //go:linkname to runtime.nanotime with no Go
		// body; return 0 (single-threaded runtime has no real clock source).
		result := createValueRelName(instr)
		fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", result, result)
		fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
		return true
	}

	if isFunctionBodySkippedPackagePath(pkgPath) {
		if funcName == "init" {
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		}

		if pkgPath == "runtime" && funcName == "Goexit" {
			ctx.switchFunctionToCallRuntimeApi("gox5_lwt_exit", "StackFrameLwtExit", "NULL", nil, nil)
			return true
		}
		if pkgPath == "runtime" && funcName == "Gosched" {
			ctx.switchFunctionToCallRuntimeApi("gox5_lwt_yield", "StackFrameLwtYield", createInstructionName(instr), nil, nil)
			return true
		}
		if pkgPath == "runtime" && funcName == "GOMAXPROCS" {
			// sync.Pool.pinSlow asks for the number of Ps; report 1.
			result := createValueRelName(instr)
			fmt.Fprintf(ctx.stream, "memset(&%s, 0, sizeof(%s));\n", result, result)
			fmt.Fprintf(ctx.stream, "%s.raw = 1;\n", result)
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		}

		if strings.HasPrefix(pkgPath, "internal/race") ||
			strings.HasPrefix(pkgPath, "internal/msan") ||
			strings.HasPrefix(pkgPath, "internal/asan") {
			fmt.Fprintf(ctx.stream, "\treturn %s;\n", wrapInFunctionObject(createInstructionName(instr)))
			return true
		}

		panic(fmt.Sprintf("unexpected call to function in skipped package: %s.%s (called from %s)", pkgPath, funcName, createFunctionName(instr.Parent())))
	}

	return false
}

func (ctx *Context) emitBuiltinPrintWrapper(name string, callCommon *ssa.CallCommon, instr ssa.Instruction) string {
	if ctx.builtinPrintWrapperNames == nil {
		ctx.builtinPrintWrapperNames = make(map[*ssa.CallCommon]string)
	}
	if cached, ok := ctx.builtinPrintWrapperNames[callCommon]; ok {
		return cached
	}

	wrapperName := fmt.Sprintf("builtin_print_wrapper_%s", createInstructionName(instr))
	ctx.builtinPrintWrapperNames[callCommon] = wrapperName

	fmt.Fprintf(&ctx.builtinPrintWrapperBuf, "__attribute__((unused)) static FunctionObject %s(LightWeightThreadContext* ctx) {\n", wrapperName)
	fmt.Fprintf(&ctx.builtinPrintWrapperBuf, "\tassert(ctx->marker == 0xdeadbeef);\n")
	fmt.Fprintf(&ctx.builtinPrintWrapperBuf, "\tStackFrameCommon* args_frame = (StackFrameCommon*)ctx->stack_pointer;\n")
	fmt.Fprintf(&ctx.builtinPrintWrapperBuf, "\tuintptr_t* arg_words = (uintptr_t*)(args_frame + 1);\n")
	fmt.Fprintf(&ctx.builtinPrintWrapperBuf, "\tStackFrameCommon* prev = (StackFrameCommon*)args_frame->prev_stack_pointer;\n")
	fmt.Fprintf(&ctx.builtinPrintWrapperBuf, "\tuintptr_t arg_word_index = 0;\n")

	for i, arg := range callCommon.Args {
		argType := createTypeName(arg.Type())
		fmt.Fprintf(&ctx.builtinPrintWrapperBuf, "\t%s arg%d_val = *(%s*)&arg_words[arg_word_index];\n", argType, i, argType)
		fmt.Fprintf(&ctx.builtinPrintWrapperBuf, "\targ_word_index += sizeof(%s) / sizeof(arg_words[0]);\n", argType)
	}

	rawExprs := make([]string, len(callCommon.Args))
	lenExprs := make([]string, len(callCommon.Args))
	for i := range callCommon.Args {
		rawExprs[i] = fmt.Sprintf("arg%d_val.raw", i)
		lenExprs[i] = fmt.Sprintf("arg%d_val.len", i)
	}
	emitInlinePrintBody(&ctx.builtinPrintWrapperBuf, name == "print", callCommon.Args, rawExprs, lenExprs)

	fmt.Fprintf(&ctx.builtinPrintWrapperBuf, "\tctx->stack_pointer = prev;\n")
	fmt.Fprintf(&ctx.builtinPrintWrapperBuf, "\treturn (FunctionObject){.raw = gox5_defer_execute};\n")
	fmt.Fprintf(&ctx.builtinPrintWrapperBuf, "}\n")

	return wrapperName
}

func emitInlinePrintBody(w io.Writer, packs bool, args []ssa.Value, rawExprs []string, lenExprs []string) {
	var emitInlinePrintArgByType func(w io.Writer, t types.Type, rawExpr string, lenExpr string)
	emitInlinePrintArgByType = func(w io.Writer, t types.Type, rawExpr string, lenExpr string) {
		switch t := t.(type) {
		case *types.Basic:
			switch t.Kind() {
			case types.Bool:
				fmt.Fprintf(w, "\tfprintf(stderr, \"%%s\", %s ? \"true\" : \"false\");\n", rawExpr)
			case types.Complex64, types.Complex128:
				fmt.Fprintf(w, "\tfprintf(stderr, \"(\");\n")
				fmt.Fprintf(w, "\tfprintf(stderr, \"%%.15g\", creal(%s) == 0.0 && signbit(creal(%s)) ? 0.0 : creal(%s));\n", rawExpr, rawExpr, rawExpr)
				fmt.Fprintf(w, "\tif (!signbit(cimag(%s))) fprintf(stderr, \"+\");\n", rawExpr)
				fmt.Fprintf(w, "\tfprintf(stderr, \"%%.15g\", cimag(%s) == 0.0 && signbit(cimag(%s)) ? 0.0 : cimag(%s));\n", rawExpr, rawExpr, rawExpr)
				fmt.Fprintf(w, "\tfprintf(stderr, \"i)\");\n")
			case types.Int, types.Int64:
				fmt.Fprintf(w, "\tfprintf(stderr, \"%%ld\", (long)(%s));\n", rawExpr)
			case types.Int8, types.Int16, types.Int32:
				fmt.Fprintf(w, "\tfprintf(stderr, \"%%d\", (int)(%s));\n", rawExpr)
			case types.Uint, types.Uint64, types.Uintptr:
				fmt.Fprintf(w, "\tfprintf(stderr, \"%%lu\", (unsigned long)(%s));\n", rawExpr)
			case types.Uint8, types.Uint16, types.Uint32:
				fmt.Fprintf(w, "\tfprintf(stderr, \"%%u\", (unsigned int)(%s));\n", rawExpr)
			case types.Float32, types.Float64:
				fmt.Fprintf(w, "\tfprintf(stderr, \"%%.15g\", (%s) == 0.0 && signbit((%s)) ? 0.0 : (%s));\n", rawExpr, rawExpr, rawExpr)
			case types.String:
				fmt.Fprintf(w, "\tif (%s) { fprintf(stderr, \"%%.*s\", (int)(%s), %s); }\n", rawExpr, lenExpr, rawExpr)
			case types.UnsafePointer:
				fmt.Fprintf(w, "\tfprintf(stderr, \"%%p\", %s);\n", rawExpr)
			default:
				panic(fmt.Sprintf("unsupported type for print/println: %s (%T)", t, t))
			}
		case *types.Named:
			emitInlinePrintArgByType(w, t.Underlying(), rawExpr, lenExpr)
		case *types.Pointer:
			fmt.Fprintf(w, "\tfprintf(stderr, \"%%p\", (void*)(%s));\n", rawExpr)
		default:
			panic(fmt.Sprintf("unsupported type for print/println: %s (%T)", t, t))
		}
	}
	for i, arg := range args {
		if !packs && i != 0 {
			fmt.Fprintf(w, "\tfprintf(stderr, \" \");\n")
		}
		emitInlinePrintArgByType(w, arg.Type(), rawExprs[i], lenExprs[i])
	}
	if !packs {
		fmt.Fprintf(w, "\tfprintf(stderr, \"\\n\");\n")
	}
}
