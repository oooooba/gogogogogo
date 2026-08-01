package main

import "unicode/utf8"

func TestUtf8Constants() int {
	if utf8.RuneError != 0xFFFD {
		return 1
	}
	if utf8.RuneSelf != 0x80 {
		return 2
	}
	if utf8.MaxRune != 0x10FFFF {
		return 3
	}
	if utf8.UTFMax != 4 {
		return 4
	}
	return 0
}

func TestUtf8EncodeRune() int {
	buf := make([]byte, 4)
	n := utf8.EncodeRune(buf, 'A')
	if n != 1 || buf[0] != 'A' {
		return 1
	}
	n = utf8.EncodeRune(buf, '€')
	if n != 3 || buf[0] != 0xE2 || buf[1] != 0x82 || buf[2] != 0xAC {
		return 2
	}
	n = utf8.EncodeRune(buf, utf8.MaxRune)
	if n != 4 || buf[0] != 0xF4 || buf[1] != 0x8F || buf[2] != 0xBF || buf[3] != 0xBF {
		return 3
	}
	n = utf8.EncodeRune(buf, utf8.MaxRune+1)
	if n != 3 || buf[0] != 0xEF || buf[1] != 0xBF || buf[2] != 0xBD {
		return 4
	}
	n = utf8.EncodeRune(buf, 0xD800)
	if n != 3 || buf[0] != 0xEF || buf[1] != 0xBF || buf[2] != 0xBD {
		return 5
	}
	return 0
}

func TestUtf8DecodeRune() int {
	r, size := utf8.DecodeRune([]byte{'A'})
	if r != 'A' || size != 1 {
		return 1
	}
	r, size = utf8.DecodeRune([]byte{0xE2, 0x82, 0xAC})
	if r != '€' || size != 3 {
		return 2
	}
	r, size = utf8.DecodeRune([]byte{})
	if r != utf8.RuneError || size != 0 {
		return 3
	}
	r, size = utf8.DecodeRune([]byte{0xFF})
	if r != utf8.RuneError || size != 1 {
		return 4
	}
	r, size = utf8.DecodeRune([]byte{0xE2})
	if r != utf8.RuneError || size != 1 {
		return 5
	}
	r, size = utf8.DecodeRune([]byte{0xED, 0xA0, 0x80})
	if r != utf8.RuneError || size != 1 {
		return 6
	}
	return 0
}

func TestUtf8DecodeRuneInString() int {
	r, size := utf8.DecodeRuneInString("A")
	if r != 'A' || size != 1 {
		return 1
	}
	r, size = utf8.DecodeRuneInString("€")
	if r != '€' || size != 3 {
		return 2
	}
	r, size = utf8.DecodeRuneInString("")
	if r != utf8.RuneError || size != 0 {
		return 3
	}
	r, size = utf8.DecodeRuneInString("\xFF")
	if r != utf8.RuneError || size != 1 {
		return 4
	}
	r, size = utf8.DecodeRuneInString("A€z")
	if r != 'A' || size != 1 {
		return 5
	}
	return 0
}

func TestUtf8DecodeLastRune() int {
	r, size := utf8.DecodeLastRune([]byte{'A'})
	if r != 'A' || size != 1 {
		return 1
	}
	r, size = utf8.DecodeLastRune([]byte{0xE2, 0x82, 0xAC})
	if r != '€' || size != 3 {
		return 2
	}
	r, size = utf8.DecodeLastRune([]byte{})
	if r != utf8.RuneError || size != 0 {
		return 3
	}
	r, size = utf8.DecodeLastRune([]byte{'A', 0x80})
	if r != utf8.RuneError || size != 1 {
		return 4
	}
	return 0
}

func TestUtf8DecodeLastRuneInString() int {
	r, size := utf8.DecodeLastRuneInString("A")
	if r != 'A' || size != 1 {
		return 1
	}
	r, size = utf8.DecodeLastRuneInString("A€")
	if r != '€' || size != 3 {
		return 2
	}
	r, size = utf8.DecodeLastRuneInString("")
	if r != utf8.RuneError || size != 0 {
		return 3
	}
	return 0
}

func TestUtf8AppendRune() int {
	if string(utf8.AppendRune([]byte("A"), '€')) != "A€" {
		return 1
	}
	if string(utf8.AppendRune([]byte("A"), utf8.MaxRune+1)) != "A\uFFFD" {
		return 2
	}
	if string(utf8.AppendRune(nil, 'A')) != "A" {
		return 3
	}
	return 0
}

func TestUtf8FullRune() int {
	if !utf8.FullRune([]byte{'A'}) {
		return 1
	}
	if !utf8.FullRune([]byte{0xE2, 0x82, 0xAC}) {
		return 2
	}
	if utf8.FullRune([]byte{0xE2, 0x82}) {
		return 3
	}
	if !utf8.FullRune([]byte{0xFF}) {
		return 4
	}
	if utf8.FullRune([]byte{}) {
		return 5
	}
	return 0
}

func TestUtf8FullRuneInString() int {
	if !utf8.FullRuneInString("A") {
		return 1
	}
	if !utf8.FullRuneInString("€") {
		return 2
	}
	if !utf8.FullRuneInString("A\xE2\x82") {
		return 3
	}
	if utf8.FullRuneInString("\xE2\x82") {
		return 4
	}
	if utf8.FullRuneInString("") {
		return 5
	}
	return 0
}

func TestUtf8RuneCount() int {
	if utf8.RuneCount([]byte("A€z")) != 3 {
		return 1
	}
	if utf8.RuneCount([]byte("日本語")) != 3 {
		return 2
	}
	if utf8.RuneCount([]byte{0xFF, 0xFF}) != 2 {
		return 3
	}
	return 0
}

func TestUtf8RuneCountInString() int {
	if utf8.RuneCountInString("A€z") != 3 {
		return 1
	}
	if utf8.RuneCountInString("日本語") != 3 {
		return 2
	}
	if utf8.RuneCountInString("A\xFFz") != 3 {
		return 3
	}
	return 0
}

func TestUtf8RuneLen() int {
	if utf8.RuneLen('A') != 1 {
		return 1
	}
	if utf8.RuneLen('€') != 3 {
		return 2
	}
	if utf8.RuneLen(utf8.MaxRune) != 4 {
		return 3
	}
	if utf8.RuneLen(utf8.MaxRune+1) != -1 {
		return 4
	}
	if utf8.RuneLen(0xD800) != -1 {
		return 5
	}
	if utf8.RuneLen(-1) != -1 {
		return 6
	}
	return 0
}

func TestUtf8RuneStart() int {
	if !utf8.RuneStart(0x41) {
		return 1
	}
	if !utf8.RuneStart(0xE2) {
		return 2
	}
	if utf8.RuneStart(0x80) {
		return 3
	}
	if !utf8.RuneStart(0xC0) {
		return 4
	}
	if !utf8.RuneStart(0xFF) {
		return 5
	}
	return 0
}

func TestUtf8Valid() int {
	if !utf8.Valid([]byte("A€z")) {
		return 1
	}
	if utf8.Valid([]byte{0xFF}) {
		return 2
	}
	if utf8.Valid([]byte{0xE2}) {
		return 3
	}
	if !utf8.Valid([]byte{}) {
		return 4
	}
	return 0
}

func TestUtf8ValidRune() int {
	if !utf8.ValidRune('A') {
		return 1
	}
	if !utf8.ValidRune(utf8.MaxRune) {
		return 2
	}
	if utf8.ValidRune(utf8.MaxRune + 1) {
		return 3
	}
	if utf8.ValidRune(0xD800) {
		return 4
	}
	if utf8.ValidRune(-1) {
		return 5
	}
	return 0
}

func TestUtf8ValidString() int {
	if !utf8.ValidString("A€z") {
		return 1
	}
	if utf8.ValidString("A\xFFz") {
		return 2
	}
	if !utf8.ValidString("") {
		return 3
	}
	return 0
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("TestUtf8Constants", TestUtf8Constants)
	runTest("TestUtf8EncodeRune", TestUtf8EncodeRune)
	runTest("TestUtf8DecodeRune", TestUtf8DecodeRune)
	runTest("TestUtf8DecodeRuneInString", TestUtf8DecodeRuneInString)
	runTest("TestUtf8DecodeLastRune", TestUtf8DecodeLastRune)
	runTest("TestUtf8DecodeLastRuneInString", TestUtf8DecodeLastRuneInString)
	runTest("TestUtf8AppendRune", TestUtf8AppendRune)
	runTest("TestUtf8FullRune", TestUtf8FullRune)
	runTest("TestUtf8FullRuneInString", TestUtf8FullRuneInString)
	runTest("TestUtf8RuneCount", TestUtf8RuneCount)
	runTest("TestUtf8RuneCountInString", TestUtf8RuneCountInString)
	runTest("TestUtf8RuneLen", TestUtf8RuneLen)
	runTest("TestUtf8RuneStart", TestUtf8RuneStart)
	runTest("TestUtf8Valid", TestUtf8Valid)
	runTest("TestUtf8ValidRune", TestUtf8ValidRune)
	runTest("TestUtf8ValidString", TestUtf8ValidString)
}
