package main

import "errors"

type wrapErr struct {
	msg string
	err error
}

func (e *wrapErr) Error() string { return e.msg }
func (e *wrapErr) Unwrap() error { return e.err }

type plainErr struct{ msg string }

func (e *plainErr) Error() string { return e.msg }

func TestNew() int {
	e1 := errors.New("boom")
	if e1 == nil {
		return 1
	}
	if e1.Error() != "boom" {
		return 1
	}
	e2 := errors.New("boom")
	if e1 == e2 {
		return 1
	}
	if e2.Error() != "boom" {
		return 1
	}
	return 0
}

func TestErrorString() int {
	e := errors.New("hello error")
	if e.Error() != "hello error" {
		return 1
	}
	var nilErr error
	if nilErr != nil {
		return 1
	}
	if errors.New("x") == nil {
		return 1
	}
	return 0
}

func TestUnwrap() int {
	inner := errors.New("inner")
	outer := &wrapErr{msg: "outer", err: inner}
	if errors.Unwrap(outer) != inner {
		return 1
	}
	if errors.Unwrap(inner) != nil {
		return 1
	}
	if errors.Unwrap(nil) != nil {
		return 1
	}
	plain := &plainErr{msg: "plain"}
	if errors.Unwrap(plain) != nil {
		return 1
	}
	return 0
}

func TestChain() int {
	leaf := errors.New("leaf")
	mid := &wrapErr{msg: "mid", err: leaf}
	top := &wrapErr{msg: "top", err: mid}
	if errors.Unwrap(top) != mid {
		return 1
	}
	if errors.Unwrap(errors.Unwrap(top)) != leaf {
		return 1
	}
	if top.Error() != "top" {
		return 1
	}
	if mid.Error() != "mid" {
		return 1
	}
	return 0
}

func TestJoinNil() int {
	if errors.Join() != nil {
		return 1
	}
	if errors.Join(nil) != nil {
		return 1
	}
	if errors.Join(nil, nil) != nil {
		return 1
	}
	return 0
}

func TestJoin() int {
	a := errors.New("a")
	b := errors.New("b")
	j := errors.Join(a, b)
	if j == nil {
		return 1
	}
	if j.Error() != "a\nb" {
		return 1
	}
	single := errors.Join(a)
	if single == nil || single == a {
		return 1
	}
	if single.Error() != "a" {
		return 1
	}
	return 0
}

func TestJoinUnwrap() int {
	a := errors.New("a")
	b := errors.New("b")
	c := errors.New("c")
	j := errors.Join(a, b, c)
	multi, ok := j.(interface{ Unwrap() []error })
	if !ok {
		return 1
	}
	errs := multi.Unwrap()
	if len(errs) != 3 {
		return 1
	}
	if errs[0] != a || errs[1] != b || errs[2] != c {
		return 1
	}
	if errors.Unwrap(j) != nil {
		return 1
	}
	return 0
}

func TestJoinSingleNonNil() int {
	a := errors.New("a")
	j := errors.Join(nil, a, nil)
	if j == nil || j == a {
		return 1
	}
	if j.Error() != "a" {
		return 1
	}
	return 0
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("TestNew", TestNew)
	runTest("TestErrorString", TestErrorString)
	runTest("TestUnwrap", TestUnwrap)
	runTest("TestChain", TestChain)
	runTest("TestJoinNil", TestJoinNil)
	runTest("TestJoin", TestJoin)
	runTest("TestJoinUnwrap", TestJoinUnwrap)
	runTest("TestJoinSingleNonNil", TestJoinSingleNonNil)
}
