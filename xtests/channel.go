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

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("Test1", Test1)
	runTest("Test2", Test2)
	runTest("Test3", Test3)
}
