package main

import (
	"fmt"
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

func main() {
	runTest := func(test func() int) {
		funcFullName := runtime.FuncForPC(reflect.ValueOf(test).Pointer()).Name()
		funcName := strings.Split(funcFullName, ".")[1]
		fmt.Printf("%s: %d\n", funcName, test())
	}
	runTest(Test1)
	runTest(Test2)
}
