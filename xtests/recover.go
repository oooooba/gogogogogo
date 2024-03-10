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

func test3_f(x *int) {
	defer func() {
		v := recover()
		if v == nil {
			*x = 3
		} else {
			*x = 1
		}
	}()
	panic(nil)
}

func Test3() int {
	x := 0
	test3_f(&x)
	if x != 3 {
		return x
	}
	return 3
}

func test4_f(x *int) {
	defer func() {
		v := recover()
		if v == nil {
			*x = 4
		} else {
			*x = 1
		}
	}()
	defer func() {
		v := recover()
		if v == nil {
			*x = 2
		} else {
			*x = 3
		}
	}()
	panic(nil)
}

func Test4() int {
	x := 0
	test4_f(&x)
	if x != 4 {
		return x
	}
	return 4
}

func test5_f(x *int) {
	defer func() {
		v := recover()
		if v == nil {
			*x = 1
		} else {
			*x = 2
		}
	}()
	panic(nil)
}

func test5_g(x *int) {
	defer func() {
		v := recover()
		if v == nil {
			if *x != 5 {
				*x = 3
			}
		} else {
			*x = 4
		}
	}()
	test5_f(x)
	if *x != 1 {
		return
	}
	*x = 5
}

func Test5() int {
	x := 0
	test5_g(&x)
	if x != 5 {
		return x
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
