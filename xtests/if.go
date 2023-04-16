package main

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
)

func Test1() int {
	count := 0
	if true {
		count = count + 1
	}
	return count
}

func Test2() int {
	count := 0
	if false {
		count = count + 1
	}
	return count
}

func Test3() int {
	count := 0
	if one := 1; true {
		count = count + one
	}
	return count
}

func Test4() int {
	count := 0
	if one := 1; false {
		count = count + 1
		_ = one
	}
	return count
}

func Test5() int {
	i5 := 5
	i7 := 7
	count := 0
	if i5 < i7 {
		count = count + 1
	}
	return count
}

func Test6() int {
	count := 0
	if true {
		count = count + 1
	} else {
		count = count - 1
	}
	return count
}

func Test7() int {
	count := 0
	if false {
		count = count + 1
	} else {
		count = count - 1
	}
	return count
}

func Test8() int {
	count := 0
	if t := 1; false {
		count = count + 1
		_ = t
		t := 7
		_ = t
	} else {
		count = count - t
	}
	return count
}

func Test9() int {
	count := 0
	t := 1
	if false {
		count = count + 1
		t := 7
		_ = t
	} else {
		count = count - t
	}
	return count
}

func Test10() int {
	a := false
	b := !a
	if b {
		return 10
	}
	return 0
}

func main() {
	runTest := func(test func() int) {
		funcFullName := runtime.FuncForPC(reflect.ValueOf(test).Pointer()).Name()
		funcName := strings.Split(funcFullName, ".")[1]
		fmt.Printf("%s: %d\n", funcName, test())
	}
	runTest(Test1)
	runTest(Test2)
	runTest(Test3)
	runTest(Test4)
	runTest(Test5)
	runTest(Test6)
	runTest(Test7)
	runTest(Test8)
	runTest(Test9)
	runTest(Test10)
}
