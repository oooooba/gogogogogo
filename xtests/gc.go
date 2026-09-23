package main

func simple_allocation() {
	var x *uint64
	for i := 0; i < 256*1000; i++ {
		x = new(uint64)
	}
	_ = x
}

type node struct {
	id   int
	next *node
}

func gc_struct_chain() int {
	var head *node
	for i := 0; i < 3000; i++ {
		head = &node{id: i, next: head}
	}
	for i := 0; i < 256*1000; i++ {
		_ = new(uint64)
	}
	count := 0
	for n := head; n != nil; n = n.next {
		count++
	}
	return count
}
func main() {
	simple_allocation()
	if gc_struct_chain() != 3000 {
		panic("gc: broken struct chain")
	}
}
