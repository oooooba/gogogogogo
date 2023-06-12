package main

import (
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

func Test3() int {
	var i I
	s := S0{n: 3}
	i = &s
	return i.f()
}

func Test4() int {
	var i I
	s := S0{n: 0}
	i = &s
	s = S0{n: 4}
	return i.f()
}

type S2 struct {
	n int
	m int
}

func (s *S2) f() int {
	return s.n - s.m
}

func Test5() int {
	var i I
	s0 := S0{n: 1}
	i = &s0
	if i.f() != 1 {
		return 0
	}
	s2 := S2{n: 6, m: 1}
	i = &s2
	return i.f()
}

func Test6() int {
	f := func(i I) int {
		return i.f()
	}
	s0 := S0{n: 1}
	if f(&s0) != 1 {
		return 1
	}
	s1 := S1{m: 2}
	if f(&s1) != 2 {
		return 2
	}
	s2 := S2{n: 5, m: 2}
	if f(&s2) != 3 {
		return 3
	}
	return 6
}

type I1 interface {
	g(int) int
}

func (s *S0) g(x int) int {
	return s.n - x
}

func Test7() int {
	var i I1
	s := S0{n: 8}
	i = &s
	return i.g(1)
}

func (s *S1) g(x int) int {
	return s.m + x
}

func (s *S2) g(x int) int {
	return s.n + s.m - x
}

func Test8() int {
	var i I1
	s0 := S0{n: 2}
	i = &s0
	if i.g(1) != 1 {
		return 1
	}
	s1 := S1{m: 0}
	i = &s1
	if i.g(2) != 2 {
		return 2
	}
	s2 := S2{n: 2, m: 4}
	i = &s2
	if i.g(3) != 3 {
		return 3
	}
	return 8
}

func Test9() int {
	g := func(i I1, x int) int {
		return i.g(x)
	}
	s0 := S0{n: 2}
	if g(&s0, 1) != 1 {
		return 1
	}
	s1 := S1{m: 0}
	if g(&s1, 2) != 2 {
		return 2
	}
	s2 := S2{n: 2, m: 4}
	if g(&s2, 3) != 3 {
		return 3
	}
	return 9
}

type T int

func (t *T) f() int {
	return int(*t)
}

func Test10() int {
	var i I
	t := T(10)
	i = &t
	return i.f()
}

type T1 int

func (t1 T1) f() int {
	return int(t1) + 1
}

func Test11() int {
	var i I
	t := T(1)
	i = &t
	if i.f() != 1 {
		return 1
	}
	t1 := T1(2)
	i = &t1
	if i.f() != 3 {
		return 3
	}
	return 11
}

func Test12() int {
	f := func(i interface{}) int {
		return 12
	}
	return f(1)
}

func Test13() int {
	var i I
	s := S0{n: 42}
	i = &s
	ss := i.(*S0)
	if ss.n != 42 {
		return 0
	}
	s.n = 43
	if ss.n != 43 {
		return 1
	}
	return 13
}

func Test14() int {
	var i interface{}
	s := S0{n: 42}
	i = &s
	ss := i.(*S0)
	if ss.n != 42 {
		return 0
	}
	s.n = 43
	if ss.n != 43 {
		return 1
	}
	return 14
}

func Test15() int {
	var i interface{}
	n := 42
	i = n
	nn := i.(int)
	if nn != 42 {
		return 0
	}
	nn = 43
	if n != 42 {
		return 1
	}
	return 15
}

type I2 interface {
	f() int
	f2() int
}

func (s *S0) f2() int {
	return s.n + s.n
}

func Test16() int {
	s := S0{n: 42}
	var i I
	i = &s
	if i.f() != 42 {
		return 0
	}
	var i2 I2
	i2 = i.(I2)
	if i2.f() != 42 {
		return 1
	}
	if i2.f2() != 84 {
		return 2
	}
	return 16
}

func Test17() int {
	var i I
	s := S0{n: 42}
	i = &s
	ss, ok := i.(*S0)
	if !ok {
		return 0
	}
	if ss.n != 42 {
		return 1
	}
	sss, ok2 := i.(*S1)
	if ok2 {
		return 2
	}
	if sss != nil {
		return 3
	}
	return 17
}

func Test18() int {
	var i interface{}
	s := S0{n: 42}
	i = &s
	ss, ok := i.(*S0)
	if !ok {
		return 0
	}
	if ss.n != 42 {
		return 1
	}
	sss, ok2 := i.(*S1)
	if ok2 {
		return 2
	}
	if sss != nil {
		return 3
	}
	return 18
}

func Test19() int {
	var i interface{}
	n := 42
	i = n
	nn, ok := i.(int)
	if !ok {
		return 0
	}
	if nn != 42 {
		return 1
	}
	s, ok2 := i.(S0)
	if ok2 {
		return 2
	}
	if s.n != 0 {
		return 3
	}
	return 19
}

func Test20() int {
	s := S0{n: 42}
	var i I
	i = &s
	if i.f() != 42 {
		return 0
	}
	var i2 I2
	i2, ok := i.(I2)
	if !ok {
		return 1
	}
	if i2.f() != 42 {
		return 2
	}
	if i2.f2() != 84 {
		return 3
	}
	sss, ok2 := i.(*S1)
	if ok2 {
		return 4
	}
	if sss != nil {
		return 5
	}
	return 20
}

func Test21() int {
	var ia, ib I
	if ia != ib {
		return 0
	}
	return 21
}

func Test22() int {
	var ia, ib I

	var nt T = 42
	ia = &nt
	ib = &nt
	if ia != ib {
		return 0
	}
	if ia == nil {
		return 1
	}

	s := S0{n: 43}
	ia = &s
	ib = &s
	if ia != ib {
		return 2
	}
	if ia == nil {
		return 3
	}

	sa := S0{n: 44}
	sb := S0{n: 44}
	ia = &sa
	ib = &sb
	if ia == ib {
		return 4
	}

	return 22
}

func Test23() int {
	var ia, ib interface{}

	n := 42
	ia = n
	ib = n
	if ia != ib {
		return 0
	}
	if ia == nil {
		return 1
	}

	var nt T = 43
	ia = nt
	ib = nt
	if ia != ib {
		return 2
	}
	if ia == nil {
		return 3
	}

	s := S0{n: 44}
	ia = &s
	ib = &s
	if ia != ib {
		return 4
	}
	if ia == nil {
		return 5
	}

	sa := S0{n: 45}
	sb := S0{n: 45}
	ia = &sa
	ib = &sb
	if ia == ib {
		return 6
	}

	return 23
}

type I3 interface{}

func Test24() int {
	var ia, ib I3

	n := 42
	ia = n
	ib = n
	if ia != ib {
		return 0
	}
	if ia == nil {
		return 1
	}

	var nt T = 43
	ia = nt
	ib = nt
	if ia != ib {
		return 2
	}
	if ia == nil {
		return 3
	}

	s := S0{n: 44}
	ia = &s
	ib = &s
	if ia != ib {
		return 4
	}
	if ia == nil {
		return 5
	}

	sa := S0{n: 45}
	sb := S0{n: 45}
	ia = &sa
	ib = &sb
	if ia == ib {
		return 6
	}

	return 24
}

func Test25() int {
	var i interface{}
	var s *S0
	i = s
	if i != s {
		return 0
	}
	return 25
}

func Test26() int {
	var x interface{}
	var v *S0
	x = 42
	if x != 42 {
		return 0
	}
	if tx, ok := x.(int); ok {
		if tx != 42 {
			return 1
		}
	} else {
		return 2
	}
	x = v
	if x != v {
		return 3
	}
	if tv, ok := x.(*S0); ok {
		if tv != nil {
			return 4
		}
	} else {
		return 5
	}
	return 26
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
	runTest(Test11)
	runTest(Test12)
	runTest(Test13)
	runTest(Test14)
	runTest(Test15)
	runTest(Test16)
	runTest(Test17)
	runTest(Test18)
	runTest(Test19)
	runTest(Test20)
	runTest(Test21)
	runTest(Test22)
	runTest(Test23)
	runTest(Test24)
	runTest(Test25)
	runTest(Test26)
}
