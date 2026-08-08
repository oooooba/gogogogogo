package main

import "iter"

func intSeq(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := 0; i < n; i++ {
			if !yield(i) {
				return
			}
		}
	}
}

func intSeq2(n int) iter.Seq2[int, string] {
	return func(yield func(int, string) bool) {
		for i := 0; i < n; i++ {
			if !yield(i, "v") {
				return
			}
		}
	}
}

func TestSeq() int {
	sum := 0
	count := 0
	for v := range intSeq(5) {
		sum += v
		count++
	}
	if count != 5 || sum != 10 {
		return 1
	}
	return 0
}

func TestSeqEarlyStop() int {
	count := 0
	for v := range intSeq(100) {
		count++
		if v == 2 {
			break
		}
	}
	if count != 3 {
		return 1
	}
	return 0
}

func TestSeqEmpty() int {
	count := 0
	for range intSeq(0) {
		count++
	}
	if count != 0 {
		return 1
	}
	return 0
}

func TestSeq2() int {
	sum := 0
	count := 0
	for i, v := range intSeq2(3) {
		sum += i + len(v)
		count++
	}
	if count != 3 || sum != 6 {
		return 1
	}
	return 0
}

func TestSeq2EarlyStop() int {
	count := 0
	for i := range intSeq2(100) {
		count++
		if i == 1 {
			break
		}
	}
	if count != 2 {
		return 1
	}
	return 0
}

func TestPull() int {
	next, stop := iter.Pull(intSeq(3))
	defer stop()
	sum := 0
	count := 0
	for {
		v, ok := next()
		if !ok {
			break
		}
		sum += v
		count++
	}
	if count != 3 || sum != 3 {
		return 1
	}
	return 0
}

func TestPullEmpty() int {
	next, stop := iter.Pull(intSeq(0))
	defer stop()
	_, ok := next()
	if ok {
		return 1
	}
	return 0
}

func TestPullEarlyStop() int {
	next, stop := iter.Pull(intSeq(100))
	sum := 0
	for i := 0; i < 3; i++ {
		v, ok := next()
		if !ok {
			return 1
		}
		sum += v
	}
	stop()
	if sum != 3 {
		return 1
	}
	return 0
}

func TestPull2() int {
	next, stop := iter.Pull2(intSeq2(4))
	defer stop()
	sum := 0
	count := 0
	for {
		i, v, ok := next()
		if !ok {
			break
		}
		sum += i + len(v)
		count++
	}
	if count != 4 || sum != 10 {
		return 1
	}
	return 0
}

func TestPull2EarlyStop() int {
	next, stop := iter.Pull2(intSeq2(100))
	for i := 0; i < 2; i++ {
		_, _, ok := next()
		if !ok {
			return 1
		}
	}
	stop()
	return 0
}

func TestPullNested() int {
	nested := func(yield func(int) bool) {
		innerNext, innerStop := iter.Pull(intSeq(4))
		defer innerStop()
		for {
			v, ok := innerNext()
			if !ok {
				return
			}
			if !yield(v * 2) {
				return
			}
		}
	}
	next, stop := iter.Pull(nested)
	defer stop()
	sum := 0
	for {
		v, ok := next()
		if !ok {
			break
		}
		sum += v
	}
	if sum != 12 {
		return 1
	}
	return 0
}

func TestPullInterleave() int {
	next1, stop1 := iter.Pull(intSeq(10))
	defer stop1()
	next2, stop2 := iter.Pull(intSeq(10))
	defer stop2()
	sum := 0
	for i := 0; i < 10; i++ {
		v1, ok1 := next1()
		v2, ok2 := next2()
		if !ok1 || !ok2 {
			return 1
		}
		sum += v1 + v2
	}
	if sum != 90 {
		return 1
	}
	return 0
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("TestSeq", TestSeq)
	runTest("TestSeqEarlyStop", TestSeqEarlyStop)
	runTest("TestSeqEmpty", TestSeqEmpty)
	runTest("TestSeq2", TestSeq2)
	runTest("TestSeq2EarlyStop", TestSeq2EarlyStop)
	runTest("TestPull", TestPull)
	runTest("TestPullEmpty", TestPullEmpty)
	runTest("TestPullEarlyStop", TestPullEarlyStop)
	runTest("TestPull2", TestPull2)
	runTest("TestPull2EarlyStop", TestPull2EarlyStop)
	runTest("TestPullNested", TestPullNested)
	runTest("TestPullInterleave", TestPullInterleave)
}
