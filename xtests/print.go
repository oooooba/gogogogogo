package main

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
)

func Test1() int {
	fmt.Println(42)
	return 1
}

func Test2() int {
	fmt.Println("abc")
	return 2
}

func Test3() int {
	n := 43
	return func(n int) int {
		fmt.Println(n)
		return 3
	}(n)
}

func Test4() int {
	s := "def"
	return func(s string) int {
		fmt.Println(s)
		return 4
	}(s)
}

func Test5() int {
	n := 44
	s := "ghi"
	return func(n int, s string) int {
		fmt.Printf("%s: %d\n", s, n)
		return 5
	}(n, s)
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
