package main

var x = 10

func init() {
	x = 1
}

func Test1() int {
	return x
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("Test1", Test1)
}
