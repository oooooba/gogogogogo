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

	// Exit must terminate the process before any subsequent code runs.
	os.Exit(0)
	println("FAIL reached-after-exit")
}
