package main

import (
	"reflect"
	"runtime"
	"strings"
)

type T int

func Test1() int {
	t := T(1)
	f := func(t T) int {
		return int(t)
	}
	return f(t)
}

func Test2() int {
	t := T(2)
	f := func(t *T) int {
		return int(*t)
	}
	return f(&t)
}

func Test3() int {
	t := T(2)
	func(t *T) {
		*t = 3
	}(&t)
	return int(t)
}

func (t T) f() int {
	return int(t)
}

func Test4() int {
	t := T(4)
	return t.f()
}

type T1 int

func (t1 T1) f() int {
	return int(t1) + 1
}

func Test5() int {
	t := T(1)
	if t.f() != 1 {
		return 1
	}
	t1 := T1(2)
	if t1.f() != 3 {
		return 3
	}
	return 5
}

func Test6() int {
	var n int
	var t T
	var t1 T1
	n = 6
	t = T(n)
	t1 = T1(t)
	return int(t1)
}

type I64 int64
type I16 int16

func Test7() int {
	x := I16(7)
	y := I64(x)
	if y != 7 {
		return 0
	}
	return 7
}

type M map[int]int

func Test8() int {
	m := M{1: 10, 2: 20, 3: 30}
	if len(m) != 3 {
		return 0
	}
	if m[1] != 10 {
		return 1
	}
	if m[2] != 20 {
		return 2
	}
	if m[3] != 30 {
		return 3
	}
	return 8
}

type T9 float64

func Test9() int {
	f := func(x T9) T9 {
		return T9(float32(x))
	}
	var x T9 = 0xe0000000
	y := f(x)
	if x != y {
		return 0
	}
	return 9
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
}
