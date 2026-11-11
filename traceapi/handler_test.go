package traceapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cantontrace "github.com/vdatacloud/canton-trace"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeReader struct {
	parties []string
	err     error
}

func (f *fakeReader) Read(_ context.Context, id string, parties []string) (*cantontrace.Trace, error) {
	f.parties = parties
	if f.err != nil {
		return nil, f.err
	}
	return &cantontrace.Trace{UpdateID: id, Source: cantontrace.SourceLedger, Roots: []*cantontrace.Node{
		{NodeID: 0, Kind: cantontrace.KindCreate, Signatories: []string{"bank::1220"}},
	}}, nil
}

func serve(h http.Handler, method, target string, header http.Header) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.Handle("/api/v1/traces/{updateId}", h)
	req := httptest.NewRequest(method, target, nil)
	for k, v := range header {
		req.Header[k] = v
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func viewerFromAuth(r *http.Request) ([]string, error) {
	switch r.Header.Get("Authorization") {
	case "":
		return nil, ErrUnauthorized
	case "Bearer nobody":
		return nil, ErrForbidden
	case "Bearer noparty":
		return nil, nil
	case "Bearer broken":
		return nil, errors.New("identity store down")
	}
	return []string{"alice::1220"}, nil
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
}

func TestHandler_ReadsAsTheResolvedViewerOnly(t *testing.T) {
	r := &fakeReader{}
	h := &Handler{Reader: r, Viewer: viewerFromAuth, Decorate: func(_ *http.Request, t *cantontrace.Trace) error {
		t.Walk(func(n *cantontrace.Node, _ int) bool {
			for i, p := range n.Signatories {
				n.Signatories[i] = cantontrace.ShortID(p)
			}
			return true
		})
		return nil
	}}
	rec := serve(h, http.MethodGet, "/api/v1/traces/1220aa", http.Header{
		"Authorization": {"Bearer alice"},
		"X-Canton-Party": {"bank::1220"}, // ignored: the party is never the caller's to choose
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if strings.Join(r.parties, ",") != "alice::1220" {
		t.Fatalf("read as %v", r.parties)
	}
	if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("headers = %v", rec.Header())
	}
	var tr cantontrace.Trace
	decode(t, rec, &tr)
	if tr.UpdateID != "1220aa" || tr.Roots[0].Signatories[0] != "bank::1220" {
		t.Fatalf("trace = %+v", tr)
	}
}

func TestHandler_ViewerFailures(t *testing.T) {
	h := &Handler{Reader: &fakeReader{}, Viewer: viewerFromAuth}
	for auth, want := range map[string]int{
		"":               http.StatusUnauthorized,
		"Bearer nobody":  http.StatusForbidden,
		"Bearer noparty": http.StatusForbidden,
		"Bearer broken":  http.StatusInternalServerError,
	} {
		hdr := http.Header{}
		if auth != "" {
			hdr.Set("Authorization", auth)
		}
		rec := serve(h, http.MethodGet, "/api/v1/traces/1220aa", hdr)
		if rec.Code != want || rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%q: status %d, want %d", auth, rec.Code, want)
		}
		if strings.Contains(rec.Body.String(), "identity store down") {
			t.Error("internal errors aren't echoed")
		}
	}
}

func TestHandler_RequestChecks(t *testing.T) {
	h := &Handler{Reader: &fakeReader{}, Viewer: viewerFromAuth}
	auth := http.Header{"Authorization": {"Bearer alice"}}
	if rec := serve(h, http.MethodPost, "/api/v1/traces/1220aa", auth); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: %d", rec.Code)
	}
	if rec := serve(h, http.MethodGet, "/api/v1/traces/has%20space", auth); rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed id: %d", rec.Code)
	}
	h.UpdateID = func(*http.Request) string { return "from-hook" }
	rec := serve(h, http.MethodGet, "/api/v1/traces/ignored", auth)
	var tr cantontrace.Trace
	decode(t, rec, &tr)
	if tr.UpdateID != "from-hook" {
		t.Fatalf("UpdateID hook: %s", tr.UpdateID)
	}
	h.Decorate = func(*http.Request, *cantontrace.Trace) error { return errors.New("no") }
	if rec := serve(h, http.MethodGet, "/api/v1/traces/1220aa", auth); rec.Code != http.StatusInternalServerError {
		t.Fatalf("decorate failure: %d", rec.Code)
	}
}

func ledgerErr(c codes.Code, reason string) error {
	st, _ := status.New(c, reason+"(11,x): contract 00secret of party carol::1220").WithDetails(
		&errdetails.ErrorInfo{Reason: reason, Metadata: map[string]string{"party": "carol::1220"}})
	return st.Err()
}

func TestHandler_LedgerErrors(t *testing.T) {
	auth := http.Header{"Authorization": {"Bearer alice"}}
	for _, tc := range []struct {
		err  error
		code int
		api  string
	}{
		{ledgerErr(codes.NotFound, "UPDATE_NOT_FOUND"), http.StatusNotFound, "TRACE_NOT_FOUND"},
		{ledgerErr(codes.PermissionDenied, "PERMISSION_DENIED"), http.StatusForbidden, "LEDGER_FORBIDDEN"},
		{ledgerErr(codes.Unavailable, "SERVICE_NOT_RUNNING"), http.StatusServiceUnavailable, "LEDGER_UNAVAILABLE"},
		{ledgerErr(codes.Internal, "INTERNAL"), http.StatusBadGateway, "LEDGER_ERROR"},
		{&cantontrace.JSONError{Status: 404, Body: []byte(`{"code":"UPDATE_NOT_FOUND"}`)}, http.StatusNotFound, "TRACE_NOT_FOUND"},
		{errors.New("dial tcp: refused"), http.StatusBadGateway, "LEDGER_UNREACHABLE"},
		{cantontrace.ErrNotATransaction, http.StatusNotFound, "NOT_A_TRANSACTION"},
	} {
		h := &Handler{Reader: &fakeReader{err: tc.err}, Viewer: viewerFromAuth}
		rec := serve(h, http.MethodGet, "/api/v1/traces/1220aa", auth)
		var body ErrorBody
		decode(t, rec, &body)
		if rec.Code != tc.code || body.Code != tc.api {
			t.Errorf("%v: %d %s, want %d %s", tc.err, rec.Code, body.Code, tc.code, tc.api)
		}
		if strings.Contains(rec.Body.String(), "carol") || strings.Contains(rec.Body.String(), "00secret") {
			t.Errorf("%v: ledger text leaked: %s", tc.err, rec.Body)
		}
	}
	h := &Handler{Reader: &fakeReader{err: ledgerErr(codes.NotFound, "UPDATE_NOT_FOUND")}, Viewer: viewerFromAuth, LedgerMessages: true}
	if rec := serve(h, http.MethodGet, "/api/v1/traces/1220aa", auth); !strings.Contains(rec.Body.String(), "carol") {
		t.Fatal("LedgerMessages shows the ledger's text")
	}
}

func TestSimulateHandler(t *testing.T) {
	post := func(h *SimulateHandler, method, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1/traces/simulate", strings.NewReader(body)))
		return rec
	}
	ok := &SimulateHandler{Prepare: func(r *http.Request) (*cantontrace.Trace, error) {
		var cmd struct{ Choice string }
		if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil || cmd.Choice == "" {
			return nil, &BadRequestError{Msg: "a choice is required"}
		}
		return &cantontrace.Trace{Source: cantontrace.SourcePrepared, Roots: []*cantontrace.Node{}}, nil
	}, MaxBody: 64}
	if rec := post(ok, http.MethodPost, `{"choice":"Release"}`); rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("simulate: %d %s", rec.Code, rec.Body)
	}
	if rec := post(ok, http.MethodPost, `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad command: %d", rec.Code)
	}
	if rec := post(ok, http.MethodPost, `{"choice":"`+strings.Repeat("x", 100)+`"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("oversized body: %d", rec.Code)
	}
	if rec := post(ok, http.MethodGet, ``); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: %d", rec.Code)
	}
	for err, want := range map[error]int{
		ErrUnauthorized: http.StatusUnauthorized,
		ErrForbidden:    http.StatusForbidden,
		ledgerErr(codes.FailedPrecondition, "DAML_AUTHORIZATION_ERROR"): http.StatusUnprocessableEntity,
	} {
		h := &SimulateHandler{Prepare: func(*http.Request) (*cantontrace.Trace, error) { return nil, err }}
		if rec := post(h, http.MethodPost, `{}`); rec.Code != want {
			t.Errorf("%v: %d, want %d", err, rec.Code, want)
		}
	}
}
