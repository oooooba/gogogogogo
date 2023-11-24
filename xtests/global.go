package main

var v1 int

func Test1() int {
	if v1 != 0 {
		return 0
	}
	return 1
}

var v2 int

func Test2() int {
	v2 = 2
	return v2
}

type S3 struct {
	x int
}

var v3 S3

func Test3() int {
	if v3.x != 0 {
		return 0
	}
	return 3
}

type S4 struct {
	x int
}

var v4 S4

func Test4() int {
	v4 = S4{4}
	return v4.x
}

var v5 struct {
	x int
}

func Test5() int {
	if v5.x != 0 {
		return 0
	}
	return 5
}

var v6 struct {
	x int
}

func Test6() int {
	v6.x = 6
	return v6.x
}

var v7 = [3]int{1, 2, 3}

func Test7() int {
	if len(v7) != 3 {
		return 0
	}
	if v7[0] != 1 {
		return 1
	}
	if v7[1] != 2 {
		return 2
	}
	if v7[2] != 3 {
		return 3
	}
	return 7
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
}
