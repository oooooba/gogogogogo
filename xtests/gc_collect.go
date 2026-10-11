package main

import "runtime"

type node struct {
	id   int
	next *node
}

var globalSlice = []string{"alpha", "beta", "gamma"}

var globalMap = map[string]int{"one": 1, "two": 2}

var globalInterface interface{} = &node{id: -1}

type boxed struct {
	s     string
	sl    []int
	m     map[int]string
	iface interface{}
	fn    func(int) int
	next  *boxed
}

func collect() {
	runtime.GC()
	runtime.GC()
}

func retainChain() {
	var head *node
	for i := 0; i < 1000; i++ {
		head = &node{id: i, next: head}
	}
	collect()
	count := 0
	for n := head; n != nil; n = n.next {
		count++
	}
	if count != 1000 {
		panic("chain did not survive collection")
	}
}

func retainBoxed() {
	b := &boxed{
		s:     "hello world",
		sl:    []int{1, 2, 3, 4, 5},
		m:     map[int]string{1: "a", 2: "b"},
		iface: &node{id: 42},
		fn:    func(x int) int { return x * 2 },
	}
	b.next = &boxed{s: "tail"}
	collect()
	if b.s != "hello world" || len(b.sl) != 5 || b.sl[4] != 5 {
		panic("boxed string/slice corrupted")
	}
	if b.m[2] != "b" {
		panic("boxed map corrupted")
	}
	if b.iface.(*node).id != 42 {
		panic("boxed interface corrupted")
	}
	if b.fn(21) != 42 {
		panic("boxed closure corrupted")
	}
	if b.next.s != "tail" {
		panic("boxed chain corrupted")
	}
}

func garbage() {
	for i := 0; i < 100000; i++ {
		_ = &node{id: i, next: &node{id: i + 1}}
	}
}

func main() {
	retainChain()
	retainBoxed()
	garbage()
	collect()
	if globalSlice[1] != "beta" {
		panic("global slice corrupted")
	}
	if globalMap["two"] != 2 {
		panic("global map corrupted")
	}
	if globalInterface.(*node).id != -1 {
		panic("global interface corrupted")
	}
	println("gc collect ok")
}
