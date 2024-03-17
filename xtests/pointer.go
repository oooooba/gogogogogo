package main

func Test1() int {
	a := 1
	b := &a
	return *b
}

func Test2() int {
	a := 2
	b := &a
	c := &b
	return **c
}

func Test3() int {
	a := 3
	b := &a
	c := &b
	d := &c
	return ***d
}

func test4_f() *int {
	a := 4
	return &a
}

func Test4() int {
	return *test4_f()
}

func test5_f() **int {
	a := 5
	b := &a
	return &b
}

func Test5() int {
	return **test5_f()
}

func test6_f(p *int) int {
	return *p
}

func Test6() int {
	a := 6
	return test6_f(&a)
}

func test7_f(p **int) int {
	return **p
}

func Test7() int {
	a := 7
	b := &a
	return test7_f(&b)
}

func test8_f(p *int) {
	*p = 8
}

func Test8() int {
	a := 7
	test8_f(&a)
	return a
}

func Test9() int {
	var a int
	b := &a
	return *b
}

func Test10() int {
	var p *int
	if p != nil {
		return 0
	}
	return 10
}

func Test11() int {
	f := func(x int) int {
		return x
	}
	if f(1) != 1 {
		return 1
	}
	f = nil
	if f != nil {
		return f(2)
	}
	return 11
}

func Test12() int {
	f := func() int {
		return 12
	}
	var g *(func() int)
	g = &f
	return (*g)()
}

func Test13() int {
	f := func() int {
		return 13
	}
	var g *(func() int)
	var h **(func() int)
	g = &f
	h = &g
	return (**h)()
}

func Test14() int {
	type Int int
	var n int = 42
	var p *int = &n
	var q *Int = (*Int)(p)
	*q = 14
	return int(*q)
}

func Test15() int {
	type Int int
	var n Int = 42
	var p *Int = &n
	var q *int = (*int)(p)
	*q = 15
	return *q
}

func Test16() int {
	type Int0 int
	type Int1 int
	var n Int0 = 42
	var p *Int0 = &n
	var q *Int1 = (*Int1)(p)
	*q = 16
	return int(*q)
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
	runTest("Test9", Test9)
	runTest("Test10", Test10)
	runTest("Test11", Test11)
	runTest("Test12", Test12)
	runTest("Test13", Test13)
	runTest("Test14", Test14)
	runTest("Test15", Test15)
	runTest("Test16", Test16)
}
