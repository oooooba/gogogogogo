package main

import (
	"reflect"
	"runtime"
	"strings"
)

func alpha() int {
	return 1
}

func beta() int {
	return 2
}

func check(bad *int, name string, ok bool) {
	if !ok {
		println("FAIL " + name)
		*bad++
	}
}

func main() {
	bad := 0

	v1 := reflect.ValueOf(alpha)
	v2 := reflect.ValueOf(alpha)
	v3 := reflect.ValueOf(beta)

	check(&bad, "ValueOfSame", v1 == v2)
	check(&bad, "ValueOfDifferent", v1 != v3)

	check(&bad, "PointerSame", v1.Pointer() == v2.Pointer())
	check(&bad, "PointerDifferent", v1.Pointer() != v3.Pointer())

	check(&bad, "FuncForPCSame", runtime.FuncForPC(v1.Pointer()) == runtime.FuncForPC(v2.Pointer()))
	check(&bad, "FuncForPCDifferent", runtime.FuncForPC(v1.Pointer()) != runtime.FuncForPC(v3.Pointer()))

	check(&bad, "NameSame", runtime.FuncForPC(v1.Pointer()).Name() == runtime.FuncForPC(v2.Pointer()).Name())
	check(&bad, "NameDifferent", runtime.FuncForPC(v1.Pointer()).Name() != runtime.FuncForPC(v3.Pointer()).Name())

	name := runtime.FuncForPC(v1.Pointer()).Name()
	check(&bad, "NameAlpha", name == "main.alpha")
	parts := strings.Split(name, ".")
	check(&bad, "SplitName", len(parts) == 2 && parts[1] == "alpha")

	name3 := runtime.FuncForPC(v3.Pointer()).Name()
	check(&bad, "NameBeta", name3 == "main.beta")

	println("bad:", bad)
}
