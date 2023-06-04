package main

import (
	"reflect"
	"runtime"
	"strings"
)

func Test1() int {
	var a [10]int
	_ = a
	return 1
}

func Test2() int {
	var a [10]int
	return a[0]
}

func Test3() int {
	var a [10]int
	a[0] = 3
	return a[0]
}

func Test4() int {
	var a [10]int
	a[1] = 4
	return a[1]
}

func Test5() int {
	var a [10]int
	for i := 0; i < len(a); i++ {
		a[i] = i
	}
	sum := 0
	for i := 0; i < len(a); i++ {
		sum += a[i]
	}
	return sum
}

type S struct {
	x int
	y int
}

func Test6() int {
	var a [10]S
	a[0].x = 6
	return a[0].x
}

func Test7() int {
	var a [10]S
	a[0].y = 6
	return a[0].y
}

func Test8() int {
	var a [10]S
	for i := 0; i < len(a); i++ {
		a[i].x = i
	}
	sum := 0
	for i := 0; i < len(a); i++ {
		sum += a[i].x
	}
	return sum
}

func Test9() int {
	var a [10]S
	for i := 0; i < len(a); i++ {
		a[i].y = i
	}
	sum := 0
	for i := 0; i < len(a); i++ {
		sum += a[i].y
	}
	return sum
}

func Test10() int {
	a := [5]int{1, 2, 3, 4, 5}
	sum := 0
	for i := 0; i < len(a); i++ {
		sum += a[i]
	}
	return sum
}

func Test11() int {
	var a [5]int
	for i := 0; i < len(a); i++ {
		a[i] = 1
	}
	b := a
	for i := 0; i < len(b); i++ {
		b[i] = 2
	}
	sum := 0
	for i := 0; i < len(a); i++ {
		sum += a[i]
	}
	return sum
}

func Test12() int {
	var a [5]int
	for i := 0; i < len(a); i++ {
		a[i] = i
	}
	b := a
	for i := 0; i < len(b); i++ {
		if b[i] != i {
			return 0
		}
	}
	return 12
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
}
