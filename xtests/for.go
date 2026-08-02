package main

func Test1() int {
	x := 1
	for false {
		x = 2
	}
	return x
}

func Test2() int {
	y := 3
	x := 1
	for false {
		y = x
		x = 2
	}
	return y
}

func Test3() int {
	s := 0
	for i := 10; i > 0; i-- {
		s += i
	}
	return s
}

func Test4() int {
	s := 0
	for i := 0; i < 10; i++ {
		s += i
	}
	return s
}

func Test5() int {
	s := 0
	for i := 0; i < 10; {
		s += i
		i++
	}
	return s
}

func Test6() int {
	s := 0
	for s < 100 {
		s = s + 9
	}
	return s
}

func Test7() int {
	i := 0
	for {
		i = i + 1
		if i > 5 {
			break
		}
	}
	return i
}

func Test8() int {
	s := 0
	for i := 0; i < 10; i++ {
		if i%2 == 0 {
			continue
		}
		s = s + i
	}
	return s
}

func Test9() int {
	a, b, c := 1, 2, 3
	for i := 0; i < 3; i++ {
		a, b, c = b, c, a
	}
	return a + b*10 + c*100
}

func Test10() int {
	b := 2.0
	a := 1.0
	for i := 0; i < 4; i++ {
		nb := b*2.0 - a
		a, b = b, nb
	}
	return int(a + b)
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
