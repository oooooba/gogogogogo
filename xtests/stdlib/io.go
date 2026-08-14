package main

import (
	"bytes"
	"io"
	"strings"
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

	chkb("EOFError", io.EOF.Error() == "EOF", true)
	chkb("UnexpectedEOFError", io.ErrUnexpectedEOF.Error() == "unexpected EOF", true)
	chkb("ShortWriteError", io.ErrShortWrite.Error() == "short write", true)
	chkb("DistinctSentinels", io.EOF != io.ErrUnexpectedEOF, true)

	b, err := io.ReadAll(strings.NewReader("hello world"))
	chkb("ReadAllErr", err == nil, true)
	chk("ReadAll", string(b), "hello world")
	b, err = io.ReadAll(strings.NewReader(""))
	chk("ReadAllEmpty", string(b), "")
	chkb("ReadAllEmptyErr", err == nil, true)

	sr := strings.NewReader("abcdef")
	buf := make([]byte, 2)
	n, err := io.ReadFull(sr, buf)
	chki("ReadFullN", n, 2)
	chkb("ReadFullErr", err == nil, true)
	chk("ReadFull", string(buf), "ab")

	sr = strings.NewReader("abcd")
	buf4 := make([]byte, 4)
	n, err = io.ReadAtLeast(sr, buf4, 4)
	chki("ReadAtLeastN", n, 4)
	chkb("ReadAtLeastErr", err == nil, true)
	chk("ReadAtLeast", string(buf4), "abcd")

	var out bytes.Buffer
	nw, err := io.Copy(&out, strings.NewReader("hello"))
	chki("CopyN", int(nw), 5)
	chkb("CopyErr", err == nil, true)
	chk("Copy", out.String(), "hello")

	out.Reset()
	nw, err = io.Copy(&out, strings.NewReader(""))
	chki("CopyEmptyN", int(nw), 0)
	chkb("CopyEmptyErr", err == nil, true)
	chk("CopyEmpty", out.String(), "")

	nw, err = io.Copy(io.Discard, strings.NewReader("discard-me"))
	chki("CopyDiscardN", int(nw), 10)
	chkb("CopyDiscardErr", err == nil, true)

	nb, err := io.CopyN(io.Discard, strings.NewReader("hello"), 3)
	chki("CopyNN", int(nb), 3)
	chkb("CopyNErr", err == nil, true)

	var cb bytes.Buffer
	nw, err = io.CopyBuffer(&cb, strings.NewReader("buffered"), make([]byte, 4))
	chki("CopyBufferN", int(nw), 8)
	chkb("CopyBufferErr", err == nil, true)
	chk("CopyBuffer", cb.String(), "buffered")

	n, err = io.WriteString(&out, "abc")
	chki("WriteStringN", n, 3)
	chkb("WriteStringErr", err == nil, true)
	chk("WriteString", out.String(), "abc")

	lb, err := io.ReadAll(io.LimitReader(strings.NewReader("hello world"), 5))
	chk("LimitReader", string(lb), "hello")
	chkb("LimitReaderErr", err == nil, true)
	lb, err = io.ReadAll(io.LimitReader(strings.NewReader("hi"), 10))
	chk("LimitReaderOver", string(lb), "hi")

	b, err = io.ReadAll(io.NopCloser(strings.NewReader("x")))
	chk("NopCloser", string(b), "x")
	chkb("NopCloserErr", err == nil, true)

	b, err = io.ReadAll(io.MultiReader(strings.NewReader("ab"), strings.NewReader("cd")))
	chk("MultiReader", string(b), "abcd")
	chkb("MultiReaderErr", err == nil, true)

	var buf2 bytes.Buffer
	mw := io.MultiWriter(&buf2, &out)
	mwn, err := mw.Write([]byte("xy"))
	chki("MultiWriterN", mwn, 2)
	chkb("MultiWriterErr", err == nil, true)
	chk("MultiWriterFirst", buf2.String(), "xy")

	var buf3 bytes.Buffer
	b, err = io.ReadAll(io.TeeReader(strings.NewReader("tee"), &buf3))
	chk("TeeReaderRead", string(b), "tee")
	chk("TeeReaderTee", buf3.String(), "tee")
	chkb("TeeReaderErr", err == nil, true)

	b, err = io.ReadAll(io.NewSectionReader(strings.NewReader("0123456789"), 2, 4))
	chk("SectionReader", string(b), "2345")
	chkb("SectionReaderErr", err == nil, true)

	rs := io.NewSectionReader(strings.NewReader("0123456789"), 0, 10)
	off, err := rs.Seek(5, io.SeekStart)
	chki("SeekOffset", int(off), 5)
	chkb("SeekErr", err == nil, true)
	p := make([]byte, 2)
	rn, err := rs.Read(p)
	chki("SeekReadN", rn, 2)
	chkb("SeekReadErr", err == nil, true)
	chk("SeekRead", string(p), "56")

	pr, pw := io.Pipe()
	go func() {
		pw.Write([]byte("pipe-data"))
		pw.Close()
	}()
	b, err = io.ReadAll(pr)
	chk("PipeRead", string(b), "pipe-data")
	chkb("PipeReadErr", err == nil, true)

	pr2, pw2 := io.Pipe()
	pw2.Close()
	rbuf := make([]byte, 1)
	_, rerr := pr2.Read(rbuf)
	chkb("PipeClosedReadErr", rerr == io.EOF, true)

	println("bad:", bad)
}
