package main

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
)

func test1_f() int {
	return 1
}

func Test1() int {
	v1 := reflect.ValueOf(test1_f)
	v2 := reflect.ValueOf(test1_f)
	if v1 == v2 {
		return 1
	} else {
		return 0
	}
}

func test2_f() int {
	return 2
}

func Test2() int {
	v1 := reflect.ValueOf(test1_f)
	v2 := reflect.ValueOf(test2_f)
	if v1 == v2 {
		return 0
	} else {
		return 2
	}
}

func Test3() int {
	v1 := reflect.ValueOf(test1_f).Pointer()
	v2 := reflect.ValueOf(test1_f).Pointer()
	if v1 == v2 {
		return 3
	} else {
		return 0
	}
}

func Test4() int {
	v1 := reflect.ValueOf(test1_f).Pointer()
	v2 := reflect.ValueOf(test2_f).Pointer()
	if v1 == v2 {
		return 0
	} else {
		return 4
	}
}

func Test5() int {
	v1 := runtime.FuncForPC(reflect.ValueOf(test1_f).Pointer())
	v2 := runtime.FuncForPC(reflect.ValueOf(test1_f).Pointer())
	if v1 == v2 {
		return 5
	} else {
		return 0
	}
}

func Test6() int {
	v1 := runtime.FuncForPC(reflect.ValueOf(test1_f).Pointer())
	v2 := runtime.FuncForPC(reflect.ValueOf(test2_f).Pointer())
	if v1 == v2 {
		return 0
	} else {
		return 6
	}
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
	runTest(Test6)
}
