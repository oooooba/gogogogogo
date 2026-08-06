package main

import "unsafe"

type Header struct {
	a uint8
	b uint64
	c uint16
	d uint32
}

func TestSizeof() int {
	if unsafe.Sizeof(uint8(0)) != 1 {
		return 1
	}
	if unsafe.Sizeof(uint16(0)) != 2 {
		return 2
	}
	if unsafe.Sizeof(uint32(0)) != 4 {
		return 3
	}
	if unsafe.Sizeof(uint64(0)) != 8 {
		return 4
	}
	if unsafe.Sizeof(int(0)) != 8 {
		return 5
	}
	if unsafe.Sizeof("") != 16 {
		return 6
	}
	if unsafe.Sizeof([]byte(nil)) != 24 {
		return 7
	}
	if unsafe.Sizeof(Header{}) != 24 {
		return 8
	}
	if unsafe.Sizeof([10]uint32{}) != 40 {
		return 9
	}
	return 0
}

func TestAlignof() int {
	if unsafe.Alignof(uint8(0)) != 1 {
		return 1
	}
	if unsafe.Alignof(uint16(0)) != 2 {
		return 2
	}
	if unsafe.Alignof(uint64(0)) != 8 {
		return 3
	}
	if unsafe.Alignof(Header{}) != 8 {
		return 4
	}
	if unsafe.Alignof([]int(nil)) != 8 {
		return 5
	}
	return 0
}

func TestOffsetof() int {
	if unsafe.Offsetof(Header{}.a) != 0 {
		return 1
	}
	if unsafe.Offsetof(Header{}.b) != 8 {
		return 2
	}
	if unsafe.Offsetof(Header{}.c) != 16 {
		return 3
	}
	if unsafe.Offsetof(Header{}.d) != 20 {
		return 4
	}
	return 0
}

func TestAdd() int {
	arr := [5]int32{10, 20, 30, 40, 50}
	base := unsafe.Pointer(&arr[2])
	if unsafe.Add(base, 0) != base {
		return 1
	}
	p := unsafe.Add(base, 8)
	if *(*int32)(p) != 50 {
		return 2
	}
	p = unsafe.Add(p, -8)
	if *(*int32)(p) != 30 {
		return 3
	}
	p = unsafe.Add(base, -8)
	if *(*int32)(p) != 10 {
		return 4
	}
	offset := 8
	p = unsafe.Add(base, offset)
	if *(*int32)(p) != 50 {
		return 5
	}
	p = unsafe.Add(base, -offset)
	if *(*int32)(p) != 10 {
		return 6
	}
	return 0
}

func TestSlice() int {
	arr := [6]int16{1, 2, 3, 4, 5, 6}
	sl := unsafe.Slice(&arr[0], 4)
	if len(sl) != 4 {
		return 1
	}
	if cap(sl) != 4 {
		return 2
	}
	if sl[0] != 1 || sl[1] != 2 || sl[2] != 3 || sl[3] != 4 {
		return 3
	}
	sl2 := unsafe.Slice(&arr[2], 2)
	if sl2[0] != 3 || sl2[1] != 4 {
		return 4
	}
	bs := unsafe.Slice(&arr[0], 6)
	if len(bs) != 6 || cap(bs) != 6 {
		return 5
	}
	sl3 := unsafe.Slice(&arr[0], 0)
	if len(sl3) != 0 {
		return 6
	}
	var p *int
	sl4 := unsafe.Slice(p, 0)
	if len(sl4) != 0 {
		return 7
	}
	return 0
}

func TestSliceData() int {
	arr := [4]int64{11, 22, 33, 44}
	sl := arr[:]
	ptr := unsafe.SliceData(sl)
	if *ptr != 11 {
		return 1
	}
	*ptr = 111
	if arr[0] != 111 {
		return 2
	}
	sub := sl[1:3]
	ptr = unsafe.SliceData(sub)
	if *ptr != 22 {
		return 3
	}
	var empty []int
	if unsafe.SliceData(empty) != nil {
		return 4
	}
	return 0
}

func TestString() int {
	bytes := []byte("hello")
	str := unsafe.String(&bytes[0], 5)
	if str != "hello" {
		return 1
	}
	if len(str) != 5 {
		return 2
	}
	str2 := unsafe.String(&bytes[0], 3)
	if str2 != "hel" {
		return 3
	}
	empty := unsafe.String(&bytes[0], 0)
	if len(empty) != 0 {
		return 4
	}
	return 0
}

func TestStringData() int {
	str := "hello"
	ptr := unsafe.StringData(str)
	if *ptr != 'h' {
		return 1
	}
	bytes := []byte("world")
	rt := unsafe.String(unsafe.StringData(str), len(str))
	if rt != str {
		return 2
	}
	_ = bytes
	empty := ""
	if len(unsafe.String(unsafe.StringData(empty), 0)) != 0 {
		return 3
	}
	return 0
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("TestSizeof", TestSizeof)
	runTest("TestAlignof", TestAlignof)
	runTest("TestOffsetof", TestOffsetof)
	runTest("TestAdd", TestAdd)
	runTest("TestSlice", TestSlice)
	runTest("TestSliceData", TestSliceData)
	runTest("TestString", TestString)
	runTest("TestStringData", TestStringData)
}
