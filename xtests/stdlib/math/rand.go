package main

import (
	"math/rand"
)

type seqSource struct {
	n int64
}

func (s *seqSource) Int63() int64 {
	s.n = (s.n*1103515245 + 12345) & 0x7fffffff
	return s.n
}

func (s *seqSource) Seed(seed int64) { s.n = seed }

func main() {
	bad := 0
	chki := func(name string, got, want int) {
		if got != want {
			println("FAIL "+name+": got=", got, " want=", want)
			bad++
		}
	}

	src := rand.NewSource(1)
	if _, ok := src.(rand.Source64); !ok {
		println("FAIL Source64Assert")
		bad++
	}
	r := rand.New(src)

	chki("Int63_0", int(r.Int63()), 5577006791947779410)
	chki("Int63_1", int(r.Int63()), 8674665223082153551)
	chki("Int63_2", int(r.Int63()), 6129484611666145821)
	chki("Uint32_0", int(r.Uint32()), 1879968118)
	chki("Uint32_1", int(r.Uint32()), 1823804162)
	chki("Uint64_0", int(r.Uint64()), 6334824724549167320)
	chki("Uint64_1", int(r.Uint64()), -8617977389221806050)
	chki("Int31_0", int(r.Int31()), 336122540)
	chki("Int31_1", int(r.Int31()), 208240456)
	chki("Int_0", int(r.Int()), 2775422040480279449)
	chki("Int_1", int(r.Int()), 4751997750760398084)
	chki("Int63n100", int(r.Int63n(100)), 87)
	chki("Int31n100", int(r.Int31n(100)), 62)
	chki("Int31n70", int(r.Int31n(70)), 59)
	chki("Intn100", int(r.Intn(100)), 28)
	chki("Intn50", int(r.Intn(50)), 24)
	chki("Float64_0", int(r.Float64()*1e9), 283034151)
	chki("Float64_1", int(r.Float64()*1e9), 293101857)
	chki("Float64_2", int(r.Float64()*1e9), 679084675)
	chki("Float64_3", int(r.Float64()*1e9), 218553052)
	chki("Float32_0", int(r.Float32()*1e9), 203186864)
	chki("Float32_1", int(r.Float32()*1e9), 360871392)
	chki("Float32_2", int(r.Float32()*1e9), 570673280)

	perm := r.Perm(8)
	chki("Perm0", perm[0], 5)
	chki("Perm1", perm[1], 2)
	chki("Perm2", perm[2], 6)
	chki("Perm3", perm[3], 4)
	chki("Perm4", perm[4], 3)
	chki("Perm5", perm[5], 7)
	chki("Perm6", perm[6], 1)
	chki("Perm7", perm[7], 0)

	sh := []int{0, 1, 2, 3, 4, 5, 6, 7}
	r.Shuffle(8, func(i, j int) { sh[i], sh[j] = sh[j], sh[i] })
	chki("Shuffle0", sh[0], 2)
	chki("Shuffle1", sh[1], 5)
	chki("Shuffle2", sh[2], 6)
	chki("Shuffle3", sh[3], 7)
	chki("Shuffle4", sh[4], 4)
	chki("Shuffle5", sh[5], 3)
	chki("Shuffle6", sh[6], 1)
	chki("Shuffle7", sh[7], 0)

	buf := make([]byte, 8)
	n, _ := r.Read(buf)
	chki("ReadN", n, 8)
	chki("ReadByte0", int(buf[0]), 244)
	chki("ReadByte1", int(buf[1]), 69)
	chki("ReadByte2", int(buf[2]), 209)
	chki("ReadByte3", int(buf[3]), 90)
	chki("ReadByte4", int(buf[4]), 253)
	chki("ReadByte5", int(buf[5]), 66)
	chki("ReadByte6", int(buf[6]), 148)
	chki("ReadByte7", int(buf[7]), 4)
	r.Seed(99)
	chki("AfterSeed", int(r.Int63()), 2431074399724039541)

	chki("NormFloat64_0", int(r.NormFloat64()*1e4), -10968)
	chki("NormFloat64_1", int(r.NormFloat64()*1e4), -3417)
	chki("NormFloat64_2", int(r.NormFloat64()*1e4), -6788)
	chki("ExpFloat64_0", int(r.ExpFloat64()*1e4), 4856)
	chki("ExpFloat64_1", int(r.ExpFloat64()*1e4), 8056)
	chki("ExpFloat64_2", int(r.ExpFloat64()*1e4), 3362)

	z := rand.NewZipf(r, 2.5, 3, 100)
	chki("Zipf0", int(z.Uint64()), 2)
	chki("Zipf1", int(z.Uint64()), 5)
	chki("Zipf2", int(z.Uint64()), 8)
	chki("Zipf3", int(z.Uint64()), 2)
	chki("Zipf4", int(z.Uint64()), 44)

	rc := rand.New(&seqSource{1})
	chki("CSource_Int63_0", int(rc.Int63()), 1103527590)
	chki("CSource_Int63_1", int(rc.Int63()), 377401575)
	chki("CSource_Int63_2", int(rc.Int63()), 662824084)
	chki("CSource_Float64_0", int(rc.Float64()*1e9), 0)
	chki("CSource_Float64_1", int(rc.Float64()*1e9), 0)
	cp := rc.Perm(5)
	chki("CSource_Perm0", cp[0], 4)
	chki("CSource_Perm1", cp[1], 0)
	chki("CSource_Perm2", cp[2], 1)
	chki("CSource_Perm3", cp[3], 2)
	chki("CSource_Perm4", cp[4], 3)

	println("bad:", bad)
}
