// An interface method call and a deferred call copy their arguments onto the
// heap, so a program that makes many of them while the collector has to reclaim
// garbage exercises the path where those copies have to run the collector
// instead of failing the allocation.
package main

type Source interface {
	Int63() int64
	Add(a int64, b int64) int64
	Seed(seed int64)
}

type rng struct {
	state int64
}

func (r *rng) Int63() int64 {
	r.state = r.state*6364136223846793005 + 1442695040888963407
	return r.state >> 1
}

func (r *rng) Add(a int64, b int64) int64 {
	return a + b
}

func (r *rng) Seed(seed int64) {
	r.state = seed
}

type Node struct {
	value int64
	next  *Node
}

func build(n int) *Node {
	var head *Node
	for i := 0; i < n; i++ {
		head = &Node{value: int64(i), next: head}
	}
	return head
}

func total(head *Node) int64 {
	var sum int64
	for node := head; node != nil; node = node.next {
		sum += node.value
	}
	return sum
}

func churn(src Source, iterations int) int64 {
	var acc int64
	for i := 0; i < iterations; i++ {
		acc += total(build(100))
		func() {
			// A deferred call with an argument, registered on every iteration.
			defer func(offset int64) { acc += offset % 3 }(int64(i))
			acc += src.Int63() % 5
		}()
		acc += src.Add(int64(i), 1)
		if i%1000 == 0 {
			src.Seed(int64(i))
		}
	}
	return acc
}

func main() {
	// The same run twice, so the answer has to come out the same no matter what
	// the collector left behind in the heap.
	first := churn(&rng{state: 1}, 20000)
	second := churn(&rng{state: 1}, 20000)
	println("churn:", first, second)
	if first != 299029854 {
		panic("gc: unexpected churn result")
	}
	if first != second {
		panic("gc: interface and deferred argument copies are not stable")
	}
}
