package main

import "container/ring"

func TestNewZero() int {
	if ring.New(0) != nil {
		return 1
	}
	if ring.New(-1) != nil {
		return 2
	}
	return 0
}

func TestNew() int {
	r := ring.New(3)
	if r == nil {
		return 1
	}
	if r.Len() != 3 {
		return 2
	}
	return 0
}

func TestLen() int {
	if ring.New(1).Len() != 1 {
		return 1
	}
	if ring.New(5).Len() != 5 {
		return 2
	}
	if ring.New(10).Len() != 10 {
		return 3
	}
	return 0
}

func TestValue() int {
	r := ring.New(3)
	r.Value = "first"
	r.Next().Value = "second"
	r.Next().Next().Value = "third"
	if r.Value.(string) != "first" {
		return 1
	}
	if r.Next().Value.(string) != "second" {
		return 2
	}
	if r.Next().Next().Value.(string) != "third" {
		return 3
	}
	return 0
}

func TestNext() int {
	r := ring.New(3)
	r.Value = 1
	r.Next().Value = 2
	r.Next().Next().Value = 3
	n := 1
	for p := r; n <= 3; p = p.Next() {
		if p.Value.(int) != n {
			return n
		}
		n++
	}
	if n != 4 {
		return 4
	}
	return 0
}

func TestPrev() int {
	r := ring.New(3)
	r.Value = 1
	r.Next().Value = 2
	r.Next().Next().Value = 3
	n := 3
	for p := r.Next().Next(); n >= 1; p = p.Prev() {
		if p.Value.(int) != n {
			return n
		}
		n--
	}
	if n != 0 {
		return 4
	}
	return 0
}

func TestMove() int {
	r := ring.New(3)
	r.Value = 1
	r.Next().Value = 2
	r.Next().Next().Value = 3
	if r.Move(0).Value.(int) != 1 {
		return 1
	}
	if r.Move(1).Value.(int) != 2 {
		return 2
	}
	if r.Move(2).Value.(int) != 3 {
		return 3
	}
	if r.Move(3).Value.(int) != 1 {
		return 4
	}
	if r.Move(4).Value.(int) != 2 {
		return 5
	}
	if r.Move(-1).Value.(int) != 3 {
		return 6
	}
	if r.Move(-3).Value.(int) != 1 {
		return 7
	}
	if r.Move(-4).Value.(int) != 3 {
		return 8
	}
	return 0
}

func TestLink() int {
	r := ring.New(3)
	r.Value = 1
	r.Next().Value = 2
	r.Next().Next().Value = 3

	s := ring.New(2)
	s.Value = 10
	s.Next().Value = 20

	ret := r.Link(s)
	if ret.Value.(int) != 2 {
		return 1
	}
	if r.Len() != 5 {
		return 2
	}
	if r.Next().Value.(int) != 10 {
		return 3
	}
	if r.Next().Next().Value.(int) != 20 {
		return 4
	}
	if r.Next().Next().Next().Value.(int) != 2 {
		return 5
	}
	if r.Next().Next().Next().Next().Value.(int) != 3 {
		return 6
	}
	return 0
}

func TestLinkSameRing() int {
	r := ring.New(5)
	r.Value = 0
	r.Next().Value = 1
	r.Next().Next().Value = 2
	r.Next().Next().Next().Value = 3
	r.Next().Next().Next().Next().Value = 4

	e3 := r.Move(3)
	sub := r.Link(e3)
	if sub.Len() != 2 {
		return 1
	}
	if sub.Value.(int) != 1 {
		return 2
	}
	if sub.Next().Value.(int) != 2 {
		return 3
	}
	if r.Len() != 3 {
		return 4
	}
	if r.Next().Value.(int) != 3 {
		return 5
	}
	if r.Next().Next().Value.(int) != 4 {
		return 6
	}
	return 0
}

func TestUnlink() int {
	r := ring.New(6)
	r.Value = 0
	r.Next().Value = 1
	r.Next().Next().Value = 2
	r.Next().Next().Next().Value = 3
	r.Next().Next().Next().Next().Value = 4
	r.Next().Next().Next().Next().Next().Value = 5

	sub := r.Unlink(2)
	if sub.Len() != 2 {
		return 1
	}
	if sub.Value.(int) != 1 {
		return 2
	}
	if sub.Next().Value.(int) != 2 {
		return 3
	}
	if r.Len() != 4 {
		return 4
	}
	if r.Next().Value.(int) != 3 {
		return 5
	}
	if r.Next().Next().Value.(int) != 4 {
		return 6
	}
	if r.Next().Next().Next().Value.(int) != 5 {
		return 7
	}
	return 0
}

func TestUnlinkZero() int {
	r := ring.New(3)
	if r.Unlink(0) != nil {
		return 1
	}
	if r.Len() != 3 {
		return 2
	}
	if r.Unlink(-1) != nil {
		return 3
	}
	if r.Len() != 3 {
		return 4
	}
	return 0
}

func TestDo() int {
	r := ring.New(3)
	r.Value = 1
	r.Next().Value = 2
	r.Next().Next().Value = 3

	sum := 0
	r.Do(func(v any) {
		sum += v.(int)
	})
	if sum != 6 {
		return 1
	}
	return 0
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("TestNewZero", TestNewZero)
	runTest("TestNew", TestNew)
	runTest("TestLen", TestLen)
	runTest("TestValue", TestValue)
	runTest("TestNext", TestNext)
	runTest("TestPrev", TestPrev)
	runTest("TestMove", TestMove)
	runTest("TestLink", TestLink)
	runTest("TestLinkSameRing", TestLinkSameRing)
	runTest("TestUnlink", TestUnlink)
	runTest("TestUnlinkZero", TestUnlinkZero)
	runTest("TestDo", TestDo)
}
