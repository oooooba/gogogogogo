package main

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
)

func Test1() int {
	ch := make(chan int)
	_ = ch
	return 1
}

func Test2() int {
	ch := make(chan int, 1)
	_ = ch
	return 2
}

func Test3() int {
	ch := make(chan int, 1)
	ch <- 42
	x := <-ch
	return x
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
