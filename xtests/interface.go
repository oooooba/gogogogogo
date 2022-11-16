package main

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
)

type I interface {
	f() int
}

type S0 struct {
	n int
}

func (s *S0) f() int {
	return s.n
}

func Test1() int {
	s := S0{n: 1}
	return s.f()
}

type S1 struct {
	m int
}

func (s *S1) f() int {
	return s.m
}

func Test2() int {
	s := S1{m: 2}
	return s.f()
}

func Test3() int {
	var i I
	s := S0{n: 3}
	i = &s
	return i.f()
}

func Test4() int {
	var i I
	s := S0{n: 0}
	i = &s
	s = S0{n: 4}
	return i.f()
}

type S2 struct {
	n int
	m int
}

func (s *S2) f() int {
	return s.n - s.m
}

func Test5() int {
	var i I
	s0 := S0{n: 1}
	i = &s0
	if i.f() != 1 {
		return 0
	}
	s2 := S2{n: 6, m: 1}
	i = &s2
	return i.f()
}

func Test6() int {
	f := func(i I) int {
		return i.f()
	}
	s0 := S0{n: 1}
	if f(&s0) != 1 {
		return 1
	}
	s1 := S1{m: 2}
	if f(&s1) != 2 {
		return 2
	}
	s2 := S2{n: 5, m: 2}
	if f(&s2) != 3 {
		return 3
	}
	return 6
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
}
