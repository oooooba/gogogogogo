package main

import (
	"reflect"
	"runtime"
	"strings"
)

func test1_f() int {
	return 1
}

func Test1() int {
	go test1_f()
	return 2
}

func test2_f(ch chan int) int {
	ch <- 2
	return 1
}

func Test2() int {
	ch := make(chan int)
	go test2_f(ch)
	v := <-ch
	return v
}

func test3_f(ch chan int) {
	ch <- 3
}

func Test3() int {
	ch := make(chan int)
	go test3_f(ch)
	v := <-ch
	return v
}

func test4_f(ch chan int, n int) {
	ch <- n
}

func Test4() int {
	ch := make(chan int)
	go test4_f(ch, 4)
	v := <-ch
	return v
}

func test5_f(ch chan int, x, y int) {
	ch <- x
}

func Test5() int {
	ch := make(chan int)
	go test5_f(ch, 5, 6)
	v := <-ch
	return v
}

func test6_f(ch chan int, x, y int) {
	ch <- y
}

func Test6() int {
	ch := make(chan int)
	go test6_f(ch, 5, 6)
	v := <-ch
	return v
}

func test7_f(ch chan int) {
	for i := 0; i < 10; i++ {
		ch <- i
	}
}

func Test7() int {
	ch := make(chan int, 1)
	go test7_f(ch)
	for i := 0; i < 10; i++ {
		v := <-ch
		if v != i {
			return 0
		}
	}
	return 7
}

func test8_f(ch chan int, x int, y int, z int) {
	ch <- x
	ch <- y
	ch <- z
}

func Test8() int {
	ch := make(chan int)
	go test8_f(ch, 1, 2, 3)
	v := <-ch
	if v != 1 {
		return 1
	}
	v = <-ch
	if v != 2 {
		return 2
	}
	v = <-ch
	if v != 3 {
		return 3
	}
	return 8
}

func test9_f(ch chan int) func() {
	return func() {
		ch <- 9
	}
}

func Test9() int {
	ch := make(chan int)
	go test9_f(ch)()
	v := <-ch
	return v
}

func Test10() int {
	ch := make(chan int)
	go func(ch chan int) {
		ch <- 10
	}(ch)
	runtime.Gosched()
	v := <-ch
	return v
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
}
