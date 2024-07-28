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

func test6_f(x *int) int {
	f := func() {
		if *x == 2 {
			*x = 3
		}
	}
	g := func() {
		defer f()
		if *x == 1 {
			*x = 2
		}
	}
	defer g()
	*x = 1
	return 6
}

func Test6() int {
	x := 0
	if test6_f(&x) != 6 {
		return 0
	}
	if x != 3 {
		return 1
	}
	return 6
}

type I interface {
	f(n int)
	g(n int)
	h() int
}

type S0 struct {
	n int
}

func (s *S0) f(n int) {
	s.n = n
}

func test7_f(s *S0) int {
	defer func() {
		if s.n == 43 {
			s.f(44)
		} else {
			s.n = 45
		}
	}()
	s.f(43)
	return 7
}

func Test7() int {
	s := S0{42}
	if test7_f(&s) != 7 {
		return 0
	}
	if s.n != 44 {
		return 1
	}
	return 7
}

func (s *S0) g(n int) {
	defer func() {
		s.n = n + 1
	}()
}

func test8_f(s *S0) int {
	defer func() {
		if s.n == 44 {
			s.g(44)
		} else {
			s.n = 46
		}
	}()
	s.g(43)
	return 8
}

func Test8() int {
	s := S0{42}
	if test8_f(&s) != 8 {
		return 0
	}
	if s.n != 45 {
		return 1
	}
	return 8
}

func test9_f(i I) int {
	defer func() {
		s := i.(*S0)
		if s.n == 44 {
			s.g(44)
		} else {
			s.n = 46
		}
	}()
	i.g(43)
	return 9
}

func Test9() int {
	s := S0{42}
	if test9_f(&s) != 9 {
		return 0
	}
	if s.n != 45 {
		return 1
	}
	return 9
}

func test10_f(i I) int {
	defer i.g(43)
	return 10
}

func Test10() int {
	s := S0{42}
	if test10_f(&s) != 10 {
		return 0
	}
	if s.n != 44 {
		return 1
	}
	return 10
}

func (s *S0) h() int {
	defer func() {
		s.n += 1
	}()
	return s.n
}

func test11_f(i I) int {
	defer i.h()
	return 11
}

func Test11() int {
	s := S0{42}
	if test11_f(&s) != 11 {
		return 0
	}
	if s.n != 43 {
		return 1
	}
	return 11
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
}
