package cantontrace

import "sort"

// Build arranges flat ledger events into a tree and returns its roots.
//
// Node ids are pre-order positions in the full transaction, and an exercise
// covers every node with an id in (NodeID, LastDescendantNodeID]. Build
// sorts by node id and keeps a stack of the exercises still open: a node
// belongs to the innermost open exercise whose range covers it, and an
// exercise is closed once a node past its range is reached.
//
// A reader sees only its projection of a transaction, so a node's real
// parent may be missing. Such a node attaches to the nearest visible
// ancestor whose range covers it, or becomes a root when none does. The
// result is never deeper than the ledger's tree and never loses an event.
//
// Build does not modify the Children of the nodes it is given beyond
// appending to them; pass fresh nodes.
func Build(nodes []*Node) []*Node {
	sorted := make([]*Node, 0, len(nodes))
	for _, n := range nodes {
		if n != nil {
			sorted = append(sorted, n)
		}
	}
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].NodeID < sorted[j].NodeID })

	roots := []*Node{}
	var open []*Node
	for _, n := range sorted {
		for len(open) > 0 && n.NodeID > open[len(open)-1].LastDescendantNodeID {
			open = open[:len(open)-1]
		}
		if len(open) == 0 {
			roots = append(roots, n)
		} else {
			parent := open[len(open)-1]
			parent.Children = append(parent.Children, n)
		}
		if n.Kind == KindExercise && n.LastDescendantNodeID > n.NodeID {
			open = append(open, n)
		}
	}
	return roots
}
