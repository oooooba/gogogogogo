package main

func Test1() int {
	v := recover()
	if v != nil {
		return 0
	}
	return 1
}

func test2_f(x *int) {
	defer func() {
		v := recover()
		if v == nil {
			*x = 2
		} else {
			*x = 1
		}
	}()
}

func Test2() int {
	x := 0
	test2_f(&x)
	if x != 2 {
		return x
	}
	return 2
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("Test1", Test1)
	runTest("Test2", Test2)
}
