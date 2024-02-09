package main

func Test1() int {
	ch := make(chan int)
	_ = ch
	return 1
}

func Test2() int {
	ch := make(chan int, 1)
	_ = ch
	return 2
}

func Test3() int {
	ch := make(chan int, 1)
	ch <- 42
	x := <-ch
	return x
}

func Test4() int {
	ch8 := make(chan uint8, 2)
	ch8 <- 0
	ch8 <- 1
	if (<-ch8) != 0 {
		return 0
	}
	if (<-ch8) != 1 {
		return 1
	}

	ch16 := make(chan uint16, 2)
	ch16 <- 2
	ch16 <- 3
	if (<-ch16) != 2 {
		return 2
	}
	if (<-ch16) != 3 {
		return 3
	}

	return 4
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("Test1", Test1)
	runTest("Test2", Test2)
	runTest("Test3", Test3)
	runTest("Test4", Test4)
}
