package main

import "unicode"

func TestUnicodeConstants() int {
	if unicode.Version != "15.0.0" {
		return 1
	}
	if unicode.MaxRune != 0x10FFFF {
		return 2
	}
	if unicode.ReplacementChar != 0xFFFD {
		return 3
	}
	if unicode.MaxASCII != 127 {
		return 4
	}
	if unicode.MaxLatin1 != 255 {
		return 5
	}
	if unicode.MaxCase != 3 {
		return 6
	}
	if unicode.UpperCase != 0 || unicode.LowerCase != 1 || unicode.TitleCase != 2 {
		return 7
	}
	return 0
}

func TestUnicodeIsLetter() int {
	if !unicode.IsLetter('A') {
		return 1
	}
	if unicode.IsLetter('1') {
		return 2
	}
	if !unicode.IsLetter('\u00C0') {
		return 3
	}
	return 0
}

func TestUnicodeIsUpper() int {
	if !unicode.IsUpper('A') {
		return 1
	}
	if unicode.IsUpper('a') {
		return 2
	}
	if unicode.IsUpper('\u00DF') {
		return 3
	}
	if !unicode.IsUpper('\u1E9E') {
		return 4
	}
	return 0
}

func TestUnicodeIsLower() int {
	if !unicode.IsLower('a') {
		return 1
	}
	if unicode.IsLower('A') {
		return 2
	}
	return 0
}

func TestUnicodeIsTitle() int {
	if !unicode.IsTitle('\u01C5') {
		return 1
	}
	if unicode.IsTitle('A') {
		return 2
	}
	return 0
}

func TestUnicodeIsDigit() int {
	if !unicode.IsDigit('5') {
		return 1
	}
	if unicode.IsDigit('x') {
		return 2
	}
	if !unicode.IsDigit('\u0663') {
		return 3
	}
	return 0
}

func TestUnicodeIsNumber() int {
	if !unicode.IsNumber('7') {
		return 1
	}
	if !unicode.IsNumber('\u00BD') {
		return 2
	}
	if unicode.IsNumber('a') {
		return 3
	}
	if !unicode.IsNumber('\u2167') {
		return 4
	}
	return 0
}

func TestUnicodeIsSpace() int {
	if !unicode.IsSpace(' ') {
		return 1
	}
	if !unicode.IsSpace('\t') {
		return 2
	}
	if !unicode.IsSpace('\n') {
		return 3
	}
	if unicode.IsSpace('a') {
		return 4
	}
	return 0
}

func TestUnicodeIsControl() int {
	if !unicode.IsControl('\n') {
		return 1
	}
	if !unicode.IsControl('\t') {
		return 2
	}
	if unicode.IsControl('a') {
		return 3
	}
	return 0
}

func TestUnicodeIsGraphic() int {
	if !unicode.IsGraphic('a') {
		return 1
	}
	if !unicode.IsGraphic(' ') {
		return 2
	}
	if unicode.IsGraphic('\n') {
		return 3
	}
	return 0
}

func TestUnicodeIsPrint() int {
	if !unicode.IsPrint('a') {
		return 1
	}
	if !unicode.IsPrint(' ') {
		return 2
	}
	if unicode.IsPrint('\n') {
		return 3
	}
	return 0
}

func TestUnicodeIsPunct() int {
	if !unicode.IsPunct('!') {
		return 1
	}
	if unicode.IsPunct('a') {
		return 2
	}
	return 0
}

func TestUnicodeIsSymbol() int {
	if !unicode.IsSymbol('$') {
		return 1
	}
	if unicode.IsSymbol('a') {
		return 2
	}
	return 0
}

func TestUnicodeIsMark() int {
	if !unicode.IsMark('\u0301') {
		return 1
	}
	if unicode.IsMark('a') {
		return 2
	}
	return 0
}

func TestUnicodeIn() int {
	if !unicode.In('A', unicode.Latin) {
		return 1
	}
	if !unicode.In('\u3042', unicode.Hiragana) {
		return 2
	}
	if unicode.In('A', unicode.Hiragana) {
		return 3
	}
	if !unicode.In('\u00DF', unicode.Latin) {
		return 4
	}
	if !unicode.In('\u03B2', unicode.Greek) {
		return 5
	}
	if unicode.In('A', unicode.Greek) {
		return 6
	}
	return 0
}

func TestUnicodeIsOneOf() int {
	if !unicode.IsOneOf([]*unicode.RangeTable{unicode.Latin}, 'A') {
		return 1
	}
	if unicode.IsOneOf([]*unicode.RangeTable{unicode.Latin}, '\u3042') {
		return 2
	}
	if !unicode.IsOneOf([]*unicode.RangeTable{unicode.Greek, unicode.Latin}, '\u03B2') {
		return 3
	}
	if unicode.IsOneOf([]*unicode.RangeTable{unicode.Greek}, '\u00DF') {
		return 4
	}
	return 0
}

func TestUnicodeIs() int {
	if !unicode.Is(unicode.Latin, 'A') {
		return 1
	}
	if !unicode.Is(unicode.Number, '\u2167') {
		return 2
	}
	if unicode.Is(unicode.Digit, 'a') {
		return 3
	}
	return 0
}

func TestUnicodeToUpper() int {
	if unicode.ToUpper('a') != 'A' {
		return 1
	}
	if unicode.ToUpper('A') != 'A' {
		return 2
	}
	if unicode.ToUpper('\u00E4') != '\u00C4' {
		return 3
	}
	if unicode.ToUpper('\u00DF') != '\u00DF' {
		return 4
	}
	if unicode.ToUpper('\u01C6') != '\u01C4' {
		return 5
	}
	return 0
}

func TestUnicodeToLower() int {
	if unicode.ToLower('A') != 'a' {
		return 1
	}
	if unicode.ToLower('a') != 'a' {
		return 2
	}
	if unicode.ToLower('\u00C4') != '\u00E4' {
		return 3
	}
	if unicode.ToLower('I') != 'i' {
		return 4
	}
	return 0
}

func TestUnicodeToTitle() int {
	if unicode.ToTitle('a') != 'A' {
		return 1
	}
	if unicode.ToTitle('\u01C6') != '\u01C5' {
		return 2
	}
	if unicode.ToTitle('\u00DF') != '\u00DF' {
		return 3
	}
	if unicode.ToTitle('\u00E4') != '\u00C4' {
		return 4
	}
	return 0
}

func TestUnicodeTo() int {
	if unicode.To(unicode.UpperCase, 'a') != 'A' {
		return 1
	}
	if unicode.To(unicode.TitleCase, '\u01C6') != '\u01C5' {
		return 2
	}
	if unicode.To(unicode.LowerCase, 'A') != 'a' {
		return 3
	}
	return 0
}

func TestUnicodeSimpleFold() int {
	if unicode.SimpleFold('k') != '\u212A' {
		return 1
	}
	if unicode.SimpleFold('K') != 'k' {
		return 2
	}
	if unicode.SimpleFold('\u212A') != 'K' {
		return 3
	}
	if unicode.SimpleFold('\u03C2') != '\u03C3' {
		return 4
	}
	if unicode.SimpleFold('\u03C3') != '\u03A3' {
		return 5
	}
	if unicode.SimpleFold('\u03A3') != '\u03C2' {
		return 6
	}
	if unicode.SimpleFold('\u00DF') != '\u1E9E' {
		return 7
	}
	return 0
}

func TestUnicodeTableLens() int {
	if len(unicode.Categories) != 38 {
		return 1
	}
	if len(unicode.Scripts) != 163 {
		return 2
	}
	if len(unicode.Properties) != 35 {
		return 3
	}
	if len(unicode.CaseRanges) != 328 {
		return 4
	}
	if len(unicode.GraphicRanges) != 6 {
		return 5
	}
	if len(unicode.PrintRanges) != 5 {
		return 6
	}
	if len(unicode.CategoryAliases) != 42 {
		return 7
	}
	if len(unicode.FoldCategory) != 6 {
		return 8
	}
	if len(unicode.FoldScript) != 3 {
		return 9
	}
	return 0
}

func TestUnicodeMapLookups() int {
	if unicode.Categories["Lu"] == nil {
		return 1
	}
	if unicode.Categories["L"] == nil {
		return 2
	}
	if unicode.Categories["nope"] != nil {
		return 3
	}
	if unicode.Scripts["Latin"] == nil {
		return 4
	}
	if unicode.Scripts["Greek"] == nil {
		return 5
	}
	if unicode.Properties["ASCII_Hex_Digit"] == nil {
		return 6
	}
	if unicode.Properties["White_Space"] == nil {
		return 7
	}
	if unicode.FoldCategory["L"] == nil {
		return 8
	}
	if unicode.FoldCategory["nope"] != nil {
		return 9
	}
	if unicode.FoldScript["Common"] == nil {
		return 10
	}
	if unicode.FoldScript["nope"] != nil {
		return 11
	}
	return 0
}

func TestUnicodeRangeTable() int {
	if len(unicode.Latin.R16) != 31 {
		return 1
	}
	if len(unicode.Latin.R32) != 5 {
		return 2
	}
	if unicode.Latin.LatinOffset != 5 {
		return 3
	}
	if unicode.Latin.R16[0].Lo != 65 || unicode.Latin.R16[0].Hi != 90 || unicode.Latin.R16[0].Stride != 1 {
		return 4
	}
	if unicode.Latin.R16[30].Lo != 65345 || unicode.Latin.R16[30].Hi != 65370 {
		return 5
	}
	if len(unicode.Hiragana.R16) != 2 || len(unicode.Hiragana.R32) != 4 || unicode.Hiragana.LatinOffset != 0 {
		return 6
	}
	if unicode.Greek.R16[0].Lo != 880 || unicode.Greek.R16[0].Hi != 883 {
		return 7
	}
	if unicode.Digit.R16[0].Lo != 48 || unicode.Digit.R16[0].Hi != 57 || unicode.Digit.R16[0].Stride != 1 {
		return 8
	}
	if unicode.Letter.R16[0].Lo != 65 || unicode.Letter.R16[0].Hi != 90 || unicode.Letter.R16[0].Stride != 1 {
		return 9
	}
	if unicode.ASCII_Hex_Digit.R16[0].Lo != 48 || unicode.ASCII_Hex_Digit.R16[0].Hi != 57 || unicode.ASCII_Hex_Digit.R16[0].Stride != 1 {
		return 10
	}
	return 0
}

func TestUnicodeCaseRanges() int {
	if unicode.CaseRanges[0].Lo != 65 || unicode.CaseRanges[0].Hi != 90 {
		return 1
	}
	if len(unicode.CaseRanges[0].Delta) != 3 {
		return 2
	}
	if unicode.CaseRanges[0].Delta[0] != 0 || unicode.CaseRanges[0].Delta[1] != 32 || unicode.CaseRanges[0].Delta[2] != 0 {
		return 3
	}
	return 0
}

func TestUnicodeSpecialCase() int {
	if unicode.ToLower('I') != 'i' {
		return 1
	}
	if unicode.TurkishCase.ToLower('I') != '\u0131' {
		return 2
	}
	if unicode.TurkishCase.ToUpper('i') != '\u0130' {
		return 3
	}
	if unicode.AzeriCase.ToLower('I') != '\u0131' {
		return 4
	}
	if unicode.TurkishCase.ToLower('A') != 'a' {
		return 5
	}
	return 0
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("TestUnicodeConstants", TestUnicodeConstants)
	runTest("TestUnicodeIsLetter", TestUnicodeIsLetter)
	runTest("TestUnicodeIsUpper", TestUnicodeIsUpper)
	runTest("TestUnicodeIsLower", TestUnicodeIsLower)
	runTest("TestUnicodeIsTitle", TestUnicodeIsTitle)
	runTest("TestUnicodeIsDigit", TestUnicodeIsDigit)
	runTest("TestUnicodeIsNumber", TestUnicodeIsNumber)
	runTest("TestUnicodeIsSpace", TestUnicodeIsSpace)
	runTest("TestUnicodeIsControl", TestUnicodeIsControl)
	runTest("TestUnicodeIsGraphic", TestUnicodeIsGraphic)
	runTest("TestUnicodeIsPrint", TestUnicodeIsPrint)
	runTest("TestUnicodeIsPunct", TestUnicodeIsPunct)
	runTest("TestUnicodeIsSymbol", TestUnicodeIsSymbol)
	runTest("TestUnicodeIsMark", TestUnicodeIsMark)
	runTest("TestUnicodeIn", TestUnicodeIn)
	runTest("TestUnicodeIsOneOf", TestUnicodeIsOneOf)
	runTest("TestUnicodeIs", TestUnicodeIs)
	runTest("TestUnicodeToUpper", TestUnicodeToUpper)
	runTest("TestUnicodeToLower", TestUnicodeToLower)
	runTest("TestUnicodeToTitle", TestUnicodeToTitle)
	runTest("TestUnicodeTo", TestUnicodeTo)
	runTest("TestUnicodeSimpleFold", TestUnicodeSimpleFold)
	runTest("TestUnicodeTableLens", TestUnicodeTableLens)
	runTest("TestUnicodeMapLookups", TestUnicodeMapLookups)
	runTest("TestUnicodeRangeTable", TestUnicodeRangeTable)
	runTest("TestUnicodeCaseRanges", TestUnicodeCaseRanges)
	runTest("TestUnicodeSpecialCase", TestUnicodeSpecialCase)
}
