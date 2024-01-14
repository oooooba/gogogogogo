package main

func Test1() int {
	s := "a"
	return len(s)
}

func Test2() int {
	s1 := "a"
	s2 := "a"
	if s1 == s2 {
		return 2
	} else {
		return 0
	}
}

func Test3() int {
	s1 := "a"
	s2 := "b"
	if s1 == s2 {
		return 0
	} else {
		return 3
	}
}

func Test4() int {
	s1 := "a"
	s2 := "a"
	return func(s1, s2 string) int {
		if s1 == s2 {
			return 4
		} else {
			return 0
		}
	}(s1, s2)
}

func Test5() int {
	s1 := "a"
	s2 := "b"
	return func(s1, s2 string) int {
		if s1 == s2 {
			return 0
		} else {
			return 5
		}
	}(s1, s2)
}

func Test6() int {
	var s []string
	s = append(s, "a")
	if len(s) != 1 {
		return 0
	}
	if s[0] != "a" {
		return 1
	}
	s = append(s, "b")
	if len(s) != 2 {
		return 2
	}
	if s[1] != "b" {
		return 3
	}
	return 6
}

/*
func Test7() int {
	s := "a"
	ss := strings.Split(s, ".")
	if len(ss) != 1 {
		return 0
	}
	if ss[0] != "a" {
		return 1
	}
	return 7
}

func Test8() int {
	s := "ab_cde_f"
	ss := strings.Split(s, "_")
	if len(ss) != 3 {
		return 0
	}
	if ss[0] != "ab" {
		return 1
	}
	if ss[1] != "cde" {
		return 2
	}
	if ss[2] != "f" {
		return 3
	}
	return 8
}
*/

func Test9() int {
	s := "abc"
	t := "def"
	if s+t != "abcdef" {
		return 0
	}
	return 9
}

func Test10() int {
	n := 0x41
	s := string(n)
	if s != "A" {
		return 0
	}
	return 10
}

func Test11() int {
	s := "abc"
	t := "def"
	if (s + t)[0] != 'a' {
		return 0
	}
	if (s + t)[1] != 'b' {
		return 1
	}
	if (s + t)[2] != 'c' {
		return 2
	}
	if (s + t)[3] != 'd' {
		return 3
	}
	if (s + t)[4] != 'e' {
		return 4
	}
	if (s + t)[5] != 'f' {
		return 5
	}
	return 11
}

func Test12() int {
	s := "abcdef"
	if s[:3] != "abc" {
		return 0
	}
	if s[3:] != "def" {
		return 1
	}
	if s[1:5] != "bcde" {
		return 2
	}
	if s[:] != "abcdef" {
		return 3
	}
	return 12
}

func Test13() int {
	n := 'A'
	s := string(n)
	if s != "A" {
		return 0
	}
	return 13
}

func Test14() int {
	var v [1]byte
	v[0] = 'A'
	s := string(v[:])
	if s != "A" {
		return 0
	}
	return 14
}

func Test15() int {
	var v [0]byte
	s := string(v[:])
	if s != "" {
		return 0
	}
	return 15
}

func Test16() int {
	var v [1]rune
	v[0] = 'A'
	s := string(v[:])
	if s != "A" {
		return 0
	}
	return 16
}

func Test17() int {
	var v [0]rune
	s := string(v[:])
	if s != "" {
		return 0
	}
	return 17
}

func Test18() int {
	n := 0x1234
	s := string(n)
	if s != "\u1234" {
		return 0
	}
	return 18
}

func Test19() int {
	c := '\u1234'
	s := string(c)
	if s != "\u1234" {
		return 0
	}
	return 19
}

func Test20() int {
	var v [1]rune
	v[0] = '\u1234'
	s := string(v[:])
	if s != "\u1234" {
		return 0
	}
	return 20
}

func Test21() int {
	if "abcd"[1:3] != "bc" {
		return 0
	}
	if "xyz"[1] != 'y' {
		return 1
	}
	return 21
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
	/*
		runTest("Test7", Test7)
		runTest("Test8", Test8)
	*/
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
	runTest("Test20", Test20)
	runTest("Test21", Test21)
}
