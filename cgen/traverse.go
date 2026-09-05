package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"go/types"

	"golang.org/x/tools/go/ssa"
)

func (ctx *Context) traverseValue(function *ssa.Function, procedure func(value ssa.Value)) {
	foundValueSet := make(map[ssa.Value]struct{})
	var f func(value ssa.Value)
	g := func(callCommon *ssa.CallCommon) {
		f(callCommon.Value)
		for _, arg := range callCommon.Args {
			f(arg)
		}
	}
	f = func(value ssa.Value) {
		_, ok := foundValueSet[value]
		if ok {
			return
		}
		foundValueSet[value] = struct{}{}

		switch val := value.(type) {
		case *ssa.Alloc, *ssa.Builtin, *ssa.Const, *ssa.FreeVar, *ssa.Function, *ssa.Global, *ssa.Parameter:
			// do nothing

		case *ssa.BinOp:
			f(val.X)
			f(val.Y)

		case *ssa.Call:
			g(val.Common())

		case *ssa.ChangeInterface:
			f(val.X)

		case *ssa.ChangeType:
			f(val.X)

		case *ssa.Convert:
			f(val.X)

		case *ssa.Extract:
			f(val.Tuple)

		case *ssa.Field:
			f(val.X)

		case *ssa.FieldAddr:
			f(val.X)

		case *ssa.Index:
			f(val.X)
			f(val.Index)

		case *ssa.IndexAddr:
			f(val.X)
			f(val.Index)

		case *ssa.Lookup:
			f(val.X)
			f(val.Index)

		case *ssa.MakeChan:
			f(val.Size)

		case *ssa.MakeClosure:
			f(val.Fn)
			for _, freeVar := range val.Bindings {
				f(freeVar)
			}

		case *ssa.MakeInterface:
			f(val.X)

		case *ssa.MakeMap:
			if val.Reserve != nil {
				f(val.Reserve)
			}

		case *ssa.MakeSlice:
			f(val.Len)
			f(val.Cap)

		case *ssa.Next:
			f(val.Iter)

		case *ssa.Phi:
			for _, edge := range val.Edges {
				f(edge)
			}

		case *ssa.Range:
			f(val.X)

		case *ssa.Select:
			for _, state := range val.States {
				f(state.Chan)
				if state.Send != nil {
					f(state.Send)
				}
			}

		case *ssa.Slice:
			f(val.X)
			if val.Low != nil {
				f(val.Low)
			}
			if val.High != nil {
				f(val.High)
			}
			if val.Max != nil {
				f(val.Max)
			}

		case *ssa.SliceToArrayPointer:
			f(val.X)

		case *ssa.TypeAssert:
			f(val.X)

		case *ssa.UnOp:
			f(val.X)

		default:
			value.Parent().WriteTo(os.Stderr)
			panic(fmt.Sprintf("unknown value: %s : %T", value.String(), value))
		}

		procedure(value)
	}

	for _, basicBlock := range function.Blocks {
		for _, instruction := range basicBlock.Instrs {
			switch instr := instruction.(type) {
			case ssa.Value:
				f(instr)

			case *ssa.Defer:
				g(instr.Common())

			case *ssa.Go:
				g(instr.Common())

			case *ssa.If:
				f(instr.Cond)

			case *ssa.Jump, *ssa.RunDefers:
				// do nothing

			case *ssa.MapUpdate:
				f(instr.Map)
				f(instr.Key)
				f(instr.Value)

			case *ssa.Panic:
				f(instr.X)

			case *ssa.Return:
				for _, result := range instr.Results {
					f(result)
				}

			case *ssa.Send:
				f(instr.Chan)
				f(instr.X)

			case *ssa.Store:
				f(instr.Addr)
				f(instr.Val)

			default:
				instr.Parent().WriteTo(os.Stderr)
				panic(fmt.Sprintf("unknown value: %s : %T", instr.String(), instr))
			}
		}
	}
}

func (ctx *Context) traverseCallCommon(function *ssa.Function, procedure func(callCommon *ssa.CallCommon)) {
	for _, basicBlock := range function.Blocks {
		for _, instruction := range basicBlock.Instrs {
			switch instr := instruction.(type) {
			case *ssa.Call:
				procedure(instr.Common())
			case *ssa.Defer:
				procedure(instr.Common())
			case *ssa.Go:
				procedure(instr.Common())
			}
		}
	}
}

func isAnyGenericInstance(fn *ssa.Function) bool {
	return fn.Pkg == nil && len(fn.TypeArgs()) > 0
}

// hasUnboundTypeArgs reports whether fn is a partially instantiated instance
// whose type arguments still contain type parameters. Such functions only
// appear inside generic origin bodies and must never be emitted.
func hasUnboundTypeArgs(fn *ssa.Function) bool {
	for _, typ := range fn.TypeArgs() {
		if hasTypeParameter(typ) {
			return true
		}
	}
	return false
}

func getCallCallee(common *ssa.CallCommon) *ssa.Function {
	if fn := common.StaticCallee(); fn != nil {
		return fn
	}
	if fn, ok := common.Value.(*ssa.Function); ok {
		return fn
	}
	return nil
}

// instanceCollector gathers generic instances reachable through function
// bodies. With monomorphization every file that uses an instance emits its
// own private copy, so instances are attributed by usage instead of origin.
type instanceCollector struct {
	seen                          map[*ssa.Function]struct{}
	result                        []*ssa.Function
	recordPlainWrappers           bool
	recordInterfaceMethodWrappers bool
}

// walk records generic instances reachable from fn. Recursion only descends
// into instance subtrees and their synthetic $bound/$thunk wrappers; entering
// ordinary foreign functions is skipped so their private instance usage stays
// attributed to the file that defines them. Starters passed with force=true
// (the collecting file's own members) are entered unconditionally.
func (collector *instanceCollector) walk(fn *ssa.Function, force bool) {
	if fn == nil {
		return
	}
	if _, ok := collector.seen[fn]; ok {
		return
	}
	// Gate BEFORE remembering: a function rejected here must stay eligible
	// for a later forced visit, otherwise bodies reached first through
	// ordinary references would never be scanned.
	if !force && !isGenericInstanceSubtreeMember(fn) {
		return
	}
	if fn.Blocks != nil && isFunctionBodySkipped(fn) && !isAnyGenericInstance(fn) {
		return
	}
	collector.seen[fn] = struct{}{}

	// Bound-method and thunk synthetics of an instance ride along with the
	// instance definition itself, so only genuine roots are recorded.
	if isAnyGenericInstance(fn) && !strings.HasSuffix(fn.Name(), "$bound") && !strings.HasSuffix(fn.Name(), "$thunk") && !hasUnboundTypeArgs(fn) {
		collector.result = append(collector.result, fn)
	}
	// Wrappers of generic-origin methods keep parameterized signatures and
	// cannot be emitted anywhere. Methods from emitted packages define their
	// own wrappers, so only wrappers of body-skipped targets belong here.
	if collector.recordPlainWrappers && isPlainSyntheticWrapper(fn) &&
		fn.TypeParams().Len() == 0 && !hasTypeParamInSignature(fn.Signature) {
		target := wrapperTargetFunction(fn)
		if target != nil && isFunctionBodySkipped(target) {
			collector.result = append(collector.result, fn)
		}
	}
	// Interface method bounds ship in shared_definition.c so that every file
	// that captures one through a closure resolves the same external symbol.
	if collector.recordInterfaceMethodWrappers && isInterfaceMethodWrapper(fn) {
		collector.result = append(collector.result, fn)
	}

	if fn.Blocks == nil {
		return
	}

	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			for _, operand := range instr.Operands(nil) {
				if ref, ok := (*operand).(*ssa.Function); ok {
					collector.walk(ref, false)
				}
			}
			var callee *ssa.Function
			switch instr := instr.(type) {
			case *ssa.Call:
				callee = getCallCallee(instr.Common())
			case *ssa.Defer:
				callee = getCallCallee(instr.Common())
			case *ssa.Go:
				callee = getCallCallee(instr.Common())
			}
			if callee != nil {
				collector.walk(callee, false)
			}
		}
	}

	for _, anon := range fn.AnonFuncs {
		collector.walk(anon, force)
	}
}

func (collector *instanceCollector) sortedResult() []*ssa.Function {
	sort.Slice(collector.result, func(i, j int) bool {
		return collector.result[i].RelString(nil) < collector.result[j].RelString(nil)
	})
	return collector.result
}

// collectInstances appends to ctx.extraFunctions every generic instance used
// by pkg's own code, directly or transitively.
func (ctx *Context) collectInstances(pkg *ssa.Package) {
	collector := &instanceCollector{seen: map[*ssa.Function]struct{}{}}
	ctx.appendReachableInstances(pkg, collector)
	ctx.extraFunctions = append(ctx.extraFunctions, collector.sortedResult()...)
}

func (ctx *Context) appendReachableInstances(pkg *ssa.Package, collector *instanceCollector) {
	for _, member := range sortedPackageMembers(pkg) {
		switch member := member.(type) {
		case *ssa.Function:
			collector.walk(member, true)
		case *ssa.Type:
			walkTypeMethods(ctx.program, member.Type(), func(fn *ssa.Function) { collector.walk(fn, true) })
			walkTypeMethods(ctx.program, types.NewPointer(member.Type()), func(fn *ssa.Function) { collector.walk(fn, true) })
		}
	}
}

// collectSharedInstances appends to ctx.extraFunctions every generic instance
// referenced from shared_definition.c itself. Interface method tables are
// defined only there, so their concrete methods seed the set; the rest comes
// from transitive usage inside the seeded instances.
func (ctx *Context) collectSharedInstances() {
	collector := &instanceCollector{seen: map[*ssa.Function]struct{}{}, recordPlainWrappers: true, recordInterfaceMethodWrappers: true}
	allowSet := ctx.buildAllowSet(nil)

	seed := func(typ types.Type) {
		methodSet := ctx.program.MethodSets.MethodSet(typ)
		for _, index := range ctx.interfaceTableEntryIndexes(typ, allowSet) {
			function := ctx.program.MethodValue(methodSet.At(index))
			if function != nil {
				collector.walk(function, isGenericInstanceSubtreeMember(function))
			}
		}
	}

	ctx.traverseBasicType(seed)
	for _, pkg := range allPackagesSorted(ctx.program) {
		for _, member := range sortedPackageMembers(pkg) {
			switch member := member.(type) {
			case *ssa.Type:
				seed(member.Type())
				seed(types.NewPointer(member.Type()))
			case *ssa.Function:
				collector.walk(member, true)
			}
		}
	}

	ctx.extraFunctions = append(ctx.extraFunctions, collector.sortedResult()...)
}

func receiverPackage(function *ssa.Function) *types.Package {
	if function.Signature == nil || function.Signature.Recv() == nil {
		return nil
	}
	t := function.Signature.Recv().Type()
	for {
		switch u := t.(type) {
		case *types.Pointer:
			t = u.Elem()
		case *types.Alias:
			t = u.Rhs()
		case *types.Named:
			return t.(*types.Named).Obj().Pkg()
		default:
			return nil
		}
	}
}

func walkTypeMethods(program *ssa.Program, t types.Type, walk func(fn *ssa.Function)) {
	methodSet := program.MethodSets.MethodSet(t)
	for i := 0; i < methodSet.Len(); i++ {
		if function := program.MethodValue(methodSet.At(i)); function != nil {
			walk(function)
		}
	}
}

func sortedPackageMembers(pkg *ssa.Package) []ssa.Member {
	mp := map[string]ssa.Member{}
	for _, member := range pkg.Members {
		mp[member.RelString(nil)] = member
	}
	keys := []string{}
	for key := range mp {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	members := []ssa.Member{}
	for _, key := range keys {
		members = append(members, mp[key])
	}
	return members
}

func (ctx *Context) traverseFunction(pkg *ssa.Package, procedure func(function *ssa.Function)) {
	if ctx.cachedFunctions == nil {
		visited := make(map[*ssa.Function]struct{})
		seenNames := make(map[string]struct{})
		var f func(function *ssa.Function, force bool)
		f = func(function *ssa.Function, force bool) {
			if function == nil {
				return
			}
			if _, ok := visited[function]; ok {
				return
			}
			visited[function] = struct{}{}
			if pkg != nil && !force {
				owner := ""
				if function.Pkg != nil {
					owner = function.Pkg.Pkg.Path()
				}
				if rp := receiverPackage(function); rp != nil {
					owner = rp.Path()
				}
				if owner != "" && owner != pkg.Pkg.Path() {
					return
				}
			}
			if function.TypeParams().Len() > 0 && len(function.TypeArgs()) == 0 {
				return
			}
			if hasTypeParamInSignature(function.Signature) {
				return
			}
			name := createFunctionName(function)
			if _, ok := seenNames[name]; ok {
				return
			}
			seenNames[name] = struct{}{}
			ctx.cachedFunctions = append(ctx.cachedFunctions, function)
			for _, anonFunc := range function.AnonFuncs {
				f(anonFunc, force)
			}
		}

		g := func(t types.Type) {
			methodSet := ctx.program.MethodSets.MethodSet(t)
			for i := 0; i < methodSet.Len(); i++ {
				function := ctx.program.MethodValue(methodSet.At(i))
				if function == nil {
					continue
				}
				f(function, false)
			}
		}

		ctx.traversePackageMember(pkg, func(member ssa.Member) {
			switch member := member.(type) {
			case *ssa.Function:
				f(member, false)
			case *ssa.Type:
				t := member.Type()
				g(t)
				g(types.NewPointer(t))
			}
		})

		for _, fn := range ctx.extraFunctions {
			// Instances are placed in this file by design; never let the
			// owner filter reassign them to their origin package.
			f(fn, true)
		}
	}

	for _, function := range ctx.cachedFunctions {
		procedure(function)
	}
}

func (ctx *Context) traverseBasicType(procedure func(typ types.Type)) {
	for _, typ := range types.Typ {
		switch typ.Kind() {
		case types.Invalid, types.UntypedBool, types.UntypedComplex, types.UntypedFloat, types.UntypedInt, types.UntypedNil, types.UntypedRune, types.UntypedString:
			continue
		}
		procedure(typ)
	}
}

func (ctx *Context) traverseType(pkg *ssa.Package, procedure func(typ types.Type)) {
	foundTypeSet := make(map[string]struct{})
	var f func(typ types.Type)
	f = func(typ types.Type) {
		if _, ok := typ.(*types.Alias); ok {
			f(typ.Underlying())
			return
		}
		if hasTypeParameter(typ) {
			return
		}
		name := createTypeName(typ)
		_, ok := foundTypeSet[name]
		if ok {
			return
		}
		foundTypeSet[name] = struct{}{}

		switch typ := typ.(type) {
		case *types.Array:
			f(typ.Elem())

		case *types.Basic:
			// handled in traverseBasicType
			return

		case *types.Chan:
			f(typ.Elem())

		case *types.Interface, *types.Signature, *types.TypeParam:
			// do nothing

		case *types.Map:
			f(typ.Key())
			f(typ.Elem())

		case *types.Named:
			f(typ.Underlying())

		case *types.Pointer:
			f(typ.Elem())

		case *types.Slice:
			f(typ.Elem())

		case *types.Struct:
			for i := 0; i < typ.NumFields(); i++ {
				f(typ.Field(i).Type())
			}

		case *types.Tuple:
			for i := 0; i < typ.Len(); i++ {
				f(typ.At(i).Type())
			}

		default:
			if typ.String() == "iter" {
				/// iterator of map or string
				return
			}
			panic(fmt.Sprintf("not implemented: %s %T", typ, typ))
		}

		procedure(typ)
	}

	ctx.traversePackageMember(pkg, func(member ssa.Member) {
		switch member := member.(type) {
		case *ssa.Global:
			f(member.Type())
		case *ssa.Type:
			f(member.Type())
		}
	})

	ctx.traverseFunction(pkg, func(function *ssa.Function) {
		sig := function.Signature

		hasTypeParam := false
		for i := 0; i < sig.Params().Len(); i++ {
			if _, ok := sig.Params().At(i).Type().(*types.TypeParam); ok {
				hasTypeParam = true
				break
			}
		}
		for i := 0; i < sig.Results().Len(); i++ {
			if _, ok := sig.Results().At(i).Type().(*types.TypeParam); ok {
				hasTypeParam = true
				break
			}
		}
		if hasTypeParam {
			return
		}

		if sig.Recv() != nil {
			f(sig.Recv().Type())
		}
		f(sig.Params())
		f(sig.Results())

		if function.Blocks == nil {
			return
		}

		ctx.traverseValue(function, func(value ssa.Value) {
			typ := value.Type()
			if _, ok := typ.(*types.TypeParam); ok {
				return
			}
			if sig, ok := typ.(*types.Signature); ok {
				hasTypeParam := false
				for i := 0; i < sig.Params().Len(); i++ {
					if _, ok := sig.Params().At(i).Type().(*types.TypeParam); ok {
						hasTypeParam = true
						break
					}
				}
				if hasTypeParam {
					return
				}
			}
			f(typ)
		})

		ctx.traverseCallCommon(function, func(callCommon *ssa.CallCommon) {
			sig := callCommon.Signature()
			if sig == nil {
				return
			}
			if sig.Recv() != nil {
				f(sig.Recv().Type())
			}
			f(sig.Params())
			f(sig.Results())
		})
	})
}

func (ctx *Context) traversePackageMember(pkg *ssa.Package, procedure func(member ssa.Member)) {
	if ctx.orderedPackageMembers == nil {
		mp := map[string]ssa.Member{}
		for _, pkg2 := range allPackagesSorted(ctx.program) {
			if pkg != nil && pkg != pkg2 {
				continue
			}
			for _, member := range pkg2.Members {
				mp[member.RelString(nil)] = member
			}
		}

		keys := []string{}
		for key := range mp {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		members := []ssa.Member{}
		for _, key := range keys {
			members = append(members, mp[key])
		}

		ctx.orderedPackageMembers = members
	}

	for _, member := range ctx.orderedPackageMembers {
		procedure(member)
	}
}
