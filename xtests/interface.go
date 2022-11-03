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

func main() {
	runTest := func(test func() int) {
		funcFullName := runtime.FuncForPC(reflect.ValueOf(test).Pointer()).Name()
		funcName := strings.Split(funcFullName, ".")[1]
		fmt.Printf("%s: %d\n", funcName, test())
	}
	runTest(Test1)
	runTest(Test2)
}
