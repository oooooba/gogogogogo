package main

type box struct {
	x int
}

func garbage() *box {
	boxes := make([]*box, 0, 100)
	for i := 0; i < 100; i++ {
		boxes = append(boxes, &box{x: i})
	}
	return boxes[50]
}

func make_with_tail_cap() {
	s := make([]*box, 64, 1024)
	for i := range s {
		s[i] = &box{x: i}
	}
	var keep *box
	for i := 0; i < 300; i++ {
		keep = garbage()
	}
	if keep.x != 50 {
		panic("gc_slice: keep lost")
	}
	for i := 0; i < len(s); i++ {
		if s[i].x != i {
			panic("gc_slice: prefix entry lost")
		}
	}
}

func extend_sub() {
	s := make([]*box, 64, 1024)
	for i := range s {
		s[i] = &box{x: i}
	}
	u := s[:1024]
	for i := 64; i < 1024; i++ {
		u[i] = &box{x: 2000 + i}
	}
	for i := 0; i < 100; i++ {
		garbage()
	}
	if u[1023].x != 3023 {
		panic("gc_slice: extended sub tail lost")
	}
}

func struct_elem_slice() {
	type pair struct {
		b *box
	}
	ps := make([]pair, 32, 32)
	for i := range ps {
		ps[i].b = &box{x: 3000 + i}
	}
	for i := 0; i < 100; i++ {
		garbage()
	}
	for i := range ps {
		if ps[i].b.x != 3000+i {
			panic("gc_slice: struct element lost")
		}
	}
}

func in_place_append() {
	t := make([]*box, 0, 1024)
	for i := 0; i < 64; i++ {
		t = append(t, &box{x: 1000 + i})
	}
	for i := 0; i < 100; i++ {
		garbage()
	}
	if t[63].x != 1063 {
		panic("gc_slice: appended entry lost")
	}
}

func grow_append() {
	var t []*box
	for i := 0; i < 3000; i++ {
		t = append(t, &box{x: i})
	}
	for i := 0; i < 100; i++ {
		garbage()
	}
	for i := range t {
		if t[i].x != i {
			panic("gc_slice: grown append entry lost")
		}
	}
}

func main() {
	make_with_tail_cap()
	extend_sub()
	struct_elem_slice()
	in_place_append()
	grow_append()
}
