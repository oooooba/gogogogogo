package main

type S struct {
	n int
}

func Test1() int {
	_ = S{}
	return 1
}

func Test2() int {
	_ = S{n: 42}
	return 2
}

func Test3() int {
	s := S{n: 3}
	return s.n
}

type S1 struct {
	n int
	m int
}

func Test4() int {
	s := S1{}
	return s.n
}

func Test5() int {
	s := S1{}
	return s.m
}

func Test6() int {
	s := S1{n: 6}
	return s.n
}

func Test7() int {
	s := S1{n: 7}
	return s.m
}

func Test8() int {
	s := S1{m: 8}
	return s.n
}

func Test9() int {
	s := S1{m: 9}
	return s.m
}

func Test10() int {
	s := S1{n: 10, m: 11}
	return s.n
}

func Test11() int {
	s := S1{n: 10, m: 11}
	return s.m
}

func (s *S1) methodRefRead() int {
	return s.n - s.m
}

func (s *S1) methodRefWrite(x, y int) {
	s.n = x
	s.m = y
}
func Test12() int {
	s := S1{n: 22, m: 10}
	return s.methodRefRead()
}

func Test13() int {
	s := S1{n: 1, m: 2}
	s.methodRefWrite(23, 10)
	if s.n != 23 {
		return 1
	}
	if s.m != 10 {
		return 2
	}
	return s.methodRefRead()
}

func (s S1) methodCopyRead() int {
	return s.n - s.m
}

func (s S1) methodCopyWrite(x, y int) {
	s.n = x
	s.m = y
}

func Test14() int {
	s := S1{n: 24, m: 10}
	return s.methodCopyRead()
}

func Test15() int {
	s := S1{n: 25, m: 10}
	s.methodCopyWrite(1, 2)
	if s.n != 25 {
		return 1
	}
	if s.m != 10 {
		return 2
	}
	return s.methodCopyRead()
}

func Test16() int {
	s := struct{ x int }{16}
	return s.x
}

type S2 struct {
	x int
	y *S2
}

func Test17() int {
	s := S2{x: 17}
	if s.y != nil {
		return 0
	}
	return s.x
}

func Test18() int {
	v0 := S2{x: 0, y: nil}
	v1 := S2{x: 1, y: &v0}
	if v1.x != 1 {
		return 0
	}
	if v1.y == nil {
		return 1
	}
	if v1.y.x != 0 {
		return 2
	}
	if v1.y.y != nil {
		return 3
	}
	return 18
}

type S3 struct {
	x int
	y *S4
}

type S4 struct {
	a int
	b *S3
}

func Test19() int {
	v0 := S3{x: 3, y: nil}
	v1 := S4{a: 4, b: &v0}
	v0.y = &v1
	if v0.x != 3 {
		return 0
	}
	if v0.y != &v1 {
		return 1
	}
	if v1.a != 4 {
		return 2
	}
	if v1.b != &v0 {
		return 3
	}
	return 19
}

type S5 struct {
	x int
}

func (s *S5) test20_f() {
	s.x = 20
}

func Test20() int {
	s := S5{}
	s.test20_f()
	return s.x
}

type S6 struct {
	x int
	y *int
}

type S7 struct {
	x int
	y string
}

type S8 struct {
	x int
	y *string
}

func Test21() int {
	l_s_1 := S{n: 1}
	r_s_1 := S{n: 1}
	if l_s_1 != r_s_1 {
		return 0
	}

	l_s_2 := S{n: 1}
	r_s_2 := S{n: 2}
	if l_s_2 == r_s_2 {
		return 1
	}

	l_s1_1 := S1{n: 2, m: 2}
	r_s1_1 := S1{n: 2, m: 2}
	if l_s1_1 != r_s1_1 {
		return 2
	}

	l_s1_2 := S1{n: 2, m: 2}
	r_s1_2 := S1{n: 2, m: 3}
	if l_s1_2 == r_s1_2 {
		return 3
	}

	l_s6_1 := S6{x: 3, y: nil}
	r_s6_1 := S6{x: 3, y: nil}
	if l_s6_1 != r_s6_1 {
		return 4
	}

	n_s6_2 := 4
	l_s6_2 := S6{x: 3, y: &n_s6_2}
	r_s6_2 := S6{x: 3, y: &n_s6_2}
	if l_s6_2 != r_s6_2 {
		return 5
	}

	n_s6_3 := 4
	l_s6_3 := S6{x: 3, y: &n_s6_2}
	r_s6_3 := S6{x: 3, y: &n_s6_3}
	if l_s6_3 == r_s6_3 {
		return 5
	}

	l_s7_1 := S7{x: 4, y: ""}
	r_s7_1 := S7{x: 4, y: ""}
	if l_s7_1 != r_s7_1 {
		return 6
	}

	l_s7_2 := S7{x: 4, y: "abc"}
	r_s7_2 := S7{x: 4, y: "abc"}
	if l_s7_2 != r_s7_2 {
		return 7
	}

	l_s7_3 := S7{x: 4, y: "abc"}
	r_s7_3 := S7{x: 4, y: "def"}
	if l_s7_3 == r_s7_3 {
		return 8
	}

	s_n7_4 := "a"
	s_n7_4 += "bc"
	l_s7_4 := S7{x: 4, y: "abc"}
	r_s7_4 := S7{x: 4, y: s_n7_4}
	if l_s7_4 != r_s7_4 {
		return 9
	}

	l_s8_1 := S8{x: 5, y: nil}
	r_s8_1 := S8{x: 5, y: nil}
	if l_s8_1 != r_s8_1 {
		return 10
	}

	s_n8_2 := "abc"
	l_s8_2 := S8{x: 5, y: &s_n8_2}
	r_s8_2 := S8{x: 5, y: &s_n8_2}
	if l_s8_2 != r_s8_2 {
		return 11
	}

	s_n8_3 := "abc"
	l_s8_3 := S8{x: 5, y: &s_n8_2}
	r_s8_3 := S8{x: 5, y: &s_n8_3}
	if l_s8_3 == r_s8_3 {
		return 12
	}

	return 21
}

type S9 struct {
	x int
}

func Test22() int {
	f := func() S9 {
		return S9{x: 22}
	}
	return f().x
}

type S10 struct {
	_ int
	x int
	_ struct{}
	y int
	_ string
}

func Test23() int {
	f := func() S10 {
		return S10{x: 1, y: 2}
	}
	s1 := f()
	s2 := S10{x: 1, y: 2}
	if s1 != s2 {
		return 0
	}
	return 23
}

func Test24() int {
	type S struct{ x int }
	type P *S
	f := func(s P) {
		s.x = 1
	}
	s0 := S{0}
	f(P(&s0))
	if s0.x != 1 {
		return 0
	}
	return 24
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
	runTest("Test17", Test17)
	runTest("Test18", Test18)
	runTest("Test19", Test19)
	runTest("Test20", Test20)
	runTest("Test21", Test21)
	runTest("Test22", Test22)
	runTest("Test23", Test23)
	runTest("Test24", Test24)
}
