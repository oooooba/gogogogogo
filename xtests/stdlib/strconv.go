package main

import "strconv"

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

	chk("Itoa0", strconv.Itoa(0), "0")
	chk("Itoa42", strconv.Itoa(42), "42")
	chk("ItoaNeg", strconv.Itoa(-987654), "-987654")
	chk("ItoaMax", strconv.Itoa(9223372036854775807), "9223372036854775807")
	chk("ItoaMin", strconv.Itoa(-9223372036854775808), "-9223372036854775808")

	n, err := strconv.Atoi("0")
	chki("Atoi0", n, 0)
	chkb("Atoi0err", err == nil, true)
	n, err = strconv.Atoi("-12345")
	chki("AtoiNeg", n, -12345)
	chkb("AtoiNegErr", err == nil, true)
	n, err = strconv.Atoi("9223372036854775807")
	chki("AtoiMax", n, 9223372036854775807)
	chkb("AtoiMaxErr", err == nil, true)
	n, err = strconv.Atoi("abc")
	chkb("AtoiBadErr", err != nil, true)
	n, err = strconv.Atoi("")
	chkb("AtoiEmptyErr", err != nil, true)
	n, err = strconv.Atoi("99999999999999999999999")
	chkb("AtoiOverflowErr", err != nil, true)

	chk("FmtInt10", strconv.FormatInt(-123, 10), "-123")
	chk("FmtInt16", strconv.FormatInt(255, 16), "ff")
	chk("FmtInt16U", strconv.FormatInt(-255, 16), "-ff")
	chk("FmtInt2", strconv.FormatInt(10, 2), "1010")
	chk("FmtInt8", strconv.FormatInt(64, 8), "100")
	chk("FmtInt36", strconv.FormatInt(123456789, 36), "21i3v9")
	chk("FmtIntNeg36", strconv.FormatInt(-123456789, 36), "-21i3v9")
	chk("FmtUint16", strconv.FormatUint(0xdeadbeef, 16), "deadbeef")
	chk("FmtUint2", strconv.FormatUint(0xff, 2), "11111111")
	chk("FmtUint10", strconv.FormatUint(18446744073709551615, 10), "18446744073709551615")

	v, err := strconv.ParseInt("ff", 16, 64)
	chki("ParseIntFF", int(v), 255)
	chkb("ParseIntFFErr", err == nil, true)
	v, err = strconv.ParseInt("-1010", 2, 64)
	chki("ParseIntBin", int(v), -10)
	chkb("ParseIntBinErr", err == nil, true)
	v, err = strconv.ParseInt("0x1f", 0, 64)
	chki("ParseIntAutoHex", int(v), 31)
	chkb("ParseIntAutoHexErr", err == nil, true)
	v, err = strconv.ParseInt("0b101", 0, 64)
	chki("ParseIntAutoBin", int(v), 5)
	chkb("ParseIntAutoBinErr", err == nil, true)
	v, err = strconv.ParseInt("z", 36, 64)
	chki("ParseIntZ36", int(v), 35)
	chkb("ParseIntZ36Err", err == nil, true)
	v, err = strconv.ParseInt("abc", 10, 64)
	chkb("ParseIntBadErr", err != nil, true)
	v, err = strconv.ParseInt("200", 2, 64)
	chkb("ParseIntInvalidDigitErr", err != nil, true)

	uv, err := strconv.ParseUint("deadbeef", 16, 64)
	chkb("ParseUintHex", uv == 0xdeadbeef, true)
	chkb("ParseUintHexErr", err == nil, true)
	uv, err = strconv.ParseUint("18446744073709551615", 10, 64)
	chkb("ParseUintMax", uv == 18446744073709551615, true)
	chkb("ParseUintMaxErr", err == nil, true)
	uv, err = strconv.ParseUint("-1", 10, 64)
	chkb("ParseUintNegErr", err != nil, true)

	chk("FmtFloatF", strconv.FormatFloat(3.14159, 'f', 2, 64), "3.14")
	chk("FmtFloatFneg", strconv.FormatFloat(-3.14159, 'f', 2, 64), "-3.14")
	chk("FmtFloatE", strconv.FormatFloat(1234.5, 'e', 3, 64), "1.234e+03")
	chk("FmtFloatG", strconv.FormatFloat(0.00001234, 'g', -1, 64), "1.234e-05")
	chk("FmtFloatGshort", strconv.FormatFloat(3.14159, 'g', -1, 64), "3.14159")
	chk("FmtFloat0", strconv.FormatFloat(0, 'f', -1, 64), "0")
	chk("FmtFloatIntVal", strconv.FormatFloat(100, 'f', -1, 64), "100")
	chk("FmtFloatExp", strconv.FormatFloat(100, 'e', -1, 64), "1e+02")
	chk("FmtFloat32", strconv.FormatFloat(3.5, 'f', -1, 32), "3.5")
	chk("FmtFloatX", strconv.FormatFloat(1.5, 'x', -1, 64), "0x1.8p+00")

	fv, err := strconv.ParseFloat("3.14159", 64)
	chkb("ParseFloatPi", fv == 3.14159, true)
	chkb("ParseFloatPiErr", err == nil, true)
	fv, err = strconv.ParseFloat("-1.5e3", 64)
	chkb("ParseFloatNeg", fv == -1500, true)
	chkb("ParseFloatNegErr", err == nil, true)
	fv, err = strconv.ParseFloat("1e-10", 64)
	chkb("ParseFloatSmall", fv == 1e-10, true)
	chkb("ParseFloatSmallErr", err == nil, true)
	fv, err = strconv.ParseFloat("abc", 64)
	chkb("ParseFloatBadErr", err != nil, true)

	chk("QuoteHello", strconv.Quote("hello"), `"hello"`)
	chk("QuoteSpace", strconv.Quote("a b"), `"a b"`)
	chk("QuoteTab", strconv.Quote("a\tb"), `"a\tb"`)
	chk("QuoteQuote", strconv.Quote(`"`), `"\""`)
	chk("QuoteBackslash", strconv.Quote(`\`), `"\\"`)
	chk("QuoteAscii", strconv.QuoteToASCII("日本語"), `"\u65e5\u672c\u8a9e"`)
	chk("QuoteRuneA", strconv.QuoteRune('A'), `'A'`)
	chk("QuoteRuneJp", strconv.QuoteRune('日'), `'日'`)
	chk("UnquoteHello", unquoteOr("`hello`"), "hello")
	chk("UnquoteEsc", unquoteOr(`"a\tb"`), "a\tb")
	chk("UnquoteHex", unquoteOr(`"\x41"`), "A")
	chk("UnquoteUnicode", unquoteOr(`"\u65e5"`), "日")

	chkb("CanBackquoteHello", strconv.CanBackquote("hello world"), true)
	chkb("CanBackquoteTab", strconv.CanBackquote("a\tb"), true)
	chkb("CanBackquoteBacktick", strconv.CanBackquote("a`b"), false)
	chkb("IsPrintA", strconv.IsPrint('A'), true)
	chkb("IsPrintSpace", strconv.IsPrint(' '), true)
	chkb("IsPrintTab", strconv.IsPrint('\t'), false)
	chkb("IsPrintJp", strconv.IsPrint('日'), true)

	buf := make([]byte, 0, 64)
	buf = strconv.AppendInt(buf, -42, 10)
	chk("AppendInt", string(buf), "-42")
	buf = buf[:0]
	buf = strconv.AppendUint(buf, 255, 16)
	chk("AppendUint", string(buf), "ff")
	buf = buf[:0]
	buf = strconv.AppendFloat(buf, 2.5, 'f', -1, 64)
	chk("AppendFloat", string(buf), "2.5")
	buf = buf[:0]
	buf = strconv.AppendQuote(buf, "a\nb")
	chk("AppendQuote", string(buf), `"a\nb"`)
	buf = buf[:0]
	buf = strconv.AppendQuoteRune(buf, '日')
	chk("AppendQuoteRune", string(buf), `'日'`)

	chki("IntSize", strconv.IntSize, 64)

	println("bad:", bad)
}

func unquoteOr(s string) string {
	r, err := strconv.Unquote(s)
	if err != nil {
		return "<err>"
	}
	return r
}
