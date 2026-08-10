package main

import "sort"

type byName struct {
	name string
	age  int
}

type byAge []byName

func (s byAge) Len() int           { return len(s) }
func (s byAge) Less(i, j int) bool { return s[i].age < s[j].age }
func (s byAge) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func TestSortInts() int {
	s := []int{5, 2, 6, 3, 1, 4}
	sort.Ints(s)
	expected := []int{1, 2, 3, 4, 5, 6}
	for i := range expected {
		if s[i] != expected[i] {
			return 1
		}
	}
	return 0
}

func TestIntsAreSorted() int {
	if !sort.IntsAreSorted([]int{1, 2, 3}) {
		return 1
	}
	if sort.IntsAreSorted([]int{1, 3, 2}) {
		return 2
	}
	if !sort.IntsAreSorted([]int{}) {
		return 3
	}
	return 0
}

func TestStrings() int {
	s := []string{"pear", "apple", "banana"}
	sort.Strings(s)
	expected := []string{"apple", "banana", "pear"}
	for i := range expected {
		if s[i] != expected[i] {
			return 1
		}
	}
	if !sort.StringsAreSorted(s) {
		return 2
	}
	return 0
}

func TestFloat64s() int {
	s := []float64{1.5, -2.5, 3.5, 0.5}
	sort.Float64s(s)
	expected := []float64{-2.5, 0.5, 1.5, 3.5}
	for i := range expected {
		if s[i] != expected[i] {
			return 1
		}
	}
	if !sort.Float64sAreSorted(s) {
		return 2
	}
	return 0
}

func TestSortInterface() int {
	s := byAge{{name: "c", age: 3}, {name: "a", age: 1}, {name: "b", age: 2}}
	sort.Sort(s)
	if s[0].name != "a" || s[1].name != "b" || s[2].name != "c" {
		return 1
	}
	if !sort.IsSorted(s) {
		return 2
	}
	return 0
}

func TestReverse() int {
	s := byAge{{name: "c", age: 3}, {name: "a", age: 1}, {name: "b", age: 2}}
	sort.Sort(sort.Reverse(s))
	if s[0].name != "c" || s[1].name != "b" || s[2].name != "a" {
		return 1
	}
	return 0
}

type pair struct {
	key int
	val string
}

type pairs []pair

func (p pairs) Len() int           { return len(p) }
func (p pairs) Less(i, j int) bool { return p[i].key < p[j].key }
func (p pairs) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func TestStable() int {
	s := pairs{{key: 1, val: "x"}, {key: 1, val: "y"}, {key: 0, val: "z"}}
	sort.Stable(s)
	if s[0].val != "z" || s[1].val != "x" || s[2].val != "y" {
		return 1
	}
	if s[1].key != 1 || s[2].key != 1 {
		return 2
	}
	return 0
}

func TestIntSlice() int {
	s := sort.IntSlice{3, 1, 2}
	sort.Sort(s)
	expected := []int{1, 2, 3}
	for i := range expected {
		if s[i] != expected[i] {
			return 1
		}
	}
	return 0
}

func TestSearch() int {
	s := []int{1, 2, 3, 4, 5}
	idx := sort.Search(len(s), func(i int) bool { return s[i] >= 3 })
	if idx != 2 {
		return 1
	}
	idx = sort.Search(len(s), func(i int) bool { return s[i] >= 99 })
	if idx != len(s) {
		return 2
	}
	return 0
}

func TestSearchInts() int {
	s := []int{1, 2, 3, 4, 5}
	if sort.SearchInts(s, 3) != 2 {
		return 1
	}
	if sort.SearchInts(s, 0) != 0 {
		return 2
	}
	if sort.SearchInts(s, 6) != 5 {
		return 3
	}
	return 0
}

func TestSearchStrings() int {
	s := []string{"a", "b", "c"}
	if sort.SearchStrings(s, "b") != 1 {
		return 1
	}
	if sort.SearchStrings(s, "d") != 3 {
		return 2
	}
	return 0
}

func main() {
	runTest := func(testName string, test func() int) {
		println(testName+":", test())
	}
	runTest("TestSortInts", TestSortInts)
	runTest("TestIntsAreSorted", TestIntsAreSorted)
	runTest("TestStrings", TestStrings)
	runTest("TestFloat64s", TestFloat64s)
	runTest("TestSortInterface", TestSortInterface)
	runTest("TestReverse", TestReverse)
	runTest("TestStable", TestStable)
	runTest("TestIntSlice", TestIntSlice)
	runTest("TestSearch", TestSearch)
	runTest("TestSearchInts", TestSearchInts)
	runTest("TestSearchStrings", TestSearchStrings)
}
