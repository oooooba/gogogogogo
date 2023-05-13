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

type S1 struct {
	n int
	m int
}

func Test4() int {
	s := S1{}
	return s.n
}

func Test5() int {
	s := S1{}
	return s.m
}

func Test6() int {
	s := S1{n: 6}
	return s.n
}

func Test7() int {
	s := S1{n: 7}
	return s.m
}

func Test8() int {
	s := S1{m: 8}
	return s.n
}

func Test9() int {
	s := S1{m: 9}
	return s.m
}

func Test10() int {
	s := S1{n: 10, m: 11}
	return s.n
}

func Test11() int {
	s := S1{n: 10, m: 11}
	return s.m
}

func (s *S1) methodRefRead() int {
	return s.n - s.m
}

func (s *S1) methodRefWrite(x, y int) {
	s.n = x
	s.m = y
}
func Test12() int {
	s := S1{n: 22, m: 10}
	return s.methodRefRead()
}

func Test13() int {
	s := S1{n: 1, m: 2}
	s.methodRefWrite(23, 10)
	if s.n != 23 {
		return 1
	}
	if s.m != 10 {
		return 2
	}
	return s.methodRefRead()
}

func (s S1) methodCopyRead() int {
	return s.n - s.m
}

func (s S1) methodCopyWrite(x, y int) {
	s.n = x
	s.m = y
}

func Test14() int {
	s := S1{n: 24, m: 10}
	return s.methodCopyRead()
}

func Test15() int {
	s := S1{n: 25, m: 10}
	s.methodCopyWrite(1, 2)
	if s.n != 25 {
		return 1
	}
	if s.m != 10 {
		return 2
	}
	return s.methodCopyRead()
}

func Test16() int {
	s := struct{ x int }{16}
	return s.x
}

type S2 struct {
	x int
	y *S2
}

func Test17() int {
	s := S2{x: 17}
	if s.y != nil {
		return 0
	}
	return s.x
}

func Test18() int {
	v0 := S2{x: 0, y: nil}
	v1 := S2{x: 1, y: &v0}
	if v1.x != 1 {
		return 0
	}
	if v1.y == nil {
		return 1
	}
	if v1.y.x != 0 {
		return 2
	}
	if v1.y.y != nil {
		return 3
	}
	return 18
}

type S3 struct {
	x int
	y *S4
}

type S4 struct {
	a int
	b *S3
}

func Test19() int {
	v0 := S3{x: 3, y: nil}
	v1 := S4{a: 4, b: &v0}
	v0.y = &v1
	if v0.x != 3 {
		return 0
	}
	if v0.y != &v1 {
		return 1
	}
	if v1.a != 4 {
		return 2
	}
	if v1.b != &v0 {
		return 3
	}
	return 19
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
	runTest(Test8)
	runTest(Test9)
	runTest(Test10)
	runTest(Test11)
	runTest(Test12)
	runTest(Test13)
	runTest(Test14)
	runTest(Test15)
	runTest(Test16)
	runTest(Test17)
	runTest(Test18)
	runTest(Test19)
}
