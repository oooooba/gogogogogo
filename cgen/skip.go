package main

import (
	"strings"

	"golang.org/x/tools/go/ssa"
)

func isFunctionBodySkippedPackage(pkg *ssa.Package) bool {
	return isFunctionBodySkippedPackagePath(pkg.Pkg.Path())
}

func isFunctionBodySkippedPackagePath(path string) bool {
	if path == "runtime" || path == "internal/race" || path == "internal/abi" {
		return true
	}
	if path == "internal/cpu" || path == "internal/strconv" || path == "internal/stringslite" || path == "internal/bytealg" || path == "internal/sync" || path == "internal/oserror" {
		return false
	}
	if path == "internal/poll" || path == "internal/intern" || path == "internal/unsafeheader" {
		return false
	}
	if path == "internal/syscall/unix" {
		return false
	}
	if strings.HasPrefix(path, "internal/") || strings.HasPrefix(path, "runtime/internal/") {
		return true
	}
	return false
}

func isFunctionBodySkipped(fn *ssa.Function) bool {
	if origin := fn.Origin(); origin != nil {
		fn = origin
	}
	if fn.Pkg != nil && isFunctionBodySkippedPackagePath(fn.Pkg.Pkg.Path()) {
		return true
	}
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			var common *ssa.CallCommon
			switch instr := instr.(type) {
			case *ssa.Call:
				common = instr.Common()
			case *ssa.Go:
				common = instr.Common()
			case *ssa.Defer:
				common = instr.Common()
			}
			if common == nil {
				continue
			}
			if f, ok := common.Value.(*ssa.Function); ok {
				if origin := f.Origin(); origin != nil {
					f = origin
				}
				if f.Pkg == nil || !isFunctionBodySkippedPackagePath(f.Pkg.Pkg.Path()) {
					continue
				}
				if f.Name() == "init" {
					continue
				}
				switch f.Pkg.Pkg.Path() {
				case "runtime":
					// These are handled by emitSpecialRuntimeCall, so don't skip
					// their callers (Goexit/Gosched for sync.Pool via pinSlow,
					// GOMAXPROCS for sync.Pool, FuncForPC/Name for the function
					// identity tests in xtests/reflect.go).
					if f.Name() == "Goexit" || f.Name() == "Gosched" || f.Name() == "GOMAXPROCS" || f.Name() == "FuncForPC" || f.Name() == "Name" || f.Name() == "SetFinalizer" || f.Name() == "fcntl" || f.Name() == "beforeExit" || f.Name() == "KeepAlive" {
						continue
					}
				case "syscall":
					// Exit is handled by emitSpecialRuntimeCall (terminates the
					// process directly), so don't skip its callers (os.Exit).
					// write/read/openat are handled by emitSpecialRuntimeCall
					// (map to the C write(2)/read(2)/openat(2) calls), so don't
					// skip their callers (os.File.Write/os.File.Read/os.Open).
					// Open and close forward to openat/close via generated
					// bodies, so they must not be skipped either.
					if f.Name() == "Exit" || f.Name() == "write" || f.Name() == "read" || f.Name() == "openat" || f.Name() == "Open" || f.Name() == "close" || f.Name() == "Close" || f.Name() == "fcntl" || f.Name() == "runtime_entersyscall" || f.Name() == "runtime_exitsyscall" {
						continue
					}
				case "internal/testlog":
					// PanicOnExit0 is intercepted by emitSpecialRuntimeCall (returns
					// false for a non-test binary); don't skip its callers (os.Exit).
					// Open records file opens for the test log; in a non-test
					// binary it is a no-op, so don't skip os.Open's callers.
					if f.Name() == "PanicOnExit0" || f.Name() == "Open" {
						continue
					}
				case "reflect":
					// ValueOf/Pointer/rtypeOf/TypeOf are handled by
					// emitSpecialRuntimeCall (function identity tests and the
					// reflect.Type fabrication used by fmt), so don't skip
					// callers of them.
					switch f.Name() {
					case "ValueOf", "Pointer", "rtypeOf", "TypeOf":
						continue
					}
				}
				if strings.HasPrefix(f.Pkg.Pkg.Path(), "internal/race") ||
					strings.HasPrefix(f.Pkg.Pkg.Path(), "internal/synctest") ||
					strings.HasPrefix(f.Pkg.Pkg.Path(), "internal/msan") ||
					strings.HasPrefix(f.Pkg.Pkg.Path(), "internal/asan") {
					continue
				}
				if f.Pkg.Pkg.Path() == "internal/godebug" {
					// godebug.New/Value/IncNonDefault are intercepted by
					// emitSpecialRuntimeCall (see below), so don't skip their
					// callers (time.init, time.syncTimer).
					switch f.Name() {
					case "New", "Value", "IncNonDefault", "init":
						continue
					}
				}
				if f.Pkg.Pkg.Path() == "internal/reflectlite" && f.Name() == "TypeOf" {
					// reflectlite.TypeOf is intercepted by emitSpecialRuntimeCall
					// (returns a zeroed Type interface); its interface method
					// calls are intercepted during *ssa.Call emission. Keep
					// callers such as context.WithValue emitted.
					continue
				}
				if f.Pkg.Pkg.Path() == "internal/abi" && f.Name() == "NoEscape" {
					continue
				}
				return true
			}
		}
	}
	return false
}
