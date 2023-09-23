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

func Test3() int {
	m := make(map[int]int)
	m[3] = 42
	if len(m) != 1 {
		return 0
	}
	if m[3] != 42 {
		return 1
	}
	return 3
}

func Test4() int {
	m := make(map[int]int)
	v, ok := m[4]
	if ok {
		return 0
	}
	if v != 0 {
		return 1
	}
	m[4] = 42
	v2, ok2 := m[4]
	if !ok2 {
		return 2
	}
	if v2 != 42 {
		return 3
	}
	return 4
}

func Test5() int {
	m := map[int]int{1: 10, 2: 20, 3: 30}
	if len(m) != 3 {
		return 0
	}
	if m[1] != 10 {
		return 1
	}
	if m[2] != 20 {
		return 2
	}
	if m[3] != 30 {
		return 3
	}
	return 5
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
