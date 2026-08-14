package main

import (
	"bufio"
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

	r := bufio.NewReader(strings.NewReader("hello world"))
	p, err := r.Peek(5)
	chk("Peek", string(p), "hello")
	chkb("PeekErr", err == nil, true)

	r = bufio.NewReader(strings.NewReader("hello world"))
	b, err := r.ReadBytes(' ')
	chk("ReadBytes", string(b), "hello ")
	chkb("ReadBytesErr", err == nil, true)

	r = bufio.NewReader(strings.NewReader("line1\nline2\n"))
	s, err := r.ReadString('\n')
	chk("ReadString1", s, "line1\n")
	chkb("ReadString1Err", err == nil, true)
	s, err = r.ReadString('\n')
	chk("ReadString2", s, "line2\n")
	chkb("ReadString2Err", err == nil, true)

	r = bufio.NewReader(strings.NewReader("no-newline"))
	s, err = r.ReadString('\n')
	chk("ReadStringEof", s, "no-newline")
	chkb("ReadStringEofErr", err == io.EOF, true)

	r = bufio.NewReader(strings.NewReader("abc"))
	sl, err := r.ReadSlice('\n')
	chk("ReadSlice", string(sl), "abc")
	chkb("ReadSliceErr", err == io.EOF, true)

	r = bufio.NewReader(strings.NewReader("héllo"))
	ru, size, err := r.ReadRune()
	chki("ReadRune", int(ru), int('h'))
	chki("ReadRuneSize", size, 1)
	chkb("ReadRuneErr", err == nil, true)
	r.UnreadRune()
	ru2, _, _ := r.ReadRune()
	chki("UnreadRune", int(ru2), int('h'))

	r = bufio.NewReader(strings.NewReader("hello"))
	line, isPref, err := r.ReadLine()
	chk("ReadLine", string(line), "hello")
	chkb("ReadLineIsPref", isPref, false)
	chkb("ReadLineErr", err == nil, true)

	r = bufio.NewReader(strings.NewReader("a"))
	r.ReadByte()
	r.UnreadByte()
	c, _ := r.ReadByte()
	chk("UnreadByte", string(rune(c)), "a")

	r = bufio.NewReader(strings.NewReader("hello world"))
	m, err := r.Discard(2)
	chki("DiscardN", m, 2)
	chkb("DiscardErr", err == nil, true)
	chki("Buffered", r.Buffered(), 9)

	r = bufio.NewReaderSize(strings.NewReader("abcdefghijklmnopqrstuvwxyz"), 16)
	_, err = r.ReadSlice(' ')
	chkb("ErrBufferFull", err == bufio.ErrBufferFull, true)

	var wb bytes.Buffer
	bw := bufio.NewWriter(&wb)
	wn, err := bw.WriteString("foo")
	chki("WriteStringN", wn, 3)
	chkb("WriteStringErr", err == nil, true)
	chki("WriterBuffered", bw.Buffered(), 3)
	bw.WriteByte('-')
	bw.WriteRune('x')
	bw.Flush()
	chk("WriterFlush", wb.String(), "foo-x")
	chki("WriterAvail", bw.Available(), 4096)

	wb.Reset()
	bw = bufio.NewWriterSize(&wb, 4)
	bw.WriteString("abcd")
	chki("WriterSizeBuffered", bw.Buffered(), 4)
	chki("WriterSizeAvail", bw.Available(), 0)
	bw.Flush()
	chk("WriterSizeFlush", wb.String(), "abcd")

	sc := bufio.NewScanner(strings.NewReader("a\nbb\nccc\n"))
	n := 0
	var lines []string
	for sc.Scan() {
		n++
		lines = append(lines, sc.Text())
	}
	chki("ScanLinesN", n, 3)
	chk("ScanLines1", lines[0], "a")
	chk("ScanLines3", lines[2], "ccc")
	chkb("ScanErr", sc.Err() == nil, true)

	sc = bufio.NewScanner(strings.NewReader("hello world foo"))
	sc.Split(bufio.ScanWords)
	n = 0
	for sc.Scan() {
		n++
	}
	chki("ScanWordsN", n, 3)

	sc = bufio.NewScanner(strings.NewReader("byebye"))
	sc.Split(bufio.ScanBytes)
	n = 0
	for sc.Scan() {
		chk("ScanBytes", sc.Text(), string("byebye"[n]))
		n++
	}
	chki("ScanBytesN", n, 6)

	sc = bufio.NewScanner(strings.NewReader("aaa bbb"))
	sc.Split(bufio.ScanRunes)
	n = 0
	for sc.Scan() {
		n++
	}
	chki("ScanRunesN", n, 7)

	sc = bufio.NewScanner(strings.NewReader("a very long line"))
	sc.Buffer(make([]byte, 4), 4)
	for sc.Scan() {
	}
	chkb("ErrTooLong", sc.Err() == bufio.ErrTooLong, true)

	sr := strings.NewReader("hello world")
	br := bufio.NewReaderSize(sr, 16)
	bb, err := br.Peek(5)
	chk("PeekSize", string(bb), "hello")
	chkb("PeekSizeErr", err == nil, true)

	rw := bufio.NewReadWriter(bufio.NewReader(strings.NewReader("hi")), bufio.NewWriter(&bytes.Buffer{}))
	cbyte, err := rw.ReadByte()
	chki("ReadWriterRead", int(cbyte), int('h'))
	chkb("ReadWriterReadErr", err == nil, true)

	chk("ErrInvalidUnreadByte", bufio.ErrInvalidUnreadByte.Error(), "bufio: invalid use of UnreadByte")
	chk("ErrInvalidUnreadRune", bufio.ErrInvalidUnreadRune.Error(), "bufio: invalid use of UnreadRune")
	chk("ErrNegativeCount", bufio.ErrNegativeCount.Error(), "bufio: negative count")

	println("bad:", bad)
}
