package main

func Test1() int {
	var a [10]int
	_ = a
	return 1
}

func Test2() int {
	var a [10]int
	return a[0]
}

func Test3() int {
	var a [10]int
	a[0] = 3
	return a[0]
}

func Test4() int {
	var a [10]int
	a[1] = 4
	return a[1]
}

func Test5() int {
	var a [10]int
	for i := 0; i < len(a); i++ {
		a[i] = i
	}
	sum := 0
	for i := 0; i < len(a); i++ {
		sum += a[i]
	}
	return sum
}

type S struct {
	x int
	y int
}

func Test6() int {
	var a [10]S
	a[0].x = 6
	return a[0].x
}

func Test7() int {
	var a [10]S
	a[0].y = 6
	return a[0].y
}

func Test8() int {
	var a [10]S
	for i := 0; i < len(a); i++ {
		a[i].x = i
	}
	sum := 0
	for i := 0; i < len(a); i++ {
		sum += a[i].x
	}
	return sum
}

func Test9() int {
	var a [10]S
	for i := 0; i < len(a); i++ {
		a[i].y = i
	}
	sum := 0
	for i := 0; i < len(a); i++ {
		sum += a[i].y
	}
	return sum
}

func Test10() int {
	a := [5]int{1, 2, 3, 4, 5}
	sum := 0
	for i := 0; i < len(a); i++ {
		sum += a[i]
	}
	return sum
}

func Test11() int {
	var a [5]int
	for i := 0; i < len(a); i++ {
		a[i] = 1
	}
	b := a
	for i := 0; i < len(b); i++ {
		b[i] = 2
	}
	sum := 0
	for i := 0; i < len(a); i++ {
		sum += a[i]
	}
	return sum
}

func Test12() int {
	var a [5]int
	for i := 0; i < len(a); i++ {
		a[i] = i
	}
	b := a
	for i := 0; i < len(b); i++ {
		if b[i] != i {
			return 0
		}
	}
	return 12
}

func Test13() int {
	a := [5]int{0, 2, 4, 6, 8}
	var found [5]bool
	for i := range a {
		if i >= 5 {
			return 0
		}
		if found[i] {
			return (i + 1) * 10
		}
		if i*2 != a[i] {
			return (i + 1) * 100
		}
		found[i] = true
	}
	for i := 0; i < 5; i++ {
		if !found[i] {
			return (i + 1) * 1000
		}
	}
	return 13
}

func Test14() int {
	a := [5]int{0, 2, 4, 6, 8}
	var found [5]bool
	for i, x := range a {
		if i >= 5 {
			return 0
		}
		if found[i] {
			return (i + 1) * 10
		}
		if i*2 != x {
			return (i + 1) * 100
		}
		found[i] = true
	}
	for i := 0; i < 5; i++ {
		if !found[i] {
			return (i + 1) * 1000
		}
	}
	return 14
}

func Test15() int {
	type Arr [3]int
	a := Arr{0, 1, 2}
	for i := 0; i < 3; i++ {
		if a[i] != i {
			return i
		}
	}
	return 15
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
	runTest("Test11", Test11)
	runTest("Test12", Test12)
	runTest("Test13", Test13)
	runTest("Test14", Test14)
	runTest("Test15", Test15)
}
