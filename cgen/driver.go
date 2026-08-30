package main

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"golang.org/x/tools/go/ssa"
)

func (ctx *Context) emitFunctionNameRegistry() {
	fmt.Fprintln(ctx.stream, "static const UserFunctionInfo userFunctionInfoTable[] = {")
	for _, pkg := range allPackagesSorted(ctx.program) {
		if isFunctionBodySkippedPackage(pkg) {
			continue
		}
		for _, member := range sortedPackageMembers(pkg) {
			fn, ok := member.(*ssa.Function)
			if !ok {
				continue
			}
			if fn.TypeParams().Len() > 0 {
				continue
			}
			name := functionRuntimeName(fn)
			fmt.Fprintf(ctx.stream, "\t{ (uintptr_t)%s, (StringObject){.raw = \"%s\", .len = sizeof(\"%s\") - 1} }, // %s\n",
				createFunctionName(fn), name, name, fn.RelString(nil))
		}
	}
	fmt.Fprintln(ctx.stream, "};")
}

func functionRuntimeName(fn *ssa.Function) string {
	if fn.Pkg == nil {
		return fn.Name()
	}
	return fn.Pkg.Pkg.Name() + "." + fn.Name()
}

func handlePackage(program *ssa.Program, pkg *ssa.Package, outputPath string) {
	f, err := os.Create(outputPath)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	ctx := Context{
		stream:        f,
		program:       program,
		latestNameMap: make(map[*ssa.BasicBlock]string),
	}

	// The package file defines static copies of every generic instance its
	// own code uses (monomorphization), so keep them in ctx.extraFunctions.
	ctx.collectInstances(pkg)
	ctx.assertedInterfaceTypes = collectAssertedInterfaceTypesOfFunctions(&ctx, pkg)
	ctx.emitPackage(pkg)
}

func handleMakefile(program *ssa.Program, outputPath string, buildDirname string, cacheDirname string, cachedPackages map[string]bool) {
	makefile, err := os.Create(outputPath)
	if err != nil {
		panic(err)
	}
	defer makefile.Close()
	generateMakefile(makefile, program, buildDirname, cacheDirname, cachedPackages)
}

func loadCachedPackages(cacheDirname string) map[string]bool {
	result := map[string]bool{}
	entries, err := os.ReadDir(cacheDirname)
	if err != nil {
		return result
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "package_") || !strings.HasSuffix(name, ".c") {
			continue
		}
		pkgName := strings.TrimSuffix(strings.TrimPrefix(name, "package_"), ".c")
		result[pkgName] = true
	}
	return result
}

func emitProgram(program *ssa.Program, buildDirname string, cacheDirname string) {
	waitGroup := sync.WaitGroup{}

	cachedPackages := loadCachedPackages(cacheDirname)
	assertedInterfaceTypes := collectAssertedInterfaceTypes(program)
	instantiatedNamedTypes := collectInstantiatedNamedTypes(program)

	waitGroup.Add(1)
	go func() {
		definitionName := "shared_definition.c"
		handleSharedDefinition(program, assertedInterfaceTypes, instantiatedNamedTypes, fmt.Sprintf("%s/%s", buildDirname, definitionName))
		waitGroup.Done()
	}()

	for _, pkg := range allPackagesSorted(program) {
		if isFunctionBodySkippedPackage(pkg) {
			continue
		}
		if cachedPackages[createPackageName(pkg.Pkg)] {
			continue
		}
		waitGroup.Add(1)
		go func(pkg *ssa.Package) {
			outputName := fmt.Sprintf("package_%s.c", createPackageName(pkg.Pkg))
			handlePackage(program, pkg, fmt.Sprintf("%s/%s", buildDirname, outputName))
			waitGroup.Done()
		}(pkg)
	}

	waitGroup.Add(1)
	go func() {
		makefileName := "Makefile"
		handleMakefile(program, fmt.Sprintf("%s/%s", buildDirname, makefileName), buildDirname, cacheDirname, cachedPackages)
		waitGroup.Done()
	}()

	waitGroup.Wait()
}
