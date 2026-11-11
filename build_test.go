package cantontrace

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
)

// shape renders roots as "id(child child)" for compact assertions.
func shape(ns []*Node) string {
	var parts []string
	for _, n := range ns {
		s := strconv.Itoa(int(n.NodeID))
		if len(n.Children) > 0 {
			s += "(" + shape(n.Children) + ")"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

func ex(id, last int32) *Node { return &Node{NodeID: id, Kind: KindExercise, LastDescendantNodeID: last} }
func cr(id int32) *Node       { return &Node{NodeID: id, Kind: KindCreate} }

func TestBuild_NestedExercises(t *testing.T) {
	// 0 Accept (1..5)
	//   1 Fetch-like exercise (no children)
	//   2 Transfer (3..4)
	//     3 create
	//     4 create
	//   5 create
	// 6 create (a second root)
	got := Build([]*Node{ex(0, 5), ex(1, 1), ex(2, 4), cr(3), cr(4), cr(5), cr(6)})
	if s := shape(got); s != "0(1 2(3 4) 5) 6" {
		t.Fatalf("shape = %s", s)
	}
}

func TestBuild_OrderOfInputDoesNotMatter(t *testing.T) {
	got := Build([]*Node{cr(5), cr(3), ex(2, 4), ex(0, 5), cr(4), ex(1, 1)})
	if s := shape(got); s != "0(1 2(3 4) 5)" {
		t.Fatalf("shape = %s", s)
	}
}

func TestBuild_ProjectionWithoutTheParent(t *testing.T) {
	// The reader can't see 0 (the root exercise) or 2; it sees a create
	// under 2 and an exercise under 0 with its own child.
	got := Build([]*Node{cr(3), ex(5, 6), cr(6)})
	if s := shape(got); s != "3 5(6)" {
		t.Fatalf("shape = %s", s)
	}
}

func TestBuild_LeafExerciseAndNils(t *testing.T) {
	got := Build([]*Node{nil, ex(0, 0), cr(1)})
	if s := shape(got); s != "0 1" {
		t.Fatalf("shape = %s", s)
	}
	if len(Build(nil)) != 0 {
		t.Fatal("no nodes, no roots")
	}
}

func TestWalkCountAndText(t *testing.T) {
	tr := &Trace{UpdateID: "1220upd", Source: SourceLedger, Roots: Build([]*Node{
		{NodeID: 0, Kind: KindExercise, LastDescendantNodeID: 1, TemplateID: "pkg:Bank.Escrow:Escrow", Choice: "Release",
			Consuming: true, ContractID: "00aabbccddeeff00112233"},
		{NodeID: 1, Kind: KindCreate, TemplateID: "pkg:Bank.Cash:Holding", ContractID: "00ff"},
	})}
	if tr.Count() != 2 {
		t.Fatalf("count = %d", tr.Count())
	}
	var b bytes.Buffer
	if err := WriteText(&b, tr); err != nil {
		t.Fatal(err)
	}
	want := "1220upd  2 nodes\n  #0 exercise Bank.Escrow:Escrow.Release (consuming) 00aabbccdd…\n    #1 create Bank.Cash:Holding 00ff\n"
	if b.String() != want {
		t.Fatalf("text =\n%s\nwant\n%s", b.String(), want)
	}
	depths := []int{}
	tr.Walk(func(_ *Node, d int) bool { depths = append(depths, d); return false })
	if len(depths) != 1 {
		t.Fatalf("returning false skips children: %v", depths)
	}
}

func TestShortID(t *testing.T) {
	for in, want := range map[string]string{
		"alice::1220abcdef0123456789": "alice::1220abcd…",
		"alice::12":                   "alice::12",
		"00aabbccddeeff":              "00aabbccdd…",
		"00ab":                        "00ab",
	} {
		if got := ShortID(in); got != want {
			t.Errorf("ShortID(%q) = %q, want %q", in, got, want)
		}
	}
}
