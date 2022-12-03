package main

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
)

func Test1() int {
	var a [10]int
	s := a[:]
	s[0] = 1
	return a[0]
}

func Test2() int {
	var a [10]int
	s := a[2:]
	s[1] = 2
	return a[3]
}

func Test3() int {
	var a [10]int
	s := a[:7]
	s[2] = 3
	return a[2]
}

func Test4() int {
	var a [10]int
	s := a[2:7]
	s[3] = 4
	return a[5]
}

func Test5() int {
	var a [10]int
	s := a[2:7]
	return len(s)
}

func Test6() int {
	var a [10]int
	s := a[2:7]
	return cap(s)
}

func Test7() int {
	var a [5]int
	s := a[:]
	for i := 0; i < len(s); i++ {
		s[i] = 1
	}
	t := s
	for i := 0; i < len(t); i++ {
		t[i] = 2
	}
	sum := 0
	for i := 0; i < len(s); i++ {
		sum += s[i]
	}
	return sum
}

func Test8() int {
	var a [5]int
	s := a[:]
	for i := 0; i < len(s); i++ {
		s[i] = i
	}
	t := s
	for i := 0; i < len(t); i++ {
		if t[i] != i {
			return 0
		}
	}
	return 8
}

func Test9() int {
	var s []int
	if s != nil {
		return 0
	}
	if len(s) != 0 {
		return 1
	}
	if cap(s) != 0 {
		return 2
	}
	return 9
}

func Test10() int {
	s := []int{}
	if s == nil {
		return 0
	}
	if len(s) != 0 {
		return 1
	}
	if cap(s) != 0 {
		return 2
	}
	return 10
}

func Test11() int {
	s := []int{}
	s = append(s)
	if s == nil {
		return 0
	}
	if len(s) != 0 {
		return len(s)
	}
	if cap(s) != 0 {
		return 2
	}
	return 11
}

func Test12() int {
	s := []int{}
	s = append(s, 12)
	if s == nil {
		return 0
	}
	if len(s) != 1 {
		return 1
	}
	if cap(s) == 0 {
		return 2
	}
	return s[0]
}

func Test13() int {
	var a [0]int
	s := a[:]
	s = append(s)
	if s == nil {
		return 0
	}
	if len(s) != 0 {
		return 1
	}
	return 13
}

func Test14() int {
	var a [0]int
	s := a[:]
	s = append(s, 14)
	if s == nil {
		return 0
	}
	if len(s) != 1 {
		return 1
	}
	if cap(s) == 0 {
		return 2
	}
	return s[0]
}

func Test15() int {
	for i := 0; i < 10; i++ {
		var s []int
		if s != nil {
			return 0
		}
		if len(s) != 0 {
			return 1
		}
		if cap(s) != 0 {
			return 2
		}

		s = append(s, i)

		if s == nil {
			return 3
		}
		if len(s) != 1 {
			return 4
		}
		if cap(s) <= 0 {
			return 5
		}
	}
	return 15
}

func Test16() int {
	s := [][]int{
		{},
		{1},
		{2, 3},
	}
	if len(s) != 3 {
		return 1
	}
	if len(s[0]) != 0 {
		return 2
	}
	if len(s[1]) != 1 {
		return 3
	}
	if s[1][0] != 1 {
		return 4
	}
	if len(s[2]) != 2 {
		return 5
	}
	if s[2][0] != 2 {
		return 6
	}
	if s[2][1] != 3 {
		return 7
	}
	return 16
}

func Test17() int {
	s := [][]int{
		{},
		{1},
		{2, 3},
	}
	t := [][]int{
		{},
		{4},
		{5, 6},
	}
	for i := 0; i < len(s); i++ {
		for j := 0; j < len(s[i]); j++ {
			t[i][j] = s[i][j]
		}
	}
	for i := 0; i < len(s); i++ {
		for j := 0; j < len(s[i]); j++ {
			if t[i][j] != s[i][j] {
				return 0
			}
		}
	}
	return 17
}

func Test18() int {
	s := []int{1, 2, 3}
	f := func(t []int) {
		for i, x := range t {
			t[i] = x + x
		}
	}
	f(s)
	if s[0] != 2 {
		return 1
	}
	if s[1] != 4 {
		return 2
	}
	if s[2] != 6 {
		return 3
	}
	return 18
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
	runTest(Test11)
	runTest(Test12)
	runTest(Test13)
	runTest(Test14)
	runTest(Test15)
	runTest(Test16)
	runTest(Test17)
	runTest(Test18)
}
