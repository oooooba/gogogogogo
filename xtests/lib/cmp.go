package main

import "cmp"

func TestLessInt() int {
	if !cmp.Less(1, 2) {
		return 1
	}
	if cmp.Less(2, 1) {
		return 2
	}
	if cmp.Less(1, 1) {
		return 3
	}
	return 0
}

func TestCompareInt() int {
	if cmp.Compare(1, 2) != -1 {
		return 1
	}
	if cmp.Compare(2, 1) != 1 {
		return 2
	}
	if cmp.Compare(1, 1) != 0 {
		return 3
	}
	return 0
}

func TestOrInt() int {
	r := cmp.Or(0, 1, 2)
	if r != 1 {
		return 1
	}
	return 0
}

func TestOrIntAllZero() int {
	var x int
	r := cmp.Or(x)
	if r != 0 {
		return 1
	}
	return 0
}

func TestLessString() int {
	if !cmp.Less("a", "b") {
		return 1
	}
	if cmp.Less("b", "a") {
		return 2
	}
	if cmp.Less("a", "a") {
		return 3
	}
	return 0
}

func TestCompareString() int {
	if cmp.Compare("a", "b") != -1 {
		return 1
	}
	if cmp.Compare("b", "a") != 1 {
		return 2
	}
	if cmp.Compare("a", "a") != 0 {
		return 3
	}
	return 0
}

func TestOrString() int {
	r := cmp.Or("", "hello", "world")
	if r != "hello" {
		return 1
	}
	return 0
}

func TestOrStringAllZero() int {
	r := cmp.Or("", "", "")
	if r != "" {
		return 1
	}
	return 0
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("TestLessInt", TestLessInt)
	runTest("TestCompareInt", TestCompareInt)
	runTest("TestOrInt", TestOrInt)
	runTest("TestOrIntAllZero", TestOrIntAllZero)
	runTest("TestLessString", TestLessString)
	runTest("TestCompareString", TestCompareString)
	runTest("TestOrString", TestOrString)
	runTest("TestOrStringAllZero", TestOrStringAllZero)
}
