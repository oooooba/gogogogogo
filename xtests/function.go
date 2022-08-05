package main

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
)

func test1_f(x int) int {
	n := 1 + 2
	m := n - 6
	return m + x
}

func Test1() int {
	x := test1_f(1)
	y := test1_f(2)
	return x + y
}

func test2_f(x int) int {
	if x > 0 {
		return x + test2_f(x-1)
	} else {
		return 0
	}
}

func Test2() int {
	return test2_f(10)
}

func test3_f(x int, y int) int {
	return x - y
}

func Test3() int {
	return test3_f(5, 2)
}

func test4_f() int {
	return 1
}

func Test4() int {
	return test4_f()
}

func test5_f(a int) int {
	return 1
}

func Test5() int {
	return test5_f(2)
}

func test6_f(n int) int {
	if n == 0 {
		return 0
	} else {
		return n + test6_f(n-1)
	}
}

func Test6() int {
	return test6_f(10)
}

func test7_f(f func() int) int {
	return f()
}

func test7_g() int {
	return 7
}

func Test7() int {
	return test7_f(test7_g)
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
}
