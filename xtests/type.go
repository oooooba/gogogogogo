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

func (t T) f() int {
	return int(t)
}

func Test4() int {
	t := T(4)
	return t.f()
}

type T1 int

func (t1 T1) f() int {
	return int(t1) + 1
}

func Test5() int {
	t := T(1)
	if t.f() != 1 {
		return 1
	}
	t1 := T1(2)
	if t1.f() != 3 {
		return 3
	}
	return 5
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
	runTest(Test4)
	runTest(Test5)
}
