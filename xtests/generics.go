package main

func subtract[T int](x, y T) T {
	return x - y
}

func less[T int | int8 | uint16](x, y T) bool {
	return x < y
}

func Test1() int {
	return subtract(2, 1)
}

func Test2() int {
	if less(1, 2) {
		return 2
	}
	return 0
}

func Test3() int {
	if less(int(1), int(2)) {
		return 3
	}
	return 0
}

func Test4() int {
	if less(int8(1), int8(2)) {
		return 4
	}
	return 0
}

func Test5() int {
	if less(uint16(1), uint16(2)) {
		return 5
	}
	return 0
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
