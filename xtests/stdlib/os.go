package main

import (
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

	// Exit must terminate the process before any subsequent code runs.
	os.Exit(0)
	println("FAIL reached-after-exit")
}
