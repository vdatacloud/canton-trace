package cantontrace

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	v2 "github.com/digital-asset/dazl-client/v8/go/api/com/daml/ledger/api/v2"
	"github.com/digital-asset/dazl-client/v8/go/api/com/daml/ledger/api/v2/interactive"
	txv1 "github.com/digital-asset/dazl-client/v8/go/api/com/daml/ledger/api/v2/interactive/transaction/v1"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var tmpl = &v2.Identifier{PackageId: "pkg", ModuleName: "Demo.Escrow", EntityName: "Escrow"}

// sampleTx is a synthetic transaction: Release (0..2) archives the escrow
// and creates a holding through a nested Transfer (1..2).
func sampleTx() *v2.Transaction {
	return &v2.Transaction{
		UpdateId: "1220aa", CommandId: "cmd-1", Offset: 42, SynchronizerId: "sync::1220",
		EffectiveAt: timestamppb.New(time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)),
		Events: []*v2.Event{
			{Event: &v2.Event_Created{Created: &v2.CreatedEvent{NodeId: 2, ContractId: "00h", TemplateId: tmpl,
				Signatories: []string{"bank::1"}, Observers: []string{"bob::1"}, WitnessParties: []string{"bob::1"},
				CreateArguments: &v2.Record{Fields: []*v2.RecordField{field("owner", text("bob::1"))}}}}},
			{Event: &v2.Event_Exercised{Exercised: &v2.ExercisedEvent{NodeId: 0, ContractId: "00e", TemplateId: tmpl,
				Choice: "Release", Consuming: true, ActingParties: []string{"alice::1"}, LastDescendantNodeId: 2,
				ChoiceArgument: rec(), ExerciseResult: text("00h")}}},
			{Event: &v2.Event_Exercised{Exercised: &v2.ExercisedEvent{NodeId: 1, ContractId: "00f", TemplateId: tmpl,
				InterfaceId: &v2.Identifier{PackageId: "api", ModuleName: "Token", EntityName: "Factory"},
				Choice: "Transfer", LastDescendantNodeId: 2}}},
			{Event: &v2.Event_Archived{Archived: &v2.ArchivedEvent{NodeId: 3, ContractId: "00x", TemplateId: tmpl}}},
		},
	}
}

func TestFromTransaction(t *testing.T) {
	tr := FromTransaction(sampleTx())
	if shape(tr.Roots) != "0(1(2)) 3" {
		t.Fatalf("shape = %s", shape(tr.Roots))
	}
	if tr.UpdateID != "1220aa" || tr.Offset != 42 || tr.Source != SourceLedger || tr.EffectiveAt.Hour() != 9 {
		t.Fatalf("header = %+v", tr)
	}
	release, transfer := tr.Roots[0], tr.Roots[0].Children[0]
	if release.Choice != "Release" || !release.Consuming || string(release.ExerciseResult) != `"00h"` || string(release.ChoiceArgument) != `{}` {
		t.Fatalf("release = %+v", release)
	}
	if transfer.InterfaceID != "api:Token:Factory" {
		t.Fatalf("interface = %s", transfer.InterfaceID)
	}
	holding := transfer.Children[0]
	if string(holding.CreateArgument) != `{"owner":"bob::1"}` || holding.Observers[0] != "bob::1" {
		t.Fatalf("holding = %+v", holding)
	}
	if tr.Roots[1].Kind != KindArchive || FromTransaction(nil) != nil {
		t.Fatal("archive root; nil in, nil out")
	}
}

func TestFromPrepared(t *testing.T) {
	create := &txv1.Node{NodeType: &txv1.Node_Create{Create: &txv1.Create{ContractId: "", TemplateId: tmpl,
		Signatories: []string{"bank::1"}, Stakeholders: []string{"bank::1", "bob::1"}, Argument: rec()}}}
	exercise := &txv1.Node{NodeType: &txv1.Node_Exercise{Exercise: &txv1.Exercise{ContractId: "00e", TemplateId: tmpl,
		ChoiceId: "Release", Consuming: true, ActingParties: []string{"alice::1"}, Children: []string{"1", "2"}}}}
	fetch := &txv1.Node{NodeType: &txv1.Node_Fetch{Fetch: &txv1.Fetch{ContractId: "00r", TemplateId: tmpl}}}
	rollback := &txv1.Node{NodeType: &txv1.Node_Rollback{Rollback: &txv1.Rollback{Children: []string{"3"}}}}
	p := &interactive.PreparedTransaction{
		Transaction: &interactive.DamlTransaction{Roots: []string{"0"}, Nodes: []*interactive.DamlTransaction_Node{
			{NodeId: "0", VersionedNode: &interactive.DamlTransaction_Node_V1{V1: exercise}},
			{NodeId: "1", VersionedNode: &interactive.DamlTransaction_Node_V1{V1: fetch}},
			{NodeId: "2", VersionedNode: &interactive.DamlTransaction_Node_V1{V1: rollback}},
			{NodeId: "3", VersionedNode: &interactive.DamlTransaction_Node_V1{V1: create}},
		}},
		Metadata: &interactive.Metadata{SynchronizerId: "sync::1220"},
	}
	tr, err := FromPrepared(p)
	if err != nil {
		t.Fatal(err)
	}
	if shape(tr.Roots) != "0(1 2(3))" || tr.Source != SourcePrepared || tr.SynchronizerID != "sync::1220" {
		t.Fatalf("trace = %s %+v", shape(tr.Roots), tr)
	}
	c := tr.Roots[0].Children[1].Children[0]
	if c.Kind != KindCreate || len(c.Observers) != 1 || c.Observers[0] != "bob::1" {
		t.Fatalf("create = %+v", c)
	}
	if tr.Roots[0].Children[0].Kind != KindFetch || tr.Roots[0].Children[1].Kind != KindRollback {
		t.Fatal("fetch and rollback kinds")
	}

	p.Transaction.Roots = []string{"0", "9"}
	if _, err := FromPrepared(p); err == nil {
		t.Fatal("a missing node is an error")
	}
	p.Transaction.Roots = []string{"0", "1"}
	if _, err := FromPrepared(p); err == nil {
		t.Fatal("a node reached twice is an error")
	}
	if _, err := FromPrepared(&interactive.PreparedTransaction{}); err == nil {
		t.Fatal("no transaction is an error")
	}
}

// fakeLedger is an in-process Ledger API: UpdateService and
// InteractiveSubmissionService over bufconn.
type fakeLedger struct {
	v2.UnimplementedUpdateServiceServer
	interactive.UnimplementedInteractiveSubmissionServiceServer
	lastUpdateReq *v2.GetUpdateByIdRequest
	lastAuth      []string
	update        *v2.GetUpdateResponse
	err           error
	prepared      *interactive.PrepareSubmissionResponse
}

func (f *fakeLedger) GetUpdateById(ctx context.Context, req *v2.GetUpdateByIdRequest) (*v2.GetUpdateResponse, error) {
	f.lastUpdateReq = req
	md, _ := metadata.FromIncomingContext(ctx)
	f.lastAuth = md.Get("authorization")
	return f.update, f.err
}

func (f *fakeLedger) PrepareSubmission(_ context.Context, _ *interactive.PrepareSubmissionRequest) (*interactive.PrepareSubmissionResponse, error) {
	return f.prepared, f.err
}

func dial(t *testing.T, f *fakeLedger) *grpc.ClientConn {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	s := grpc.NewServer()
	v2.RegisterUpdateServiceServer(s, f)
	interactive.RegisterInteractiveSubmissionServiceServer(s, f)
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestGRPCReader(t *testing.T) {
	f := &fakeLedger{update: &v2.GetUpdateResponse{Update: &v2.GetUpdateResponse_Transaction{Transaction: sampleTx()}}}
	r := NewGRPCReader(dial(t, f))
	tr, err := r.Read(WithBearer(context.Background(), "tok"), "1220aa", []string{"alice::1", "bob::1"})
	if err != nil {
		t.Fatal(err)
	}
	if tr.Count() != 4 {
		t.Fatalf("count = %d", tr.Count())
	}
	tf := f.lastUpdateReq.GetUpdateFormat().GetIncludeTransactions()
	if tf.GetTransactionShape() != v2.TransactionShape_TRANSACTION_SHAPE_LEDGER_EFFECTS || !tf.GetEventFormat().GetVerbose() {
		t.Fatal("reads ledger effects, verbose")
	}
	if len(tf.GetEventFormat().GetFiltersByParty()) != 2 {
		t.Fatal("one filter per party")
	}
	if len(f.lastAuth) != 1 || f.lastAuth[0] != "Bearer tok" {
		t.Fatalf("authorization = %v", f.lastAuth)
	}

	if _, err := r.Read(context.Background(), "1220aa", nil); !errors.Is(err, ErrNoParties) {
		t.Fatalf("no parties: %v", err)
	}
	f.update = &v2.GetUpdateResponse{}
	if _, err := r.Read(context.Background(), "1220aa", []string{"alice::1"}); !errors.Is(err, ErrNotATransaction) {
		t.Fatalf("not a transaction: %v", err)
	}
}

func cantonStatus(t *testing.T, c codes.Code, reason string) error {
	t.Helper()
	st, err := status.New(c, reason+"(11,abc): synthetic").WithDetails(
		&errdetails.ErrorInfo{Reason: reason, Metadata: map[string]string{"category": "11", "participant": "p1"}},
		&errdetails.RequestInfo{RequestId: "corr-1"},
		&errdetails.RetryInfo{},
	)
	if err != nil {
		t.Fatal(err)
	}
	return st.Err()
}

func TestGRPCReader_LedgerErrorIsExplained(t *testing.T) {
	f := &fakeLedger{err: cantonStatus(t, codes.NotFound, "UPDATE_NOT_FOUND")}
	_, err := NewGRPCReader(dial(t, f)).Read(context.Background(), "1220aa", []string{"alice::1"})
	x := Explain(err)
	if x == nil || x.Code != "UPDATE_NOT_FOUND" || x.GRPCCode != "NotFound" || x.CorrelationID != "corr-1" ||
		x.Category != "11" || !x.Retryable || x.Metadata["participant"] != "p1" {
		t.Fatalf("explained = %+v", x)
	}
}

func TestSimulator(t *testing.T) {
	create := &txv1.Node{NodeType: &txv1.Node_Create{Create: &txv1.Create{TemplateId: tmpl}}}
	f := &fakeLedger{prepared: &interactive.PrepareSubmissionResponse{PreparedTransaction: &interactive.PreparedTransaction{
		Transaction: &interactive.DamlTransaction{Roots: []string{"0"}, Nodes: []*interactive.DamlTransaction_Node{
			{NodeId: "0", VersionedNode: &interactive.DamlTransaction_Node_V1{V1: create}}}}}}}
	s := NewSimulator(dial(t, f))
	tr, err := s.Simulate(context.Background(), &interactive.PrepareSubmissionRequest{CommandId: "c"})
	if err != nil || tr.Count() != 1 || tr.Source != SourcePrepared {
		t.Fatalf("simulate = %+v, %v", tr, err)
	}
	f.err = cantonStatus(t, codes.FailedPrecondition, "DAML_AUTHORIZATION_ERROR")
	_, err = s.Simulate(context.Background(), &interactive.PrepareSubmissionRequest{CommandId: "c"})
	if x := Explain(err); x == nil || x.Code != "DAML_AUTHORIZATION_ERROR" {
		t.Fatalf("explained = %+v", x)
	}
}

func TestExplain_NotALedgerError(t *testing.T) {
	if Explain(nil) != nil || Explain(errors.New("dial tcp: refused")) != nil || Explain(context.Canceled) != nil {
		t.Fatal("non-ledger errors aren't explained")
	}
	// A status without ErrorInfo still yields the code from its message.
	err := fmt.Errorf("wrapped: %w", status.Error(codes.InvalidArgument, "COMMAND_PREPROCESSING_FAILED(8,x): bad"))
	if x := Explain(err); x == nil || x.Code != "COMMAND_PREPROCESSING_FAILED" {
		t.Fatalf("explained = %+v", x)
	}
	if codeFromMessage("lower(1)") != "" || codeFromMessage("no paren") != "" {
		t.Fatal("only an upper-case code prefix is read")
	}
}
