package main

import "path"

func TestClean() int {
	cases := []string{
		"", "a", "a/b", "a/b/", "a//b", "a/./b", "a/../b", "a/./../b",
		"/../a", "a/./.././b", "/", "//", "/a/b/c", "a/b/c/../../d", "a/b/./../../c/",
		".", "..", "./", "../", "../..", "../../a", "a/..", "./a/../b/./c",
		"///", "a/.../b", "a/....//b",
	}
	want := []string{
		".", "a", "a/b", "a/b", "a/b", "a/b", "b", "b",
		"/a", "b", "/", "/", "/a/b/c", "a/d", "c",
		".", "..", ".", "..", "../..", "../../a", ".", "b/c",
		"/", "a/.../b", "a/..../b",
	}
	for i := range cases {
		got := path.Clean(cases[i])
		if got != want[i] {
			println("Clean(", cases[i], ") = ", got, " want ", want[i])
			return i + 1
		}
	}
	return 1
}

func TestSplit() int {
	cases := []struct{ in, dir, file string }{
		{"a/b", "a/", "b"},
		{"a/b/", "a/b/", ""},
		{"a", "", "a"},
		{"", "", ""},
		{"/", "/", ""},
		{"abc/", "abc/", ""},
		{"a/b/c", "a/b/", "c"},
	}
	for i, c := range cases {
		d, f := path.Split(c.in)
		if d != c.dir || f != c.file {
			println("Split(", c.in, ") = (", d, ",", f, ") want (", c.dir, ",", c.file, ")")
			return i + 1
		}
	}
	return 2
}

func TestJoin() int {
	cases := []struct {
		elems []string
		want  string
	}{
		{[]string{}, ""},
		{[]string{""}, ""},
		{[]string{"a"}, "a"},
		{[]string{"a", "b"}, "a/b"},
		{[]string{"a", ""}, "a"},
		{[]string{"", "b"}, "b"},
		{[]string{"", ""}, ""},
		{[]string{"a", "b", "c"}, "a/b/c"},
		{[]string{"a/", "b"}, "a/b"},
		{[]string{"/a", "b"}, "/a/b"},
		{[]string{"a", "../b"}, "b"},
		{[]string{"a/../b", ""}, "b"},
		{[]string{"a//b", "c"}, "a/b/c"},
	}
	for i, c := range cases {
		got := path.Join(c.elems...)
		if got != c.want {
			println("Join", i, "=", got, " want ", c.want)
			return i + 1
		}
	}
	return 3
}

func TestExt() int {
	cases := []struct{ in, want string }{
		{"", ""},
		{".", "."},
		{"..", "."},
		{"abc", ""},
		{"abc.go", ".go"},
		{"a/b/c.go", ".go"},
		{"a/b.c/c", ""},
		{"a/b.c/c.go", ".go"},
		{"a.b.c", ".c"},
		{"a.", "."},
		{"a///b.c.d", ".d"},
		{"a/b/", ""},
		{"..a", ".a"},
	}
	for i, c := range cases {
		got := path.Ext(c.in)
		if got != c.want {
			println("Ext(", c.in, ") = ", got, " want ", c.want)
			return i + 1
		}
	}
	return 4
}

func TestBase() int {
	cases := []struct{ in, want string }{
		{"", "."},
		{".", "."},
		{"..", ".."},
		{"/", "/"},
		{"a", "a"},
		{"a/b", "b"},
		{"a//b", "b"},
		{"a/b/", "b"},
		{"abc/", "abc"},
		{"/a", "a"},
		{"/a/b/c", "c"},
		{"../..", ".."},
		{"/../../", ".."},
	}
	for i, c := range cases {
		got := path.Base(c.in)
		if got != c.want {
			println("Base(", c.in, ") = ", got, " want ", c.want)
			return i + 1
		}
	}
	return 5
}

func TestDir() int {
	cases := []struct{ in, want string }{
		{"", "."},
		{".", "."},
		{"..", "."},
		{"/", "/"},
		{"abc", "."},
		{"abc/", "abc"},
		{"a/b", "a"},
		{"a/b/", "a/b"},
		{"a//b", "a"},
		{"/a", "/"},
		{"/a/b", "/a"},
		{"/a/b/c", "/a/b"},
		{"a/b.c/c", "a/b.c"},
	}
	for i, c := range cases {
		got := path.Dir(c.in)
		if got != c.want {
			println("Dir(", c.in, ") = ", got, " want ", c.want)
			return i + 1
		}
	}
	return 6
}

func TestIsAbs() int {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"/", true},
		{"/abc/d", true},
		{"abc/d", false},
		{"a", false},
	}
	for i, c := range cases {
		got := path.IsAbs(c.in)
		if got != c.want {
			println("IsAbs(", c.in, ") = ", got, " want ", c.want)
			return i + 1
		}
	}
	return 7
}

func TestMatch() int {
	cases := []struct {
		pattern, name string
		matched       bool
	}{
		{"abc", "abc", true},
		{"*", "abc", true},
		{"*c", "abc", true},
		{"a*", "a", true},
		{"a*", "abc", true},
		{"a*", "ab/c", false},
		{"a\\*b", "a*b", true},
		{"a\\*b", "ab", false},
		{"a?b", "aab", true},
		{"a?b", "ab", false},
		{"a[bcd]", "ab", true},
		{"a[bcd]", "ae", false},
		{"a[^bcd]", "ae", true},
		{"a[^bcd]", "ab", false},
		{"*", "", true},
		{"", "", true},
		{"", "a", false},
		{"a/*/b", "a/x/b", true},
		{"a/*/b", "a/x/y/b", false},
		{"**", "a", true},
		{"a/**/d", "a/b/c/d", false},
	}
	for i, c := range cases {
		got, err := path.Match(c.pattern, c.name)
		if got != c.matched || err != nil {
			println("Match(", c.pattern, ",", c.name, ") = ", got, " want ", c.matched)
			return i + 1
		}
	}
	return 8
}

func TestMatchError() int {
	cases := []struct {
		pattern, name string
	}{
		{"a[", "a"},
		{"a[b", "ab"},
		{"\\", "a"},
		{"a\\", "a"},
	}
	for i, c := range cases {
		_, err := path.Match(c.pattern, c.name)
		if err == nil {
			println("Match(", c.pattern, ",", c.name, ") want error")
			return i + 1
		}
		if err.Error() != "syntax error in pattern" {
			println("Match(", c.pattern, ",", c.name, ") err = ", err.Error(), " want syntax error in pattern")
			return i + 2
		}
	}
	return 9
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("TestClean", TestClean)
	runTest("TestSplit", TestSplit)
	runTest("TestJoin", TestJoin)
	runTest("TestExt", TestExt)
	runTest("TestBase", TestBase)
	runTest("TestDir", TestDir)
	runTest("TestIsAbs", TestIsAbs)
	runTest("TestMatch", TestMatch)
	runTest("TestMatchError", TestMatchError)
}
