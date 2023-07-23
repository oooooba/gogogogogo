package main

import (
	"reflect"
	"runtime"
	"strings"
)

func Test1() int {
	var m map[int]int
	if m != nil {
		return 0
	}
	if len(m) != 0 {
		return 2
	}
	return 1
}

func Test2() int {
	m := make(map[int]int)
	if m == nil {
		return 0
	}
	if len(m) != 0 {
		return 1
	}
	return 2
}

func main() {
	runTest := func(test func() int) {
		funcFullName := runtime.FuncForPC(reflect.ValueOf(test).Pointer()).Name()
		funcName := strings.Split(funcFullName, ".")[1]
		println(funcName+":", test())
	}
	runTest(Test1)
	runTest(Test2)
}
