package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/ssa"
)

func allPackagesSorted(program *ssa.Program) []*ssa.Package {
	pkgs := program.AllPackages()
	sort.Slice(pkgs, func(i, j int) bool {
		return createPackageName(pkgs[i].Pkg) < createPackageName(pkgs[j].Pkg)
	})
	return pkgs
}

func generateMakefile(makefile *os.File, program *ssa.Program, buildDirname string, cacheDirname string, cachedPackages map[string]bool) {
	cgenDir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	buildDirAbs, err := filepath.Abs(buildDirname)
	if err != nil {
		panic(err)
	}
	relToCgen, err := filepath.Rel(buildDirAbs, cgenDir)
	if err != nil {
		panic(err)
	}
	relToTarget, err := filepath.Rel(buildDirAbs, filepath.Join(cgenDir, "..", "target"))
	if err != nil {
		panic(err)
	}
	cacheDirAbs, err := filepath.Abs(cacheDirname)
	if err != nil {
		panic(err)
	}
	relToCache, err := filepath.Rel(buildDirAbs, cacheDirAbs)
	if err != nil {
		panic(err)
	}

	cCompiler := "gcc"
	cCompilerWrapper := "ccache"
	cc := fmt.Sprintf("$(shell command -v %s >/dev/null 2>&1 && echo %s %s || echo %s)", cCompilerWrapper, cCompilerWrapper, cCompiler, cCompiler)

	cflags := []string{
		"-Wall", "-Wextra", "-Werror", "-std=c11", "-O2",
		"-fstrict-aliasing", "-Wstrict-aliasing",
	}
	cflagsDebug := []string{
		"-Wall", "-Wextra", "-Werror", "-std=c11",
		"-g", "-O1", "-fno-var-tracking-assignments",
		"-fsanitize=undefined", "-fsanitize=address",
		"-fno-omit-frame-pointer", "-fno-sanitize-recover=all",
		"-fstrict-aliasing", "-Wstrict-aliasing",
	}
	ldflags := []string{
		"-lpthread", "-ldl", "-lm",
	}
	ldflagsDebug := []string{
		"-fsanitize=undefined", "-fsanitize=address", "-fno-sanitize-recover=all",
		"-lpthread", "-ldl", "-lm",
	}
	libsRelease := filepath.Join(relToTarget, "release", "libgogogogogo.a")
	libsDebug := filepath.Join(relToTarget, "debug", "libgogogogogo.a")

	fmt.Fprintf(makefile, "CC = %s\n", cc)
	fmt.Fprintf(makefile, "CFLAGS = %s\n", strings.Join(cflags, " "))
	fmt.Fprintf(makefile, "CFLAGS_DEBUG = %s\n", strings.Join(cflagsDebug, " "))
	fmt.Fprintf(makefile, "LDFLAGS = %s\n", strings.Join(ldflags, " "))
	fmt.Fprintf(makefile, "LDFLAGS_DEBUG = %s\n", strings.Join(ldflagsDebug, " "))
	fmt.Fprintf(makefile, "LIBS_RELEASE = %s\n", libsRelease)
	fmt.Fprintf(makefile, "LIBS_DEBUG = %s\n", libsDebug)

	objsRelease := []string{}
	objsDebug := []string{}

	cFileRule := func(outputName string, extraDeps ...string) {
		objRelease := fmt.Sprintf("%s.o", outputName)
		objDebug := fmt.Sprintf("%s.debug.o", outputName)
		deps := "predefined.h " + outputName
		for _, d := range extraDeps {
			deps += " " + d
		}
		fmt.Fprintf(makefile, "%s: %s\n", objRelease, deps)
		fmt.Fprintf(makefile, "\t@$(CC) $(CFLAGS) -c -o %s %s\n", objRelease, outputName)
		fmt.Fprintf(makefile, "%s: %s\n", objDebug, deps)
		fmt.Fprintf(makefile, "\t@$(CC) $(CFLAGS_DEBUG) -c -o %s %s\n", objDebug, outputName)
		objsRelease = append(objsRelease, objRelease)
		objsDebug = append(objsDebug, objDebug)
	}

	cFileRule("shared_definition.c")
	for _, pkg := range allPackagesSorted(program) {
		if isFunctionBodySkippedPackage(pkg) {
			continue
		}
		outputName := fmt.Sprintf("package_%s.c", createPackageName(pkg.Pkg))
		if cachedPackages[createPackageName(pkg.Pkg)] {
			fmt.Fprintf(makefile, "%s:\n", outputName)
			fmt.Fprintf(makefile, "\t@ln -sf %s/%s %s\n", relToCache, outputName, outputName)
		}
		cFileRule(outputName, "shared_definition.c")
	}

	fmt.Fprintf(makefile, "\n")
	fmt.Fprintf(makefile, "predefined.h:\n")
	fmt.Fprintf(makefile, "\t@ln -sf %s/predefined.h predefined.h\n", relToCgen)
	fmt.Fprintf(makefile, "\n")
	fmt.Fprintf(makefile, "all: bin.exe\n")
	fmt.Fprintf(makefile, "\n")
	fmt.Fprintf(makefile, "bin.exe: %s\n", strings.Join(objsRelease, " "))
	fmt.Fprintf(makefile, "\t@$(CC) -o bin.exe %s $(LIBS_RELEASE) $(LDFLAGS)\n", strings.Join(objsRelease, " "))
	fmt.Fprintf(makefile, "\n")
	fmt.Fprintf(makefile, "bin-debug-user.exe: %s\n", strings.Join(objsDebug, " "))
	fmt.Fprintf(makefile, "\t@$(CC) -o bin-debug-user.exe %s $(LIBS_RELEASE) $(LDFLAGS_DEBUG)\n", strings.Join(objsDebug, " "))
	fmt.Fprintf(makefile, "\n")
	fmt.Fprintf(makefile, "bin-debug-runtime.exe: %s\n", strings.Join(objsRelease, " "))
	fmt.Fprintf(makefile, "\t@$(CC) -o bin-debug-runtime.exe %s $(LIBS_DEBUG) $(LDFLAGS)\n", strings.Join(objsRelease, " "))
	fmt.Fprintf(makefile, "\n")
	fmt.Fprintf(makefile, "bin-debug-user-debug-runtime.exe: %s\n", strings.Join(objsDebug, " "))
	fmt.Fprintf(makefile, "\t@$(CC) -o bin-debug-user-debug-runtime.exe %s $(LIBS_DEBUG) $(LDFLAGS_DEBUG)\n", strings.Join(objsDebug, " "))
}
