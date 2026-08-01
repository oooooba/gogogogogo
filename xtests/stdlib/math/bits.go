package main

import (
	"math/bits"
)

func TestUintSize() int {
	if bits.UintSize != 64 {
		return 1
	}
	return 0
}

func TestLeadingZeros() int {
	if bits.LeadingZeros(0) != 64 {
		return 1
	}
	if bits.LeadingZeros(1) != 63 {
		return 2
	}
	if bits.LeadingZeros(0x8000000000000000) != 0 {
		return 3
	}
	if bits.LeadingZeros(0x0F) != 60 {
		return 4
	}
	return 0
}

func TestLeadingZeros8() int {
	if bits.LeadingZeros8(0) != 8 {
		return 1
	}
	if bits.LeadingZeros8(1) != 7 {
		return 2
	}
	if bits.LeadingZeros8(0x80) != 0 {
		return 3
	}
	if bits.LeadingZeros8(0x0F) != 4 {
		return 4
	}
	return 0
}

func TestLeadingZeros16() int {
	if bits.LeadingZeros16(0) != 16 {
		return 1
	}
	if bits.LeadingZeros16(1) != 15 {
		return 2
	}
	if bits.LeadingZeros16(0x8000) != 0 {
		return 3
	}
	if bits.LeadingZeros16(0x00FF) != 8 {
		return 4
	}
	return 0
}

func TestLeadingZeros32() int {
	if bits.LeadingZeros32(0) != 32 {
		return 1
	}
	if bits.LeadingZeros32(1) != 31 {
		return 2
	}
	if bits.LeadingZeros32(0x80000000) != 0 {
		return 3
	}
	if bits.LeadingZeros32(0x0000FFFF) != 16 {
		return 4
	}
	return 0
}

func TestLeadingZeros64() int {
	if bits.LeadingZeros64(0) != 64 {
		return 1
	}
	if bits.LeadingZeros64(1) != 63 {
		return 2
	}
	if bits.LeadingZeros64(0x8000000000000000) != 0 {
		return 3
	}
	if bits.LeadingZeros64(0x00000000FFFFFFFF) != 32 {
		return 4
	}
	return 0
}

func TestTrailingZeros() int {
	if bits.TrailingZeros(0) != 64 {
		return 1
	}
	if bits.TrailingZeros(1) != 0 {
		return 2
	}
	if bits.TrailingZeros(2) != 1 {
		return 3
	}
	if bits.TrailingZeros(0x8000000000000000) != 63 {
		return 4
	}
	if bits.TrailingZeros(0x800) != 11 {
		return 5
	}
	return 0
}

func TestTrailingZeros8() int {
	if bits.TrailingZeros8(0) != 8 {
		return 1
	}
	if bits.TrailingZeros8(1) != 0 {
		return 2
	}
	if bits.TrailingZeros8(2) != 1 {
		return 3
	}
	if bits.TrailingZeros8(0x80) != 7 {
		return 4
	}
	if bits.TrailingZeros8(0x10) != 4 {
		return 5
	}
	return 0
}

func TestTrailingZeros16() int {
	if bits.TrailingZeros16(0) != 16 {
		return 1
	}
	if bits.TrailingZeros16(0x8000) != 15 {
		return 2
	}
	if bits.TrailingZeros16(0x0100) != 8 {
		return 3
	}
	if bits.TrailingZeros16(0x0040) != 6 {
		return 4
	}
	return 0
}

func TestTrailingZeros32() int {
	if bits.TrailingZeros32(0) != 32 {
		return 1
	}
	if bits.TrailingZeros32(0x80000000) != 31 {
		return 2
	}
	if bits.TrailingZeros32(0x00010000) != 16 {
		return 3
	}
	if bits.TrailingZeros32(0x00200000) != 21 {
		return 4
	}
	return 0
}

func TestTrailingZeros64() int {
	if bits.TrailingZeros64(0) != 64 {
		return 1
	}
	if bits.TrailingZeros64(0x8000000000000000) != 63 {
		return 2
	}
	if bits.TrailingZeros64(0x0000000001000000) != 24 {
		return 3
	}
	if bits.TrailingZeros64(0x4000000000000000) != 62 {
		return 4
	}
	return 0
}

func TestOnesCount() int {
	if bits.OnesCount(0) != 0 {
		return 1
	}
	if bits.OnesCount(0xFF) != 8 {
		return 2
	}
	if bits.OnesCount(0xFFFFFFFFFFFFFFFF) != 64 {
		return 3
	}
	if bits.OnesCount(0x12345678) != 13 {
		return 4
	}
	return 0
}

func TestOnesCount8() int {
	if bits.OnesCount8(0) != 0 {
		return 1
	}
	if bits.OnesCount8(0xFF) != 8 {
		return 2
	}
	if bits.OnesCount8(0b10101010) != 4 {
		return 3
	}
	if bits.OnesCount8(0b00001111) != 4 {
		return 4
	}
	return 0
}

func TestOnesCount16() int {
	if bits.OnesCount16(0) != 0 {
		return 1
	}
	if bits.OnesCount16(0xFFFF) != 16 {
		return 2
	}
	if bits.OnesCount16(0x0F0F) != 8 {
		return 3
	}
	if bits.OnesCount16(0xFFFF) != 16 {
		return 4
	}
	return 0
}

func TestOnesCount32() int {
	if bits.OnesCount32(0) != 0 {
		return 1
	}
	if bits.OnesCount32(0xFFFFFFFF) != 32 {
		return 2
	}
	if bits.OnesCount32(0x12345678) != 13 {
		return 3
	}
	if bits.OnesCount32(0x80000001) != 2 {
		return 4
	}
	return 0
}

func TestOnesCount64() int {
	if bits.OnesCount64(0) != 0 {
		return 1
	}
	if bits.OnesCount64(0xFFFFFFFFFFFFFFFF) != 64 {
		return 2
	}
	if bits.OnesCount64(0x00FF00FF00FF00FF) != 32 {
		return 3
	}
	if bits.OnesCount64(0x8000000000000001) != 2 {
		return 4
	}
	return 0
}

func TestRotateLeft() int {
	if bits.RotateLeft(0x8000000000000000, 1) != 1 {
		return 1
	}
	if bits.RotateLeft(1, 63) != 0x8000000000000000 {
		return 2
	}
	if bits.RotateLeft(1, 64) != 1 {
		return 3
	}
	if bits.RotateLeft(1, -1) != 0x8000000000000000 {
		return 4
	}
	if bits.RotateLeft(0x123456789ABCDEF0, 4) != 0x23456789ABCDEF01 {
		return 5
	}
	return 0
}

func TestRotateLeft8() int {
	if bits.RotateLeft8(0x01, 1) != 0x02 {
		return 1
	}
	if bits.RotateLeft8(0x80, 1) != 0x01 {
		return 2
	}
	if bits.RotateLeft8(0x01, -1) != 0x80 {
		return 3
	}
	if bits.RotateLeft8(0xA0, 3) != 0x05 {
		return 4
	}
	if bits.RotateLeft8(0x01, 8) != 0x01 {
		return 5
	}
	return 0
}

func TestRotateLeft16() int {
	if bits.RotateLeft16(0x8000, 1) != 0x0001 {
		return 1
	}
	if bits.RotateLeft16(0x0001, -1) != 0x8000 {
		return 2
	}
	if bits.RotateLeft16(0x1234, 8) != 0x3412 {
		return 3
	}
	if bits.RotateLeft16(0x0001, 16) != 0x0001 {
		return 4
	}
	return 0
}

func TestRotateLeft32() int {
	if bits.RotateLeft32(0x80000000, 1) != 1 {
		return 1
	}
	if bits.RotateLeft32(0x00000001, -1) != 0x80000000 {
		return 2
	}
	if bits.RotateLeft32(0x12345678, 8) != 0x34567812 {
		return 3
	}
	if bits.RotateLeft32(0x00000001, 32) != 1 {
		return 4
	}
	return 0
}

func TestRotateLeft64() int {
	if bits.RotateLeft64(0x8000000000000000, 1) != 1 {
		return 1
	}
	if bits.RotateLeft64(0x0000000000000001, -1) != 0x8000000000000000 {
		return 2
	}
	if bits.RotateLeft64(0x123456789ABCDEF0, 8) != 0x3456789ABCDEF012 {
		return 3
	}
	if bits.RotateLeft64(0x0000000000000001, 64) != 1 {
		return 4
	}
	return 0
}

func TestReverse() int {
	if bits.Reverse(0) != 0 {
		return 1
	}
	if bits.Reverse(1) != 0x8000000000000000 {
		return 2
	}
	if bits.Reverse(0x8000000000000000) != 1 {
		return 3
	}
	if bits.Reverse(0x0F) != 0xF000000000000000 {
		return 4
	}
	return 0
}

func TestReverse8() int {
	if bits.Reverse8(0) != 0 {
		return 1
	}
	if bits.Reverse8(0x01) != 0x80 {
		return 2
	}
	if bits.Reverse8(0x80) != 0x01 {
		return 3
	}
	if bits.Reverse8(0b10110000) != 0b00001101 {
		return 4
	}
	return 0
}

func TestReverse16() int {
	if bits.Reverse16(0) != 0 {
		return 1
	}
	if bits.Reverse16(0x0001) != 0x8000 {
		return 2
	}
	if bits.Reverse16(0x8000) != 0x0001 {
		return 3
	}
	if bits.Reverse16(0x1234) != 0x2C48 {
		return 4
	}
	return 0
}

func TestReverse32() int {
	if bits.Reverse32(0) != 0 {
		return 1
	}
	if bits.Reverse32(0x00000001) != 0x80000000 {
		return 2
	}
	if bits.Reverse32(0x80000000) != 0x00000001 {
		return 3
	}
	if bits.Reverse32(0x0000FFFF) != 0xFFFF0000 {
		return 4
	}
	return 0
}

func TestReverse64() int {
	if bits.Reverse64(0) != 0 {
		return 1
	}
	if bits.Reverse64(0x0000000000000001) != 0x8000000000000000 {
		return 2
	}
	if bits.Reverse64(0x8000000000000000) != 0x0000000000000001 {
		return 3
	}
	if bits.Reverse64(0x00000000FFFFFFFF) != 0xFFFFFFFF00000000 {
		return 4
	}
	return 0
}

func TestReverseBytes() int {
	if bits.ReverseBytes(0) != 0 {
		return 1
	}
	if bits.ReverseBytes(0x1122334455667788) != 0x8877665544332211 {
		return 2
	}
	if bits.ReverseBytes(0x01) != 0x0100000000000000 {
		return 3
	}
	return 0
}

func TestReverseBytes16() int {
	if bits.ReverseBytes16(0) != 0 {
		return 1
	}
	if bits.ReverseBytes16(0x1234) != 0x3412 {
		return 2
	}
	if bits.ReverseBytes16(0x8000) != 0x0080 {
		return 3
	}
	return 0
}

func TestReverseBytes32() int {
	if bits.ReverseBytes32(0) != 0 {
		return 1
	}
	if bits.ReverseBytes32(0x12345678) != 0x78563412 {
		return 2
	}
	if bits.ReverseBytes32(0x01020304) != 0x04030201 {
		return 3
	}
	return 0
}

func TestReverseBytes64() int {
	if bits.ReverseBytes64(0) != 0 {
		return 1
	}
	if bits.ReverseBytes64(0x1122334455667788) != 0x8877665544332211 {
		return 2
	}
	if bits.ReverseBytes64(0x0102030405060708) != 0x0807060504030201 {
		return 3
	}
	return 0
}

func TestLen() int {
	if bits.Len(0) != 0 {
		return 1
	}
	if bits.Len(1) != 1 {
		return 2
	}
	if bits.Len(0x8000000000000000) != 64 {
		return 3
	}
	if bits.Len(0x0F) != 4 {
		return 4
	}
	if bits.Len(0x100) != 9 {
		return 5
	}
	return 0
}

func TestLen8() int {
	if bits.Len8(0) != 0 {
		return 1
	}
	if bits.Len8(1) != 1 {
		return 2
	}
	if bits.Len8(0x80) != 8 {
		return 3
	}
	if bits.Len8(0x0F) != 4 {
		return 4
	}
	return 0
}

func TestLen16() int {
	if bits.Len16(0) != 0 {
		return 1
	}
	if bits.Len16(1) != 1 {
		return 2
	}
	if bits.Len16(0x8000) != 16 {
		return 3
	}
	if bits.Len16(0x00FF) != 8 {
		return 4
	}
	return 0
}

func TestLen32() int {
	if bits.Len32(0) != 0 {
		return 1
	}
	if bits.Len32(1) != 1 {
		return 2
	}
	if bits.Len32(0x80000000) != 32 {
		return 3
	}
	if bits.Len32(0x0000FFFF) != 16 {
		return 4
	}
	return 0
}

func TestLen64() int {
	if bits.Len64(0) != 0 {
		return 1
	}
	if bits.Len64(1) != 1 {
		return 2
	}
	if bits.Len64(0x8000000000000000) != 64 {
		return 3
	}
	if bits.Len64(0x00000000FFFFFFFF) != 32 {
		return 4
	}
	return 0
}

func TestAdd() int {
	sum, carryOut := bits.Add(1, 2, 0)
	if sum != 3 || carryOut != 0 {
		return 1
	}
	sum, carryOut = bits.Add(0xFFFFFFFFFFFFFFFF, 1, 0)
	if sum != 0 || carryOut != 1 {
		return 2
	}
	sum, carryOut = bits.Add(0xFFFFFFFFFFFFFFFF, 0, 1)
	if sum != 0 || carryOut != 1 {
		return 3
	}
	sum, carryOut = bits.Add(0xFFFFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF, 1)
	if sum != 0xFFFFFFFFFFFFFFFF || carryOut != 1 {
		return 4
	}
	return 0
}

func TestAdd32() int {
	sum, carryOut := bits.Add32(1, 2, 0)
	if sum != 3 || carryOut != 0 {
		return 1
	}
	sum, carryOut = bits.Add32(0xFFFFFFFF, 1, 0)
	if sum != 0 || carryOut != 1 {
		return 2
	}
	sum, carryOut = bits.Add32(0xFFFFFFFF, 0, 1)
	if sum != 0 || carryOut != 1 {
		return 3
	}
	sum, carryOut = bits.Add32(0xFFFFFFFF, 0xFFFFFFFF, 1)
	if sum != 0xFFFFFFFF || carryOut != 1 {
		return 4
	}
	return 0
}

func TestAdd64() int {
	sum, carryOut := bits.Add64(1, 2, 0)
	if sum != 3 || carryOut != 0 {
		return 1
	}
	sum, carryOut = bits.Add64(0xFFFFFFFFFFFFFFFF, 1, 0)
	if sum != 0 || carryOut != 1 {
		return 2
	}
	sum, carryOut = bits.Add64(0xFFFFFFFFFFFFFFFF, 0, 1)
	if sum != 0 || carryOut != 1 {
		return 3
	}
	sum, carryOut = bits.Add64(0xFFFFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF, 1)
	if sum != 0xFFFFFFFFFFFFFFFF || carryOut != 1 {
		return 4
	}
	return 0
}

func TestSub() int {
	diff, borrowOut := bits.Sub(3, 1, 0)
	if diff != 2 || borrowOut != 0 {
		return 1
	}
	diff, borrowOut = bits.Sub(0, 1, 0)
	if diff != 0xFFFFFFFFFFFFFFFF || borrowOut != 1 {
		return 2
	}
	diff, borrowOut = bits.Sub(0, 0, 1)
	if diff != 0xFFFFFFFFFFFFFFFF || borrowOut != 1 {
		return 3
	}
	diff, borrowOut = bits.Sub(1, 1, 1)
	if diff != 0xFFFFFFFFFFFFFFFF || borrowOut != 1 {
		return 4
	}
	return 0
}

func TestSub32() int {
	diff, borrowOut := bits.Sub32(3, 1, 0)
	if diff != 2 || borrowOut != 0 {
		return 1
	}
	diff, borrowOut = bits.Sub32(0, 1, 0)
	if diff != 0xFFFFFFFF || borrowOut != 1 {
		return 2
	}
	diff, borrowOut = bits.Sub32(1, 1, 1)
	if diff != 0xFFFFFFFF || borrowOut != 1 {
		return 3
	}
	return 0
}

func TestSub64() int {
	diff, borrowOut := bits.Sub64(3, 1, 0)
	if diff != 2 || borrowOut != 0 {
		return 1
	}
	diff, borrowOut = bits.Sub64(0, 1, 0)
	if diff != 0xFFFFFFFFFFFFFFFF || borrowOut != 1 {
		return 2
	}
	diff, borrowOut = bits.Sub64(1, 1, 1)
	if diff != 0xFFFFFFFFFFFFFFFF || borrowOut != 1 {
		return 3
	}
	return 0
}

func TestMul() int {
	hi, lo := bits.Mul(2, 3)
	if hi != 0 || lo != 6 {
		return 1
	}
	hi, lo = bits.Mul(0xFFFFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF)
	if hi != 0xFFFFFFFFFFFFFFFE || lo != 1 {
		return 2
	}
	hi, lo = bits.Mul(0x100000000, 0x100000000)
	if hi != 1 || lo != 0 {
		return 3
	}
	return 0
}

func TestMul32() int {
	hi, lo := bits.Mul32(2, 3)
	if hi != 0 || lo != 6 {
		return 1
	}
	hi, lo = bits.Mul32(0xFFFFFFFF, 0xFFFFFFFF)
	if hi != 0xFFFFFFFE || lo != 1 {
		return 2
	}
	hi, lo = bits.Mul32(0x10000, 0x10000)
	if hi != 1 || lo != 0 {
		return 3
	}
	return 0
}

func TestMul64() int {
	hi, lo := bits.Mul64(2, 3)
	if hi != 0 || lo != 6 {
		return 1
	}
	hi, lo = bits.Mul64(0xFFFFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF)
	if hi != 0xFFFFFFFFFFFFFFFE || lo != 1 {
		return 2
	}
	hi, lo = bits.Mul64(0x100000000, 0x100000000)
	if hi != 1 || lo != 0 {
		return 3
	}
	return 0
}

func TestDiv() int {
	quo, rem := bits.Div(0, 20, 3)
	if quo != 6 || rem != 2 {
		return 1
	}
	quo, rem = bits.Div(0, 100, 7)
	if quo != 14 || rem != 2 {
		return 2
	}
	quo, rem = bits.Div(1, 0, 0xFFFFFFFFFFFFFFFF)
	if quo != 1 || rem != 1 {
		return 3
	}
	quo, rem = bits.Div(0, 0, 1)
	if quo != 0 || rem != 0 {
		return 4
	}
	quo, rem = bits.Div(0xFFFFFFFF, 0xFFFFFFFF, 0x100000000)
	if quo != 0xFFFFFFFF00000000 || rem != 0xFFFFFFFF {
		return 5
	}
	return 0
}

func TestDiv32() int {
	quo, rem := bits.Div32(0, 20, 3)
	if quo != 6 || rem != 2 {
		return 1
	}
	quo, rem = bits.Div32(0, 100, 7)
	if quo != 14 || rem != 2 {
		return 2
	}
	quo, rem = bits.Div32(0, 0, 1)
	if quo != 0 || rem != 0 {
		return 3
	}
	quo, rem = bits.Div32(0xFFFF, 0xFFFF, 0x10000)
	if quo != 0xFFFF0000 || rem != 0xFFFF {
		return 4
	}
	return 0
}

func TestDiv64() int {
	quo, rem := bits.Div64(0, 20, 3)
	if quo != 6 || rem != 2 {
		return 1
	}
	quo, rem = bits.Div64(0, 100, 7)
	if quo != 14 || rem != 2 {
		return 2
	}
	quo, rem = bits.Div64(1, 0, 0xFFFFFFFFFFFFFFFF)
	if quo != 1 || rem != 1 {
		return 3
	}
	quo, rem = bits.Div64(0, 0, 1)
	if quo != 0 || rem != 0 {
		return 4
	}
	quo, rem = bits.Div64(0xFFFFFFFF, 0xFFFFFFFF, 0x100000000)
	if quo != 0xFFFFFFFF00000000 || rem != 0xFFFFFFFF {
		return 5
	}
	return 0
}

func TestRem() int {
	if bits.Rem(0, 20, 3) != 2 {
		return 1
	}
	if bits.Rem(0, 100, 7) != 2 {
		return 2
	}
	if bits.Rem(1, 0, 0xFFFFFFFFFFFFFFFF) != 1 {
		return 3
	}
	return 0
}

func TestRem32() int {
	if bits.Rem32(0, 20, 3) != 2 {
		return 1
	}
	if bits.Rem32(0, 100, 7) != 2 {
		return 2
	}
	if bits.Rem32(0xFFFF, 0xFFFF, 0x10000) != 0xFFFF {
		return 3
	}
	return 0
}

func TestRem64() int {
	if bits.Rem64(0, 20, 3) != 2 {
		return 1
	}
	if bits.Rem64(0, 100, 7) != 2 {
		return 2
	}
	if bits.Rem64(1, 0, 0xFFFFFFFFFFFFFFFF) != 1 {
		return 3
	}
	return 0
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("TestUintSize", TestUintSize)
	runTest("TestLeadingZeros", TestLeadingZeros)
	runTest("TestLeadingZeros8", TestLeadingZeros8)
	runTest("TestLeadingZeros16", TestLeadingZeros16)
	runTest("TestLeadingZeros32", TestLeadingZeros32)
	runTest("TestLeadingZeros64", TestLeadingZeros64)
	runTest("TestTrailingZeros", TestTrailingZeros)
	runTest("TestTrailingZeros8", TestTrailingZeros8)
	runTest("TestTrailingZeros16", TestTrailingZeros16)
	runTest("TestTrailingZeros32", TestTrailingZeros32)
	runTest("TestTrailingZeros64", TestTrailingZeros64)
	runTest("TestOnesCount", TestOnesCount)
	runTest("TestOnesCount8", TestOnesCount8)
	runTest("TestOnesCount16", TestOnesCount16)
	runTest("TestOnesCount32", TestOnesCount32)
	runTest("TestOnesCount64", TestOnesCount64)
	runTest("TestRotateLeft", TestRotateLeft)
	runTest("TestRotateLeft8", TestRotateLeft8)
	runTest("TestRotateLeft16", TestRotateLeft16)
	runTest("TestRotateLeft32", TestRotateLeft32)
	runTest("TestRotateLeft64", TestRotateLeft64)
	runTest("TestReverse", TestReverse)
	runTest("TestReverse8", TestReverse8)
	runTest("TestReverse16", TestReverse16)
	runTest("TestReverse32", TestReverse32)
	runTest("TestReverse64", TestReverse64)
	runTest("TestReverseBytes", TestReverseBytes)
	runTest("TestReverseBytes16", TestReverseBytes16)
	runTest("TestReverseBytes32", TestReverseBytes32)
	runTest("TestReverseBytes64", TestReverseBytes64)
	runTest("TestLen", TestLen)
	runTest("TestLen8", TestLen8)
	runTest("TestLen16", TestLen16)
	runTest("TestLen32", TestLen32)
	runTest("TestLen64", TestLen64)
	runTest("TestAdd", TestAdd)
	runTest("TestAdd32", TestAdd32)
	runTest("TestAdd64", TestAdd64)
	runTest("TestSub", TestSub)
	runTest("TestSub32", TestSub32)
	runTest("TestSub64", TestSub64)
	runTest("TestMul", TestMul)
	runTest("TestMul32", TestMul32)
	runTest("TestMul64", TestMul64)
	runTest("TestDiv", TestDiv)
	runTest("TestDiv32", TestDiv32)
	runTest("TestDiv64", TestDiv64)
	runTest("TestRem", TestRem)
	runTest("TestRem32", TestRem32)
	runTest("TestRem64", TestRem64)
}
