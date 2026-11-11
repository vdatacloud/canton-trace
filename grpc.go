package cantontrace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	v2 "github.com/digital-asset/dazl-client/v8/go/api/com/daml/ledger/api/v2"
	"github.com/digital-asset/dazl-client/v8/go/api/com/daml/ledger/api/v2/interactive"
	txv1 "github.com/digital-asset/dazl-client/v8/go/api/com/daml/ledger/api/v2/interactive/transaction/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// FromTransaction builds a trace from a gRPC Ledger API transaction. Read it
// with TRANSACTION_SHAPE_LEDGER_EFFECTS so exercises and their
// last-descendant ids are present, and verbose so record fields carry
// labels.
func FromTransaction(tx *v2.Transaction) *Trace {
	if tx == nil {
		return nil
	}
	t := &Trace{
		UpdateID:       tx.GetUpdateId(),
		CommandID:      tx.GetCommandId(),
		WorkflowID:     tx.GetWorkflowId(),
		Offset:         tx.GetOffset(),
		EffectiveAt:    ts(tx.GetEffectiveAt()),
		RecordTime:     ts(tx.GetRecordTime()),
		SynchronizerID: tx.GetSynchronizerId(),
		Source:         SourceLedger,
	}
	nodes := make([]*Node, 0, len(tx.GetEvents()))
	for _, e := range tx.GetEvents() {
		if n := nodeFromEvent(e); n != nil {
			nodes = append(nodes, n)
		}
	}
	t.Roots = Build(nodes)
	return t
}

func nodeFromEvent(e *v2.Event) *Node {
	switch ev := e.GetEvent().(type) {
	case *v2.Event_Created:
		c := ev.Created
		return &Node{
			NodeID: c.GetNodeId(), Kind: KindCreate, ContractID: c.GetContractId(),
			TemplateID: IdentifierString(c.GetTemplateId()), PackageName: c.GetPackageName(),
			Signatories: c.GetSignatories(), Observers: c.GetObservers(), Witnesses: c.GetWitnessParties(),
			CreateArgument: RecordJSON(c.GetCreateArguments()),
		}
	case *v2.Event_Exercised:
		x := ev.Exercised
		return &Node{
			NodeID: x.GetNodeId(), Kind: KindExercise, ContractID: x.GetContractId(),
			TemplateID: IdentifierString(x.GetTemplateId()), PackageName: x.GetPackageName(),
			Choice: x.GetChoice(), InterfaceID: IdentifierString(x.GetInterfaceId()), Consuming: x.GetConsuming(),
			ActingParties: x.GetActingParties(), Witnesses: x.GetWitnessParties(),
			ChoiceArgument: optionalValue(x.GetChoiceArgument()), ExerciseResult: optionalValue(x.GetExerciseResult()),
			LastDescendantNodeID: x.GetLastDescendantNodeId(),
		}
	case *v2.Event_Archived:
		a := ev.Archived
		return &Node{
			NodeID: a.GetNodeId(), Kind: KindArchive, ContractID: a.GetContractId(),
			TemplateID: IdentifierString(a.GetTemplateId()), PackageName: a.GetPackageName(),
			Witnesses: a.GetWitnessParties(),
		}
	}
	return nil
}

// optionalValue is a node's argument or result: absent when the ledger sent
// none, as the JSON transport leaves it out, so both transports give the
// same trace.
func optionalValue(v *v2.Value) json.RawMessage {
	if v == nil {
		return nil
	}
	return ValueJSON(v)
}

func ts(t *timestamppb.Timestamp) *time.Time {
	if t == nil {
		return nil
	}
	v := t.AsTime().UTC()
	return &v
}

// FromPrepared builds a trace from a transaction prepared through the
// interactive submission service (PrepareSubmission): what the command would
// do, before anyone signs or commits it. Prepared nodes name their children
// directly, so the tree is read as given; node ids are the prepared
// transaction's, which carry no meaning on the ledger.
func FromPrepared(p *interactive.PreparedTransaction) (*Trace, error) {
	tx := p.GetTransaction()
	if tx == nil {
		return nil, errors.New("prepared transaction has no transaction")
	}
	byID := map[string]*txv1.Node{}
	for _, n := range tx.GetNodes() {
		if v1 := n.GetV1(); v1 != nil {
			byID[n.GetNodeId()] = v1
		}
	}
	seen := map[string]bool{}
	var build func(id string) (*Node, error)
	build = func(id string) (*Node, error) {
		if seen[id] {
			return nil, fmt.Errorf("prepared transaction node %s appears twice", id)
		}
		seen[id] = true
		raw, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("prepared transaction names node %s but doesn't hold it", id)
		}
		nid, _ := strconv.ParseInt(id, 10, 32)
		n := &Node{NodeID: int32(nid)}
		var children []string
		switch k := raw.GetNodeType().(type) {
		case *txv1.Node_Create:
			c := k.Create
			n.Kind, n.ContractID, n.TemplateID, n.PackageName = KindCreate, c.GetContractId(), IdentifierString(c.GetTemplateId()), c.GetPackageName()
			n.Signatories, n.Observers = c.GetSignatories(), minus(c.GetStakeholders(), c.GetSignatories())
			n.CreateArgument = optionalValue(c.GetArgument())
		case *txv1.Node_Exercise:
			x := k.Exercise
			n.Kind, n.ContractID, n.TemplateID, n.PackageName = KindExercise, x.GetContractId(), IdentifierString(x.GetTemplateId()), x.GetPackageName()
			n.Choice, n.InterfaceID, n.Consuming = x.GetChoiceId(), IdentifierString(x.GetInterfaceId()), x.GetConsuming()
			n.ActingParties, n.Signatories = x.GetActingParties(), x.GetSignatories()
			n.ChoiceArgument, n.ExerciseResult = optionalValue(x.GetChosenValue()), optionalValue(x.GetExerciseResult())
			children = x.GetChildren()
		case *txv1.Node_Fetch:
			f := k.Fetch
			n.Kind, n.ContractID, n.TemplateID, n.PackageName = KindFetch, f.GetContractId(), IdentifierString(f.GetTemplateId()), f.GetPackageName()
			n.InterfaceID, n.ActingParties, n.Signatories = IdentifierString(f.GetInterfaceId()), f.GetActingParties(), f.GetSignatories()
		case *txv1.Node_Rollback:
			n.Kind = KindRollback
			children = k.Rollback.GetChildren()
		default:
			return nil, fmt.Errorf("prepared transaction node %s has an unknown type", id)
		}
		for _, c := range children {
			child, err := build(c)
			if err != nil {
				return nil, err
			}
			n.Children = append(n.Children, child)
		}
		return n, nil
	}
	t := &Trace{Source: SourcePrepared, Roots: []*Node{}}
	if m := p.GetMetadata(); m != nil {
		t.SynchronizerID = m.GetSynchronizerId()
	}
	for _, r := range tx.GetRoots() {
		n, err := build(r)
		if err != nil {
			return nil, err
		}
		t.Roots = append(t.Roots, n)
	}
	return t, nil
}

func minus(all, drop []string) []string {
	skip := map[string]bool{}
	for _, d := range drop {
		skip[d] = true
	}
	var out []string
	for _, a := range all {
		if !skip[a] {
			out = append(out, a)
		}
	}
	return out
}

// GRPCReader reads transactions by update id over the gRPC Ledger API.
type GRPCReader struct {
	Updates v2.UpdateServiceClient
}

// NewGRPCReader reads through conn.
func NewGRPCReader(conn grpc.ClientConnInterface) *GRPCReader {
	return &GRPCReader{Updates: v2.NewUpdateServiceClient(conn)}
}

// Read fetches updateID as the given parties see it. The caller's context
// carries the ledger credentials (see [WithBearer]); the ledger answers
// NOT_FOUND when none of the parties may see the transaction.
func (r *GRPCReader) Read(ctx context.Context, updateID string, parties []string) (*Trace, error) {
	if len(parties) == 0 {
		return nil, ErrNoParties
	}
	filters := map[string]*v2.Filters{}
	for _, p := range parties {
		filters[p] = &v2.Filters{Cumulative: []*v2.CumulativeFilter{{
			IdentifierFilter: &v2.CumulativeFilter_WildcardFilter{WildcardFilter: &v2.WildcardFilter{}},
		}}}
	}
	resp, err := r.Updates.GetUpdateById(ctx, &v2.GetUpdateByIdRequest{
		UpdateId: updateID,
		UpdateFormat: &v2.UpdateFormat{IncludeTransactions: &v2.TransactionFormat{
			EventFormat:      &v2.EventFormat{FiltersByParty: filters, Verbose: true},
			TransactionShape: v2.TransactionShape_TRANSACTION_SHAPE_LEDGER_EFFECTS,
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("reading update %s: %w", updateID, err)
	}
	tx := resp.GetTransaction()
	if tx == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotATransaction, updateID)
	}
	return FromTransaction(tx), nil
}

// Simulator prepares commands without submitting them, through the
// interactive submission service.
type Simulator struct {
	Interactive interactive.InteractiveSubmissionServiceClient
}

// NewSimulator prepares through conn.
func NewSimulator(conn grpc.ClientConnInterface) *Simulator {
	return &Simulator{Interactive: interactive.NewInteractiveSubmissionServiceClient(conn)}
}

// Simulate prepares req and returns the transaction it would commit. Nothing
// is signed or submitted. A command the ledger would reject fails here with
// the ledger's error; [Explain] reads it.
func (s *Simulator) Simulate(ctx context.Context, req *interactive.PrepareSubmissionRequest) (*Trace, error) {
	resp, err := s.Interactive.PrepareSubmission(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("preparing command %s: %w", req.GetCommandId(), err)
	}
	return FromPrepared(resp.GetPreparedTransaction())
}

// WithBearer returns ctx carrying a bearer token for the Ledger API's
// authorization metadata.
func WithBearer(ctx context.Context, token string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
}

var (
	// ErrNoParties is a read that names no party to read as.
	ErrNoParties = errors.New("a trace is read as at least one party")
	// ErrNotATransaction is an update id that names a reassignment or a
	// topology change rather than a transaction.
	ErrNotATransaction = errors.New("update is not a transaction")
)
