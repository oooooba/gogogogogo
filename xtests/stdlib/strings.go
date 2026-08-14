package main

import "strings"

func main() {
	bad := 0
	chk := func(name string, got, want string) {
		if got != want {
			println("FAIL " + name + ": got=" + got + " want=" + want)
			bad++
		}
	}
	chki := func(name string, got, want int) {
		if got != want {
			println("FAIL " + name)
			bad++
		}
	}
	chkb := func(name string, got, want bool) {
		if got != want {
			println("FAIL " + name)
			bad++
		}
	}

	chk("ToUpper", strings.ToUpper("Hello, 世界"), "HELLO, 世界")
	chk("ToUpperAll", strings.ToUpper("mixed CASE 123"), "MIXED CASE 123")
	chk("ToLower", strings.ToLower("Hello, 世界"), "hello, 世界")
	chk("ToTitle", strings.ToTitle("loud noises"), "LOUD NOISES")

	chki("Index", strings.Index("hello", "ll"), 2)
	chki("IndexNo", strings.Index("hello", "xyz"), -1)
	chki("IndexEmpty", strings.Index("hello", ""), 0)
	chki("IndexEmptyInEmpty", strings.Index("", ""), 0)
	chki("IndexByte", strings.IndexByte("hello", 'l'), 2)
	chki("IndexByteNo", strings.IndexByte("hello", 'z'), -1)
	chki("IndexRune", strings.IndexRune("héllo", 'é'), 1)
	chki("IndexRuneNo", strings.IndexRune("hello", 'z'), -1)
	chki("IndexAny", strings.IndexAny("hello", "ol"), 2)
	chki("IndexAnyNo", strings.IndexAny("hello", "xyz"), -1)
	chki("IndexFunc", strings.IndexFunc("hello", func(r rune) bool { return r == 'l' }), 2)
	chki("IndexFuncNo", strings.IndexFunc("hello", func(r rune) bool { return r == 'z' }), -1)
	chki("LastIndex", strings.LastIndex("hello hello", "lo"), 8)
	chki("LastIndexNo", strings.LastIndex("hello", "xyz"), -1)
	chki("LastIndexEmpty", strings.LastIndex("hello", ""), 5)
	chki("LastIndexByte", strings.LastIndexByte("hello hello", 'l'), 9)
	chki("LastIndexAny", strings.LastIndexAny("hello", "lo"), 4)
	chki("LastIndexFunc", strings.LastIndexFunc("hello", func(r rune) bool { return r == 'l' }), 3)

	chki("Count", strings.Count("banana", "ana"), 1)
	chki("CountByte", strings.Count("banana", "a"), 3)
	chki("CountNone", strings.Count("hello", "x"), 0)
	chki("CountEmpty", strings.Count("hello", ""), 6)
	chki("CountEmptyEmpty", strings.Count("", ""), 1)

	chkb("HasPrefix", strings.HasPrefix("hello world", "hello"), true)
	chkb("HasPrefixNo", strings.HasPrefix("hello world", "world"), false)
	chkb("HasPrefixEmpty", strings.HasPrefix("", ""), true)
	chkb("HasSuffix", strings.HasSuffix("hello world", "world"), true)
	chkb("HasSuffixNo", strings.HasSuffix("hello world", "hello"), false)

	chkb("Contains", strings.Contains("hello", "ell"), true)
	chkb("ContainsNo", strings.Contains("hello", "xyz"), false)
	chkb("ContainsEmpty", strings.Contains("hello", ""), true)
	chkb("ContainsAny", strings.ContainsAny("hello", "xyz"), false)
	chkb("ContainsAny2", strings.ContainsAny("hello", "le"), true)
	chkb("ContainsRune", strings.ContainsRune("hello", 'l'), true)
	chkb("ContainsRuneNo", strings.ContainsRune("hello", 'z'), false)

	chk("Join", strings.Join([]string{"a", "b", "c"}, "-"), "a-b-c")
	chk("JoinEmpty", strings.Join(nil, ","), "")
	chk("SplitLast", strings.Split("a,b,c", ",")[2], "c")
	chk("SplitNoSep", strings.Split("abc", ",")[0], "abc")
	chk("SplitEmpty", strings.Split("", ",")[0], "")
	chki("SplitLen", len(strings.Split("a,b,c", ",")), 3)
	chk("SplitN", strings.SplitN("a,b,c", ",", 2)[1], "b,c")
	chk("SplitAfter", strings.SplitAfter("a,b,c", ",")[0], "a,")
	chki("SplitSeqCount", func() (n int) {
		for range strings.SplitSeq("a,b,c", ",") {
			n++
		}
		return
	}(), 3)

	chk("Repeat", strings.Repeat("ab", 3), "ababab")
	chk("RepeatEmpty", strings.Repeat("ab", 0), "")
	chk("RepeatEmptySep", strings.Repeat("", 3), "")
	chk("Replace", strings.Replace("aaa", "a", "b", 2), "bbaa")
	chk("ReplaceAll", strings.ReplaceAll("aaa", "a", "b"), "bbb")
	chk("ReplaceNo", strings.Replace("hello", "z", "x", -1), "hello")

	chk("Trim", strings.Trim("  hi  ", " "), "hi")
	chk("TrimLeft", strings.TrimLeft("xxhi", "x"), "hi")
	chk("TrimRight", strings.TrimRight("hixx", "x"), "hi")
	chk("TrimSpace", strings.TrimSpace(" \t\n hi \n"), "hi")
	chk("TrimPrefix", strings.TrimPrefix("helloworld", "hello"), "world")
	chk("TrimSuffix", strings.TrimSuffix("helloworld", "world"), "hello")
	chk("TrimFunc", strings.TrimFunc("xxhello", func(r rune) bool { return r == 'x' }), "hello")
	chk("TrimPrefixNo", strings.TrimPrefix("hello", "xyz"), "hello")

	chk("FieldsLast", strings.Fields("  a b  c ")[2], "c")
	chki("FieldsLen", len(strings.Fields("  a b  c ")), 3)
	chk("FieldsFunc", strings.FieldsFunc("a1b22c", func(r rune) bool { return r >= '0' && r <= '9' })[1], "b")
	chki("FieldsSeqCount", func() (n int) {
		for range strings.FieldsSeq("  a  b ") {
			n++
		}
		return
	}(), 2)

	chki("CompareLess", strings.Compare("a", "b"), -1)
	chki("CompareGreater", strings.Compare("b", "a"), 1)
	chki("CompareEqual", strings.Compare("ab", "ab"), 0)
	chkb("EqualFold", strings.EqualFold("Go", "gO"), true)
	chkb("EqualFoldNo", strings.EqualFold("Go", "Ga"), false)

	chk("Map", strings.Map(func(r rune) rune { return r + 1 }, "abc"), "bcd")
	chk("MapDel", strings.Map(func(r rune) rune { return -1 }, "abc"), "")
	chk("MapIdentity", strings.Map(func(r rune) rune { return r }, "hello"), "hello")

	chk("ToValidUTF8", strings.ToValidUTF8("a\xffb", "?"), "a?b")
	chk("ToValidUTF8Ok", strings.ToValidUTF8("héllo", "?"), "héllo")

	var b strings.Builder
	b.WriteString("foo")
	b.WriteByte('-')
	b.WriteRune('x')
	chk("BuilderString", b.String(), "foo-x")
	chki("BuilderLen", b.Len(), 5)
	chkb("BuilderCap", b.Cap() >= b.Len(), true)
	b.Reset()
	chki("BuilderResetLen", b.Len(), 0)
	chk("BuilderResetString", b.String(), "")
	b.Grow(10)
	b.WriteString("a")
	chk("BuilderGrow", b.String(), "a")

	r := strings.NewReplacer("old", "new", "a", "b")
	chk("Replacer", r.Replace("old a"), "new b")
	chk("ReplacerAll", r.Replace("a old"), "b new")

	println("bad:", bad)
}
