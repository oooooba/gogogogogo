package main

func Test1() int {
	a := 0
	func() {
		defer func() {
			a = 1
		}()
	}()
	if a != 1 {
		return 0
	}
	return 1
}

func Test2() int {
	a := 0
	func() {
		for i := 0; i < 10; i++ {
			defer func() {
				a += 1
			}()
		}
	}()
	if a != 10 {
		return 0
	}
	return 2
}

func test3_f(p *int) {
	*p = 3
}

func Test3() int {
	a := 0
	func() {
		defer test3_f(&a)
	}()
	if a != 3 {
		return 0
	}
	return 3
}

func Test4() int {
	a := 0
	func() {
		defer func() int {
			a = 4
			return 42
		}()
	}()
	if a != 4 {
		return 0
	}
	return 4
}

func test5_f(p *int) int {
	defer func() {
		*p = 100
	}()
	return 50
}

func test5_g(p *int, q *int) int {
	defer func() {
		*q = 21
	}()
	x := test5_f(p)
	if x != 50 {
		return 54
	}
	if *p != 100 {
		return 53
	}
	if *q != 20 {
		return 52
	}
	*p = 11
	return 51
}

func Test5() int {
	a := 10
	b := 20
	x := test5_g(&a, &b)
	if x != 51 {
		return 0
	}
	if a != 11 {
		return 1
	}
	if b != 21 {
		return 2
	}
	return 5
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
}
