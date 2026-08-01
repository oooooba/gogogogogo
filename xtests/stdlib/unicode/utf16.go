package main

import "unicode/utf16"

func TestUtf16IsSurrogate() int {
	if utf16.IsSurrogate(0xD7FF) {
		return 1
	}
	if !utf16.IsSurrogate(0xD800) {
		return 2
	}
	if !utf16.IsSurrogate(0xDBFF) {
		return 3
	}
	if !utf16.IsSurrogate(0xDC00) {
		return 4
	}
	if !utf16.IsSurrogate(0xDFFF) {
		return 5
	}
	if utf16.IsSurrogate(0xE000) {
		return 6
	}
	if utf16.IsSurrogate('A') {
		return 7
	}
	return 0
}

func TestUtf16DecodeRune() int {
	if utf16.DecodeRune(0xD800, 0xDC00) != 0x10000 {
		return 1
	}
	if utf16.DecodeRune(0xD834, 0xDD1E) != 0x1D11E {
		return 2
	}
	if utf16.DecodeRune(0xDBFF, 0xDFFF) != 0x10FFFF {
		return 3
	}
	if utf16.DecodeRune(0xD800, 0x0041) != 0xFFFD {
		return 4
	}
	if utf16.DecodeRune(0x0041, 0xDC00) != 0xFFFD {
		return 5
	}
	if utf16.DecodeRune(0xE000, 0xDC00) != 0xFFFD {
		return 6
	}
	return 0
}

func TestUtf16EncodeRune() int {
	r1, r2 := utf16.EncodeRune(0x10000)
	if r1 != 0xD800 || r2 != 0xDC00 {
		return 1
	}
	r1, r2 = utf16.EncodeRune(0x1D11E)
	if r1 != 0xD834 || r2 != 0xDD1E {
		return 2
	}
	r1, r2 = utf16.EncodeRune(0x10FFFF)
	if r1 != 0xDBFF || r2 != 0xDFFF {
		return 3
	}
	r1, r2 = utf16.EncodeRune('A')
	if r1 != 0xFFFD || r2 != 0xFFFD {
		return 4
	}
	r1, r2 = utf16.EncodeRune(0x110000)
	if r1 != 0xFFFD || r2 != 0xFFFD {
		return 5
	}
	return 0
}

func TestUtf16RuneLen() int {
	if utf16.RuneLen('A') != 1 {
		return 1
	}
	if utf16.RuneLen('€') != 1 {
		return 2
	}
	if utf16.RuneLen(0xD7FF) != 1 {
		return 3
	}
	if utf16.RuneLen(0xD800) != -1 {
		return 4
	}
	if utf16.RuneLen(0xDFFF) != -1 {
		return 5
	}
	if utf16.RuneLen(0x10000) != 2 {
		return 6
	}
	if utf16.RuneLen(0x10FFFF) != 2 {
		return 7
	}
	if utf16.RuneLen(0x110000) != -1 {
		return 8
	}
	if utf16.RuneLen(-1) != -1 {
		return 9
	}
	return 0
}

func TestUtf16Encode() int {
	s := utf16.Encode([]rune{'A', '€', '\U0001F600', 0xD800})
	if len(s) != 5 {
		return 1
	}
	expect := []uint16{0x0041, 0x20AC, 0xD83D, 0xDE00, 0xFFFD}
	for i := range expect {
		if s[i] != expect[i] {
			return 2 + i
		}
	}
	s2 := utf16.Encode(nil)
	if len(s2) != 0 {
		return 8
	}
	return 0
}

func TestUtf16AppendRune() int {
	a := utf16.AppendRune(nil, 'A')
	if len(a) != 1 || a[0] != 0x0041 {
		return 1
	}
	a = utf16.AppendRune(a, '€')
	if len(a) != 2 || a[1] != 0x20AC {
		return 2
	}
	a = utf16.AppendRune(a, '\U0001F600')
	if len(a) != 4 || a[2] != 0xD83D || a[3] != 0xDE00 {
		return 3
	}
	a = utf16.AppendRune(a, 0xD800)
	if len(a) != 5 || a[4] != 0xFFFD {
		return 4
	}
	a = utf16.AppendRune(a, 0x110000)
	if len(a) != 6 || a[5] != 0xFFFD {
		return 5
	}
	return 0
}

func TestUtf16Decode() int {
	s := utf16.Decode([]uint16{0x0041, 0x20AC, 0xD83D, 0xDE00, 0xD800, 0x0042})
	if len(s) != 5 {
		return 1
	}
	expect := []rune{'A', '€', '\U0001F600', 0xFFFD, 'B'}
	for i := range expect {
		if s[i] != expect[i] {
			return 2 + i
		}
	}
	s2 := utf16.Decode([]uint16{0xD800, 0xD800, 0xDC00, 0x0041})
	if len(s2) != 3 {
		return 8
	}
	expect2 := []rune{0xFFFD, 0x10000, 'A'}
	for i := range expect2 {
		if s2[i] != expect2[i] {
			return 9 + i
		}
	}
	s3 := utf16.Decode(nil)
	if len(s3) != 0 {
		return 12
	}
	return 0
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("TestUtf16IsSurrogate", TestUtf16IsSurrogate)
	runTest("TestUtf16DecodeRune", TestUtf16DecodeRune)
	runTest("TestUtf16EncodeRune", TestUtf16EncodeRune)
	runTest("TestUtf16RuneLen", TestUtf16RuneLen)
	runTest("TestUtf16Encode", TestUtf16Encode)
	runTest("TestUtf16AppendRune", TestUtf16AppendRune)
	runTest("TestUtf16Decode", TestUtf16Decode)
}
