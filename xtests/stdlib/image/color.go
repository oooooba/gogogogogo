package main

import "image/color"

func eqColor(c0, c1 color.Color) bool {
	r0, g0, b0, a0 := c0.RGBA()
	r1, g1, b1, a1 := c1.RGBA()
	return r0 == r1 && g0 == g1 && b0 == b1 && a0 == a1
}

func eq4(r, g, b, a, er, eg, eb, ea uint32) bool {
	return r == er && g == eg && b == eb && a == ea
}

func TestRGBA() int {
	r, g, b, a := color.RGBA{0x12, 0x34, 0x56, 0x78}.RGBA()
	if !eq4(r, g, b, a, 0x1212, 0x3434, 0x5656, 0x7878) {
		return 1
	}
	var c color.Color = color.RGBA{0xff, 0x00, 0x00, 0xff}
	r, g, b, a = c.RGBA()
	if !eq4(r, g, b, a, 0xffff, 0x0000, 0x0000, 0xffff) {
		return 2
	}
	return 0
}

func TestRGBA64() int {
	r, g, b, a := color.RGBA64{0x1234, 0x5678, 0x9abc, 0xdef0}.RGBA()
	if !eq4(r, g, b, a, 0x1234, 0x5678, 0x9abc, 0xdef0) {
		return 1
	}
	var c color.Color = color.RGBA64{0xffff, 0x8000, 0x0000, 0x8000}
	r, g, b, a = c.RGBA()
	if !eq4(r, g, b, a, 0xffff, 0x8000, 0x0000, 0x8000) {
		return 2
	}
	return 0
}

func TestNRGBA() int {
	r, g, b, a := color.NRGBA{0xff, 0x80, 0x00, 0xff}.RGBA()
	if !eq4(r, g, b, a, 0xffff, 0x8080, 0x0000, 0xffff) {
		return 1
	}
	r, g, b, a = color.NRGBA{0x80, 0x40, 0x20, 0x80}.RGBA()
	if !eq4(r, g, b, a, 0x4080, 0x2040, 0x1020, 0x8080) {
		return 2
	}
	var c color.Color = color.NRGBA{0x00, 0x80, 0x80, 0x80}
	r, g, b, a = c.RGBA()
	if !eq4(r, g, b, a, 0x0000, 0x4080, 0x4080, 0x8080) {
		return 3
	}
	return 0
}

func TestNRGBA64() int {
	r, g, b, a := color.NRGBA64{0x8000, 0x4000, 0x2000, 0xffff}.RGBA()
	if !eq4(r, g, b, a, 0x8000, 0x4000, 0x2000, 0xffff) {
		return 1
	}
	var c color.Color = color.NRGBA64{0x8000, 0x4000, 0x2000, 0x8000}
	r, g, b, a = c.RGBA()
	if !eq4(r, g, b, a, 0x4000, 0x2000, 0x1000, 0x8000) {
		return 2
	}
	return 0
}

func TestAlpha() int {
	r, g, b, a := color.Alpha{0x80}.RGBA()
	if !eq4(r, g, b, a, 0x8080, 0x8080, 0x8080, 0x8080) {
		return 1
	}
	var c color.Color = color.Alpha{0xff}
	r, g, b, a = c.RGBA()
	if !eq4(r, g, b, a, 0xffff, 0xffff, 0xffff, 0xffff) {
		return 2
	}
	return 0
}

func TestAlpha16() int {
	r, g, b, a := color.Alpha16{0x1234}.RGBA()
	if !eq4(r, g, b, a, 0x1234, 0x1234, 0x1234, 0x1234) {
		return 1
	}
	var c color.Color = color.Alpha16{0xffff}
	r, g, b, a = c.RGBA()
	if !eq4(r, g, b, a, 0xffff, 0xffff, 0xffff, 0xffff) {
		return 2
	}
	return 0
}

func TestGray() int {
	r, g, b, a := color.Gray{0x80}.RGBA()
	if !eq4(r, g, b, a, 0x8080, 0x8080, 0x8080, 0xffff) {
		return 1
	}
	var c color.Color = color.Gray{0x00}
	r, g, b, a = c.RGBA()
	if !eq4(r, g, b, a, 0x0000, 0x0000, 0x0000, 0xffff) {
		return 2
	}
	return 0
}

func TestGray16() int {
	r, g, b, a := color.Gray16{0x1234}.RGBA()
	if !eq4(r, g, b, a, 0x1234, 0x1234, 0x1234, 0xffff) {
		return 1
	}
	var c color.Color = color.Gray16{0xffff}
	r, g, b, a = c.RGBA()
	if !eq4(r, g, b, a, 0xffff, 0xffff, 0xffff, 0xffff) {
		return 2
	}
	return 0
}

func TestYCbCr() int {
	r, g, b, a := color.YCbCr{0x80, 0x80, 0x80}.RGBA()
	if !eq4(r, g, b, a, 0x8080, 0x8080, 0x8080, 0xffff) {
		return 1
	}
	if !eqColor(color.YCbCr{0xff, 0x80, 0x80}, color.Gray{0xff}) {
		return 2
	}
	var c color.Color = color.YCbCr{0x7f, 0x80, 0x80}
	r, g, b, a = c.RGBA()
	if !eq4(r, g, b, a, 0x7f7f, 0x7f7f, 0x7f7f, 0xffff) {
		return 3
	}
	return 0
}

func TestNYCbCrA() int {
	if !eqColor(color.NYCbCrA{color.YCbCr{0xff, 0x80, 0x80}, 0xff}, color.Alpha{0xff}) {
		return 1
	}
	if !eqColor(color.NYCbCrA{color.YCbCr{0xff, 0x80, 0x80}, 0x80}, color.Alpha{0x80}) {
		return 2
	}
	if !eqColor(color.NYCbCrA{color.YCbCr{0x7f, 0x40, 0xc0}, 0xff}, color.YCbCr{0x7f, 0x40, 0xc0}) {
		return 3
	}
	var c color.Color = color.NYCbCrA{color.YCbCr{0x80, 0x80, 0x80}, 0xff}
	if !eqColor(c, color.Gray{0x80}) {
		return 4
	}
	return 0
}

func TestCMYK() int {
	r, g, b, a := color.CMYK{0x00, 0x00, 0x00, 0x7f}.RGBA()
	if !eq4(r, g, b, a, 0x8080, 0x8080, 0x8080, 0xffff) {
		return 1
	}
	if !eqColor(color.CMYK{0x00, 0x00, 0x00, 0xff}, color.Gray{0x00}) {
		return 2
	}
	var c color.Color = color.CMYK{0xff, 0xff, 0xff, 0xff}
	r, g, b, a = c.RGBA()
	if !eq4(r, g, b, a, 0x0000, 0x0000, 0x0000, 0xffff) {
		return 3
	}
	return 0
}

func TestRGBToYCbCr() int {
	y, cb, cr := color.RGBToYCbCr(0x00, 0x00, 0x00)
	if y != 0 || cb != 0x80 || cr != 0x80 {
		return 1
	}
	y, cb, cr = color.RGBToYCbCr(0xff, 0xff, 0xff)
	if y != 0xff || cb != 0x80 || cr != 0x80 {
		return 2
	}
	y, cb, cr = color.RGBToYCbCr(0xff, 0x00, 0x00)
	if y != 0x4c || cb != 0x55 || cr != 0xff {
		return 3
	}
	y, cb, cr = color.RGBToYCbCr(0x00, 0xff, 0x00)
	if y != 0x96 || cb != 0x2c || cr != 0x15 {
		return 4
	}
	y, cb, cr = color.RGBToYCbCr(0x00, 0x00, 0xff)
	if y != 0x1d || cb != 0xff || cr != 0x6b {
		return 5
	}
	return 0
}

func TestYCbCrToRGB() int {
	r, g, b := color.YCbCrToRGB(0x00, 0x80, 0x80)
	if r != 0 || g != 0 || b != 0 {
		return 1
	}
	r, g, b = color.YCbCrToRGB(0xff, 0x80, 0x80)
	if r != 0xff || g != 0xff || b != 0xff {
		return 2
	}
	x := color.YCbCr{0x80, 0x40, 0xc0}
	r0, g0, b0, _ := x.RGBA()
	r1, g1, b1 := color.YCbCrToRGB(x.Y, x.Cb, x.Cr)
	if uint8(r0>>8) != r1 || uint8(g0>>8) != g1 || uint8(b0>>8) != b1 {
		return 3
	}
	return 0
}

func TestRGBToCMYK() int {
	c, m, y, k := color.RGBToCMYK(0x00, 0x00, 0x00)
	if c != 0 || m != 0 || y != 0 || k != 0xff {
		return 1
	}
	c, m, y, k = color.RGBToCMYK(0xff, 0xff, 0xff)
	if c != 0 || m != 0 || y != 0 || k != 0 {
		return 2
	}
	c, m, y, k = color.RGBToCMYK(0xff, 0x00, 0x00)
	if c != 0 || m != 0xff || y != 0xff || k != 0 {
		return 3
	}
	c, m, y, k = color.RGBToCMYK(0x00, 0x80, 0x40)
	if c != 0xff || m != 0x00 || y != 0x7f || k != 0x7f {
		return 4
	}
	return 0
}

func TestCMYKToRGB() int {
	r, g, b := color.CMYKToRGB(0x00, 0x00, 0x00, 0x00)
	if r != 0xff || g != 0xff || b != 0xff {
		return 1
	}
	r, g, b = color.CMYKToRGB(0x00, 0x00, 0x00, 0xff)
	if r != 0 || g != 0 || b != 0 {
		return 2
	}
	r, g, b = color.CMYKToRGB(0x80, 0x00, 0x00, 0x00)
	if r != 0x7f || g != 0xff || b != 0xff {
		return 3
	}
	x := color.CMYK{0x40, 0x80, 0xc0, 0x20}
	r0, g0, b0, _ := x.RGBA()
	r1, g1, b1 := color.CMYKToRGB(x.C, x.M, x.Y, x.K)
	if uint8(r0>>8) != r1 || uint8(g0>>8) != g1 || uint8(b0>>8) != b1 {
		return 4
	}
	return 0
}

func TestModels() int {
	r, g, b, a := color.RGBAModel.Convert(color.RGBA{0x12, 0x34, 0x56, 0x78}).RGBA()
	if !eq4(r, g, b, a, 0x1212, 0x3434, 0x5656, 0x7878) {
		return 1
	}
	if !eqColor(color.RGBA64Model.Convert(color.RGBA{0x12, 0x34, 0x56, 0x78}), color.RGBA64{0x1212, 0x3434, 0x5656, 0x7878}) {
		return 2
	}
	if !eqColor(color.NRGBAModel.Convert(color.NRGBA{0x80, 0x40, 0x20, 0xff}), color.NRGBA{0x80, 0x40, 0x20, 0xff}) {
		return 3
	}
	if !eqColor(color.NRGBA64Model.Convert(color.NRGBA64{0x8000, 0x4000, 0x2000, 0xffff}), color.NRGBA64{0x8000, 0x4000, 0x2000, 0xffff}) {
		return 4
	}
	if !eqColor(color.AlphaModel.Convert(color.Alpha{0x80}), color.Alpha{0x80}) {
		return 5
	}
	if !eqColor(color.AlphaModel.Convert(color.RGBA{0xff, 0xff, 0xff, 0x80}), color.Alpha{0x80}) {
		return 6
	}
	if !eqColor(color.Alpha16Model.Convert(color.Alpha16{0x1234}), color.Alpha16{0x1234}) {
		return 7
	}
	if !eqColor(color.GrayModel.Convert(color.Gray{0x80}), color.Gray{0x80}) {
		return 8
	}
	if !eqColor(color.GrayModel.Convert(color.RGBA{0xff, 0x00, 0x00, 0xff}), color.Gray{0x4c}) {
		return 9
	}
	if !eqColor(color.Gray16Model.Convert(color.Gray16{0x1234}), color.Gray16{0x1234}) {
		return 10
	}
	if !eqColor(color.YCbCrModel.Convert(color.YCbCr{0x80, 0x40, 0xc0}), color.YCbCr{0x80, 0x40, 0xc0}) {
		return 11
	}
	if !eqColor(color.NYCbCrAModel.Convert(color.NYCbCrA{color.YCbCr{0x80, 0x40, 0xc0}, 0xff}), color.NYCbCrA{color.YCbCr{0x80, 0x40, 0xc0}, 0xff}) {
		return 12
	}
	if !eqColor(color.CMYKModel.Convert(color.CMYK{0x10, 0x20, 0x30, 0x40}), color.CMYK{0x10, 0x20, 0x30, 0x40}) {
		return 13
	}
	return 0
}

func TestModelFunc() int {
	m := color.ModelFunc(func(c color.Color) color.Color {
		r, g, b, a := c.RGBA()
		return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
	})
	r, g, b, a := m.Convert(color.RGBA{0x12, 0x34, 0x56, 0x78}).RGBA()
	if !eq4(r, g, b, a, 0x1212, 0x3434, 0x5656, 0x7878) {
		return 1
	}
	if !eqColor(m.Convert(color.RGBA{0xff, 0x00, 0x00, 0xff}), color.RGBA{0xff, 0x00, 0x00, 0xff}) {
		return 2
	}
	return 0
}

func TestPalette() int {
	p := color.Palette{
		color.RGBA{0xff, 0xff, 0xff, 0xff},
		color.RGBA{0x80, 0x00, 0x00, 0xff},
		color.RGBA{0x7f, 0x00, 0x00, 0x7f},
		color.RGBA{0x00, 0x00, 0x00, 0x7f},
		color.RGBA{0x00, 0x00, 0x00, 0x00},
		color.RGBA{0x40, 0x40, 0x40, 0x40},
	}
	for i, c := range p {
		if p.Index(c) != i {
			return 1
		}
	}
	if !eqColor(p.Convert(color.RGBA{0x80, 0x00, 0x00, 0x80}), color.RGBA{0x7f, 0x00, 0x00, 0x7f}) {
		return 2
	}
	if !eqColor(p.Convert(color.RGBA{0x40, 0x40, 0x40, 0x40}), color.RGBA{0x40, 0x40, 0x40, 0x40}) {
		return 3
	}
	return 0
}

func TestStandardColors() int {
	r, g, b, a := color.Black.RGBA()
	if !eq4(r, g, b, a, 0x0000, 0x0000, 0x0000, 0xffff) {
		return 1
	}
	r, g, b, a = color.White.RGBA()
	if !eq4(r, g, b, a, 0xffff, 0xffff, 0xffff, 0xffff) {
		return 2
	}
	r, g, b, a = color.Transparent.RGBA()
	if !eq4(r, g, b, a, 0x0000, 0x0000, 0x0000, 0x0000) {
		return 3
	}
	r, g, b, a = color.Opaque.RGBA()
	if !eq4(r, g, b, a, 0xffff, 0xffff, 0xffff, 0xffff) {
		return 4
	}
	return 0
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("TestRGBA", TestRGBA)
	runTest("TestRGBA64", TestRGBA64)
	runTest("TestNRGBA", TestNRGBA)
	runTest("TestNRGBA64", TestNRGBA64)
	runTest("TestAlpha", TestAlpha)
	runTest("TestAlpha16", TestAlpha16)
	runTest("TestGray", TestGray)
	runTest("TestGray16", TestGray16)
	runTest("TestYCbCr", TestYCbCr)
	runTest("TestNYCbCrA", TestNYCbCrA)
	runTest("TestCMYK", TestCMYK)
	runTest("TestRGBToYCbCr", TestRGBToYCbCr)
	runTest("TestYCbCrToRGB", TestYCbCrToRGB)
	runTest("TestRGBToCMYK", TestRGBToCMYK)
	runTest("TestCMYKToRGB", TestCMYKToRGB)
	runTest("TestModels", TestModels)
	runTest("TestModelFunc", TestModelFunc)
	runTest("TestPalette", TestPalette)
	runTest("TestStandardColors", TestStandardColors)
}
