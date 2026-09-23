package main

type Box struct {
	value int
}

func makeClosure() func() int {
	b := &Box{value: 42}
	return func() int {
		return b.value
	}
}

func main() {
	f := makeClosure()
	for i := 0; i < 40; i++ {
		_ = make([]byte, 64*1024)
	}
	if f() == 42 {
		println("gc_closure: ok")
	} else {
		println("gc_closure: failed")
	}
}
