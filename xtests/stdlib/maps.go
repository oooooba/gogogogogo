package main

import (
	"iter"
	"maps"
)

func TestEqual() int {
	m1 := map[string]int{"a": 1, "b": 2}
	m2 := map[string]int{"a": 1, "b": 2}
	if !maps.Equal(m1, m2) {
		return 1
	}
	m2["b"] = 3
	if maps.Equal(m1, m2) {
		return 1
	}
	m3 := map[string]int{"a": 1}
	if maps.Equal(m1, m3) {
		return 1
	}
	if !maps.Equal(m1, m1) {
		return 1
	}
	var nilMap map[string]int
	if !maps.Equal(nilMap, map[string]int{}) {
		return 1
	}
	if !maps.Equal(nilMap, nilMap) {
		return 1
	}
	return 0
}

func TestEqualFunc() int {
	m1 := map[string]int{"a": 1, "b": 22}
	m2 := map[string]int{"a": 1, "b": 2}
	eqLastDigit := func(a, b int) bool { return a%10 == b%10 }
	if !maps.EqualFunc(m1, m2, eqLastDigit) {
		return 1
	}
	if !maps.EqualFunc(m2, m1, eqLastDigit) {
		return 1
	}
	if maps.EqualFunc(m1, m2, func(a, b int) bool { return a == b }) {
		return 1
	}
	return 0
}

func TestClone() int {
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	c := maps.Clone(m)
	if len(c) != 3 || !maps.Equal(m, c) {
		return 1
	}
	c["d"] = 4
	if len(m) != 3 || maps.Equal(m, c) {
		return 1
	}
	delete(c, "a")
	if _, ok := m["a"]; !ok {
		return 1
	}
	var nilMap map[string]int
	nc := maps.Clone(nilMap)
	if nc != nil || len(nc) != 0 {
		return 1
	}
	empty := map[string]int{}
	ec := maps.Clone(empty)
	if ec == nil || !maps.Equal(empty, ec) {
		return 1
	}
	return 0
}

func TestCopy() int {
	dst := map[string]int{"a": 1}
	src := map[string]int{"a": 10, "b": 20}
	maps.Copy(dst, src)
	if len(dst) != 2 || dst["a"] != 10 || dst["b"] != 20 {
		return 1
	}
	maps.Copy(dst, map[string]int{})
	if len(dst) != 2 {
		return 1
	}
	maps.Copy(dst, map[string]int{"c": 30})
	if len(dst) != 3 || dst["c"] != 30 {
		return 1
	}
	return 0
}

func TestDeleteFunc() int {
	m := map[string]int{"a": 1, "b": 2, "c": 3, "d": 4}
	maps.DeleteFunc(m, func(k string, v int) bool { return v%2 == 0 })
	if len(m) != 2 {
		return 1
	}
	for _, k := range []string{"a", "c"} {
		if _, ok := m[k]; !ok {
			return 1
		}
	}
	for _, k := range []string{"b", "d"} {
		if _, ok := m[k]; ok {
			return 1
		}
	}
	return 0
}

func TestAll() int {
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	count := 0
	sum := 0
	for k, v := range maps.All(m) {
		count++
		if k == "a" {
			sum += v
		}
	}
	if count != 3 || sum != 1 {
		return 1
	}
	return 0
}

func TestKeys() int {
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	count := 0
	for k := range maps.Keys(m) {
		if k != "a" && k != "b" && k != "c" {
			return 1
		}
		count++
	}
	if count != 3 {
		return 1
	}
	return 0
}

func TestValues() int {
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	sum := 0
	count := 0
	for v := range maps.Values(m) {
		sum += v
		count++
	}
	if count != 3 || sum != 6 {
		return 1
	}
	return 0
}

func TestInsert() int {
	m := map[string]int{"a": 1}
	maps.Insert(m, maps.All(map[string]int{"b": 2, "c": 3}))
	if len(m) != 3 || m["a"] != 1 || m["b"] != 2 || m["c"] != 3 {
		return 1
	}
	maps.Insert(m, maps.All(map[string]int{"a": 10, "d": 4}))
	if len(m) != 4 || m["a"] != 10 || m["d"] != 4 {
		return 1
	}
	return 0
}

func TestCollect() int {
	src := map[string]int{"a": 1, "b": 2}
	m := maps.Collect(maps.All(src))
	if len(m) != 2 || m["a"] != 1 || m["b"] != 2 {
		return 1
	}
	if !maps.Equal(m, src) {
		return 1
	}
	m2 := maps.Collect(func(yield func(string, int) bool) {})
	if len(m2) != 0 {
		return 1
	}
	return 0
}

func TestAllWithPull2() int {
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	next, stop := iter.Pull2(maps.All(m))
	defer stop()
	sum := 0
	count := 0
	for {
		k, v, ok := next()
		if !ok {
			break
		}
		if k == "a" || k == "b" || k == "c" {
			sum += v
		}
		count++
	}
	if count != 3 || sum != 6 {
		return 1
	}
	return 0
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("TestEqual", TestEqual)
	runTest("TestEqualFunc", TestEqualFunc)
	runTest("TestClone", TestClone)
	runTest("TestCopy", TestCopy)
	runTest("TestDeleteFunc", TestDeleteFunc)
	runTest("TestAll", TestAll)
	runTest("TestKeys", TestKeys)
	runTest("TestValues", TestValues)
	runTest("TestInsert", TestInsert)
	runTest("TestCollect", TestCollect)
	runTest("TestAllWithPull2", TestAllWithPull2)
}
