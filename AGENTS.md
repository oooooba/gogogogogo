# Project Purpose

gogogogogo is a Go compiler and runtime.
The compiler front end (in `cgen/`, written in Go) lowers Go programs to C using the [`go/ssa`](https://pkg.go.dev/golang.org/x/tools/go/ssa) intermediate representation, emitting continuation-passing style (CPS) C code.
The runtime (in `src/`, written in Rust) executes that C as a static library; it runs many goroutines on a single OS thread and implements Go features such as channels, defer, and goroutine switching, plus data structures like slices and maps.

Dynamic memory allocation is supported, but garbage collection is not yet implemented.
The project validates the generated C against the Go standard compiler with equivalence tests (`check_equivalence.sh`), asserting the two produce identical behavior and output.

# Repository Structure

* `cgen/` — the compiler front end (Go). `main.go` turns a `.go` file into CPS C code.
The front end is split across several `package main` files (e.g. `emit.go` for instructions, `call.go`/`builtin.go` for calls and builtins, `type.go`/`function.go`/`traverse.go`/`package.go`/`driver.go` for the rest); `predefined.h` is the C preamble; `supported_packages.go` (build-tagged `supported_packages`) lists the standard-library packages translated for `xtests`; `cache/` holds the generated per-package `package_*.c` files.
Since the compiler is compiled with `go run .`, all `.go` files in `cgen/` must be part of one `package main`; `supported_packages.go` is excluded from the default build via its build tag.
* `src/` — the runtime (Rust, static lib). `api/` implements the object-model operations (channels, maps, slices, strings, goroutines, etc.) exported to C; `object/` defines the corresponding boxed value representations; `light_weight_thread.rs`/`world_chunk.rs` (`word_chunk.rs`) support the goroutine scheduler.
* `xtests/` — Go test programs executed by the generated binary and the Go compiler to check equivalence. `xtests/stdlib/` contains per-package standard library tests (e.g. `os.go`).
* `build.sh` / `run.sh` — build a single `.go` file into a C binary and run it (see "Building and Running a Single Program").
* `check_equivalence.sh` / `run_xtests.sh` / `check_regression.sh` / `check_reproducibility.sh` / `check_go_compiler_tests.sh` / `check_formatting.sh` — verification scripts (see "Equivalence Tests" and below).
* `run_formatter.sh` — runs `cargo fmt`/`clippy`, `gofmt`, and `clang-format`.
* `.github/workflows/regression-test.yml` — the CI job that runs the full regression suite (reproducibility, debug-mode xtests, GOROOT/test equivalence), which the "Prohibitions" section says not to run locally.
* `tmp/` — scratch space for build directories and temporary files (required by the "Temporary Directory" section).

# Commands

## Building and Running a Single Program

For a quick iteration on one program (e.g. an `xtests` file or a scratch program in `tmp/`), build it into a build directory of your choice with:

```
$ bash build.sh -b tmp/<dir> <file.go>
```

`build.sh` prints the path of the generated binary (by default `<dir>/bin.exe`).
Run it directly with `./tmp/<dir>/bin.exe`.

* Without `-b`, `build.sh` creates a build directory under `/tmp` (via `mktemp`), which may be deleted at any time. Always pass `-b <dir>` (preferably under `tmp`) so the build survives and can be reused or debugged later.
* `build.sh` recompiles the Rust front end itself when needed and regenerates the C from the `.go` source, so the output always reflects the current `cgen/*` sources and `<file.go>`.
* `run.sh` is a thin wrapper: `bash run.sh [-b <dir>] [--debug-user] [--debug-runtime] <file.go>` builds and immediately executes the binary. It exports `ASAN_OPTIONS=detect_leaks=0` in `--debug-user` mode.

## Manual Modification for Generated C Code

To debug the generated C code, you can modify it directly.
Edit the files in the `build` directory, change the current directory to `build`, and then run the `make` command.
The available targets are `bin.exe`, `bin-debug-user.exe`, `bin-debug-runtime.exe` and `bin-debug-user-debug-runtime.exe`.

These manual C edits are for debugging only and are transient: `build.sh` (and the equivalence/regression scripts) regenerate the C from the Go source, so any manual edits are overwritten on the next build.
Do not rely on them for a final fix; put the real fix in `cgen/*`.

## Temporary Directory

You must use the `tmp` directory as a temporary file storage location.
Because the `/tmp` directory may be deleted while you are doing task, you must not use it.
If it does not exist, you may create it.

## Unit Tests of Runtime

If you add new APIs to the runtime or add public methods to objects, you must add unit tests for them.

## Equivalence Tests

If you add or modify files in the `xtests` directory, you must compare the behavior of the generated binary with that of the Go standard compiler.
You can verify this by running the script described below.
There are four combinations of command-line arguments.

```
$ bash check_equivalence.sh                              file.go # uses release mode  binary generated from C and use release mode runtime
$ bash check_equivalence.sh --debug-runtime              file.go # uses release mode binary generated from C and debug mode runtime
$ bash check_equivalence.sh --debug-user                 file.go # uses release mode binary generated from C and release mode runtime
$ bash check_equivalence.sh --debug-user --debug-runtime file.go # uses debug mode binary generated from C and release mode runtime
```

Notes:
* Both `go run <file.go>` and the generated binary run from the repository root, so relative paths (e.g. a temporary data file created by a test) resolve the same way for both. Open files by relative path if you want the resolve to match, and `os.Remove` them so no debris is left behind.
* `check_equivalence.sh` does NOT feed stdin to the binary. A test that reads `os.Stdin` will block forever on an empty/pipe stdin unless it drains to EOF, or you pipe `</dev/null` when running the binary yourself. Keep stdin-dependent tests non-blocking.
* If you edit `cgen/*` and then re-run this script, remove `cgen/cache` first so the per-package `package_*.c` files are regenerated, not reused from a stale cache (a stale cache can silently "not fire" a handler you just added).

# Prohibitions

* Do not run the full regression tests (reproducibility check, debug-mode xtests, GOROOT/test equivalence tests) because they take too much time.
They run only in CI as separate steps of `.github/workflows/regression-test.yml`.

# Definition of Done

* If you perform any tasks that involve modifying the source code, run `run_formatter.sh` after all processes are complete.
* If you perform any tasks that involve modifying the source code, run `check_regression.sh` after all processes are complete and verify that its exit status is 0.
* List all the difficulties you encountered while working on the assignment, tasks that took a long time, points you didn't understand, and areas you felt needed improvement, and provide this feedback in AGENTS.md.
