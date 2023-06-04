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
}
