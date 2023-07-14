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

func Test12() int {
	f := func() uint {
		return 42
	}
	if f() != f() {
		return 0
	}
	return 12
}

func Test13() int {
	var x uint = 1
	var y uint = 64
	if x<<y != 0 {
		return 0
	}

	var x8 uint8 = 1
	var y8 uint = 8
	if x8<<y8 != 0 {
		return 1
	}

	var x16 uint16 = 1
	var y16 uint = 16
	if x16<<y16 != 0 {
		return 2
	}

	var x32 uint32 = 1
	var y32 uint = 32
	if x32<<y32 != 0 {
		return 3
	}

	var x64 uint64 = 1
	var y64 uint = 64
	if x64<<y64 != 0 {
		return 4
	}

	return 13
}

func Test14() int {
	var x int = 1
	var y uint = 64
	if x<<y != 0 {
		return 0
	}

	var x8 int8 = 1
	var y8 uint = 8
	if x8<<y8 != 0 {
		return 1
	}

	var x16 int16 = 1
	var y16 uint = 16
	if x16<<y16 != 0 {
		return 2
	}

	var x32 int32 = 1
	var y32 uint = 32
	if x32<<y32 != 0 {
		return 3
	}

	var x64 int64 = 1
	var y64 uint = 64
	if x64<<y64 != 0 {
		return 4
	}

	return 14
}

func Test15() int {
	var x uint = 1
	var y uint = 64
	if x>>y != 0 {
		return 0
	}

	var x8 uint8 = 1
	var y8 uint = 8
	if x8>>y8 != 0 {
		return 1
	}

	var x16 uint16 = 1
	var y16 uint = 16
	if x16>>y16 != 0 {
		return 2
	}

	var x32 uint32 = 1
	var y32 uint = 32
	if x32>>y32 != 0 {
		return 3
	}

	var x64 uint64 = 1
	var y64 uint = 64
	if x64>>y64 != 0 {
		return 4
	}

	return 15
}

func Test16() int {
	var x int = 1
	var y uint = 64
	if x>>y != 0 {
		return 0
	}

	var x8 int8 = 1
	var y8 uint = 8
	if x8>>y8 != 0 {
		return 1
	}

	var x16 int16 = 1
	var y16 uint = 16
	if x16>>y16 != 0 {
		return 2
	}

	var x32 int32 = 1
	var y32 uint = 32
	if x32>>y32 != 0 {
		return 3
	}

	var x64 int64 = 1
	var y64 uint = 64
	if x64>>y64 != 0 {
		return 4
	}

	return 16
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
	runTest(Test12)
	runTest(Test13)
	runTest(Test14)
	runTest(Test15)
	runTest(Test16)
}
