package main

import (
	"reflect"
	"runtime"
	"strings"
)

func Test1() int {
	var m map[int]int
	if m != nil {
		return 0
	}
	if len(m) != 0 {
		return 2
	}
	return 1
}

func Test2() int {
	m := make(map[int]int)
	if m == nil {
		return 0
	}
	if len(m) != 0 {
		return 1
	}
	return 2
}

func Test3() int {
	m := make(map[int]int)
	m[3] = 42
	if len(m) != 1 {
		return 0
	}
	if m[3] != 42 {
		return 1
	}
	return 3
}

func Test4() int {
	m := make(map[int]int)
	v, ok := m[4]
	if ok {
		return 0
	}
	if v != 0 {
		return 1
	}
	m[4] = 42
	v2, ok2 := m[4]
	if !ok2 {
		return 2
	}
	if v2 != 42 {
		return 3
	}
	return 4
}

func Test5() int {
	m := map[int]int{1: 10, 2: 20, 3: 30}
	if len(m) != 3 {
		return 0
	}
	if m[1] != 10 {
		return 1
	}
	if m[2] != 20 {
		return 2
	}
	if m[3] != 30 {
		return 3
	}
	return 5
}

func Test6() int {
	m := map[int]int{1: 10, 2: 20, 3: 30}
	var found [4]bool
	for k := range m {
		if !(k == 1 || k == 2 || k == 3) {
			return k * 10
		}
		if found[k] {
			return k * 100
		}
		found[k] = true
	}
	if found[0] {
		return 0
	}
	for i := 1; i < 4; i++ {
		if !found[i] {
			return i * 1000
		}
	}
	return 6
}

func Test7() int {
	m := map[int]int{1: 10, 2: 20, 3: 30}
	var found [4]bool
	for k, v := range m {
		if !(k == 1 || k == 2 || k == 3) {
			return k * 10
		}
		if found[k] {
			return k * 100
		}
		if k*10 != v {
			return k * 100
		}
		found[k] = true
	}
	if found[0] {
		return 0
	}
	for i := 1; i < 4; i++ {
		if !found[i] {
			return i * 1000
		}
	}
	return 7
}

type S struct {
	x int
	y int
}

func Test8() int {
	m := map[S]S{}
	if len(m) != 0 {
		return 0
	}
	s := S{x: 1, y: 2}
	m[s] = s
	if len(m) != 1 {
		return 1
	}
	if m[s] != s {
		return 2
	}
	if m[s].x != 1 {
		return 3
	}
	if m[s].y != 2 {
		return 4
	}
	return 8
}

func main() {
	runTest := func(test func() int) {
		funcFullName := runtime.FuncForPC(reflect.ValueOf(test).Pointer()).Name()
		funcName := strings.Split(funcFullName, ".")[1]
		println(funcName+":", test())
	}
	runTest(Test1)
	runTest(Test2)
	runTest(Test3)
	runTest(Test4)
	runTest(Test5)
	runTest(Test6)
	runTest(Test7)
	runTest(Test8)
}
