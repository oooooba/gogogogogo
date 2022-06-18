package main

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
)

func test1_f() int {
	return 1
}

func Test1() int {
	go test1_f()
	return 2
}

func test2_f(ch chan int) int {
	ch <- 2
	return 1
}

func Test2() int {
	ch := make(chan int)
	go test2_f(ch)
	v := <-ch
	return v
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
