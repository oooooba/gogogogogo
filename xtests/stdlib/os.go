package main

import (
	"io"
	"os"
)

func main() {
	bad := 0
	chkb := func(name string, got, want bool) {
		if got != want {
			println("FAIL " + name)
			bad++
		}
	}
	chki := func(name string, got, want int) {
		if got != want {
			println("FAIL " + name)
			bad++
		}
	}
	chk := func(name string, got, want string) {
		if got != want {
			println("FAIL " + name)
			bad++
		}
	}

	// Stdin, Stdout and Stderr are pre-initialized *os.File values that
	// correspond to the standard streams.
	chkb("stdin-nonnil", os.Stdin != nil, true)
	chkb("stdout-nonnil", os.Stdout != nil, true)
	chkb("stderr-nonnil", os.Stderr != nil, true)
	chkb("stdout-same", os.Stdout == os.Stdout, true)
	chkb("stdin-not-stdout", os.Stdin != os.Stdout, true)
	chki("stdin-fd", int(os.Stdin.Fd()), 0)
	chki("stdout-fd", int(os.Stdout.Fd()), 1)
	chki("stderr-fd", int(os.Stderr.Fd()), 2)
	chk("stdin-name", os.Stdin.Name(), "/dev/stdin")
	chk("stdout-name", os.Stdout.Name(), "/dev/stdout")
	chk("stderr-name", os.Stderr.Name(), "/dev/stderr")

	// NewFile constructs a *os.File from an explicit file descriptor and name.
	f := os.NewFile(3, "myfile")
	chkb("newfile-nonnil", f != nil, true)
	chki("newfile-fd", int(f.Fd()), 3)
	chk("newfile-name", f.Name(), "myfile")

	// Write writes len(p) bytes to the underlying file descriptor and returns
	// the number of bytes written. os.Stdout/Stderr map to real descriptors.
	n, err := os.Stderr.Write([]byte("write-test\n"))
	chki("stderr-write-n", n, 11)
	chkb("stderr-write-err", err != nil, false)

	// An empty slice writes nothing and returns 0 with a nil error.
	n, err = os.Stdout.Write(nil)
	chki("stdout-write-nil-n", n, 0)
	chkb("stdout-write-nil-err", err != nil, false)

	// Write again returns the length of the second slice.
	n, err = os.Stdout.Write([]byte("xyz"))
	chki("stdout-write-n", n, 3)
	chkb("stdout-write-err", err != nil, false)

	// Read transfers bytes from the underlying file descriptor. The test
	// harness does not feed stdin, so drain whatever is available until EOF,
	// then verify that a read past EOF returns 0 bytes with io.EOF. This works
	// whether the input stream is empty or contains data.
	rbuf := make([]byte, 16)
	total := 0
	nothang := 0
	for {
		rn, rerr := os.Stdin.Read(rbuf)
		total += rn
		if rerr == io.EOF {
			chkb("stdin-read-eof-zero-byte", rn == 0, true)
			break
		}
		if rerr != nil {
			chkb("stdin-read-error", rerr == io.EOF, true)
			break
		}
		nothang++
		if nothang > 1024 {
			chkb("stdin-read-too-many", false, true)
			break
		}
	}
	chkb("stdin-read-nonnegative", total >= 0, true)

	// A read past EOF yields 0 bytes and io.EOF.
	n2, e2 := os.Stdin.Read(rbuf)
	chki("stdin-eof-read-n", n2, 0)
	chkb("stdin-eof-err-is-eof", e2 == io.EOF, true)

	// os.Open opens a file for reading. Create a temporary file with known
	// content, reopen it via os.Open, and verify the bytes read back match.
	// The relative path resolves the same in both the C-generated binary and
	// the Go compiler when run from the repository root.
	ofileName := "tmp/os_open_test.txt"
	of, oerr := os.Create(ofileName)
	chkb("open-create-err", oerr != nil, false)
	ofdata := "os.Open round-trip\n"
	own, owerr := of.Write([]byte(ofdata))
	chki("open-create-write-n", own, len(ofdata))
	chkb("open-create-write-err", owerr != nil, false)
	of.Close()

	rf, err := os.Open(ofileName)
	chkb("open-nonnil-file", rf != nil, true)
	chkb("open-err-nil", err != nil, false)
	opbuf := make([]byte, 128)
	opgot := ""
	for {
		opn, ope := rf.Read(opbuf)
		opgot += string(opbuf[:opn])
		if ope == io.EOF {
			break
		}
		if ope != nil {
			chkb("open-read-error-is-eof", ope == io.EOF, true)
			break
		}
	}
	chk("open-read-content", opgot, ofdata)
	rf.Close()
	os.Remove(ofileName)

	// os.Pipe creates a connected pair of Files; reads from r return the bytes
	// written to w. Closing the write end makes the reader see EOF.
	pr, pw, perr := os.Pipe()
	chkb("pipe-nonnil-r", pr != nil, true)
	chkb("pipe-nonnil-w", pw != nil, true)
	chkb("pipe-err", perr != nil, false)
	pn, pwerr := pw.Write([]byte("abcde"))
	chki("pipe-write-n", pn, 5)
	chkb("pipe-write-err", pwerr != nil, false)
	pbuf := make([]byte, 8)
	prn, prerr := pr.Read(pbuf)
	chkb("pipe-read-err", prerr != nil, false)
	chk("pipe-read-content", string(pbuf[:prn]), "abcde")
	pw.Close()
	prn2, prerr2 := pr.Read(pbuf)
	chki("pipe-eof-n", prn2, 0)
	chkb("pipe-eof-err", prerr2 == io.EOF, true)

	// os.File.Read follows the (n, err) contract: a read that fills the buffer
	// returns n == len(buf) with a nil error; the final partial read is
	// followed by a read of (0, io.EOF). The content length is chosen not to be
	// a multiple of the chunk so the partial-final-read path is exercised.
	const rdata = "0123456789abc" // 13 bytes, not a multiple of 5
	rfile, rcerr := os.Create("tmp/os_read_test.txt")
	chkb("read-create-err", rcerr != nil, false)
	_, rwerr := rfile.WriteString(rdata)
	chkb("read-write-err", rwerr != nil, false)
	rfile.Close()

	rrf, roerr := os.Open("tmp/os_read_test.txt")
	chkb("read-open-err", roerr != nil, false)
	rchunk := make([]byte, 5)
	rgot := ""
	rloop := 0
	for {
		rn, re := rrf.Read(rchunk)
		rgot += string(rchunk[:rn])
		if re == io.EOF {
			chkb("read-eof-n-zero", rn == 0, true)
			break
		}
		if re != nil {
			chkb("read-loop-error", re == io.EOF, true)
			break
		}
		rloop++
		if rloop > 16 {
			chkb("read-too-many", false, true)
			break
		}
	}
	chk("read-file-content", rgot, rdata)
	rrf.Close()
	os.Remove("tmp/os_read_test.txt")

	// Exit must terminate the process before any subsequent code runs.
	os.Exit(0)
	println("FAIL reached-after-exit")
}
