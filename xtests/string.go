package main

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
)

func Test1() int {
	s := "a"
	return len(s)
}

func Test2() int {
	s1 := "a"
	s2 := "a"
	if s1 == s2 {
		return 2
	} else {
		return 0
	}
}

func Test3() int {
	s1 := "a"
	s2 := "b"
	if s1 == s2 {
		return 0
	} else {
		return 3
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
}
