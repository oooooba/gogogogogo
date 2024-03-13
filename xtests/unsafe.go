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

func Test5() int {
	type T uintptr
	var x T = 5
	p := unsafe.Pointer(x)
	q := (T)(p)
	if x != q {
		return 0
	}
	return 5
}

func Test6() int {
	type T uintptr
	p := unsafe.Pointer(T(6))
	q := (uintptr)(p)
	if q != 6 {
		return 0
	}
	return 6
}

func Test7() int {
	type P unsafe.Pointer
	var x uintptr = 7
	p := P(x)
	q := (uintptr)(p)
	if x != q {
		return 0
	}
	return 7
}

func Test8() int {
	type P unsafe.Pointer
	p := P(uintptr(8))
	q := (uintptr)(p)
	if q != 8 {
		return 0
	}
	return 8
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("Test1", Test1)
	runTest("Test2", Test2)
	runTest("Test3", Test3)
	runTest("Test4", Test4)
	runTest("Test5", Test5)
	runTest("Test6", Test6)
	runTest("Test7", Test7)
	runTest("Test8", Test8)
}
