package main

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
)

func Test1() int {
	a := 1
	b := &a
	return *b
}

func Test2() int {
	a := 2
	b := &a
	c := &b
	return **c
}

func Test3() int {
	a := 3
	b := &a
	c := &b
	d := &c
	return ***d
}

func test4_f() *int {
	a := 4
	return &a
}

func Test4() int {
	return *test4_f()
}

func test5_f() **int {
	a := 5
	b := &a
	return &b
}

func Test5() int {
	return **test5_f()
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
