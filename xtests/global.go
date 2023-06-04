package main

import (
	"reflect"
	"runtime"
	"strings"
)

var v1 int

func Test1() int {
	if v1 != 0 {
		return 0
	}
	return 1
}

var v2 int

func Test2() int {
	v2 = 2
	return v2
}

type S3 struct {
	x int
}

var v3 S3

func Test3() int {
	if v3.x != 0 {
		return 0
	}
	return 3
}

type S4 struct {
	x int
}

var v4 S4

func Test4() int {
	v4 = S4{4}
	return v4.x
}

var v5 struct {
	x int
}

func Test5() int {
	if v5.x != 0 {
		return 0
	}
	return 5
}

var v6 struct {
	x int
}

func Test6() int {
	v6.x = 6
	return v6.x
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
}
