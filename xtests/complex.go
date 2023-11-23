package main

import (
	"reflect"
	"runtime"
	"strings"
)

func Test1() int {
	c := 0i
	if c == 1i {
		return 0
	}
	return 1
}

func Test2() int {
	c := 1 + 2i
	if real(c) != 1 {
		return 0
	}
	if imag(c) != 2 {
		return 1
	}
	return 2
}

func Test3() int {
	x := func() float64 {
		return 3
	}()
	y := func() float64 {
		return 4
	}()
	c := complex(x, y)
	if c != 3+4i {
		return 0
	}
	return 3
}

func Test4() int {
	c1 := 10 + 20i
	c2 := 2 + 1i
	if c1+c2 != 12+21i {
		return 0
	}
	if c1-c2 != 8+19i {
		return 1
	}
	if c1*c2 != 50i {
		return 2
	}
	if c1/c2 != 8+6i {
		return 3
	}
	return 4
}

func Test5() int {
	f := func() complex64 {
		return 1 + 2i
	}
	c1 := f()
	if real(c1) != 1 {
		return 0
	}
	if imag(c1) != 2 {
		return 1
	}
	var c2 complex64 = complex(3, 4)
	var c3 complex64 = f()
	if c2+c3 != 4+6i {
		return 2
	}
	return 5
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
}
