package main

import "container/list"

func TestNew() int {
	l := list.New()
	if l.Len() != 0 {
		return 1
	}
	if l.Front() != nil {
		return 2
	}
	if l.Back() != nil {
		return 3
	}
	return 0
}

func TestLen() int {
	l := list.New()
	if l.Len() != 0 {
		return 1
	}
	l.PushBack(1)
	if l.Len() != 1 {
		return 2
	}
	l.PushFront(0)
	l.PushBack(2)
	if l.Len() != 3 {
		return 3
	}
	return 0
}

func TestPushBack() int {
	l := list.New()
	l.PushBack(1)
	l.PushBack(2)
	l.PushBack(3)
	if l.Len() != 3 {
		return 1
	}
	if l.Front().Value.(int) != 1 {
		return 2
	}
	if l.Back().Value.(int) != 3 {
		return 3
	}
	return 0
}

func TestPushFront() int {
	l := list.New()
	l.PushFront(1)
	l.PushFront(2)
	l.PushFront(3)
	if l.Len() != 3 {
		return 1
	}
	if l.Front().Value.(int) != 3 {
		return 2
	}
	if l.Back().Value.(int) != 1 {
		return 3
	}
	return 0
}

func TestIterateForward() int {
	l := list.New()
	for _, v := range []int{1, 2, 3, 4, 5} {
		l.PushBack(v)
	}
	n := 1
	for e := l.Front(); e != nil; e = e.Next() {
		if e.Value.(int) != n {
			return n
		}
		n++
	}
	if n != 6 {
		return 6
	}
	return 0
}

func TestIterateBackward() int {
	l := list.New()
	for _, v := range []int{1, 2, 3, 4, 5} {
		l.PushBack(v)
	}
	n := 5
	for e := l.Back(); e != nil; e = e.Prev() {
		if e.Value.(int) != n {
			return n
		}
		n--
	}
	if n != 0 {
		return 6
	}
	return 0
}

func TestInsertBefore() int {
	l := list.New()
	a := l.PushBack(1)
	b := l.PushBack(3)
	c := l.InsertBefore(2, b)
	if c != l.Front().Next() {
		return 1
	}
	if c != b.Prev() {
		return 2
	}
	if c.Value.(int) != 2 {
		return 3
	}
	if a.Next() != c {
		return 4
	}
	if l.Len() != 3 {
		return 5
	}
	return 0
}

func TestInsertAfter() int {
	l := list.New()
	a := l.PushBack(1)
	b := l.PushBack(3)
	c := l.InsertAfter(2, a)
	if c != b.Prev() {
		return 1
	}
	if c != a.Next() {
		return 2
	}
	if c.Value.(int) != 2 {
		return 3
	}
	if l.Len() != 3 {
		return 4
	}
	return 0
}

func TestRemove() int {
	l := list.New()
	a := l.PushBack(1)
	l.PushBack(2)
	c := l.PushBack(3)
	if l.Remove(c) != 3 {
		return 1
	}
	if l.Len() != 2 {
		return 2
	}
	if l.Back().Value.(int) != 2 {
		return 3
	}
	l.Remove(a)
	if l.Len() != 1 {
		return 4
	}
	if l.Front().Value.(int) != 2 {
		return 5
	}
	if l.Back().Value.(int) != 2 {
		return 6
	}
	l.Remove(l.Front())
	if l.Len() != 0 {
		return 7
	}
	if l.Front() != nil {
		return 8
	}
	if l.Back() != nil {
		return 9
	}
	return 0
}

func TestMoveToFront() int {
	l := list.New()
	l.PushBack(1)
	l.PushBack(2)
	l.PushBack(3)
	l.MoveToFront(l.Back())
	if l.Front().Value.(int) != 3 {
		return 1
	}
	l.MoveToFront(l.Front())
	if l.Front().Value.(int) != 3 {
		return 2
	}
	if l.Len() != 3 {
		return 3
	}
	return 0
}

func TestMoveToBack() int {
	l := list.New()
	l.PushBack(1)
	l.PushBack(2)
	l.PushBack(3)
	l.MoveToBack(l.Front())
	if l.Back().Value.(int) != 1 {
		return 1
	}
	l.MoveToBack(l.Back())
	if l.Back().Value.(int) != 1 {
		return 2
	}
	if l.Len() != 3 {
		return 3
	}
	return 0
}

func TestMoveBefore() int {
	l := list.New()
	l.PushBack(1)
	l.PushBack(2)
	l.PushBack(3)
	l.MoveBefore(l.Back(), l.Front())
	if l.Front().Value.(int) != 3 {
		return 1
	}
	if l.Front().Next().Value.(int) != 1 {
		return 2
	}
	if l.Len() != 3 {
		return 3
	}
	return 0
}

func TestMoveAfter() int {
	l := list.New()
	l.PushBack(1)
	l.PushBack(2)
	l.PushBack(3)
	l.MoveAfter(l.Front(), l.Back())
	if l.Back().Value.(int) != 1 {
		return 1
	}
	if l.Back().Prev().Value.(int) != 3 {
		return 2
	}
	if l.Len() != 3 {
		return 3
	}
	return 0
}

func TestPushFrontList() int {
	l := list.New()
	l.PushBack(3)
	l.PushBack(4)
	other := list.New()
	other.PushBack(1)
	other.PushBack(2)
	l.PushFrontList(other)
	if l.Len() != 4 {
		return 1
	}
	n := 1
	for e := l.Front(); e != nil; e = e.Next() {
		if e.Value.(int) != n {
			return n
		}
		n++
	}
	if n != 5 {
		return 5
	}
	if other.Len() != 2 {
		return 6
	}
	return 0
}

func TestPushBackList() int {
	l := list.New()
	l.PushBack(1)
	l.PushBack(2)
	other := list.New()
	other.PushBack(3)
	other.PushBack(4)
	l.PushBackList(other)
	if l.Len() != 4 {
		return 1
	}
	n := 1
	for e := l.Front(); e != nil; e = e.Next() {
		if e.Value.(int) != n {
			return n
		}
		n++
	}
	if n != 5 {
		return 5
	}
	if other.Len() != 2 {
		return 6
	}
	return 0
}

func TestInit() int {
	l := list.New()
	l.PushBack(1)
	l.PushBack(2)
	l.Init()
	if l.Len() != 0 {
		return 1
	}
	if l.Front() != nil {
		return 2
	}
	if l.Back() != nil {
		return 3
	}
	l.PushBack(3)
	if l.Front().Value.(int) != 3 {
		return 4
	}
	return 0
}

func TestElementValue() int {
	l := list.New()
	e := l.PushBack(42)
	if e.Value.(int) != 42 {
		return 1
	}
	e.Value = 43
	if e.Value.(int) != 43 {
		return 2
	}
	return 0
}

func TestRemoveRemovedElement() int {
	l := list.New()
	l.PushBack(1)
	e := l.Front()
	if l.Remove(e) != 1 {
		return 1
	}
	if l.Len() != 0 {
		return 2
	}
	if l.Remove(e) != 1 {
		return 3
	}
	if l.Len() != 0 {
		return 4
	}
	return 0
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("TestNew", TestNew)
	runTest("TestLen", TestLen)
	runTest("TestPushBack", TestPushBack)
	runTest("TestPushFront", TestPushFront)
	runTest("TestIterateForward", TestIterateForward)
	runTest("TestIterateBackward", TestIterateBackward)
	runTest("TestInsertBefore", TestInsertBefore)
	runTest("TestInsertAfter", TestInsertAfter)
	runTest("TestRemove", TestRemove)
	runTest("TestMoveToFront", TestMoveToFront)
	runTest("TestMoveToBack", TestMoveToBack)
	runTest("TestMoveBefore", TestMoveBefore)
	runTest("TestMoveAfter", TestMoveAfter)
	runTest("TestPushFrontList", TestPushFrontList)
	runTest("TestPushBackList", TestPushBackList)
	runTest("TestInit", TestInit)
	runTest("TestElementValue", TestElementValue)
	runTest("TestRemoveRemovedElement", TestRemoveRemovedElement)
}
