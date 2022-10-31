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
}
