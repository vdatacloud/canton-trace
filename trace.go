// Package cantontrace reconstructs the tree of a Canton / Daml ledger
// transaction from the flat event list the Ledger API v2 returns, so a
// person can see which exercise caused which create, archive or nested
// exercise.
//
// The Ledger API returns a transaction's events in pre-order (by node id)
// and marks each exercise with the id of its last descendant node. That is
// enough to rebuild the tree with one pass and a stack; [Build] does it.
// Adapters turn the gRPC types ([FromTransaction], [FromPrepared]) and the
// JSON Ledger API ([ParseJSONTransaction]) into a [Trace].
//
// The package never decides who may see a transaction. The ledger does:
// a read returns only the events the requesting parties are entitled to see,
// and nodes whose parent is outside that projection become roots.
package cantontrace

import (
	"encoding/json"
	"time"
)

// Source says where a trace came from.
type Source string

const (
	// SourceLedger is a committed transaction read back from the ledger.
	SourceLedger Source = "ledger"
	// SourcePrepared is a transaction prepared (simulated) through the
	// interactive submission service and not committed.
	SourcePrepared Source = "prepared"
)

// Kind is the kind of a node in the tree.
type Kind string

const (
	KindCreate   Kind = "create"
	KindExercise Kind = "exercise"
	KindArchive  Kind = "archive"
	KindFetch    Kind = "fetch"    // prepared transactions only
	KindRollback Kind = "rollback" // prepared transactions only
)

// Trace is one transaction as a tree.
type Trace struct {
	UpdateID       string     `json:"updateId,omitempty"`
	CommandID      string     `json:"commandId,omitempty"`
	WorkflowID     string     `json:"workflowId,omitempty"`
	Offset         int64      `json:"offset,omitempty"`
	EffectiveAt    *time.Time `json:"effectiveAt,omitempty"`
	RecordTime     *time.Time `json:"recordTime,omitempty"`
	SynchronizerID string     `json:"synchronizerId,omitempty"`
	Source         Source     `json:"source"`
	Roots          []*Node    `json:"roots"`
}

// Node is one event (or prepared node) and its consequences.
type Node struct {
	NodeID               int32           `json:"nodeId"`
	Kind                 Kind            `json:"kind"`
	ContractID           string          `json:"contractId,omitempty"`
	TemplateID           string          `json:"templateId,omitempty"`
	PackageName          string          `json:"packageName,omitempty"`
	Choice               string          `json:"choice,omitempty"`
	InterfaceID          string          `json:"interfaceId,omitempty"`
	Consuming            bool            `json:"consuming,omitempty"`
	ActingParties        []string        `json:"actingParties,omitempty"`
	Signatories          []string        `json:"signatories,omitempty"`
	Observers            []string        `json:"observers,omitempty"`
	Witnesses            []string        `json:"witnesses,omitempty"`
	CreateArgument       json.RawMessage `json:"createArgument,omitempty"`
	ChoiceArgument       json.RawMessage `json:"choiceArgument,omitempty"`
	ExerciseResult       json.RawMessage `json:"exerciseResult,omitempty"`
	LastDescendantNodeID int32           `json:"lastDescendantNodeId,omitempty"`
	Children             []*Node         `json:"children,omitempty"`
}

// Walk calls fn on every node, parents before children. An application uses
// it to decorate or redact a trace before showing it (for example replacing
// party ids with display names, or dropping arguments a viewer shouldn't
// read). fn returning false skips that node's children.
func (t *Trace) Walk(fn func(n *Node, depth int) bool) {
	var walk func(ns []*Node, depth int)
	walk = func(ns []*Node, depth int) {
		for _, n := range ns {
			if fn(n, depth) {
				walk(n.Children, depth+1)
			}
		}
	}
	walk(t.Roots, 0)
}

// Count is the number of nodes in the trace.
func (t *Trace) Count() int {
	n := 0
	t.Walk(func(*Node, int) bool { n++; return true })
	return n
}
