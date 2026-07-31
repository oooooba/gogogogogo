package main

import (
	"structs"
	"unsafe"
)

type HostLayoutS struct {
	_ structs.HostLayout
	b byte
	q uint64
	w uint32
}

type HostLayoutMiddle struct {
	x uint64
	_ structs.HostLayout
	y uint64
}

type HostLayoutTrailing struct {
	x uint64
	_ structs.HostLayout
}

type HostLayoutNamed struct {
	_ HostLayoutAlias
	n uint32
}

type HostLayoutAlias = structs.HostLayout

type HostLayoutNewName struct {
	_ HostLayoutNew
	n uint32
}

type HostLayoutNew structs.HostLayout

type HostLayoutNested struct {
	_     structs.HostLayout
	inner MarkedInner
}

type MarkedInner struct {
	a byte
	b uint64
}

func TestHostLayoutSize() int {
	if unsafe.Sizeof(structs.HostLayout{}) != 0 {
		return 1
	}
	if unsafe.Sizeof(HostLayoutAlias{}) != 0 {
		return 2
	}
	if unsafe.Sizeof(HostLayoutNew{}) != 0 {
		return 3
	}
	if unsafe.Alignof(structs.HostLayout{}) != 1 {
		return 4
	}
	return 0
}

func TestHostLayoutBasicStruct() int {
	s := HostLayoutS{b: 0x12, q: 0x1122334455667788, w: 0xaabbccdd}
	if unsafe.Sizeof(HostLayoutS{}) != 24 {
		return 1
	}
	if unsafe.Alignof(HostLayoutS{}) != 8 {
		return 2
	}
	if unsafe.Offsetof(HostLayoutS{}.b) != 0 {
		return 3
	}
	if unsafe.Offsetof(HostLayoutS{}.q) != 8 {
		return 4
	}
	if unsafe.Offsetof(HostLayoutS{}.w) != 16 {
		return 5
	}
	if s.b != 0x12 || s.q != 0x1122334455667788 || s.w != 0xaabbccdd {
		return 6
	}
	s.q = 0xfeedbeefdeadbeef
	s.w = 0xffffffff
	if s.q != 0xfeedbeefdeadbeef || s.w != 0xffffffff {
		return 7
	}
	return 0
}

func TestHostLayoutMiddle() int {
	m := HostLayoutMiddle{x: 1, y: 2}
	if unsafe.Sizeof(HostLayoutMiddle{}) != 16 {
		return 1
	}
	if unsafe.Offsetof(HostLayoutMiddle{}.x) != 0 {
		return 2
	}
	if unsafe.Offsetof(HostLayoutMiddle{}.y) != 8 {
		return 3
	}
	if m.x != 1 || m.y != 2 {
		return 4
	}
	return 0
}

func TestHostLayoutTrailing() int {
	t := HostLayoutTrailing{x: 0xdeadbeef}
	if unsafe.Sizeof(HostLayoutTrailing{}) != 16 {
		return 1
	}
	if unsafe.Offsetof(HostLayoutTrailing{}.x) != 0 {
		return 2
	}
	if t.x != 0xdeadbeef {
		return 3
	}
	return 0
}

func TestHostLayoutNamed() int {
	s := HostLayoutNamed{n: 0x12345678}
	if unsafe.Sizeof(HostLayoutNamed{}) != 4 {
		return 1
	}
	if unsafe.Offsetof(HostLayoutNamed{}.n) != 0 {
		return 2
	}
	if s.n != 0x12345678 {
		return 3
	}
	return 0
}

func TestHostLayoutNewName() int {
	s := HostLayoutNewName{n: 0x87654321}
	if unsafe.Sizeof(HostLayoutNewName{}) != 4 {
		return 1
	}
	if unsafe.Offsetof(HostLayoutNewName{}.n) != 0 {
		return 2
	}
	if s.n != 0x87654321 {
		return 3
	}
	return 0
}

func TestHostLayoutNested() int {
	s := HostLayoutNested{inner: MarkedInner{a: 0xab, b: 0x0102030405060708}}
	if unsafe.Sizeof(HostLayoutNested{}) != 16 {
		return 1
	}
	if unsafe.Offsetof(HostLayoutNested{}.inner) != 0 {
		return 2
	}
	if unsafe.Offsetof(MarkedInner{}.a) != 0 {
		return 3
	}
	if unsafe.Offsetof(MarkedInner{}.b) != 8 {
		return 4
	}
	if s.inner.a != 0xab || s.inner.b != 0x0102030405060708 {
		return 5
	}
	return 0
}

func TestHostLayoutComparable() int {
	var h1 structs.HostLayout
	h2 := structs.HostLayout{}
	if h1 != h2 {
		return 1
	}
	var h3 HostLayoutNew
	h4 := HostLayoutNew{}
	if h3 != h4 {
		return 2
	}
	return 0
}

func TestHostLayoutSlice() int {
	slice := make([]HostLayoutS, 3)
	slice[0] = HostLayoutS{b: 1, q: 2, w: 3}
	slice[1] = HostLayoutS{b: 4, q: 5, w: 6}
	slice[2] = HostLayoutS{b: 7, q: 8, w: 9}
	if unsafe.Sizeof(HostLayoutS{}) != 24 {
		return 1
	}
	if len(slice) != 3 {
		return 2
	}
	if slice[0].b != 1 || slice[1].q != 5 || slice[2].w != 9 {
		return 3
	}
	total := uint64(0)
	for i := range slice {
		total += slice[i].q
	}
	if total != 15 {
		return 4
	}
	return 0
}

func TestHostLayoutArray() int {
	arr := [2]HostLayoutS{{b: 1, q: 2, w: 3}, {b: 4, q: 5, w: 6}}
	if unsafe.Sizeof(arr) != 48 {
		return 1
	}
	if arr[1].q != 5 {
		return 2
	}
	return 0
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("TestHostLayoutSize", TestHostLayoutSize)
	runTest("TestHostLayoutBasicStruct", TestHostLayoutBasicStruct)
	runTest("TestHostLayoutMiddle", TestHostLayoutMiddle)
	runTest("TestHostLayoutTrailing", TestHostLayoutTrailing)
	runTest("TestHostLayoutNamed", TestHostLayoutNamed)
	runTest("TestHostLayoutNewName", TestHostLayoutNewName)
	runTest("TestHostLayoutNested", TestHostLayoutNested)
	runTest("TestHostLayoutComparable", TestHostLayoutComparable)
	runTest("TestHostLayoutSlice", TestHostLayoutSlice)
	runTest("TestHostLayoutArray", TestHostLayoutArray)
}
