package main

import "bytes"

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

	chkb("ContainsHello", bytes.Contains([]byte("hello world"), []byte("lo wo")), true)
	chkb("ContainsNo", bytes.Contains([]byte("hello"), []byte("xyz")), false)
	chkb("ContainsEmpty", bytes.Contains([]byte("hello"), []byte{}), true)
	chkb("ContainsNil", bytes.Contains([]byte{}, nil), true)
	chkb("ContainsAny1", bytes.ContainsAny([]byte("hello"), "xyz"), false)
	chkb("ContainsAny2", bytes.ContainsAny([]byte("hello"), "le"), true)
	chkb("ContainsRune", bytes.ContainsRune([]byte("hello"), 'l'), true)
	chkb("ContainsRuneJp", bytes.ContainsRune([]byte("日本語"), '本'), true)
	chkb("ContainsFunc", bytes.ContainsFunc([]byte("hello"), func(r rune) bool { return r == 'o' }), true)
	chkb("ContainsFuncNo", bytes.ContainsFunc([]byte("hello"), func(r rune) bool { return r == 'z' }), false)

	chki("CountSub", bytes.Count([]byte("banana"), []byte("ana")), 1)
	chki("CountByte", bytes.Count([]byte("banana"), []byte("a")), 3)
	chki("CountNone", bytes.Count([]byte("hello"), []byte("z")), 0)
	chki("CountEmpty", bytes.Count([]byte("hello"), []byte{}), 6)
	chki("CountEmptyEmpty", bytes.Count([]byte{}, []byte{}), 1)

	chkb("HasPrefix", bytes.HasPrefix([]byte("hello world"), []byte("hello")), true)
	chkb("HasPrefixNo", bytes.HasPrefix([]byte("hello world"), []byte("world")), false)
	chkb("HasSuffix", bytes.HasSuffix([]byte("hello world"), []byte("world")), true)
	chkb("HasSuffixNo", bytes.HasSuffix([]byte("hello world"), []byte("hello")), false)

	chki("IndexSub", bytes.Index([]byte("banana"), []byte("nan")), 2)
	chki("IndexSubNo", bytes.Index([]byte("banana"), []byte("xyz")), -1)
	chki("IndexByte", bytes.IndexByte([]byte("hello"), 'l'), 2)
	chki("IndexByteNo", bytes.IndexByte([]byte("hello"), 'z'), -1)
	chki("IndexAny", bytes.IndexAny([]byte("hello"), "ol"), 2)
	chki("IndexAnyNo", bytes.IndexAny([]byte("hello"), "xyz"), -1)
	chki("IndexRune", bytes.IndexRune([]byte("hello"), 'e'), 1)
	chki("IndexRuneJp", bytes.IndexRune([]byte("日本語"), '本'), 3)
	chki("IndexRuneNo", bytes.IndexRune([]byte("hello"), 'z'), -1)
	chki("IndexFunc", bytes.IndexFunc([]byte("hello"), func(r rune) bool { return r == 'l' }), 2)
	chki("IndexFuncNo", bytes.IndexFunc([]byte("hello"), func(r rune) bool { return r == 'z' }), -1)
	chki("LastIndexSub", bytes.LastIndex([]byte("banana"), []byte("an")), 3)
	chki("LastIndexSubNo", bytes.LastIndex([]byte("banana"), []byte("xyz")), -1)
	chki("LastIndexByte", bytes.LastIndexByte([]byte("banana"), 'a'), 5)
	chki("LastIndexByteNo", bytes.LastIndexByte([]byte("banana"), 'z'), -1)
	chki("LastIndexAny", bytes.LastIndexAny([]byte("banana"), "ab"), 5)
	chki("LastIndexAnyNo", bytes.LastIndexAny([]byte("banana"), "xz"), -1)
	chki("LastIndexFunc", bytes.LastIndexFunc([]byte("hello"), func(r rune) bool { return r == 'l' }), 3)
	chki("LastIndexFuncNo", bytes.LastIndexFunc([]byte("hello"), func(r rune) bool { return r == 'z' }), -1)

	chkb("EqualYes", bytes.Equal([]byte("abc"), []byte("abc")), true)
	chkb("EqualNo", bytes.Equal([]byte("abc"), []byte("abd")), false)
	chkb("EqualLenDiff", bytes.Equal([]byte("abc"), []byte("abcd")), false)
	chkb("EqualEmpty", bytes.Equal([]byte{}, []byte{}), true)
	chkb("EqualFoldYes", bytes.EqualFold([]byte("Hello"), []byte("hELLO")), true)
	chkb("EqualFoldNo", bytes.EqualFold([]byte("Hello"), []byte("hELLa")), false)

	chki("CompareEq", bytes.Compare([]byte("abc"), []byte("abc")), 0)
	chki("CompareLess", bytes.Compare([]byte("abc"), []byte("abd")), -1)
	chki("CompareMore", bytes.Compare([]byte("abd"), []byte("abc")), 1)
	chki("ComparePrefix", bytes.Compare([]byte("ab"), []byte("abc")), -1)
	chki("CompareEmpty", bytes.Compare([]byte{}, []byte{}), 0)

	joined := bytes.Join([][]byte{[]byte("a"), []byte("b"), []byte("c")}, []byte("-"))
	chk("Join", string(joined), "a-b-c")
	joinedEmpty := bytes.Join([][]byte{[]byte("a"), []byte("b")}, []byte{})
	chk("JoinEmptySep", string(joinedEmpty), "ab")

	parts := bytes.Split([]byte("a,b,c"), []byte(","))
	chki("SplitLen", len(parts), 3)
	chk("Split0", string(parts[0]), "a")
	chk("Split2", string(parts[2]), "c")
	partsNoSep := bytes.Split([]byte("abc"), []byte(","))
	chki("SplitNoSep", len(partsNoSep), 1)
	partsN := bytes.SplitN([]byte("a,b,c,d"), []byte(","), 3)
	chki("SplitNLen", len(partsN), 3)
	chk("SplitN2", string(partsN[2]), "c,d")
	after := bytes.SplitAfter([]byte("a,b,c"), []byte(","))
	chki("SplitAfterLen", len(after), 3)
	chk("SplitAfter0", string(after[0]), "a,")
	chk("SplitAfter2", string(after[2]), "c")
	afterN := bytes.SplitAfterN([]byte("a,b,c,d"), []byte(","), 3)
	chki("SplitAfterNLen", len(afterN), 3)
	chk("SplitAfterN2", string(afterN[2]), "c,d")

	fields := bytes.Fields([]byte("  hello   world  foo "))
	chki("FieldsLen", len(fields), 3)
	chk("Fields0", string(fields[0]), "hello")
	chk("Fields2", string(fields[2]), "foo")
	fieldsEmpty := bytes.Fields([]byte("   \t\n "))
	chki("FieldsEmpty", len(fieldsEmpty), 0)
	ff := bytes.FieldsFunc([]byte("a1b2c3"), func(r rune) bool { return r >= '0' && r <= '9' })
	chki("FieldsFuncLen", len(ff), 3)
	chk("FieldsFunc1", string(ff[1]), "b")

	chk("Repeat3", string(bytes.Repeat([]byte("ab"), 3)), "ababab")
	chk("Repeat0", string(bytes.Repeat([]byte("ab"), 0)), "")

	chk("ReplaceN", string(bytes.Replace([]byte("aaa"), []byte("a"), []byte("b"), 2)), "bba")
	chk("Replace0", string(bytes.Replace([]byte("aaa"), []byte("a"), []byte("b"), 0)), "aaa")
	chk("ReplaceAll", string(bytes.ReplaceAll([]byte("banana"), []byte("an"), []byte("AN"))), "bANANa")

	chk("Map", string(bytes.Map(func(r rune) rune {
		if r == 'a' {
			return 'A'
		}
		return r
	}, []byte("banana"))), "bAnAnA")

	chk("ToUpper", string(bytes.ToUpper([]byte("Hello World"))), "HELLO WORLD")
	chk("ToLower", string(bytes.ToLower([]byte("Hello World"))), "hello world")
	chk("ToTitle", string(bytes.ToTitle([]byte("hello"))), "HELLO")
	chk("ToUpperUnicode", string(bytes.ToUpper([]byte("héllo"))), "HÉLLO")
	chk("ToLowerUnicode", string(bytes.ToLower([]byte("HÉLLO"))), "héllo")

	chk("TrimSpace", string(bytes.TrimSpace([]byte("  \tfoo\n "))), "foo")
	chk("TrimSpaceEmpty", string(bytes.TrimSpace([]byte("   "))), "")
	chk("Trim", string(bytes.Trim([]byte("xxabcx"), "x")), "abc")
	chk("TrimPrefix", string(bytes.TrimPrefix([]byte("foobar"), []byte("foo"))), "bar")
	chk("TrimPrefixNo", string(bytes.TrimPrefix([]byte("foobar"), []byte("bar"))), "foobar")
	chk("TrimSuffix", string(bytes.TrimSuffix([]byte("foobar"), []byte("bar"))), "foo")
	chk("TrimLeft", string(bytes.TrimLeft([]byte("xxabcxx"), "x")), "abcxx")
	chk("TrimRight", string(bytes.TrimRight([]byte("xxabcxx"), "x")), "xxabc")
	chk("TrimFunc", string(bytes.TrimFunc([]byte("123abc321"), func(r rune) bool { return r >= '0' && r <= '9' })), "abc")
	chk("TrimLeftFunc", string(bytes.TrimLeftFunc([]byte("123abc"), func(r rune) bool { return r >= '0' && r <= '9' })), "abc")
	chk("TrimRightFunc", string(bytes.TrimRightFunc([]byte("abc321"), func(r rune) bool { return r >= '0' && r <= '9' })), "abc")

	chk("Clone", string(bytes.Clone([]byte("hello"))), "hello")
	chk("CloneEmpty", string(bytes.Clone([]byte{})), "")

	cutBefore, cutAfter, cutFound := bytes.Cut([]byte("a=b=c"), []byte("="))
	chkb("CutFound", cutFound, true)
	chk("CutBefore", string(cutBefore), "a")
	chk("CutAfter", string(cutAfter), "b=c")
	_, _, cutNo := bytes.Cut([]byte("abc"), []byte("="))
	chkb("CutNo", cutNo, false)
	prefAfter, prefFound := bytes.CutPrefix([]byte("prefix-xyz"), []byte("prefix-"))
	chkb("CutPrefixFound", prefFound, true)
	chk("CutPrefixAfter", string(prefAfter), "xyz")
	_, prefNo := bytes.CutPrefix([]byte("xyz"), []byte("prefix-"))
	chkb("CutPrefixNo", prefNo, false)
	sufBefore, sufFound := bytes.CutSuffix([]byte("file.txt"), []byte(".txt"))
	chkb("CutSuffixFound", sufFound, true)
	chk("CutSuffixBefore", string(sufBefore), "file")
	_, sufNo := bytes.CutSuffix([]byte("file.txt"), []byte(".zip"))
	chkb("CutSuffixNo", sufNo, false)

	var buf bytes.Buffer
	buf.Write([]byte("hello"))
	buf.WriteString(" world")
	buf.WriteByte('!')
	chk("BufString", buf.String(), "hello world!")
	chki("BufLen", buf.Len(), 12)
	chkb("BufCapGe", buf.Cap() >= buf.Len(), true)

	readBuf := make([]byte, 5)
	readN, _ := buf.Read(readBuf)
	chki("BufReadN", readN, 5)
	chk("BufRead", string(readBuf), "hello")
	chk("BufRest", buf.String(), " world!")
	readB, _ := buf.ReadByte()
	chki("BufReadByte", int(readB), ' ')
	chk("BufRest2", buf.String(), "world!")
	readR, readSize, _ := buf.ReadRune()
	chki("BufReadRune", int(readR), 'w')
	chki("BufReadRuneSize", readSize, 1)
	next := buf.Next(3)
	chk("BufNext", string(next), "orl")
	chk("BufRest3", buf.String(), "d!")

	var buf2 bytes.Buffer
	buf2.WriteRune('日')
	chk("WriteRune", buf2.String(), "日")
	r2, size2, _ := buf2.ReadRune()
	chki("Buf2Rune", int(r2), '日')
	chki("Buf2RuneSize", size2, 3)

	var buf6 bytes.Buffer
	buf6.WriteString("a,b,c")
	line, _ := buf6.ReadString(',')
	chk("ReadString", line, "a,")
	chk("ReadStringRest", buf6.String(), "b,c")

	var buf5 bytes.Buffer
	buf5.WriteString("hello world")
	buf5.Truncate(5)
	chk("Truncate", buf5.String(), "hello")
	buf5.Reset()
	chki("ResetLen", buf5.Len(), 0)

	var buf4 bytes.Buffer
	buf4.Grow(100)
	chkb("GrowCap", buf4.Cap() >= 100, true)

	var src bytes.Buffer
	src.WriteString("hello from src")
	var dst bytes.Buffer
	writeToN, _ := src.WriteTo(&dst)
	chki("WriteToN", int(writeToN), len("hello from src"))
	chk("WriteToDst", dst.String(), "hello from src")
	chki("WriteToSrcEmpty", src.Len(), 0)

	var src2 bytes.Buffer
	src2.WriteString("read from src2")
	var dst2 bytes.Buffer
	readFromN, _ := dst2.ReadFrom(&src2)
	chki("ReadFromN", int(readFromN), len("read from src2"))
	chk("ReadFromDst", dst2.String(), "read from src2")

	r := bytes.NewReader([]byte("hello reader"))
	chki("ReaderLen", r.Len(), len("hello reader"))
	chki("ReaderSize", int(r.Size()), len("hello reader"))
	readerBuf := make([]byte, 5)
	readerN, _ := r.Read(readerBuf)
	chki("ReaderReadN", readerN, 5)
	chk("ReaderRead", string(readerBuf), "hello")
	chki("ReaderLenAfter", r.Len(), len(" reader"))

	r.Seek(0, 0)
	readB2, _ := r.ReadByte()
	chki("ReaderReadByte", int(readB2), 'h')
	r.Seek(6, 0)
	rr, rsz, _ := r.ReadRune()
	chki("ReaderRune", int(rr), 'r')
	chki("ReaderRuneSize", rsz, 1)
	r.UnreadRune()
	rr2, _, _ := r.ReadRune()
	chki("ReaderUnreadRune", int(rr2), 'r')

	atBuf := make([]byte, 5)
	atN, _ := r.ReadAt(atBuf, 0)
	chki("ReadAtN", atN, 5)
	chk("ReadAt", string(atBuf), "hello")

	var rb bytes.Buffer
	var rd = bytes.NewReader([]byte("rd writer"))
	rd.WriteTo(&rb)
	chk("ReaderWriteTo", rb.String(), "rd writer")

	sc := 0
	for part := range bytes.SplitSeq([]byte("a,b,c"), []byte(",")) {
		sc++
		_ = part
	}
	chki("SplitSeqCount", sc, 3)
	lc := 0
	for line := range bytes.Lines([]byte("a\nb\nc")) {
		lc++
		_ = line
	}
	chki("LinesCount", lc, 3)
	fc := 0
	for field := range bytes.FieldsSeq([]byte("  a  b ")) {
		fc++
		_ = field
	}
	chki("FieldsSeqCount", fc, 2)

	println("bad:", bad)
}
