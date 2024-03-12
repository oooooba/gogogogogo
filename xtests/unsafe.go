package main

import "unsafe"

func Test1() int {
	var p unsafe.Pointer
	if p != nil {
		return 0
	}
	return 1
}

func Test2() int {
	var x int
	p := unsafe.Pointer(&x)
	q := (*int)(p)
	if q != &x {
		return 0
	}
	return 2
}

func Test3() int {
	var x uintptr = 3
	p := unsafe.Pointer(x)
	q := (uintptr)(p)
	if x != q {
		return 0
	}
	return 3
}

func Test4() int {
	p := unsafe.Pointer(uintptr(4))
	q := (uintptr)(p)
	if q != 4 {
		return 0
	}
	return 4
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("Test1", Test1)
	runTest("Test2", Test2)
	runTest("Test3", Test3)
	runTest("Test4", Test4)
}
