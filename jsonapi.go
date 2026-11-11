package cantontrace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// jsonTransaction is a JSON Ledger API v2 transaction.
type jsonTransaction struct {
	UpdateID       string            `json:"updateId"`
	CommandID      string            `json:"commandId"`
	WorkflowID     string            `json:"workflowId"`
	Offset         int64             `json:"offset"`
	EffectiveAt    *time.Time        `json:"effectiveAt"`
	RecordTime     *time.Time        `json:"recordTime"`
	SynchronizerID string            `json:"synchronizerId"`
	Events         []json.RawMessage `json:"events"`
}

type jsonEvent struct {
	Created *struct {
		NodeID         int32           `json:"nodeId"`
		ContractID     string          `json:"contractId"`
		TemplateID     string          `json:"templateId"`
		PackageName    string          `json:"packageName"`
		CreateArgument json.RawMessage `json:"createArgument"`
		Signatories    []string        `json:"signatories"`
		Observers      []string        `json:"observers"`
		WitnessParties []string        `json:"witnessParties"`
	} `json:"CreatedEvent"`
	Exercised *struct {
		NodeID               int32           `json:"nodeId"`
		ContractID           string          `json:"contractId"`
		TemplateID           string          `json:"templateId"`
		InterfaceID          string          `json:"interfaceId"`
		PackageName          string          `json:"packageName"`
		Choice               string          `json:"choice"`
		ChoiceArgument       json.RawMessage `json:"choiceArgument"`
		ActingParties        []string        `json:"actingParties"`
		Consuming            bool            `json:"consuming"`
		WitnessParties       []string        `json:"witnessParties"`
		LastDescendantNodeID int32           `json:"lastDescendantNodeId"`
		ExerciseResult       json.RawMessage `json:"exerciseResult"`
	} `json:"ExercisedEvent"`
	Archived *struct {
		NodeID         int32    `json:"nodeId"`
		ContractID     string   `json:"contractId"`
		TemplateID     string   `json:"templateId"`
		PackageName    string   `json:"packageName"`
		WitnessParties []string `json:"witnessParties"`
	} `json:"ArchivedEvent"`
}

// ParseJSONTransaction builds a trace from a JSON Ledger API v2 transaction
// object (the "value" of an update's "Transaction"), read with
// TRANSACTION_SHAPE_LEDGER_EFFECTS and verbose: true.
func ParseJSONTransaction(raw []byte) (*Trace, error) {
	var tx jsonTransaction
	if err := json.Unmarshal(raw, &tx); err != nil {
		return nil, fmt.Errorf("decoding transaction: %w", err)
	}
	t := &Trace{UpdateID: tx.UpdateID, CommandID: tx.CommandID, WorkflowID: tx.WorkflowID, Offset: tx.Offset,
		EffectiveAt: utc(tx.EffectiveAt), RecordTime: utc(tx.RecordTime), SynchronizerID: tx.SynchronizerID, Source: SourceLedger}
	nodes := make([]*Node, 0, len(tx.Events))
	for i, raw := range tx.Events {
		var e jsonEvent
		if err := json.Unmarshal(raw, &e); err != nil {
			return nil, fmt.Errorf("decoding event %d: %w", i, err)
		}
		switch {
		case e.Created != nil:
			c := e.Created
			nodes = append(nodes, &Node{NodeID: c.NodeID, Kind: KindCreate, ContractID: c.ContractID, TemplateID: c.TemplateID,
				PackageName: c.PackageName, Signatories: c.Signatories, Observers: c.Observers, Witnesses: c.WitnessParties,
				CreateArgument: present(c.CreateArgument)})
		case e.Exercised != nil:
			x := e.Exercised
			nodes = append(nodes, &Node{NodeID: x.NodeID, Kind: KindExercise, ContractID: x.ContractID, TemplateID: x.TemplateID,
				InterfaceID: x.InterfaceID, PackageName: x.PackageName, Choice: x.Choice, Consuming: x.Consuming,
				ActingParties: x.ActingParties, Witnesses: x.WitnessParties, ChoiceArgument: present(x.ChoiceArgument),
				ExerciseResult: present(x.ExerciseResult), LastDescendantNodeID: x.LastDescendantNodeID})
		case e.Archived != nil:
			a := e.Archived
			nodes = append(nodes, &Node{NodeID: a.NodeID, Kind: KindArchive, ContractID: a.ContractID, TemplateID: a.TemplateID,
				PackageName: a.PackageName, Witnesses: a.WitnessParties})
		}
	}
	t.Roots = Build(nodes)
	return t, nil
}

func present(r json.RawMessage) json.RawMessage {
	if len(r) == 0 {
		return nil
	}
	return r
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := t.UTC()
	return &v
}

// JSONReader reads transactions by update id over the JSON Ledger API v2
// (POST /v2/updates/update-by-id).
type JSONReader struct {
	// BaseURL is the JSON Ledger API's root, e.g. http://localhost:7575.
	BaseURL string
	// HTTPClient defaults to http.DefaultClient.
	HTTPClient *http.Client
	// Token returns the bearer token for a read, or "" for an unsecured
	// ledger. It is asked per read so a caller can use a short-lived or
	// per-user token.
	Token func(ctx context.Context) (string, error)
}

// Read fetches updateID as the given parties see it.
func (r *JSONReader) Read(ctx context.Context, updateID string, parties []string) (*Trace, error) {
	if len(parties) == 0 {
		return nil, ErrNoParties
	}
	wildcard := []any{map[string]any{"identifierFilter": map[string]any{
		"WildcardFilter": map[string]any{"value": map[string]any{"includeCreatedEventBlob": false}},
	}}}
	filters := map[string]any{}
	for _, p := range parties {
		filters[p] = map[string]any{"cumulative": wildcard}
	}
	body, err := json.Marshal(map[string]any{
		"updateId": updateID,
		"updateFormat": map[string]any{"includeTransactions": map[string]any{
			"eventFormat":      map[string]any{"filtersByParty": filters, "verbose": true},
			"transactionShape": "TRANSACTION_SHAPE_LEDGER_EFFECTS",
		}},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(r.BaseURL, "/")+"/v2/updates/update-by-id", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if r.Token != nil {
		tok, err := r.Token(ctx)
		if err != nil {
			return nil, fmt.Errorf("ledger token: %w", err)
		}
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
	}
	client := r.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reading update %s: %w", updateID, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("reading update %s: %w", updateID, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &JSONError{Status: resp.StatusCode, Body: raw}
	}
	var out struct {
		Update struct {
			Transaction *struct {
				Value json.RawMessage `json:"value"`
			} `json:"Transaction"`
		} `json:"update"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decoding update %s: %w", updateID, err)
	}
	if out.Update.Transaction == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotATransaction, updateID)
	}
	return ParseJSONTransaction(out.Update.Transaction.Value)
}

// JSONError is a JSON Ledger API refusal. [Explain] reads its body.
type JSONError struct {
	Status int
	Body   []byte
}

func (e *JSONError) Error() string {
	if x := Explain(e); x != nil && x.Code != "" {
		return fmt.Sprintf("ledger refused (%d): %s", e.Status, x.Code)
	}
	return fmt.Sprintf("ledger refused (%d)", e.Status)
}
