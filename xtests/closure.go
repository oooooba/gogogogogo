package main

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
)

func Test1() int {
	f := func() int {
		return 1
	}
	return f()
}

func Test2() int {
	f := func(x int) int {
		return x
	}
	return f(2)
}

func Test3() int {
	f := func(x, y int) int {
		return x - y
	}
	return f(5, 2)
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
