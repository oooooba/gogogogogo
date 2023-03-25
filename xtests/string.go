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

func Test4() int {
	s1 := "a"
	s2 := "a"
	return func(s1, s2 string) int {
		if s1 == s2 {
			return 4
		} else {
			return 0
		}
	}(s1, s2)
}

func Test5() int {
	s1 := "a"
	s2 := "b"
	return func(s1, s2 string) int {
		if s1 == s2 {
			return 0
		} else {
			return 5
		}
	}(s1, s2)
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
