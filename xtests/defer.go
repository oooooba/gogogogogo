package main

func Test1() int {
	a := 0
	func() {
		defer func() {
			a = 1
		}()
	}()
	if a != 1 {
		return 0
	}
	return 1
}

func Test2() int {
	a := 0
	func() {
		for i := 0; i < 10; i++ {
			defer func() {
				a += 1
			}()
		}
	}()
	if a != 10 {
		return 0
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
