package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"go/types"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

func main() {
	filename := flag.String("i", "/dev/stdin", "input file")
	buildDirname := flag.String("b", "/tmp", "build directory")
	cacheDirname := flag.String("cache", "cache", "cache directory")
	flag.Parse()

	cfg := packages.Config{Mode: packages.LoadAllSyntax}
	initPkgs, err := packages.Load(&cfg, *filename)
	if err != nil {
		log.Fatal(err)
	}
	prog, _ := ssautil.AllPackages(initPkgs, ssa.SanityCheckFunctions|ssa.InstantiateGenerics)
	prog.Build()

	if false {
		var keywords []string
		ctx := Context{
			stream:        nil,
			program:       prog,
			latestNameMap: make(map[*ssa.BasicBlock]string),
		}
		for _, pkg := range allPackagesSorted(prog) {
			ctx.traverseFunction(pkg, func(function *ssa.Function) {
				for _, keyword := range keywords {
					if strings.Contains(function.Name(), keyword) {
						function.WriteTo(os.Stderr)
					}
				}
			})
		}
	}

	emitProgram(prog, *buildDirname, *cacheDirname)
}

type Context struct {
	stream                   *os.File
	program                  *ssa.Program
	latestNameMap            map[*ssa.BasicBlock]string
	orderedPackageMembers    []ssa.Member
	builtinPrintWrapperBuf   strings.Builder
	builtinPrintWrapperNames map[*ssa.CallCommon]string
	extraFunctions           []*ssa.Function
	cachedFunctions          []*ssa.Function
	assertedInterfaceTypes   map[string]types.Type
	instantiatedNamedTypes   map[string]types.Type
	visitedInterfaceNames    map[string]bool
	emittedTypeDefinitions   map[string]struct{}
	instanceOrderedTypes     []types.Type
}

func (ctx *Context) markTypeDefinition(kind, name string) bool {
	if ctx.emittedTypeDefinitions == nil {
		ctx.emittedTypeDefinitions = make(map[string]struct{})
	}
	key := kind + ":" + name
	if _, ok := ctx.emittedTypeDefinitions[key]; ok {
		return false
	}
	ctx.emittedTypeDefinitions[key] = struct{}{}
	return true
}

func encode(str string) string {
	var buf strings.Builder
	for _, c := range str {
		if c >= 0x80 {
			panic(str)
		}
		if ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') {
			buf.WriteByte(byte(c))
		} else {
			fmt.Fprintf(&buf, "_%02X_", c)
		}
	}
	return buf.String()
}

func wrapInFunctionObject(s string) string {
	return fmt.Sprintf("(FunctionObject){.raw=%s}", s)
}

func wrapInObject(s string, t types.Type) string {
	return fmt.Sprintf("(%s){.raw=%s}", createTypeName(t), s)
}

func wrapInTypeId(typ types.Type) string {
	return fmt.Sprintf("(TypeId){ .info = &%s }", createTypeIdName(typ))
}

func isNumericKind(kind types.BasicKind) bool {
	switch kind {
	case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
		types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr,
		types.Float32, types.Float64:
		return true
	default:
		return false
	}
}
