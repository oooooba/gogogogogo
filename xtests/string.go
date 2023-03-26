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

func Test6() int {
	var s []string
	s = append(s, "a")
	if len(s) != 1 {
		return 0
	}
	if s[0] != "a" {
		return 1
	}
	s = append(s, "b")
	if len(s) != 2 {
		return 2
	}
	if s[1] != "b" {
		return 3
	}
	return 6
}

func Test7() int {
	s := "a"
	ss := strings.Split(s, ".")
	if len(ss) != 1 {
		return 0
	}
	if ss[0] != "a" {
		return 1
	}
	return 7
}

func Test8() int {
	s := "ab_cde_f"
	ss := strings.Split(s, "_")
	if len(ss) != 3 {
		return 0
	}
	if ss[0] != "ab" {
		return 1
	}
	if ss[1] != "cde" {
		return 2
	}
	if ss[2] != "f" {
		return 3
	}
	return 8
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
	runTest(Test7)
	runTest(Test8)
}
