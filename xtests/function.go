package main

import (
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

func test8_f(s ...int) int {
	if len(s) != 0 {
		return 0
	}
	return 8
}

func Test8() int {
	return test8_f()
}

func test9_f(s ...int) int {
	if len(s) != 3 {
		return 0
	}
	for i, x := range s {
		if x != i+1 {
			return i + 1
		}
	}
	return 9
}

func Test9() int {
	return test9_f(1, 2, 3)
}

func test10_f(s ...int) int {
	if len(s) != 4 {
		return 0
	}
	for i, x := range s {
		if x != i+i {
			return i + 1
		}
	}
	return 10
}

func Test10() int {
	s := []int{0, 2, 4, 6}
	return test10_f(s...)
}

func test11_f(s ...int) {
	for i := 0; i < len(s); i++ {
		s[i] = i + i + i
	}
}

func Test11() int {
	s := []int{0, 1, 2, 3, 4}
	test11_f(s...)
	for i, x := range s {
		if x != i+i+i {
			return i
		}
	}
	return 11
}

func Test12() int {
	f := func() (int, int) {
		return 12, 10
	}
	f()
	return 12
}

func Test13() int {
	f := func() (int, int) {
		return 13, 10
	}
	a, _ := f()
	return a
}

func main() {
	runTest := func(test func() int) {
		funcFullName := runtime.FuncForPC(reflect.ValueOf(test).Pointer()).Name()
		funcName := strings.Split(funcFullName, ".")[1]
		println(funcName+":", test())
	}
	runTest(Test1)
	runTest(Test2)
	runTest(Test3)
	runTest(Test4)
	runTest(Test5)
	runTest(Test6)
	runTest(Test7)
	runTest(Test8)
	runTest(Test9)
	runTest(Test10)
	runTest(Test11)
	runTest(Test12)
	runTest(Test13)
}
