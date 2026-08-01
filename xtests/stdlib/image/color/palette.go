package main

import (
	"image/color"
	"image/color/palette"
)

func eqColor(c0, c1 color.Color) bool {
	r0, g0, b0, a0 := c0.RGBA()
	r1, g1, b1, a1 := c1.RGBA()
	return r0 == r1 && g0 == g1 && b0 == b1 && a0 == a1
}

func TestPaletteLengths() int {
	if len(palette.Plan9) != 256 {
		return 1
	}
	if len(palette.WebSafe) != 216 {
		return 2
	}
	return 0
}

func TestPaletteNonNil() int {
	for i, c := range palette.Plan9 {
		if c == nil {
			return 1 + i
		}
	}
	for i, c := range palette.WebSafe {
		if c == nil {
			return 257 + i
		}
	}
	return 0
}

func TestPlan9Entries() int {
	index := []int{0, 3, 4, 13, 16, 74, 87, 134, 136, 150, 248, 255}
	expect := []color.Color{
		color.RGBA{0x00, 0x00, 0x00, 0xff},
		color.RGBA{0x00, 0x00, 0xcc, 0xff},
		color.RGBA{0x00, 0x44, 0x00, 0xff},
		color.RGBA{0x00, 0xcc, 0x44, 0xff},
		color.RGBA{0x00, 0xdd, 0xdd, 0xff},
		color.RGBA{0x44, 0x88, 0xcc, 0xff},
		color.RGBA{0x49, 0x49, 0xdd, 0xff},
		color.RGBA{0x88, 0x88, 0x00, 0xff},
		color.RGBA{0x88, 0x88, 0x88, 0xff},
		color.RGBA{0x93, 0x49, 0xdd, 0xff},
		color.RGBA{0xff, 0xaa, 0x00, 0xff},
		color.RGBA{0xff, 0xff, 0xff, 0xff},
	}
	for i, idx := range index {
		if !eqColor(palette.Plan9[idx], expect[i]) {
			return 1 + i
		}
	}
	return 0
}

func TestWebSafeEntries() int {
	index := []int{0, 5, 26, 37, 108, 215}
	expect := []color.Color{
		color.RGBA{0x00, 0x00, 0x00, 0xff},
		color.RGBA{0x00, 0x00, 0xff, 0xff},
		color.RGBA{0x00, 0xcc, 0x66, 0xff},
		color.RGBA{0x33, 0x00, 0x33, 0xff},
		color.RGBA{0x99, 0x00, 0x00, 0xff},
		color.RGBA{0xff, 0xff, 0xff, 0xff},
	}
	for i, idx := range index {
		if !eqColor(palette.WebSafe[idx], expect[i]) {
			return 1 + i
		}
	}
	return 0
}

func TestPaletteIndexWebSafe() int {
	p := color.Palette(palette.WebSafe)
	if p.Index(color.RGBA{0x00, 0x00, 0x00, 0xff}) != 0 {
		return 1
	}
	if p.Index(color.RGBA{0x00, 0x00, 0xff, 0xff}) != 5 {
		return 2
	}
	if p.Index(color.RGBA{0x00, 0xcc, 0x66, 0xff}) != 26 {
		return 3
	}
	if p.Index(color.RGBA{0x33, 0x66, 0x99, 0xff}) != 51 {
		return 4
	}
	if p.Index(color.RGBA{0xff, 0x00, 0x00, 0xff}) != 180 {
		return 5
	}
	if p.Index(color.RGBA{0xff, 0xff, 0xff, 0xff}) != 215 {
		return 6
	}
	return 0
}

func TestPaletteIndexPlan9() int {
	p := color.Palette(palette.Plan9)
	if p.Index(color.RGBA{0x00, 0x00, 0xcc, 0xff}) != 3 {
		return 1
	}
	if p.Index(color.RGBA{0x00, 0xdd, 0xdd, 0xff}) != 16 {
		return 2
	}
	if p.Index(color.RGBA{0x88, 0x88, 0x88, 0xff}) != 136 {
		return 3
	}
	if p.Index(color.RGBA{0xff, 0xff, 0xff, 0xff}) != 255 {
		return 4
	}
	return 0
}

func TestPaletteConvert() int {
	if !eqColor(color.Palette(palette.WebSafe).Convert(color.RGBA{0x80, 0x00, 0x00, 0xff}), color.RGBA{0x99, 0x00, 0x00, 0xff}) {
		return 1
	}
	if !eqColor(color.Palette(palette.WebSafe).Convert(color.RGBA{0x00, 0x80, 0x00, 0xff}), color.RGBA{0x00, 0x99, 0x00, 0xff}) {
		return 2
	}
	if !eqColor(color.Palette(palette.Plan9).Convert(color.RGBA{0x80, 0x80, 0x80, 0xff}), color.RGBA{0x88, 0x88, 0x88, 0xff}) {
		return 3
	}
	if (color.Palette{}).Convert(color.RGBA{0x00, 0x00, 0x00, 0xff}) != nil {
		return 4
	}
	return 0
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("TestPaletteLengths", TestPaletteLengths)
	runTest("TestPaletteNonNil", TestPaletteNonNil)
	runTest("TestPlan9Entries", TestPlan9Entries)
	runTest("TestWebSafeEntries", TestWebSafeEntries)
	runTest("TestPaletteIndexWebSafe", TestPaletteIndexWebSafe)
	runTest("TestPaletteIndexPlan9", TestPaletteIndexPlan9)
	runTest("TestPaletteConvert", TestPaletteConvert)
}
