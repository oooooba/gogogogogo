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

func Test5() int {
	ch := make(chan int)
	close(ch)
	v, ok := <-ch
	if ok {
		return 0
	}
	if v != 0 {
		return 1
	}
	return 5
}

func Test6() int {
	ch := make(chan int, 1)
	ch <- 42
	close(ch)
	v, ok := <-ch
	if !ok {
		return 0
	}
	if v != 42 {
		return 1
	}
	v, ok = <-ch
	if ok {
		return 2
	}
	if v != 0 {
		return 3
	}
	return 6
}

func Test7() int {
	ch1 := make(chan int)
	ch2 := make(chan int)
	close(ch1)
	close(ch2)
	select {
	case v, ok := <-ch1:
		if ok {
			return 0
		}
		if v != 0 {
			return 1
		}
	case v, ok := <-ch2:
		if ok {
			return 2
		}
		if v != 0 {
			return 3
		}
	}
	return 7
}

func Test8() int {
	ch1 := make(chan int, 1)
	ch2 := make(chan int, 1)
	ch1 <- 1
	ch2 <- 2
	close(ch1)
	close(ch2)
	reached1 := false
	reached2 := false
	for !(reached1 && reached2) {
		select {
		case v, ok := <-ch1:
			if ok {
				if reached1 {
					return 0
				}
				if v != 1 {
					return 1
				}
				reached1 = true
			}
		case v, ok := <-ch2:
			if ok {
				if reached2 {
					return 2
				}
				if v != 2 {
					return 3
				}
				reached2 = true
			}
		}
	}
	return 8
}

func Test9() int {
	ch := make(chan int, 1)
	for {
		select {
		case ch <- 42:
		case v := <-ch:
			if v != 42 {
				return 0
			}
			goto L
		}
	}
L:
	return 9
}

func Test10() int {
	ch0 := make(chan int)
	ch1 := make(chan int, 1)
	ch2 := make(chan int, 1)
	ch2 <- 2
	for {
		select {
		case <-ch0:
			return 0
		case <-ch1:
			return 1
		case ch0 <- 20:
			return 2
		case ch2 <- 30:
			return 3
		default:
			goto L
		}
	}
L:
	return 10
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
}
