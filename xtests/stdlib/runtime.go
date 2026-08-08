package main

import "runtime"

func TestGosched() int {
	runtime.Gosched()
	return 0
}

func TestGoexitInSpawnedGoroutine() int {
	done := make(chan struct{})
	go func() {
		defer close(done)
		runtime.Goexit()
	}()
	if _, ok := <-done; ok {
		return 1
	}
	return 0
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("TestGoexitInSpawnedGoroutine", TestGoexitInSpawnedGoroutine)
	runTest("TestGosched", TestGosched)
}
