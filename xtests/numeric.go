package main

import (
	"reflect"
	"runtime"
	"strings"
)

func Test1() int {
	f := func() int {
		return 42
	}
	if f() != f() {
		return 0
	}
	return 1
}

func Test2() int {
	f := func() uint8 {
		return 42
	}
	if f() != f() {
		return 0
	}
	return 2
}

func Test3() int {
	f := func() uint16 {
		return 42
	}
	if f() != f() {
		return 0
	}
	return 3
}

func Test4() int {
	f := func() uint32 {
		return 42
	}
	if f() != f() {
		return 0
	}
	return 4
}

func Test5() int {
	f := func() uint64 {
		return 42
	}
	if f() != f() {
		return 0
	}
	return 5
}

func Test6() int {
	f := func() int8 {
		return 42
	}
	if f() != f() {
		return 0
	}
	return 6
}

func Test7() int {
	f := func() int16 {
		return 42
	}
	if f() != f() {
		return 0
	}
	return 7
}

func Test8() int {
	f := func() int32 {
		return 42
	}
	if f() != f() {
		return 0
	}
	return 8
}

func Test9() int {
	f := func() int64 {
		return 42
	}
	if f() != f() {
		return 0
	}
	return 9
}

func Test10() int {
	f := func() float32 {
		return 42
	}
	if f() != f() {
		return 0
	}
	return 10
}

func Test11() int {
	f := func() float64 {
		return 42
	}
	if f() != f() {
		return 0
	}
	return 11
}

func main() {
	runTest := func(test func() int) {
		funcFullName := runtime.FuncForPC(reflect.ValueOf(test).Pointer()).Name()
		funcName := strings.Split(funcFullName, ".")[1]
		println(funcName+":", test())
	}
	runTest(Test1)
	runTest(Test2)
	runTest(Test3)
	runTest(Test4)
	runTest(Test5)
	runTest(Test6)
	runTest(Test7)
	runTest(Test8)
	runTest(Test9)
	runTest(Test10)
	runTest(Test11)
}
