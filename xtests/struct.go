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

func main() {
	runTest := func(test func() int) {
		funcFullName := runtime.FuncForPC(reflect.ValueOf(test).Pointer()).Name()
		funcName := strings.Split(funcFullName, ".")[1]
		fmt.Printf("%s: %d\n", funcName, test())
	}
	runTest(Test1)
}
