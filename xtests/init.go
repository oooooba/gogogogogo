package main

import (
	"reflect"
	"runtime"
	"strings"
)

var x = 10

func init() {
	x = 1
}

func Test1() int {
	return x
}

func main() {
	runTest := func(test func() int) {
		funcFullName := runtime.FuncForPC(reflect.ValueOf(test).Pointer()).Name()
		funcName := strings.Split(funcFullName, ".")[1]
		println(funcName+":", test())
	}
	runTest(Test1)
}
