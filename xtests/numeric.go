package main

func Test1() int {
	f := func() int {
		return 42
	}
	if f() != f() {
		return 0
	}
	return 1
}

func Test2() int {
	f := func() uint8 {
		return 42
	}
	if f() != f() {
		return 0
	}
	return 2
}

func Test3() int {
	f := func() uint16 {
		return 42
	}
	if f() != f() {
		return 0
	}
	return 3
}

func Test4() int {
	f := func() uint32 {
		return 42
	}
	if f() != f() {
		return 0
	}
	return 4
}

func Test5() int {
	f := func() uint64 {
		return 42
	}
	if f() != f() {
		return 0
	}
	return 5
}

func Test6() int {
	f := func() int8 {
		return 42
	}
	if f() != f() {
		return 0
	}
	return 6
}

func Test7() int {
	f := func() int16 {
		return 42
	}
	if f() != f() {
		return 0
	}
	return 7
}

func Test8() int {
	f := func() int32 {
		return 42
	}
	if f() != f() {
		return 0
	}
	return 8
}

func Test9() int {
	f := func() int64 {
		return 42
	}
	if f() != f() {
		return 0
	}
	return 9
}

func Test10() int {
	f := func() float32 {
		return 42
	}
	if f() != f() {
		return 0
	}
	return 10
}

func Test11() int {
	f := func() float64 {
		return 42
	}
	if f() != f() {
		return 0
	}
	return 11
}

func Test12() int {
	f := func() uint {
		return 42
	}
	if f() != f() {
		return 0
	}
	return 12
}

func Test13() int {
	var x uint = 1
	var y uint = 64
	if x<<y != 0 {
		return 0
	}

	var x8 uint8 = 1
	var y8 uint = 8
	if x8<<y8 != 0 {
		return 1
	}

	var x16 uint16 = 1
	var y16 uint = 16
	if x16<<y16 != 0 {
		return 2
	}

	var x32 uint32 = 1
	var y32 uint = 32
	if x32<<y32 != 0 {
		return 3
	}

	var x64 uint64 = 1
	var y64 uint = 64
	if x64<<y64 != 0 {
		return 4
	}

	return 13
}

func Test14() int {
	var x int = 1
	var y uint = 64
	if x<<y != 0 {
		return 0
	}

	var x8 int8 = 1
	var y8 uint = 8
	if x8<<y8 != 0 {
		return 1
	}

	var x16 int16 = 1
	var y16 uint = 16
	if x16<<y16 != 0 {
		return 2
	}

	var x32 int32 = 1
	var y32 uint = 32
	if x32<<y32 != 0 {
		return 3
	}

	var x64 int64 = 1
	var y64 uint = 64
	if x64<<y64 != 0 {
		return 4
	}

	var xus uint = 1
	var ys int = 64
	if xus<<ys != 0 {
		return 5
	}

	var xs int = 1
	if xs<<ys != 0 {
		return 6
	}

	return 14
}

func Test15() int {
	var x uint = 1
	var y uint = 64
	if x>>y != 0 {
		return 0
	}

	var x8 uint8 = 1
	var y8 uint = 8
	if x8>>y8 != 0 {
		return 1
	}

	var x16 uint16 = 1
	var y16 uint = 16
	if x16>>y16 != 0 {
		return 2
	}

	var x32 uint32 = 1
	var y32 uint = 32
	if x32>>y32 != 0 {
		return 3
	}

	var x64 uint64 = 1
	var y64 uint = 64
	if x64>>y64 != 0 {
		return 4
	}

	return 15
}

func Test16() int {
	var x int = 1
	var y uint = 64
	if x>>y != 0 {
		return 0
	}

	var x8 int8 = 1
	var y8 uint = 8
	if x8>>y8 != 0 {
		return 1
	}

	var x16 int16 = 1
	var y16 uint = 16
	if x16>>y16 != 0 {
		return 2
	}

	var x32 int32 = 1
	var y32 uint = 32
	if x32>>y32 != 0 {
		return 3
	}

	var x64 int64 = 1
	var y64 uint = 64
	if x64>>y64 != 0 {
		return 4
	}

	var xus uint = 1
	var ys int = 64
	if xus>>ys != 0 {
		return 5
	}

	var xs int = 1
	if xs>>ys != 0 {
		return 6
	}

	return 16
}

func Test17() int {
	var x0 int = -1
	var y0 uint = 64
	if x0<<y0 != 0 {
		return 0
	}

	var x1 int = -1
	var y1 uint = 1
	if x1<<y1 != -2 {
		return 1
	}

	var x2 int = -1
	var y2 uint = 64
	if x2>>y2 != -1 {
		return 2
	}

	var x3 int = -1
	var y3 uint = 1
	if x3>>y3 != -1 {
		return 3
	}

	var x4 int = -2
	var y4 uint = 0
	if x4>>y4 != -2 {
		return 4
	}

	return 17
}

func Test18() int {
	f := func(x float64) float64 {
		return float64(float32(x))
	}
	var x float64 = 0xe0000000
	y := f(x)
	if x != y {
		return 0
	}
	return 18
}

func Test19() int {
	var x0 uint = 0x8000_0000_0000_0000
	var y0 uint = 0x4000_0000_0000_0000
	if x0 != y0+y0 {
		return 0
	}

	var x1 uint64 = 0x8000_0000_0000_0000
	var y1 uint64 = 0x4000_0000_0000_0000
	if x1 != y1+y1 {
		return 1
	}

	var x2 uint32 = 0x8000_0000
	var y2 uint32 = 0x4000_0000
	if x2 != y2+y2 {
		return 2
	}

	var x3 int = -9223372036854775808
	var y3 int = -4611686018427387904
	if x3 != y3+y3 {
		return 3
	}

	var x4 int64 = -9223372036854775808
	var y4 int64 = -4611686018427387904
	if x4 != y4+y4 {
		return 4
	}

	var x5 int32 = -2147483648
	var y5 int32 = -1073741824
	if x5 != y5+y5 {
		return 5
	}

	return 19
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("Test1", Test1)
	runTest("Test2", Test2)
	runTest("Test3", Test3)
	runTest("Test4", Test4)
	runTest("Test5", Test5)
	runTest("Test6", Test6)
	runTest("Test7", Test7)
	runTest("Test8", Test8)
	runTest("Test9", Test9)
	runTest("Test10", Test10)
	runTest("Test11", Test11)
	runTest("Test12", Test12)
	runTest("Test13", Test13)
	runTest("Test14", Test14)
	runTest("Test15", Test15)
	runTest("Test16", Test16)
	runTest("Test17", Test17)
	runTest("Test18", Test18)
	runTest("Test19", Test19)
}
