package main

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
)

func Test1() int {
	var a [10]int
	s := a[:]
	s[0] = 1
	return a[0]
}

func Test2() int {
	var a [10]int
	s := a[2:]
	s[1] = 2
	return a[3]
}

func Test3() int {
	var a [10]int
	s := a[:7]
	s[2] = 3
	return a[2]
}

func Test4() int {
	var a [10]int
	s := a[2:7]
	s[3] = 4
	return a[5]
}

func Test5() int {
	var a [10]int
	s := a[2:7]
	return len(s)
}

func Test6() int {
	var a [10]int
	s := a[2:7]
	return cap(s)
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
