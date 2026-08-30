package main

import "fmt"

func main() {
	bad := 0
	chk := func(name string, got, want string) {
		if got != want {
			println("FAIL " + name)
			bad++
		}
	}

	// Sprintf with the common verbs over basic operands.
	chk("d", fmt.Sprintf("%d", 42), "42")
	chk("d-neg", fmt.Sprintf("%d", -7), "-7")
	chk("s", fmt.Sprintf("%s", "hello"), "hello")
	chk("f", fmt.Sprintf("%.2f", 3.14159), "3.14")
	chk("t", fmt.Sprintf("%t", true), "true")
	chk("q", fmt.Sprintf("%q", "a\nb"), `"a\nb"`)
	chk("x", fmt.Sprintf("%x", 255), "ff")
	chk("X", fmt.Sprintf("%X", 255), "FF")
	chk("o", fmt.Sprintf("%o", 8), "10")
	chk("b", fmt.Sprintf("%b", 5), "101")
	chk("c", fmt.Sprintf("%c", 65), "A")

	// Padding and width.
	chk("pad-r", fmt.Sprintf("%5d", 42), "   42")
	chk("pad-l", fmt.Sprintf("%-5d|", 42), "42   |")
	chk("pad-s", fmt.Sprintf("%10s|", "hi"), "        hi|")

	// Multiple operands / mixing.
	chk("mixed", fmt.Sprintf("x=%d y=%s", 1, "z"), "x=1 y=z")

	// %T over basic operands.
	chk("T-int", fmt.Sprintf("%T", 42), "int")
	chk("T-str", fmt.Sprintf("%T", "s"), "string")
	chk("T-flt", fmt.Sprintf("%T", 3.5), "float64")
	chk("T-bool", fmt.Sprintf("%T", true), "bool")

	// Sprint/Sprintln/Errorf.
	chk("sp", fmt.Sprint("a", 1, 2.5), "a1 2.5")
	chk("sln", fmt.Sprintln(1, 2), "1 2\n")
	chk("err", fmt.Errorf("e%d", 7).Error(), "e7")

	// Direct output: Print and Println spacing behavior. The equivalence check
	// compares this byte-for-byte against go run.
	fmt.Print(1, 2, 3.5, "x")
	fmt.Println()
	fmt.Print("a", 1, "b", 2)
	fmt.Println()
	fmt.Print("x", "y", "z")
	fmt.Println()
	fmt.Println(1, 2, 3)
	fmt.Println("done", 3.14, true)

	println("bad:", bad)
}
