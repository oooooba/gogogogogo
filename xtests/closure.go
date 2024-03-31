package main

func Test1() int {
	f := func() int {
		return 1
	}
	return f()
}

func Test2() int {
	f := func(x int) int {
		return x
	}
	return f(2)
}

func Test3() int {
	f := func(x, y int) int {
		return x - y
	}
	return f(5, 2)
}

func Test4() int {
	x := 0
	f := func() int {
		return x
	}
	x = 4
	return f()
}

func Test5() int {
	x := 0
	func() {
		x = 5
	}()
	return x
}

func Test6() int {
	f := func() int {
		g := func() int {
			return 6
		}
		return g()
	}
	return f()
}

func Test7() int {
	x := 0
	y := 0
	func() {
		x = 3
		y = 4
	}()
	if x != 3 {
		return 0
	}
	if y != 4 {
		return 1
	}
	return 7
}

type T int

func (t *T) test8_f() int {
	return int(*t)
}

func test8_g(f func() int) int {
	return f()
}

func Test8() int {
	t := T(8)
	return test8_g(t.test8_f)
}

func (t *T) test9_f(n int) int {
	return int(*t) + n
}

func test9_g(f func(int) int) int {
	return f(1)
}

func Test9() int {
	t := T(8)
	return test9_g(t.test9_f)
}

func (t *T) test10_f(n int) {
	*t = T(n)
}

func test10_g(f func(int)) {
	f(10)
}

func Test10() int {
	t := T(42)
	test10_g(t.test10_f)
	return int(t)
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
}
