package main

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
)

type T int

func Test1() int {
	t := T(1)
	f := func(t T) int {
		return int(t)
	}
	return f(t)
}

func Test2() int {
	t := T(2)
	f := func(t *T) int {
		return int(*t)
	}
	return f(&t)
}

func Test3() int {
	t := T(2)
	func(t *T) {
		*t = 3
	}(&t)
	return int(t)
}

func main() {
	runTest := func(test func() int) {
		funcFullName := runtime.FuncForPC(reflect.ValueOf(test).Pointer()).Name()
		funcName := strings.Split(funcFullName, ".")[1]
		fmt.Printf("%s: %d\n", funcName, test())
	}
	runTest(Test1)
	runTest(Test2)
	runTest(Test3)
}
