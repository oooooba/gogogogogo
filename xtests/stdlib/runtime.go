package main

import "runtime"

func TestGosched() int {
	runtime.Gosched()
	return 0
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("TestGosched", TestGosched)
}
