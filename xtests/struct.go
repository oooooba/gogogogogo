package main

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
)

type S struct {
	n int
}

func Test1() int {
	_ = S{}
	return 1
}

func Test2() int {
	_ = S{n: 42}
	return 2
}

func Test3() int {
	s := S{n: 3}
	return s.n
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
