package main

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
)

func Test1() int {
	x := 1
	for false {
		x = 2
	}
	return x
}

func Test2() int {
	y := 3
	x := 1
	for false {
		y = x
		x = 2
	}
	return y
}

func Test3() int {
	s := 0
	for i := 10; i > 0; i-- {
		s += i
	}
	return s
}

func Test4() int {
	s := 0
	for i := 0; i < 10; i++ {
		s += i
	}
	return s
}

func Test5() int {
	s := 0
	for i := 0; i < 10; {
		s += i
		i++
	}
	return s
}

func Test6() int {
	s := 0
	for s < 100 {
		s = s + 9
	}
	return s
}

func Test7() int {
	i := 0
	for {
		i = i + 1
		if i > 5 {
			break
		}
	}
	return i
}

func Test8() int {
	s := 0
	for i := 0; i < 10; i++ {
		if i%2 == 0 {
			continue
		}
		s = s + i
	}
	return s
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
}
