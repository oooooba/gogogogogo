package main

import "container/heap"

type IntHeap []int

func (h IntHeap) Len() int           { return len(h) }
func (h IntHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h IntHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *IntHeap) Push(x any) {
	*h = append(*h, x.(int))
}

func (h *IntHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func TestInit() int {
	h := &IntHeap{3, 1, 4, 1, 5, 9, 2, 6}
	heap.Init(h)
	if h.Len() != 8 {
		return 1
	}
	if (*h)[0] != 1 {
		return 2
	}
	expected := []int{1, 1, 2, 3, 4, 5, 6, 9}
	for i := range expected {
		x := heap.Pop(h).(int)
		if x != expected[i] {
			return 3
		}
	}
	if h.Len() != 0 {
		return 4
	}
	return 0
}

func TestInitEmpty() int {
	h := &IntHeap{}
	heap.Init(h)
	if h.Len() != 0 {
		return 1
	}
	return 0
}

func TestInitSorted() int {
	h := &IntHeap{1, 2, 3, 4}
	heap.Init(h)
	if (*h)[0] != 1 {
		return 1
	}
	expected := []int{1, 2, 3, 4}
	for i := range expected {
		x := heap.Pop(h).(int)
		if x != expected[i] {
			return 2
		}
	}
	return 0
}

func TestPush() int {
	h := &IntHeap{}
	heap.Push(h, 3)
	heap.Push(h, 1)
	heap.Push(h, 4)
	heap.Push(h, 1)
	heap.Push(h, 5)
	if h.Len() != 5 {
		return 1
	}
	if (*h)[0] != 1 {
		return 2
	}
	expected := []int{1, 1, 3, 4, 5}
	for i := range expected {
		x := heap.Pop(h).(int)
		if x != expected[i] {
			return 3
		}
	}
	return 0
}

func TestPushDuplicates() int {
	h := &IntHeap{}
	heap.Push(h, 5)
	heap.Push(h, 5)
	heap.Push(h, 2)
	heap.Push(h, 2)
	if (*h)[0] != 2 {
		return 1
	}
	expected := []int{2, 2, 5, 5}
	for i := range expected {
		x := heap.Pop(h).(int)
		if x != expected[i] {
			return 2
		}
	}
	return 0
}

func TestPop() int {
	h := &IntHeap{1, 2, 3}
	heap.Init(h)
	if heap.Pop(h).(int) != 1 {
		return 1
	}
	if heap.Pop(h).(int) != 2 {
		return 2
	}
	if heap.Pop(h).(int) != 3 {
		return 3
	}
	if h.Len() != 0 {
		return 4
	}
	return 0
}

func TestRemoveRoot() int {
	h := &IntHeap{1, 2, 3, 4, 5, 6, 7}
	heap.Init(h)
	if heap.Remove(h, 0).(int) != 1 {
		return 1
	}
	if h.Len() != 6 {
		return 2
	}
	expected := []int{2, 3, 4, 5, 6, 7}
	for i := range expected {
		x := heap.Pop(h).(int)
		if x != expected[i] {
			return 3
		}
	}
	return 0
}

func TestRemoveMiddle() int {
	h := &IntHeap{1, 2, 3, 4, 5, 6, 7}
	heap.Init(h)
	if heap.Remove(h, 2).(int) != 3 {
		return 1
	}
	if h.Len() != 6 {
		return 2
	}
	expected := []int{1, 2, 4, 5, 6, 7}
	for i := range expected {
		x := heap.Pop(h).(int)
		if x != expected[i] {
			return 3
		}
	}
	return 0
}

func TestRemoveLast() int {
	h := &IntHeap{1, 2, 3}
	heap.Init(h)
	if heap.Remove(h, 2).(int) != 3 {
		return 1
	}
	if h.Len() != 2 {
		return 2
	}
	expected := []int{1, 2}
	for i := range expected {
		x := heap.Pop(h).(int)
		if x != expected[i] {
			return 3
		}
	}
	return 0
}

func TestFixDown() int {
	h := &IntHeap{1, 2, 3, 4, 5, 6, 7}
	heap.Init(h)
	(*h)[0] = 10
	heap.Fix(h, 0)
	expected := []int{2, 3, 4, 5, 6, 7, 10}
	for i := range expected {
		x := heap.Pop(h).(int)
		if x != expected[i] {
			return 1
		}
	}
	return 0
}

func TestFixUp() int {
	h := &IntHeap{1, 2, 3, 4, 5, 6, 7}
	heap.Init(h)
	(*h)[6] = 0
	heap.Fix(h, 6)
	expected := []int{0, 1, 2, 3, 4, 5, 6}
	for i := range expected {
		x := heap.Pop(h).(int)
		if x != expected[i] {
			return 1
		}
	}
	return 0
}

func TestFixNoChange() int {
	h := &IntHeap{1, 2, 3, 4, 5, 6, 7}
	heap.Init(h)
	(*h)[1] = 4
	heap.Fix(h, 1)
	expected := []int{1, 3, 4, 4, 5, 6, 7}
	for i := range expected {
		x := heap.Pop(h).(int)
		if x != expected[i] {
			return 1
		}
	}
	return 0
}

type Item struct {
	value    string
	priority int
}

type PriorityQueue []*Item

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	return pq[i].priority < pq[j].priority
}

func (pq PriorityQueue) Swap(i, j int) { pq[i], pq[j] = pq[j], pq[i] }

func (pq *PriorityQueue) Push(x any) { *pq = append(*pq, x.(*Item)) }

func (pq *PriorityQueue) Pop() any {
	old := *pq
	n := len(old)
	x := old[n-1]
	*pq = old[0 : n-1]
	return x
}

func TestPriorityQueue() int {
	pq := &PriorityQueue{}
	heap.Push(pq, &Item{value: "a", priority: 3})
	heap.Push(pq, &Item{value: "b", priority: 1})
	heap.Push(pq, &Item{value: "c", priority: 2})
	if pq.Len() != 3 {
		return 1
	}
	expected := []string{"b", "c", "a"}
	for i := range expected {
		it := heap.Pop(pq).(*Item)
		if it.value != expected[i] {
			return 2
		}
	}
	return 0
}

func TestPriorityQueueTie() int {
	pq := &PriorityQueue{}
	heap.Push(pq, &Item{value: "a", priority: 1})
	heap.Push(pq, &Item{value: "b", priority: 1})
	heap.Push(pq, &Item{value: "c", priority: 0})
	first := heap.Pop(pq).(*Item)
	if first.value != "c" {
		return 1
	}
	first = heap.Pop(pq).(*Item)
	if first.value != "a" && first.value != "b" {
		return 2
	}
	if pq.Len() != 1 {
		return 3
	}
	return 0
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("TestInit", TestInit)
	runTest("TestInitEmpty", TestInitEmpty)
	runTest("TestInitSorted", TestInitSorted)
	runTest("TestPush", TestPush)
	runTest("TestPushDuplicates", TestPushDuplicates)
	runTest("TestPop", TestPop)
	runTest("TestRemoveRoot", TestRemoveRoot)
	runTest("TestRemoveMiddle", TestRemoveMiddle)
	runTest("TestRemoveLast", TestRemoveLast)
	runTest("TestFixDown", TestFixDown)
	runTest("TestFixUp", TestFixUp)
	runTest("TestFixNoChange", TestFixNoChange)
	runTest("TestPriorityQueue", TestPriorityQueue)
	runTest("TestPriorityQueueTie", TestPriorityQueueTie)
}
