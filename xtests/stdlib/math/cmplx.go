package main

import (
	"math"
	"math/cmplx"
)

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func cr(z complex128) uint64 { return math.Float64bits(real(z)) }
func ci(z complex128) uint64 { return math.Float64bits(imag(z)) }

func polarR(x complex128) float64 {
	r, _ := cmplx.Polar(x)
	return r
}

func polarT(x complex128) float64 {
	_, t := cmplx.Polar(x)
	return t
}

func main() {
	bad := 0
	chk := func(name string, got, want uint64) {
		if got != want {
			println("FAIL " + name)
			bad++
		}
	}
	chkint := func(name string, got, want int) {
		if got != want {
			println("FAIL " + name)
			bad++
		}
	}

	chk("Abs_3_4", math.Float64bits(cmplx.Abs(3+4i)), 0x4014000000000000)
	chk("Abs_n3_n4", math.Float64bits(cmplx.Abs(-3-4i)), 0x4014000000000000)
	chk("Abs_0_0", math.Float64bits(cmplx.Abs(0)), 0x0000000000000000)
	chk("Abs_1_1", math.Float64bits(cmplx.Abs(1+1i)), 0x3ff6a09e667f3bcd)
	chk("Abs_1e300", math.Float64bits(cmplx.Abs(complex(1e300, 1e300))), 0x7e40e4d50f99b211)

	chk("Acos_0_0r", cr(cmplx.Acos(0)), 0x3ff921fb54442d18)
	chk("Acos_0_0i", ci(cmplx.Acos(0)), 0x8000000000000000)
	chk("Acos_1_0r", cr(cmplx.Acos(1)), 0x0000000000000000)
	chk("Acos_1_0i", ci(cmplx.Acos(1)), 0x8000000000000000)
	chk("Acos_m1_0r", cr(cmplx.Acos(-1)), 0x400921fb54442d18)
	chk("Acos_m1_0i", ci(cmplx.Acos(-1)), 0x8000000000000000)
	chk("Acos_05_05r", cr(cmplx.Acos(0.5+0.5i)), 0x3ff1e5730110f9c7)
	chk("Acos_05_05i", ci(cmplx.Acos(0.5+0.5i)), 0xbfe0fafb8f2f147e)
	chk("Acos_2_3r", cr(cmplx.Acos(2+3i)), 0x3ff0009683e3b052)
	chk("Acos_2_3i", ci(cmplx.Acos(2+3i)), 0xbfffbbf409ccd560)

	chk("Acosh_0_0r", cr(cmplx.Acosh(0)), 0x0000000000000000)
	chk("Acosh_0_0i", ci(cmplx.Acosh(0)), 0x3ff921fb54442d18)
	chk("Acosh_1_0r", cr(cmplx.Acosh(1)), 0x0000000000000000)
	chk("Acosh_1_0i", ci(cmplx.Acosh(1)), 0x0000000000000000)
	chk("Acosh_2_3r", cr(cmplx.Acosh(2+3i)), 0x3fffbbf409ccd560)
	chk("Acosh_2_3i", ci(cmplx.Acosh(2+3i)), 0x3ff0009683e3b052)
	chk("Acosh_05_05r", cr(cmplx.Acosh(0.5+0.5i)), 0x3fe0fafb8f2f147e)
	chk("Acosh_05_05i", ci(cmplx.Acosh(0.5+0.5i)), 0x3ff1e5730110f9c7)

	chk("Asin_0_0r", cr(cmplx.Asin(0)), 0x0000000000000000)
	chk("Asin_0_0i", ci(cmplx.Asin(0)), 0x0000000000000000)
	chk("Asin_1_0r", cr(cmplx.Asin(1)), 0x3ff921fb54442d18)
	chk("Asin_1_0i", ci(cmplx.Asin(1)), 0x0000000000000000)
	chk("Asin_05_05r", cr(cmplx.Asin(0.5+0.5i)), 0x3fdcf2214ccccd44)
	chk("Asin_05_05i", ci(cmplx.Asin(0.5+0.5i)), 0x3fe0fafb8f2f147e)
	chk("Asin_2_3r", cr(cmplx.Asin(2+3i)), 0x3fe242c9a0c0f98b)
	chk("Asin_2_3i", ci(cmplx.Asin(2+3i)), 0x3fffbbf409ccd560)

	chk("Asinh_0_0r", cr(cmplx.Asinh(0)), 0x0000000000000000)
	chk("Asinh_0_0i", ci(cmplx.Asinh(0)), 0x0000000000000000)
	chk("Asinh_1_0r", cr(cmplx.Asinh(1)), 0x3fec34366179d427)
	chk("Asinh_1_0i", ci(cmplx.Asinh(1)), 0x0000000000000000)
	chk("Asinh_2_3r", cr(cmplx.Asinh(2+3i)), 0x3fff7f8a7b4f255f)
	chk("Asinh_2_3i", ci(cmplx.Asinh(2+3i)), 0x3feede7b8307a547)
	chk("Asinh_05_05r", cr(cmplx.Asinh(0.5+0.5i)), 0x3fe0fafb8f2f147e)
	chk("Asinh_05_05i", ci(cmplx.Asinh(0.5+0.5i)), 0x3fdcf2214ccccd43)

	chk("Atan_0_0r", cr(cmplx.Atan(0)), 0x0000000000000000)
	chk("Atan_0_0i", ci(cmplx.Atan(0)), 0x0000000000000000)
	chk("Atan_1_0r", cr(cmplx.Atan(1)), 0x3fe921fb54442d18)
	chk("Atan_1_0i", ci(cmplx.Atan(1)), 0x0000000000000000)
	chk("Atan_05_05r", cr(cmplx.Atan(0.5+0.5i)), 0x3fe1b6e192ebbe44)
	chk("Atan_05_05i", ci(cmplx.Atan(0.5+0.5i)), 0x3fd9c041f7ed8d33)
	chk("Atan_2_3r", cr(cmplx.Atan(2+3i)), 0x3ff68f095fdf593c)
	chk("Atan_2_3i", ci(cmplx.Atan(2+3i)), 0x3fcd5240f0e0e078)

	chk("Atanh_0_0r", cr(cmplx.Atanh(0)), 0x0000000000000000)
	chk("Atanh_0_0i", ci(cmplx.Atanh(0)), 0x0000000000000000)
	chk("Atanh_1_0r", cr(cmplx.Atanh(1)), 0x7ff0000000000000)
	chk("Atanh_1_0i", ci(cmplx.Atanh(1)), 0x0000000000000000)
	chk("Atanh_05_05r", cr(cmplx.Atanh(0.5+0.5i)), 0x3fd9c041f7ed8d33)
	chk("Atanh_05_05i", ci(cmplx.Atanh(0.5+0.5i)), 0x3fe1b6e192ebbe44)
	chk("Atanh_2_3r", cr(cmplx.Atanh(2+3i)), 0x3fc2cf25fad8f1c4)
	chk("Atanh_2_3i", ci(cmplx.Atanh(2+3i)), 0x3ff56c6e7397f5ae)

	chk("Conj_1_2r", cr(cmplx.Conj(1+2i)), 0x3ff0000000000000)
	chk("Conj_1_2i", ci(cmplx.Conj(1+2i)), 0xc000000000000000)
	chk("Conj_n1_m2r", cr(cmplx.Conj(-1-2i)), 0xbff0000000000000)
	chk("Conj_n1_m2i", ci(cmplx.Conj(-1-2i)), 0x4000000000000000)

	chk("Cos_0_0r", cr(cmplx.Cos(0)), 0x3ff0000000000000)
	chk("Cos_0_0i", ci(cmplx.Cos(0)), 0x8000000000000000)
	chk("Cos_1_1r", cr(cmplx.Cos(1+1i)), 0x3feaadea96f4359b)
	chk("Cos_1_1i", ci(cmplx.Cos(1+1i)), 0xbfefa50ccd2ae8f3)
	chk("Cos_05_05r", cr(cmplx.Cos(0.5+0.5i)), 0x3fefaaadeada3236)
	chk("Cos_05_05i", ci(cmplx.Cos(0.5+0.5i)), 0xbfcffa4fb77892c0)
	chk("Cos_2_3r", cr(cmplx.Cos(2+3i)), 0xc010c22d3cb4c50c)
	chk("Cos_2_3i", ci(cmplx.Cos(2+3i)), 0xc02237ecb7eefaf3)

	chk("Cosh_0_0r", cr(cmplx.Cosh(0)), 0x3ff0000000000000)
	chk("Cosh_0_0i", ci(cmplx.Cosh(0)), 0x0000000000000000)
	chk("Cosh_1_1r", cr(cmplx.Cosh(1+1i)), 0x3feaadea96f4359b)
	chk("Cosh_1_1i", ci(cmplx.Cosh(1+1i)), 0x3fefa50ccd2ae8f3)
	chk("Cosh_2_3r", cr(cmplx.Cosh(2+3i)), 0xc00dcbde838099d7)
	chk("Cosh_2_3i", ci(cmplx.Cosh(2+3i)), 0x3fe060d9b9ee6a66)

	chk("Cot_1_1r", cr(cmplx.Cot(1+1i)), 0x3fcbdb05f988d84a)
	chk("Cot_1_1i", ci(cmplx.Cot(1+1i)), 0xbfebc6c5988682d1)
	chk("Cot_05_05r", cr(cmplx.Cot(0.5+0.5i)), 0x3feada3b3f161b81)
	chk("Cot_05_05i", ci(cmplx.Cot(0.5+0.5i)), 0xbff2c0498d470263)
	chk("Cot_2_3r", cr(cmplx.Cot(2+3i)), 0xbf6ea2bdb8698163)
	chk("Cot_2_3i", ci(cmplx.Cot(2+3i)), 0xbfefe5709b498c9d)

	chk("Exp_0_0r", cr(cmplx.Exp(0)), 0x3ff0000000000000)
	chk("Exp_0_0i", ci(cmplx.Exp(0)), 0x0000000000000000)
	chk("Exp_1_1r", cr(cmplx.Exp(1+1i)), 0x3ff77fc5377c5a96)
	chk("Exp_1_1i", ci(cmplx.Exp(1+1i)), 0x40024c80edc62064)
	chk("Exp_1_2r", cr(cmplx.Exp(1+2i)), 0xbff21969c4953cd1)
	chk("Exp_1_2i", ci(cmplx.Exp(1+2i)), 0x4003c618a2274afc)
	chk("Exp_0_pi2r", cr(cmplx.Exp(complex(0, math.Pi/2))), 0x3c91a62633145c00)
	chk("Exp_0_pi2i", ci(cmplx.Exp(complex(0, math.Pi/2))), 0x3ff0000000000000)
	chk("Exp_m1_0r", cr(cmplx.Exp(-1)), 0x3fd78b56362cef38)
	chk("Exp_m1_0i", ci(cmplx.Exp(-1)), 0x0000000000000000)

	chk("Log_1_0r", cr(cmplx.Log(1)), 0x0000000000000000)
	chk("Log_1_0i", ci(cmplx.Log(1)), 0x0000000000000000)
	chk("Log_1_1r", cr(cmplx.Log(1+1i)), 0x3fd62e42fefa39f0)
	chk("Log_1_1i", ci(cmplx.Log(1+1i)), 0x3fe921fb54442d18)
	chk("Log_m1_0r", cr(cmplx.Log(-1)), 0x0000000000000000)
	chk("Log_m1_0i", ci(cmplx.Log(-1)), 0x400921fb54442d18)
	chk("Log_0_1r", cr(cmplx.Log(1i)), 0x0000000000000000)
	chk("Log_0_1i", ci(cmplx.Log(1i)), 0x3ff921fb54442d18)
	chk("Log_2_3r", cr(cmplx.Log(2+3i)), 0x3ff485042b318c51)
	chk("Log_2_3i", ci(cmplx.Log(2+3i)), 0x3fef730bd281f69b)

	chk("Log10_1_0r", cr(cmplx.Log10(1)), 0x0000000000000000)
	chk("Log10_1_0i", ci(cmplx.Log10(1)), 0x0000000000000000)
	chk("Log10_10_0r", cr(cmplx.Log10(10)), 0x3ff0000000000000)
	chk("Log10_10_0i", ci(cmplx.Log10(10)), 0x0000000000000000)
	chk("Log10_1_1r", cr(cmplx.Log10(1+1i)), 0x3fc34413509f79ff)
	chk("Log10_1_1i", ci(cmplx.Log10(1+1i)), 0x3fd5d47c4cb2fba0)

	chk("Phase_1_1", math.Float64bits(cmplx.Phase(1+1i)), 0x3fe921fb54442d18)
	chk("Phase_m1_0", math.Float64bits(cmplx.Phase(-1)), 0x400921fb54442d18)
	chk("Phase_0_1", math.Float64bits(cmplx.Phase(1i)), 0x3ff921fb54442d18)
	chk("Phase_n1_n1", math.Float64bits(cmplx.Phase(-1-1i)), 0xc002d97c7f3321d2)

	chk("Polar_1_1r", math.Float64bits(polarR(1+1i)), 0x3ff6a09e667f3bcd)
	chk("Polar_1_1t", math.Float64bits(polarT(1+1i)), 0x3fe921fb54442d18)
	chk("Polar_m1_0r", math.Float64bits(polarR(-1)), 0x3ff0000000000000)
	chk("Polar_m1_0t", math.Float64bits(polarT(-1)), 0x400921fb54442d18)
	chk("Polar_2_3r", math.Float64bits(polarR(2+3i)), 0x400cd82b446159f4)
	chk("Polar_2_3t", math.Float64bits(polarT(2+3i)), 0x3fef730bd281f69b)

	chk("Pow_2_0_3_4r", cr(cmplx.Pow(2, 3+4i)), 0xc01dd892918d355e)
	chk("Pow_2_0_3_4i", ci(cmplx.Pow(2, 3+4i)), 0x4007157d35c768c8)
	chk("Pow_2_3_05_1r", cr(cmplx.Pow(2+3i, 0.5+1i)), 0xbfc2589453adeae4)
	chk("Pow_2_3_05_1i", ci(cmplx.Pow(2+3i, 0.5+1i)), 0x3fe6461f8ef5caaf)
	chk("Pow_0_0r", cr(cmplx.Pow(0, 0)), 0x3ff0000000000000)
	chk("Pow_0_0i", ci(cmplx.Pow(0, 0)), 0x0000000000000000)
	chk("Pow_1_0_2_0r", cr(cmplx.Pow(1, 2)), 0x3ff0000000000000)
	chk("Pow_1_0_2_0i", ci(cmplx.Pow(1, 2)), 0x0000000000000000)
	chk("Pow_2_3_0_0r", cr(cmplx.Pow(2+3i, 0)), 0x3ff0000000000000)
	chk("Pow_2_3_0_0i", ci(cmplx.Pow(2+3i, 0)), 0x0000000000000000)

	chk("Rect_1_1r", cr(cmplx.Rect(1, 1)), 0x3fe14a280fb5068c)
	chk("Rect_1_1i", ci(cmplx.Rect(1, 1)), 0x3feaed548f090cee)
	chk("Rect_2_0r", cr(cmplx.Rect(2, 0)), 0x4000000000000000)
	chk("Rect_2_0i", ci(cmplx.Rect(2, 0)), 0x0000000000000000)
	chk("Rect_5_05r", cr(cmplx.Rect(5, 0.5)), 0x40118d3903f92e52)
	chk("Rect_5_05i", ci(cmplx.Rect(5, 0.5)), 0x40032d5148aee3b6)

	chk("Sin_0_0r", cr(cmplx.Sin(0)), 0x0000000000000000)
	chk("Sin_0_0i", ci(cmplx.Sin(0)), 0x0000000000000000)
	chk("Sin_1_1r", cr(cmplx.Sin(1+1i)), 0x3ff4c67b74f6cc4f)
	chk("Sin_1_1i", ci(cmplx.Sin(1+1i)), 0x3fe4519fd8047f92)
	chk("Sin_05_05r", cr(cmplx.Sin(0.5+0.5i)), 0x3fe14cb2f99e1a61)
	chk("Sin_05_05i", ci(cmplx.Sin(0.5+0.5i)), 0x3fdd4478a39015b5)
	chk("Sin_2_3r", cr(cmplx.Sin(2+3i)), 0x40224f1a831e7d2d)
	chk("Sin_2_3i", ci(cmplx.Sin(2+3i)), 0xc010acf5f2347e22)

	chk("Sinh_0_0r", cr(cmplx.Sinh(0)), 0x0000000000000000)
	chk("Sinh_0_0i", ci(cmplx.Sinh(0)), 0x0000000000000000)
	chk("Sinh_1_1r", cr(cmplx.Sinh(1+1i)), 0x3fe4519fd8047f92)
	chk("Sinh_1_1i", ci(cmplx.Sinh(1+1i)), 0x3ff4c67b74f6cc4f)
	chk("Sinh_2_3r", cr(cmplx.Sinh(2+3i)), 0xc00cb979ed81510c)
	chk("Sinh_2_3i", ci(cmplx.Sinh(2+3i)), 0x3fe0fd4e37c636c9)

	chk("Sqrt_m4_0r", cr(cmplx.Sqrt(-4)), 0x0000000000000000)
	chk("Sqrt_m4_0i", ci(cmplx.Sqrt(-4)), 0x4000000000000000)
	chk("Sqrt_1_1r", cr(cmplx.Sqrt(1+1i)), 0x3ff19435caffa9f9)
	chk("Sqrt_1_1i", ci(cmplx.Sqrt(1+1i)), 0x3fdd203138f6c828)
	chk("Sqrt_2_3r", cr(cmplx.Sqrt(2+3i)), 0x3ffac950b37094a6)
	chk("Sqrt_2_3i", ci(cmplx.Sqrt(2+3i)), 0x3fecabd8f4bdc4d1)
	chk("Sqrt_m1_m2r", cr(cmplx.Sqrt(-1-2i)), 0x3fe92826ef258d1b)
	chk("Sqrt_m1_m2i", ci(cmplx.Sqrt(-1-2i)), 0xbff45a3146a88456)
	chk("Sqrt_0_0r", cr(cmplx.Sqrt(0)), 0x0000000000000000)
	chk("Sqrt_0_0i", ci(cmplx.Sqrt(0)), 0x0000000000000000)
	chk("Sqrt_4_0r", cr(cmplx.Sqrt(4)), 0x4000000000000000)
	chk("Sqrt_4_0i", ci(cmplx.Sqrt(4)), 0x0000000000000000)

	chk("Tan_0_0r", cr(cmplx.Tan(0)), 0x0000000000000000)
	chk("Tan_0_0i", ci(cmplx.Tan(0)), 0x0000000000000000)
	chk("Tan_1_1r", cr(cmplx.Tan(1+1i)), 0x3fd16464f4a33f87)
	chk("Tan_1_1i", ci(cmplx.Tan(1+1i)), 0x3ff157bffca4a8be)
	chk("Tan_05_05r", cr(cmplx.Tan(0.5+0.5i)), 0x3fd9d97084a35eb7)
	chk("Tan_05_05i", ci(cmplx.Tan(0.5+0.5i)), 0x3fe20cf8167f00da)
	chk("Tan_2_3r", cr(cmplx.Tan(2+3i)), 0xbf6ed5bbe102970d)
	chk("Tan_2_3i", ci(cmplx.Tan(2+3i)), 0x3ff00d43f269153d)

	chk("Tanh_0_0r", cr(cmplx.Tanh(0)), 0x0000000000000000)
	chk("Tanh_0_0i", ci(cmplx.Tanh(0)), 0x0000000000000000)
	chk("Tanh_1_1r", cr(cmplx.Tanh(1+1i)), 0x3ff157bffca4a8be)
	chk("Tanh_1_1i", ci(cmplx.Tanh(1+1i)), 0x3fd16464f4a33f87)
	chk("Tanh_2_3r", cr(cmplx.Tanh(2+3i)), 0x3feee470ed4d72e2)
	chk("Tanh_2_3i", ci(cmplx.Tanh(2+3i)), 0xbf843e425c3f79b5)

	chkint("Inf", b2i(cmplx.IsInf(cmplx.Inf())), 1)
	chkint("IsInf1", b2i(cmplx.IsInf(complex(math.Inf(1), 2))), 1)
	chkint("IsInf2", b2i(cmplx.IsInf(complex(1, math.Inf(1)))), 1)
	chkint("IsInf3", b2i(cmplx.IsInf(1+1i)), 0)
	chkint("IsNaN1", b2i(cmplx.IsNaN(cmplx.NaN())), 1)
	chkint("IsNaN2", b2i(cmplx.IsNaN(1+1i)), 0)
	chkint("IsNaN3", b2i(cmplx.IsNaN(cmplx.Sqrt(-1))), 0)

	println("bad:", bad)
}
