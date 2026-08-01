package main

import (
	"sync/atomic"
)

func TestLoadInt32() int {
	var v int32 = 12345
	if atomic.LoadInt32(&v) != 12345 {
		return 1
	}
	atomic.StoreInt32(&v, -5678)
	if atomic.LoadInt32(&v) != -5678 {
		return 2
	}
	return 0
}

func TestAddInt32() int {
	var v int32 = 100
	if atomic.AddInt32(&v, 50) != 150 {
		return 1
	}
	if atomic.AddInt32(&v, -30) != 120 {
		return 2
	}
	if atomic.LoadInt32(&v) != 120 {
		return 3
	}
	return 0
}

func TestCompareAndSwapInt32() int {
	var v int32 = 42
	if atomic.CompareAndSwapInt32(&v, 42, 43) != true {
		return 1
	}
	if atomic.CompareAndSwapInt32(&v, 42, 44) != false {
		return 2
	}
	if atomic.LoadInt32(&v) != 43 {
		return 3
	}
	return 0
}

func TestSwapInt32() int {
	var v int32 = 1
	if atomic.SwapInt32(&v, 2) != 1 {
		return 1
	}
	if atomic.SwapInt32(&v, 3) != 2 {
		return 2
	}
	if atomic.LoadInt32(&v) != 3 {
		return 3
	}
	return 0
}

func TestAndOrInt32() int {
	var v int32 = 0b11110000
	if atomic.AndInt32(&v, 0b11001100) != 0b11110000 {
		return 1
	}
	if atomic.LoadInt32(&v) != 0b11000000 {
		return 2
	}
	if atomic.OrInt32(&v, 0b00001111) != 0b11000000 {
		return 3
	}
	if atomic.LoadInt32(&v) != 0b11001111 {
		return 4
	}
	return 0
}

func TestLoadUint32() int {
	var v uint32 = 0xDEADBEEF
	if atomic.LoadUint32(&v) != 0xDEADBEEF {
		return 1
	}
	atomic.StoreUint32(&v, 0x12345678)
	if atomic.LoadUint32(&v) != 0x12345678 {
		return 2
	}
	return 0
}

func TestAddUint32() int {
	var v uint32 = 0
	if atomic.AddUint32(&v, 0xFFFFFFFF) != 0xFFFFFFFF {
		return 1
	}
	if atomic.AddUint32(&v, 1) != 0 {
		return 2
	}
	if atomic.AddUint32(&v, 0xFFFFFFFF) != 0xFFFFFFFF {
		return 3
	}
	return 0
}

func TestCompareAndSwapUint32() int {
	var v uint32 = 0xABCD
	if atomic.CompareAndSwapUint32(&v, 0xABCD, 0x1234) != true {
		return 1
	}
	if atomic.CompareAndSwapUint32(&v, 0xABCD, 0x5678) != false {
		return 2
	}
	if atomic.LoadUint32(&v) != 0x1234 {
		return 3
	}
	return 0
}

func TestSwapUint32() int {
	var v uint32 = 0x1111
	if atomic.SwapUint32(&v, 0x2222) != 0x1111 {
		return 1
	}
	if atomic.SwapUint32(&v, 0x3333) != 0x2222 {
		return 2
	}
	if atomic.LoadUint32(&v) != 0x3333 {
		return 3
	}
	return 0
}

func TestAndOrUint32() int {
	var v uint32 = 0xFFFF0000
	if atomic.AndUint32(&v, 0xFF00FF00) != 0xFFFF0000 {
		return 1
	}
	if atomic.LoadUint32(&v) != 0xFF000000 {
		return 2
	}
	if atomic.OrUint32(&v, 0x0000FFFF) != 0xFF000000 {
		return 3
	}
	if atomic.LoadUint32(&v) != 0xFF00FFFF {
		return 4
	}
	return 0
}

func TestLoadUintptr() int {
	var v uintptr = 0xABCDEF01
	if atomic.LoadUintptr(&v) != 0xABCDEF01 {
		return 1
	}
	atomic.StoreUintptr(&v, 0x87654321)
	if atomic.LoadUintptr(&v) != 0x87654321 {
		return 2
	}
	return 0
}

func TestAddUintptr() int {
	var v uintptr = 10
	if atomic.AddUintptr(&v, 20) != 30 {
		return 1
	}
	if atomic.AddUintptr(&v, 70) != 100 {
		return 2
	}
	if atomic.LoadUintptr(&v) != 100 {
		return 3
	}
	return 0
}

func TestCompareAndSwapUintptr() int {
	var v uintptr = 0xAAAA
	if atomic.CompareAndSwapUintptr(&v, 0xAAAA, 0xBBBB) != true {
		return 1
	}
	if atomic.CompareAndSwapUintptr(&v, 0xAAAA, 0xCCCC) != false {
		return 2
	}
	if atomic.LoadUintptr(&v) != 0xBBBB {
		return 3
	}
	return 0
}

func TestSwapUintptr() int {
	var v uintptr = 5
	if atomic.SwapUintptr(&v, 6) != 5 {
		return 1
	}
	if atomic.SwapUintptr(&v, 7) != 6 {
		return 2
	}
	if atomic.LoadUintptr(&v) != 7 {
		return 3
	}
	return 0
}

func TestAndOrUintptr() int {
	var v uintptr = 0x00FF00FF
	if atomic.AndUintptr(&v, 0x0F0F0F0F) != 0x00FF00FF {
		return 1
	}
	if atomic.LoadUintptr(&v) != 0x000F000F {
		return 2
	}
	if atomic.OrUintptr(&v, 0xF0F0F0F0) != 0x000F000F {
		return 3
	}
	if atomic.LoadUintptr(&v) != 0xF0FFF0FF {
		return 4
	}
	return 0
}

func TestLoadInt64() int {
	var v int64 = 0x1122334455667788
	if atomic.LoadInt64(&v) != 0x1122334455667788 {
		return 1
	}
	atomic.StoreInt64(&v, -0x1122334455667788)
	if atomic.LoadInt64(&v) != -0x1122334455667788 {
		return 2
	}
	return 0
}

func TestAddInt64() int {
	var v int64 = 1000
	if atomic.AddInt64(&v, 2000) != 3000 {
		return 1
	}
	if atomic.AddInt64(&v, -500) != 2500 {
		return 2
	}
	if atomic.LoadInt64(&v) != 2500 {
		return 3
	}
	return 0
}

func TestCompareAndSwapInt64() int {
	var v int64 = 0x0102030405060708
	if atomic.CompareAndSwapInt64(&v, 0x0102030405060708, 0x1112131415161718) != true {
		return 1
	}
	if atomic.CompareAndSwapInt64(&v, 0x0102030405060708, 0x2122232425262728) != false {
		return 2
	}
	if atomic.LoadInt64(&v) != 0x1112131415161718 {
		return 3
	}
	return 0
}

func TestSwapInt64() int {
	var v int64 = 100
	if atomic.SwapInt64(&v, 200) != 100 {
		return 1
	}
	if atomic.SwapInt64(&v, 300) != 200 {
		return 2
	}
	if atomic.LoadInt64(&v) != 300 {
		return 3
	}
	return 0
}

func TestAndOrInt64() int {
	var v int64 = 0x00FF00FF00FF00FF
	if atomic.AndInt64(&v, 0x0F0F0F0F0F0F0F0F) != 0x00FF00FF00FF00FF {
		return 1
	}
	if atomic.LoadInt64(&v) != 0x000F000F000F000F {
		return 2
	}
	if atomic.OrInt64(&v, 0x00000000000000F0) != 0x000F000F000F000F {
		return 3
	}
	if atomic.LoadInt64(&v) != 0x000F000F000F00FF {
		return 4
	}
	return 0
}

func TestLoadUint64() int {
	var v uint64 = 0xFFFFFFFFFFFFFFFF
	if atomic.LoadUint64(&v) != 0xFFFFFFFFFFFFFFFF {
		return 1
	}
	atomic.StoreUint64(&v, 0x0123456789ABCDEF)
	if atomic.LoadUint64(&v) != 0x0123456789ABCDEF {
		return 2
	}
	return 0
}

func TestAddUint64() int {
	var v uint64 = 0
	if atomic.AddUint64(&v, 0xFFFFFFFFFFFFFFFF) != 0xFFFFFFFFFFFFFFFF {
		return 1
	}
	if atomic.AddUint64(&v, 1) != 0 {
		return 2
	}
	if atomic.LoadUint64(&v) != 0 {
		return 3
	}
	return 0
}

func TestCompareAndSwapUint64() int {
	var v uint64 = 0xDEADBEEFDEADBEEF
	if atomic.CompareAndSwapUint64(&v, 0xDEADBEEFDEADBEEF, 0x1234567812345678) != true {
		return 1
	}
	if atomic.CompareAndSwapUint64(&v, 0xDEADBEEFDEADBEEF, 0x8765432187654321) != false {
		return 2
	}
	if atomic.LoadUint64(&v) != 0x1234567812345678 {
		return 3
	}
	return 0
}

func TestSwapUint64() int {
	var v uint64 = 0x1111222233334444
	if atomic.SwapUint64(&v, 0x5555666677778888) != 0x1111222233334444 {
		return 1
	}
	if atomic.LoadUint64(&v) != 0x5555666677778888 {
		return 2
	}
	return 0
}

func TestAndOrUint64() int {
	var v uint64 = 0xFF00FF00FF00FF00
	if atomic.AndUint64(&v, 0xF0F0F0F0F0F0F0F0) != 0xFF00FF00FF00FF00 {
		return 1
	}
	if atomic.LoadUint64(&v) != 0xF000F000F000F000 {
		return 2
	}
	if atomic.OrUint64(&v, 0x0F0F0F0F0F0F0F0F) != 0xF000F000F000F000 {
		return 3
	}
	if atomic.LoadUint64(&v) != 0xFF0FFF0FFF0FFF0F {
		return 4
	}
	return 0
}

func TestTypedInt32() int {
	var v atomic.Int32
	v.Store(10)
	if v.Load() != 10 {
		return 1
	}
	if v.Add(5) != 15 {
		return 2
	}
	if v.CompareAndSwap(15, 16) != true {
		return 3
	}
	if v.CompareAndSwap(15, 17) != false {
		return 4
	}
	if v.Swap(20) != 16 {
		return 5
	}
	if v.Load() != 20 {
		return 6
	}
	v.And(-16)
	if v.Load() != 16 {
		return 7
	}
	v.Or(0x0000000F)
	if v.Load() != 31 {
		return 8
	}
	return 0
}

func TestTypedInt64() int {
	var v atomic.Int64
	v.Store(-1234567890123456789)
	if v.Load() != -1234567890123456789 {
		return 1
	}
	if v.Add(1000000000000000000) != -234567890123456789 {
		return 2
	}
	if v.CompareAndSwap(-234567890123456789, 42) != true {
		return 3
	}
	if v.CompareAndSwap(-234567890123456789, 43) != false {
		return 4
	}
	if v.Swap(7) != 42 {
		return 5
	}
	if v.Load() != 7 {
		return 6
	}
	return 0
}

func TestTypedUint32() int {
	var v atomic.Uint32
	v.Store(0x0F0F0F0F)
	if v.Load() != 0x0F0F0F0F {
		return 1
	}
	if v.Add(1) != 0x0F0F0F10 {
		return 2
	}
	if v.CompareAndSwap(0x0F0F0F10, 0) != true {
		return 3
	}
	if v.CompareAndSwap(0x0F0F0F10, 1) != false {
		return 4
	}
	if v.Swap(0xA5A5A5A5) != 0 {
		return 5
	}
	if v.Load() != 0xA5A5A5A5 {
		return 6
	}
	v.And(0xFFFF0000)
	if v.Load() != 0xA5A50000 {
		return 7
	}
	v.Or(0x0000FFFF)
	if v.Load() != 0xA5A5FFFF {
		return 8
	}
	return 0
}

func TestTypedUint64() int {
	var v atomic.Uint64
	v.Store(0xFEDCBA9876543210)
	if v.Load() != 0xFEDCBA9876543210 {
		return 1
	}
	if v.Add(0x1111111111111111) != 0x0FEDCBA987654321 {
		return 2
	}
	if v.CompareAndSwap(0x0FEDCBA987654321, 0x1234567890ABCDEF) != true {
		return 3
	}
	if v.CompareAndSwap(0x0FEDCBA987654321, 0x1122334455667788) != false {
		return 4
	}
	if v.Swap(0xAAAAAAAAAAAAAAAA) != 0x1234567890ABCDEF {
		return 5
	}
	if v.Load() != 0xAAAAAAAAAAAAAAAA {
		return 6
	}
	return 0
}

func TestTypedUintptr() int {
	var v atomic.Uintptr
	v.Store(0x1111)
	if v.Load() != 0x1111 {
		return 1
	}
	if v.Add(0x2222) != 0x3333 {
		return 2
	}
	if v.CompareAndSwap(0x3333, 0x4444) != true {
		return 3
	}
	if v.CompareAndSwap(0x3333, 0x5555) != false {
		return 4
	}
	if v.Swap(0x6666) != 0x4444 {
		return 5
	}
	if v.Load() != 0x6666 {
		return 6
	}
	v.And(0xFF00)
	if v.Load() != 0x6600 {
		return 7
	}
	v.Or(0x00FF)
	if v.Load() != 0x66FF {
		return 8
	}
	return 0
}

func TestTypedBool() int {
	var v atomic.Bool
	if v.Load() != false {
		return 1
	}
	v.Store(true)
	if v.Load() != true {
		return 2
	}
	if v.Swap(false) != true {
		return 3
	}
	if v.Load() != false {
		return 4
	}
	if v.CompareAndSwap(false, true) != true {
		return 5
	}
	if v.CompareAndSwap(false, true) != false {
		return 6
	}
	if v.Load() != true {
		return 7
	}
	return 0
}

func TestSequence() int {
	var v int32
	for i := 0; i < 100; i++ {
		atomic.AddInt32(&v, 1)
	}
	if v != 100 {
		return 1
	}
	var w uint64 = 0xFFFFFFFFFFFFFFFF
	atomic.AddUint64(&w, 1)
	if w != 0 {
		return 2
	}
	var u uintptr
	atomic.StoreUintptr(&u, 100)
	if atomic.LoadUintptr(&u) != 100 {
		return 3
	}
	return 0
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("TestLoadInt32", TestLoadInt32)
	runTest("TestAddInt32", TestAddInt32)
	runTest("TestCompareAndSwapInt32", TestCompareAndSwapInt32)
	runTest("TestSwapInt32", TestSwapInt32)
	runTest("TestAndOrInt32", TestAndOrInt32)
	runTest("TestLoadUint32", TestLoadUint32)
	runTest("TestAddUint32", TestAddUint32)
	runTest("TestCompareAndSwapUint32", TestCompareAndSwapUint32)
	runTest("TestSwapUint32", TestSwapUint32)
	runTest("TestAndOrUint32", TestAndOrUint32)
	runTest("TestLoadUintptr", TestLoadUintptr)
	runTest("TestAddUintptr", TestAddUintptr)
	runTest("TestCompareAndSwapUintptr", TestCompareAndSwapUintptr)
	runTest("TestSwapUintptr", TestSwapUintptr)
	runTest("TestAndOrUintptr", TestAndOrUintptr)
	runTest("TestLoadInt64", TestLoadInt64)
	runTest("TestAddInt64", TestAddInt64)
	runTest("TestCompareAndSwapInt64", TestCompareAndSwapInt64)
	runTest("TestSwapInt64", TestSwapInt64)
	runTest("TestAndOrInt64", TestAndOrInt64)
	runTest("TestLoadUint64", TestLoadUint64)
	runTest("TestAddUint64", TestAddUint64)
	runTest("TestCompareAndSwapUint64", TestCompareAndSwapUint64)
	runTest("TestSwapUint64", TestSwapUint64)
	runTest("TestAndOrUint64", TestAndOrUint64)
	runTest("TestTypedInt32", TestTypedInt32)
	runTest("TestTypedInt64", TestTypedInt64)
	runTest("TestTypedUint32", TestTypedUint32)
	runTest("TestTypedUint64", TestTypedUint64)
	runTest("TestTypedUintptr", TestTypedUintptr)
	runTest("TestTypedBool", TestTypedBool)
	runTest("TestSequence", TestSequence)
}
