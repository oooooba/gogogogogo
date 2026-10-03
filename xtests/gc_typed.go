package main

type leaf struct {
	x int
}

type node struct {
	next  *node
	name  string
	nums  []int
	boxes []leaf
	tag   interface{}
	fn    func() int
	ch    chan leaf
}

// churn allocates enough garbage to force several collections, so anything the
// collector fails to keep through a typed root is gone by the time it is read.
// The garbage deliberately has the same shapes as the values under test, so a
// block that is freed too early is handed straight back out with other contents.
func churn() *leaf {
	leaves := make([]*leaf, 0, 100)
	for i := 0; i < 100; i++ {
		leaves = append(leaves, &leaf{x: i})
	}
	nodes := make([]*node, 0, 20)
	for i := 0; i < 20; i++ {
		nodes = append(nodes, &node{name: "garbage", next: &node{name: "garbage-inner"}, boxes: []leaf{{x: -1}}})
	}
	ptrs := make([]*node, 0, 20)
	for i := 0; i < 20; i++ {
		ptrs = append(ptrs, &node{name: "garbage-ptr"})
	}
	values := make([]node, 0, 20)
	for i := 0; i < 20; i++ {
		values = append(values, node{name: "garbage-value"})
	}
	tags := make([]interface{}, 0, 20)
	for i := 0; i < 20; i++ {
		tags = append(tags, &node{name: "garbage-tag"})
	}
	entries := make(map[string]node, 20)
	for i := 0; i < 20; i++ {
		entries["garbage"] = node{name: "garbage-entry", next: &node{name: "garbage-entry-next"}}
	}
	_ = nodes
	_ = ptrs
	_ = values
	_ = tags
	_ = entries
	return leaves[7]
}

var globalNode node
var globalBoxes []leaf
var globalPtrs []*node
var globalTag interface{}
var globalMap map[string]node
var globalChan chan leaf

func through_slice_of_structs() {
	nodes := make([]node, 0, 8)
	for i := 0; i < 8; i++ {
		nodes = append(nodes, node{next: &node{name: "in-slice"}})
	}
	var last *leaf
	for i := 0; i < 20; i++ {
		last = churn()
	}
	if last.x != 7 {
		panic("gc_typed: slice of structs")
	}
	if nodes[3].next == nil || nodes[3].next.name != "in-slice" {
		panic("gc_typed: slice of struct members")
	}
}

func through_slice_of_pointers() {
	nodes := make([]*node, 0, 8)
	for i := 0; i < 8; i++ {
		nodes = append(nodes, &node{name: "ptr-in-slice"})
	}
	for i := 0; i < 20; i++ {
		churn()
	}
	for i := 0; i < len(nodes); i++ {
		if nodes[i] == nil || nodes[i].name != "ptr-in-slice" {
			panic("gc_typed: slice of pointers")
		}
	}
}

func through_nested_slices() {
	grid := make([][]*node, 4)
	for i := range grid {
		grid[i] = make([]*node, 4)
		for j := range grid[i] {
			grid[i][j] = &node{name: "cell"}
		}
	}
	for i := 0; i < 20; i++ {
		churn()
	}
	if grid[3][3] == nil || grid[3][3].name != "cell" {
		panic("gc_typed: nested slices")
	}
}

func through_array_member() {
	var holder struct {
		cells [4]*node
		head  *node
	}
	for i := range holder.cells {
		holder.cells[i] = &node{name: "array-cell"}
	}
	holder.head = &node{name: "array-head"}
	for i := 0; i < 20; i++ {
		churn()
	}
	if holder.cells[2] == nil || holder.cells[2].name != "array-cell" {
		panic("gc_typed: array member")
	}
	if holder.head == nil || holder.head.name != "array-head" {
		panic("gc_typed: array member head")
	}
}

func through_struct_member() {
	n := &node{
		name:  "struct-member",
		nums:  []int{1, 2, 3},
		boxes: []leaf{{x: 11}, {x: 12}},
		tag:   &leaf{x: 13},
		fn:    func() int { return 14 },
		ch:    make(chan leaf, 1),
	}
	n.ch <- leaf{x: 15}
	for i := 0; i < 20; i++ {
		churn()
	}
	if n.name != "struct-member" || len(n.nums) != 3 || n.nums[2] != 3 {
		panic("gc_typed: struct member fields")
	}
	if n.boxes[1].x != 12 {
		panic("gc_typed: slice of structs in a member")
	}
	if tagged, ok := n.tag.(*leaf); !ok || tagged.x != 13 {
		panic("gc_typed: interface in a member")
	}
	if n.fn() != 14 {
		panic("gc_typed: func in a member")
	}
	if got := <-n.ch; got.x != 15 {
		panic("gc_typed: chan in a member")
	}
}

func through_pointer_chain() {
	head := &node{name: "head"}
	cursor := head
	for i := 0; i < 16; i++ {
		cursor.next = &node{name: "link"}
		cursor = cursor.next
	}
	for i := 0; i < 20; i++ {
		churn()
	}
	depth := 0
	for cursor := head; cursor != nil; cursor = cursor.next {
		depth++
	}
	if depth != 17 {
		panic("gc_typed: pointer chain")
	}
}

func through_closure_captures() {
	boxes := []leaf{{x: 21}, {x: 22}}
	tag := interface{}(&leaf{x: 23})
	s := "captured"
	makeNode := func() func() int {
		return func() int {
			total := 0
			for i := range boxes {
				total += boxes[i].x
			}
			if tagged, ok := tag.(*leaf); ok {
				total += tagged.x
			}
			return total + len(s)
		}
	}
	f := makeNode()
	for i := 0; i < 20; i++ {
		churn()
	}
	if f() != 21+22+23+len("captured") {
		panic("gc_typed: closure captures")
	}
}

func through_globals() {
	globalNode = node{
		next: &node{name: "global-next"},
		name: "global",
		nums: []int{1, 2, 3, 4},
		tag:  &leaf{x: 31},
		ch:   make(chan leaf, 1),
	}
	globalNode.ch <- leaf{x: 32}
	globalBoxes = []leaf{{x: 33}, {x: 34}}
	globalPtrs = []*node{{name: "global-ptr"}}
	globalTag = &node{name: "global-tag", next: &node{name: "global-tag-next"}}
	globalMap = map[string]node{"k": {name: "global-map", next: &node{name: "global-map-next"}}}
	globalChan = make(chan leaf, 1)
	globalChan <- leaf{x: 35}

	for i := 0; i < 20; i++ {
		churn()
	}
	if globalNode.next == nil || globalNode.next.name != "global-next" {
		panic("gc_typed: global struct member")
	}
	if globalNode.name != "global" || globalNode.nums[3] != 4 {
		panic("gc_typed: global struct fields")
	}
	if got := <-globalNode.ch; got.x != 32 {
		panic("gc_typed: global struct chan")
	}
	if globalBoxes[1].x != 34 {
		panic("gc_typed: global slice of structs")
	}
	if globalPtrs[0] == nil || globalPtrs[0].name != "global-ptr" {
		panic("gc_typed: global slice of pointers")
	}
	tagged, ok := globalTag.(*node)
	if !ok || tagged.next == nil || tagged.next.name != "global-tag-next" {
		panic("gc_typed: global interface")
	}
	entry, found := globalMap["k"]
	if !found || entry.next == nil || entry.next.name != "global-map-next" {
		panic("gc_typed: global map value")
	}
	if got := <-globalChan; got.x != 35 {
		panic("gc_typed: global chan")
	}
}

func main() {
	through_slice_of_structs()
	through_slice_of_pointers()
	through_nested_slices()
	through_array_member()
	through_struct_member()
	through_pointer_chain()
	through_closure_captures()
	through_globals()
	println("gc_typed: ok")
}
