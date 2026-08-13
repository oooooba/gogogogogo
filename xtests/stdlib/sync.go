package main

import (
	"sync"
)

func TestMutex() int {
	var mu sync.Mutex
	n := 0
	mu.Lock()
	n++
	mu.Unlock()
	mu.Lock()
	if n != 1 {
		return 0
	}
	mu.Unlock()
	return 1
}

func TestTryLock() int {
	var mu sync.Mutex
	if !mu.TryLock() {
		return 0
	}
	if mu.TryLock() {
		return 0
	}
	mu.Unlock()
	if !mu.TryLock() {
		return 0
	}
	mu.Unlock()
	return 1
}

func TestRWMutex() int {
	var mu sync.RWMutex
	mu.RLock()
	mu.RUnlock()
	mu.Lock()
	mu.Unlock()
	mu.RLock()
	mu.RUnlock()
	return 1
}

func TestWaitGroup() int {
	var wg sync.WaitGroup
	wg.Add(1)
	wg.Done()
	wg.Wait()
	return 1
}

func TestWaitGroupAddMany() int {
	var wg sync.WaitGroup
	wg.Add(3)
	wg.Done()
	wg.Done()
	wg.Done()
	wg.Wait()
	wg.Add(2)
	wg.Done()
	wg.Done()
	wg.Wait()
	return 1
}

func TestOnce() int {
	var once sync.Once
	calls := 0
	once.Do(func() { calls++ })
	once.Do(func() { calls++ })
	if calls != 1 {
		return 0
	}
	return 1
}

func TestOnceNoGo() int {
	var once sync.Once
	n := 0
	for i := 0; i < 10; i++ {
		once.Do(func() { n += 3 })
	}
	if n != 3 {
		return 0
	}
	return 1
}

var counter int
var counterMu sync.Mutex

func worker(wg *sync.WaitGroup) {
	for i := 0; i < 100; i++ {
		counterMu.Lock()
		counter++
		counterMu.Unlock()
	}
	wg.Done()
}

func TestConcurrent() int {
	var wg sync.WaitGroup
	counter = 0
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go worker(&wg)
	}
	wg.Wait()
	if counter != 400 {
		return 0
	}
	return 1
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("TestMutex", TestMutex)
	runTest("TestTryLock", TestTryLock)
	runTest("TestRWMutex", TestRWMutex)
	runTest("TestWaitGroup", TestWaitGroup)
	runTest("TestWaitGroupAddMany", TestWaitGroupAddMany)
	runTest("TestOnce", TestOnce)
	runTest("TestOnceNoGo", TestOnceNoGo)
	runTest("TestConcurrent", TestConcurrent)
}
