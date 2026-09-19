package main

func main() {
	var x *uint64
	for i := 0; i < 256*1000; i++ {
		x = new(uint64)
	}
	_ = x
}
