package main

import (
	"reflect"
	"runtime"
	"strings"
)

func Test1() int {
	println(42)
	return 1
}

func Test2() int {
	println("abc")
	return 2
}

func Test3() int {
	n := 43
	return func(n int) int {
		println(n)
		return 3
	}(n)
}

func Test4() int {
	s := "def"
	return func(s string) int {
		println(s)
		return 4
	}(s)
}

func Test5() int {
	n := 44
	s := "ghi"
	return func(n int, s string) int {
		println(s, n)
		return 5
	}(n, s)
}

func Test6() int {
	print(42)
	println("a")
	return 6
}

func Test7() int {
	print(42, "a")
	println(43)
	return 7
}

func Test8() int {
	var n_int int = 42
	println(n_int)
	var n_uint uint = 42
	println(n_uint)
	var n_int8 int8 = 42
	println(n_int8)
	var n_int16 int16 = 42
	println(n_int16)
	var n_int32 int32 = 42
	println(n_int32)
	var n_int64 int64 = 42
	println(n_int64)
	var n_uint8 uint8 = 42
	println(n_uint8)
	var n_uint16 uint16 = 42
	println(n_uint16)
	var n_uint32 uint32 = 42
	println(n_uint32)
	var n_uint64 uint64 = 42
	println(n_uint64)
	return 8
}

func Test9() int {
	println(true)
	println(false)
	return 9
}

func Test10() int {
	println(0.0)
	println(-0.0)
	println(0.3)
	println(3.3)
	println(1.2345678)
	println(-12.345678)
	return 10
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
}
