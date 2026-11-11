package cantontrace

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// sampleJSONTx is sampleTx as the JSON Ledger API v2 returns it.
const sampleJSONTx = `{"updateId":"1220aa","commandId":"cmd-1","workflowId":"","offset":42,
 "effectiveAt":"2026-11-11T09:00:00Z","recordTime":"2026-11-11T09:00:01.5Z","synchronizerId":"sync::1220",
 "events":[
  {"ExercisedEvent":{"offset":42,"nodeId":0,"contractId":"00e","templateId":"pkg:Demo.Escrow:Escrow","choice":"Release",
    "choiceArgument":{},"actingParties":["alice::1"],"consuming":true,"witnessParties":["alice::1"],
    "lastDescendantNodeId":2,"exerciseResult":"00h","packageName":"demo","implementedInterfaces":[]}},
  {"ExercisedEvent":{"offset":42,"nodeId":1,"contractId":"00f","templateId":"pkg:Demo.Escrow:Escrow","interfaceId":"api:Token:Factory",
    "choice":"Transfer","actingParties":["alice::1"],"consuming":false,"witnessParties":["alice::1"],
    "lastDescendantNodeId":2,"packageName":"demo"}},
  {"CreatedEvent":{"offset":42,"nodeId":2,"contractId":"00h","templateId":"pkg:Demo.Escrow:Escrow",
    "createArgument":{"owner":"bob::1"},"signatories":["bank::1"],"observers":["bob::1"],"witnessParties":["bob::1"],
    "packageName":"demo"}},
  {"ArchivedEvent":{"offset":42,"nodeId":3,"contractId":"00x","templateId":"pkg:Demo.Escrow:Escrow","witnessParties":["alice::1"],"packageName":"demo"}}
 ]}`

func TestParseJSONTransaction_MatchesTheGRPCAdapter(t *testing.T) {
	fromJSON, err := ParseJSONTransaction([]byte(sampleJSONTx))
	if err != nil {
		t.Fatal(err)
	}
	if shape(fromJSON.Roots) != shape(FromTransaction(sampleTx()).Roots) {
		t.Fatalf("shapes differ: %s", shape(fromJSON.Roots))
	}
	// Field for field, not only shape: a node without an argument or result
	// carries none on either transport.
	a, _ := json.Marshal(fromJSON.Roots)
	b, _ := json.Marshal(FromTransaction(sampleTx()).Roots)
	var ja, jb any
	_ = json.Unmarshal(a, &ja)
	_ = json.Unmarshal(b, &jb)
	if !reflect.DeepEqual(stripWitnesses(ja), stripWitnesses(jb)) {
		t.Fatalf("transports differ:\njson %s\ngrpc %s", a, b)
	}
	h := fromJSON.Roots[0].Children[0].Children[0]
	if string(h.CreateArgument) != `{"owner":"bob::1"}` || fromJSON.RecordTime.Nanosecond() != 5e8 {
		t.Fatalf("holding = %+v", h)
	}
	if _, err := ParseJSONTransaction([]byte(`{"events":[1]}`)); err == nil {
		t.Fatal("a malformed event is an error")
	}
}

func TestJSONReader(t *testing.T) {
	var gotBody map[string]any
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/updates/update-by-id" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		switch gotBody["updateId"] {
		case "1220aa":
			_, _ = io.WriteString(w, `{"update":{"Transaction":{"value":`+sampleJSONTx+`}}}`)
		case "reassign":
			_, _ = io.WriteString(w, `{"update":{"Reassignment":{"value":{}}}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"code":"UPDATE_NOT_FOUND","cause":"Update not found","correlationId":"corr-2",`+
				`"context":{"participant":"p1"},"errorCategory":11,"grpcCodeValue":5,"retryInfo":null}`)
		}
	}))
	defer srv.Close()
	r := &JSONReader{BaseURL: srv.URL + "/", Token: func(context.Context) (string, error) { return "tok", nil }}

	tr, err := r.Read(context.Background(), "1220aa", []string{"alice::1"})
	if err != nil || tr.Count() != 4 {
		t.Fatalf("read = %+v, %v", tr, err)
	}
	if gotAuth != "Bearer tok" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	inc := gotBody["updateFormat"].(map[string]any)["includeTransactions"].(map[string]any)
	if inc["transactionShape"] != "TRANSACTION_SHAPE_LEDGER_EFFECTS" || inc["eventFormat"].(map[string]any)["verbose"] != true {
		t.Fatalf("format = %v", inc)
	}

	if _, err := r.Read(context.Background(), "reassign", []string{"alice::1"}); !errors.Is(err, ErrNotATransaction) {
		t.Fatalf("reassignment: %v", err)
	}
	_, err = r.Read(context.Background(), "missing", []string{"alice::1"})
	x := Explain(err)
	if x == nil || x.Code != "UPDATE_NOT_FOUND" || x.HTTPStatus != 404 || x.GRPCCode != "NotFound" || x.CorrelationID != "corr-2" || x.Retryable {
		t.Fatalf("explained = %+v", x)
	}
	if err.Error() != "ledger refused (404): UPDATE_NOT_FOUND" {
		t.Fatalf("error text = %q", err.Error())
	}
	if _, err := r.Read(context.Background(), "x", nil); !errors.Is(err, ErrNoParties) {
		t.Fatal("no parties")
	}
	r.Token = func(context.Context) (string, error) { return "", errors.New("expired") }
	if _, err := r.Read(context.Background(), "1220aa", []string{"alice::1"}); err == nil {
		t.Fatal("a token failure stops the read")
	}
}

func TestExplain_JSONWithoutABody(t *testing.T) {
	x := Explain(&JSONError{Status: 503, Body: []byte("upstream")})
	if x == nil || x.HTTPStatus != 503 || x.Message != "Service Unavailable" {
		t.Fatalf("explained = %+v", x)
	}
}

// stripWitnesses drops the fields the two fixtures were written to differ
// in (witnesses, package names, acting parties), keeping the rest.
func stripWitnesses(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, val := range x {
			switch k {
			case "witnesses", "packageName", "actingParties":
				continue
			}
			out[k] = stripWitnesses(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = stripWitnesses(e)
		}
		return out
	}
	return v
}
