package main

import (
	"regexp"
)

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

	ok, err := regexp.MatchString("h.*o", "hello")
	chkb("MatchHello", ok, true)
	chkb("MatchHelloErr", err == nil, true)
	ok, err = regexp.MatchString("^a+$", "aaab")
	chkb("MatchAnchoredNo", ok, false)
	chkb("MatchAnchoredErr", err == nil, true)
	ok, err = regexp.MatchString(`\d+`, "abc123")
	chkb("MatchDigits", ok, true)
	chkb("MatchDigitsErr", err == nil, true)
	ok, err = regexp.MatchString("a|b", "cat")
	chkb("MatchAlt", ok, true)
	chkb("MatchAltErr", err == nil, true)

	ok, err = regexp.Match(`a.c`, []byte("abc"))
	chkb("MatchBytes", ok, true)
	chkb("MatchBytesErr", err == nil, true)

	chk("QuoteMeta", regexp.QuoteMeta("a.b*c"), `a\.b\*c`)

	_, err = regexp.Compile("(")
	chkb("CompileInvalid", err != nil, true)
	_, err = regexp.Compile("a+")
	chkb("CompileValid", err == nil, true)

	re := regexp.MustCompile(`(\w+)=(\d+)`)
	chk("String", re.String(), `(\w+)=(\d+)`)
	chki("NumSubexp", re.NumSubexp(), 2)
	names := re.SubexpNames()
	chki("SubexpNamesLen", len(names), 3)
	chk("SubexpNames0", names[0], "")
	chk("SubexpNames1", names[1], "")
	chk("SubexpNames2", names[2], "")

	chkb("ReFullMatch", re.MatchString("x=1 y=22"), true)
	chkb("ReNoMatch", re.MatchString("nope"), false)
	chk("FindString", re.FindString("x=1 y=22"), "x=1")
	chk("FindStringNo", re.FindString("nope"), "")
	idx := re.FindStringIndex("x=1 y=22")
	chki("FindIndex0", idx[0], 0)
	chki("FindIndex1", idx[1], 3)
	if re.FindStringIndex("nope") != nil {
		println("FAIL FindIndexNoNil")
		bad++
	}

	all := re.FindAllString("x=1 y=22 z=333", -1)
	chki("FindAllLen", len(all), 3)
	chk("FindAll0", all[0], "x=1")
	chk("FindAll1", all[1], "y=22")
	chk("FindAll2", all[2], "z=333")
	chki("FindAllLLen", len(re.FindAllString("a=1 b=2 c=3", 2)), 2)
	chki("FindAllZero", len(re.FindAllString("a=1 b=2", 0)), 0)
	chki("FindAllNo", len(re.FindAllString("nope", -1)), 0)

	sub := re.FindStringSubmatch("x=42")
	chki("SubLen", len(sub), 3)
	chk("Sub0", sub[0], "x=42")
	chk("Sub1", sub[1], "x")
	chk("Sub2", sub[2], "42")
	si := re.FindStringSubmatchIndex("x=42")
	chki("SubIndex0", si[0], 0)
	chki("SubIndex1", si[1], 4)
	chki("SubIndex2", si[2], 0)
	chki("SubIndex3", si[3], 1)
	chki("SubIndex4", si[4], 2)
	chki("SubIndex5", si[5], 4)

	chk("ReplaceAll", re.ReplaceAllString("a=1 b=2", "[$2:$1]"), "[1:a] [2:b]")
	chk("ReplaceLiteral", re.ReplaceAllLiteralString("a=1", "X"), "X")
	chk("ReplaceFunc", re.ReplaceAllStringFunc("a=1 b=22", func(s string) string {
		return "$" + s
	}), "$a=1 $b=22")
	idx7 := re.FindStringSubmatchIndex("x=7")
	chk("Expand", string(re.ExpandString(nil, "($1)"+".($2)", "x=7", idx7)), "(x).(7)")

	parts := re.Split("a=1,b=2", -1)
	chki("SplitLen", len(parts), 2)
	chk("Split0", parts[0], "a")
	chk("Split1", parts[1], ",b=2")
	chk("SplitNo", re.Split("plain", -1)[0], "plain")
	chki("SplitTLen", len(re.Split("a=1,b=2,c=3", 2)), 2)
	chki("SplitNoSep", len(re.Split("plain", -1)), 1)

	re2 := regexp.MustCompile(`^a(b*)(c+)$`)
	chkb("Re2Full", re2.MatchString("abbccc"), true)
	chkb("Re2No", re2.MatchString("bxabbccc"), false)
	chki("Re2NumSubexp", re2.NumSubexp(), 2)
	sub2 := re2.FindStringSubmatch("abbccc")
	chk("Re2Group1", sub2[1], "bb")
	chk("Re2Group2", sub2[2], "ccc")

	re3 := regexp.MustCompile(`(?i)hello`)
	chkb("IgnoreCase", re3.MatchString("HeLLo"), true)
	chkb("IgnoreCaseNo", re3.MatchString("hxllo"), false)

	re4 := regexp.MustCompile(`\bfoo\b`)
	chkb("WordBoundary", re4.MatchString("a foo b"), true)
	chkb("WordBoundaryNo", re4.MatchString("afoob"), false)

	re5 := regexp.MustCompile(`a|ab`)
	chk("Leftmost", re5.FindString("ab"), "a")
	re5.Longest()
	chk("LeftmostLongest", re5.FindString("ab"), "ab")

	re6 := regexp.MustCompile(`[0-9]+`)
	chkb("ClassDigits", re6.MatchString("2024"), true)
	chkb("ClassDigitsNo", re6.MatchString("abc"), false)
	all6 := re6.FindAllString("7 x 88 y 999", -1)
	chki("ClassAllLen", len(all6), 3)
	chk("ClassAll0", all6[0], "7")
	chk("ClassAll1", all6[1], "88")
	chk("ClassAll2", all6[2], "999")

	re7 := regexp.MustCompile("^[[:alpha:]]+$")
	chkb("PosixClass", re7.MatchString("Words"), true)
	chkb("PosixClassNo", re7.MatchString("123"), false)

	println("bad:", bad)
}
