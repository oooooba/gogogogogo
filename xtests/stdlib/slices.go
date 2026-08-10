package main

import "slices"

type kv struct{ k, v int }

func TestEqual() int {
	if !slices.Equal([]int{}, []int{}) {
		return 1
	}
	if !slices.Equal([]int{1, 2, 3}, []int{1, 2, 3}) {
		return 1
	}
	if slices.Equal([]int{1, 2}, []int{1, 2, 3}) {
		return 1
	}
	if slices.Equal([]int{1, 2, 3}, []int{3, 2, 1}) {
		return 1
	}
	if !slices.Equal([]string{"a", "b"}, []string{"a", "b"}) {
		return 1
	}
	if slices.Equal([]string{"a", "b"}, []string{"a", "c"}) {
		return 1
	}
	return 0
}

func TestEqualFunc() int {
	eq := func(a, b int) bool { return a/10 == b/10 }
	if !slices.EqualFunc([]int{11, 22}, []int{10, 20}, eq) {
		return 1
	}
	if slices.EqualFunc([]int{11, 25}, []int{10, 20}, eq) {
		return 1
	}
	return 0
}

func TestCompare() int {
	if slices.Compare([]int{}, []int{}) != 0 {
		return 1
	}
	if slices.Compare([]int{1, 2}, []int{1, 2}) != 0 {
		return 1
	}
	if slices.Compare([]int{1, 2}, []int{1, 3}) != -1 {
		return 1
	}
	if slices.Compare([]int{1, 3}, []int{1, 2}) != 1 {
		return 1
	}
	if slices.Compare([]int{1, 2}, []int{1, 2, 3}) != -1 {
		return 1
	}
	if slices.Compare([]string{"a", "b"}, []string{"a", "c"}) != -1 {
		return 1
	}
	if slices.Compare([]string{"a", "c"}, []string{"a", "b"}) != 1 {
		return 1
	}
	return 0
}

func TestCompareFunc() int {
	cmp := func(a, b int) int { return b - a }
	if slices.CompareFunc([]int{1, 3}, []int{1, 5}, cmp) != 1 {
		return 1
	}
	if slices.CompareFunc([]int{1, 5}, []int{1, 3}, cmp) != -1 {
		return 1
	}
	if slices.CompareFunc([]int{1, 5}, []int{1, 5}, cmp) != 0 {
		return 1
	}
	return 0
}

func TestIndex() int {
	s := []int{1, 2, 3}
	if slices.Index(s, 2) != 1 {
		return 1
	}
	if slices.Index(s, 9) != -1 {
		return 1
	}
	if slices.Index([]int{}, 1) != -1 {
		return 1
	}
	if slices.Index([]string{"a", "b"}, "b") != 1 {
		return 1
	}
	return 0
}

func TestIndexFunc() int {
	if slices.IndexFunc([]int{1, 4, 7}, func(v int) bool { return v > 3 }) != 1 {
		return 1
	}
	if slices.IndexFunc([]int{1, 2}, func(v int) bool { return v > 9 }) != -1 {
		return 1
	}
	return 0
}

func TestContains() int {
	if !slices.Contains([]int{1, 2, 3}, 2) {
		return 1
	}
	if slices.Contains([]int{1, 2, 3}, 9) {
		return 1
	}
	if slices.Contains([]int{}, 1) {
		return 1
	}
	if !slices.Contains([]string{"a"}, "a") {
		return 1
	}
	return 0
}

func TestContainsFunc() int {
	if !slices.ContainsFunc([]int{1, 4, 7}, func(v int) bool { return v == 7 }) {
		return 1
	}
	if slices.ContainsFunc([]int{1, 2}, func(v int) bool { return v == 9 }) {
		return 1
	}
	return 0
}

func TestInsert() int {
	if r := slices.Insert([]int{1, 4}, 1, 2, 3); len(r) != 4 || r[0] != 1 || r[1] != 2 || r[2] != 3 || r[3] != 4 {
		return 1
	}
	if r := slices.Insert([]int{2, 3}, 0, 1); len(r) != 3 || r[0] != 1 || r[1] != 2 || r[2] != 3 {
		return 1
	}
	if r := slices.Insert([]int{1, 2}, 2, 3); len(r) != 3 || r[0] != 1 || r[1] != 2 || r[2] != 3 {
		return 1
	}
	if r := slices.Insert([]int{1, 2}, 0); len(r) != 2 || r[0] != 1 || r[1] != 2 {
		return 1
	}
	return 0
}

func TestDelete() int {
	if r := slices.Delete([]int{1, 2, 3, 4, 5}, 1, 3); len(r) != 3 || r[0] != 1 || r[1] != 4 || r[2] != 5 {
		return 1
	}
	if r := slices.Delete([]int{1, 2, 3}, 0, 1); len(r) != 2 || r[0] != 2 || r[1] != 3 {
		return 1
	}
	if r := slices.Delete([]int{1, 2, 3}, 0, 3); len(r) != 0 {
		return 1
	}
	if r := slices.Delete([]int{1, 2, 3}, 2, 2); len(r) != 3 {
		return 1
	}
	return 0
}

func TestDeleteFunc() int {
	r := slices.DeleteFunc([]int{1, 2, 3, 4, 5}, func(v int) bool { return v%2 == 0 })
	if len(r) != 3 || r[0] != 1 || r[1] != 3 || r[2] != 5 {
		return 1
	}
	r = slices.DeleteFunc([]int{1, 2, 3}, func(v int) bool { return false })
	if len(r) != 3 {
		return 1
	}
	return 0
}

func TestReplace() int {
	if r := slices.Replace([]int{1, 2, 3, 4}, 1, 3, 9); len(r) != 3 || r[0] != 1 || r[1] != 9 || r[2] != 4 {
		return 1
	}
	if r := slices.Replace([]int{1, 2, 3, 4}, 1, 2, 8, 9); len(r) != 5 || r[0] != 1 || r[1] != 8 || r[2] != 9 || r[3] != 3 || r[4] != 4 {
		return 1
	}
	if r := slices.Replace([]int{1, 2, 3, 4}, 1, 3, 7, 6, 5); len(r) != 5 || r[0] != 1 || r[1] != 7 || r[2] != 6 || r[3] != 5 || r[4] != 4 {
		return 1
	}
	return 0
}

func TestClone() int {
	s := []int{1, 2, 3}
	c := slices.Clone(s)
	if !slices.Equal(s, c) {
		return 1
	}
	c[0] = 99
	if s[0] != 1 {
		return 1
	}
	if c := slices.Clone([]int{}); len(c) != 0 {
		return 1
	}
	var nilSlice []int
	if c := slices.Clone(nilSlice); c != nil {
		return 1
	}
	return 0
}

func TestCompact() int {
	if r := slices.Compact([]int{1, 1, 2, 2, 3, 3, 3}); len(r) != 3 || r[0] != 1 || r[1] != 2 || r[2] != 3 {
		return 1
	}
	if r := slices.Compact([]int{1, 2, 3}); len(r) != 3 {
		return 1
	}
	if r := slices.Compact([]string{"a", "a", "b"}); len(r) != 2 || r[0] != "a" || r[1] != "b" {
		return 1
	}
	return 0
}

func TestCompactFunc() int {
	r := slices.CompactFunc([]string{"a", "a", "b", "b", "c"}, func(a, b string) bool { return a == b })
	if len(r) != 3 || r[0] != "a" || r[1] != "b" || r[2] != "c" {
		return 1
	}
	return 0
}

func TestGrow() int {
	s := []int{1, 2, 3}
	g := slices.Grow(s, 10)
	if len(g) != 3 {
		return 1
	}
	if g[0] != 1 || g[1] != 2 || g[2] != 3 {
		return 1
	}
	if cap(g) < 10 {
		return 1
	}
	large := slices.Grow(s, 100)
	if cap(large) < 100 {
		return 1
	}
	return 0
}

func TestClip() int {
	big := make([]int, 5, 10)
	clipped := slices.Clip(big)
	if len(clipped) != 5 || cap(clipped) != 5 {
		return 1
	}
	if slices.Clip([]int{1, 2})[1] != 2 {
		return 1
	}
	return 0
}

func TestReverse() int {
	s := []int{1, 2, 3, 4}
	slices.Reverse(s)
	if s[0] != 4 || s[1] != 3 || s[2] != 2 || s[3] != 1 {
		return 1
	}
	strs := []string{"a", "b", "c"}
	slices.Reverse(strs)
	if strs[0] != "c" || strs[1] != "b" || strs[2] != "a" {
		return 1
	}
	return 0
}

func TestConcat() int {
	r := slices.Concat([]int{1}, []int{2, 3}, []int{4})
	if len(r) != 4 || r[0] != 1 || r[1] != 2 || r[2] != 3 || r[3] != 4 {
		return 1
	}
	if len(slices.Concat([]int{})) != 0 {
		return 1
	}
	rs := slices.Concat([]string{"a"}, []string{"b"})
	if len(rs) != 2 || rs[0] != "a" || rs[1] != "b" {
		return 1
	}
	return 0
}

func TestRepeat() int {
	r := slices.Repeat([]int{1, 2}, 3)
	if len(r) != 6 || r[0] != 1 || r[1] != 2 || r[2] != 1 || r[3] != 2 || r[4] != 1 || r[5] != 2 {
		return 1
	}
	if len(slices.Repeat([]int{1}, 0)) != 0 {
		return 1
	}
	one := slices.Repeat([]int{7}, 1)
	if len(one) != 1 || one[0] != 7 {
		return 1
	}
	return 0
}

func TestSort() int {
	s := []int{3, 1, 2}
	slices.Sort(s)
	if !slices.IsSorted(s) {
		return 1
	}
	if s[0] != 1 || s[1] != 2 || s[2] != 3 {
		return 1
	}
	strs := []string{"c", "a", "b"}
	slices.Sort(strs)
	if strs[0] != "a" || strs[1] != "b" || strs[2] != "c" {
		return 1
	}
	return 0
}

func TestIsSorted() int {
	if !slices.IsSorted([]int{1, 2, 3}) {
		return 1
	}
	if slices.IsSorted([]int{1, 3, 2}) {
		return 1
	}
	if !slices.IsSorted([]int{}) {
		return 1
	}
	return 0
}

func TestSortFunc() int {
	s := []int{3, 1, 2}
	slices.SortFunc(s, func(a, b int) int { return b - a })
	if s[0] != 3 || s[1] != 2 || s[2] != 1 {
		return 1
	}
	if !slices.IsSortedFunc(s, func(a, b int) int { return b - a }) {
		return 1
	}
	return 0
}

func TestSortStableFunc() int {
	kvs := []kv{{k: 2, v: 1}, {k: 1, v: 2}, {k: 2, v: 3}}
	slices.SortStableFunc(kvs, func(a, b kv) int { return a.k - b.k })
	if kvs[0].k != 1 || kvs[0].v != 2 {
		return 1
	}
	if kvs[1].k != 2 || kvs[1].v != 1 {
		return 1
	}
	if kvs[2].k != 2 || kvs[2].v != 3 {
		return 1
	}
	return 0
}

func TestMinMax() int {
	if slices.Min([]int{5, 2, 9}) != 2 {
		return 1
	}
	if slices.Max([]int{5, 2, 9}) != 9 {
		return 1
	}
	if slices.Min([]int{7}) != 7 {
		return 1
	}
	if slices.Max([]string{"a", "c", "b"}) != "c" {
		return 1
	}
	if slices.Min([]string{"a", "c", "b"}) != "a" {
		return 1
	}
	if slices.MinFunc([]int{5, 2, 9}, func(a, b int) int { return b - a }) != 9 {
		return 1
	}
	if slices.MaxFunc([]int{5, 2, 9}, func(a, b int) int { return b - a }) != 2 {
		return 1
	}
	return 0
}

func TestBinarySearch() int {
	i, found := slices.BinarySearch([]int{1, 3, 5}, 3)
	if i != 1 || !found {
		return 1
	}
	i, found = slices.BinarySearch([]int{1, 3, 5}, 4)
	if i != 2 || found {
		return 1
	}
	i, found = slices.BinarySearch([]int{}, 1)
	if i != 0 || found {
		return 1
	}
	return 0
}

func TestBinarySearchFunc() int {
	i, found := slices.BinarySearchFunc([]string{"a", "b", "d"}, "c", func(e, t string) int {
		if e < t {
			return -1
		}
		if e > t {
			return 1
		}
		return 0
	})
	if i != 2 || found {
		return 1
	}
	i, found = slices.BinarySearchFunc([]string{"a", "b", "d"}, "b", func(e, t string) int {
		if e < t {
			return -1
		}
		if e > t {
			return 1
		}
		return 0
	})
	if i != 1 || !found {
		return 1
	}
	return 0
}

func TestAll() int {
	sum := 0
	count := 0
	for i, v := range slices.All([]int{5, 6, 7}) {
		sum += i + v
		count++
	}
	if count != 3 || sum != 21 {
		return 1
	}
	return 0
}

func TestValues() int {
	sum := 0
	for v := range slices.Values([]int{10, 20, 30}) {
		sum += v
	}
	if sum != 60 {
		return 1
	}
	return 0
}

func TestBackward() int {
	sum := 0
	count := 0
	for i, v := range slices.Backward([]int{10, 20, 30}) {
		sum += i + v
		count++
	}
	if count != 3 || sum != 63 {
		return 1
	}
	return 0
}

func TestCollect() int {
	c := slices.Collect(slices.Values([]int{1, 2, 3}))
	if len(c) != 3 || c[0] != 1 || c[1] != 2 || c[2] != 3 {
		return 1
	}
	if c := slices.Collect(slices.Values([]int{})); len(c) != 0 {
		return 1
	}
	return 0
}

func TestSorted() int {
	s := slices.Sorted(slices.Values([]int{3, 1, 2}))
	if len(s) != 3 || s[0] != 1 || s[1] != 2 || s[2] != 3 {
		return 1
	}
	return 0
}

func TestSortedFunc() int {
	s := slices.SortedFunc(slices.Values([]int{3, 1, 2}), func(a, b int) int { return b - a })
	if len(s) != 3 || s[0] != 3 || s[1] != 2 || s[2] != 1 {
		return 1
	}
	return 0
}

func TestSortedStableFunc() int {
	seq := slices.Values([]kv{{k: 2, v: 1}, {k: 1, v: 2}, {k: 2, v: 3}})
	kvs := slices.SortedStableFunc(seq, func(a, b kv) int { return a.k - b.k })
	if len(kvs) != 3 {
		return 1
	}
	if kvs[0].k != 1 || kvs[0].v != 2 {
		return 1
	}
	if kvs[1].k != 2 || kvs[1].v != 1 {
		return 1
	}
	if kvs[2].k != 2 || kvs[2].v != 3 {
		return 1
	}
	return 0
}

func TestAppendSeq() int {
	app := slices.AppendSeq([]int{9}, slices.Values([]int{1, 2}))
	if len(app) != 3 || app[0] != 9 || app[1] != 1 || app[2] != 2 {
		return 1
	}
	app = slices.AppendSeq([]int{}, slices.Values([]int{}))
	if len(app) != 0 {
		return 1
	}
	return 0
}

func TestChunk() int {
	chunks := 0
	sum := 0
	maxSize := 0
	for chunk := range slices.Chunk([]int{1, 2, 3, 4, 5}, 2) {
		chunks++
		sum += len(chunk)
		if len(chunk) > maxSize {
			maxSize = len(chunk)
		}
	}
	if chunks != 3 || sum != 5 || maxSize != 2 {
		return 1
	}
	chunks = 0
	for range slices.Chunk([]int{}, 2) {
		chunks++
	}
	if chunks != 0 {
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
	runTest("TestCompare", TestCompare)
	runTest("TestCompareFunc", TestCompareFunc)
	runTest("TestIndex", TestIndex)
	runTest("TestIndexFunc", TestIndexFunc)
	runTest("TestContains", TestContains)
	runTest("TestContainsFunc", TestContainsFunc)
	runTest("TestInsert", TestInsert)
	runTest("TestDelete", TestDelete)
	runTest("TestDeleteFunc", TestDeleteFunc)
	runTest("TestReplace", TestReplace)
	runTest("TestClone", TestClone)
	runTest("TestCompact", TestCompact)
	runTest("TestCompactFunc", TestCompactFunc)
	runTest("TestGrow", TestGrow)
	runTest("TestClip", TestClip)
	runTest("TestReverse", TestReverse)
	runTest("TestConcat", TestConcat)
	runTest("TestRepeat", TestRepeat)
	runTest("TestSort", TestSort)
	runTest("TestIsSorted", TestIsSorted)
	runTest("TestSortFunc", TestSortFunc)
	runTest("TestSortStableFunc", TestSortStableFunc)
	runTest("TestMinMax", TestMinMax)
	runTest("TestBinarySearch", TestBinarySearch)
	runTest("TestBinarySearchFunc", TestBinarySearchFunc)
	runTest("TestAll", TestAll)
	runTest("TestValues", TestValues)
	runTest("TestBackward", TestBackward)
	runTest("TestCollect", TestCollect)
	runTest("TestSorted", TestSorted)
	runTest("TestSortedFunc", TestSortedFunc)
	runTest("TestSortedStableFunc", TestSortedStableFunc)
	runTest("TestAppendSeq", TestAppendSeq)
	runTest("TestChunk", TestChunk)
}
