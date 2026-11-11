// Package traceapi serves traces over HTTP:
//
//	GET  {prefix}/{updateId}   a committed transaction as the viewer sees it
//	POST {prefix}/simulate     what a command would do, prepared, not committed
//
// The handler never takes the parties to read as from the request. A header
// or query parameter naming a party would let any caller read as anyone the
// server's ledger credentials can act for. The application resolves the
// viewer from its own verified authentication ([Handler.Viewer]) and the
// ledger then returns only that viewer's projection.
//
// Responses carry Cache-Control: no-store: a trace can hold business data
// and must not sit in shared or browser caches.
package traceapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"

	cantontrace "github.com/vdatacloud/canton-trace"
	"google.golang.org/grpc/codes"
)

// Handler serves committed traces.
type Handler struct {
	// Reader reads from the ledger (required).
	Reader cantontrace.Reader
	// Viewer returns the parties the authenticated caller reads as (required).
	// Return ErrUnauthorized for an unauthenticated request and ErrForbidden
	// for one that may not read traces at all.
	Viewer func(r *http.Request) ([]string, error)
	// UpdateID returns the update id from the request path. It defaults to
	// r.PathValue("updateId") (net/http ServeMux patterns).
	UpdateID func(r *http.Request) string
	// Decorate, when set, runs on every trace before it is written: replace
	// party ids with display names, drop arguments the viewer shouldn't read.
	Decorate func(r *http.Request, t *cantontrace.Trace) error
	// LedgerMessages, when true, includes the ledger's own error text in error
	// responses. It is off by default because that text can name parties and
	// contracts.
	LedgerMessages bool
}

var (
	// ErrUnauthorized is a request without valid authentication.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrForbidden is an authenticated caller who may not read traces.
	ErrForbidden = errors.New("forbidden")
)

// updateIDPattern bounds what reaches the ledger: update ids are hex or
// base64url-ish strings, never paths or whitespace.
var updateIDPattern = regexp.MustCompile(`^[A-Za-z0-9_:=+\-]{1,256}$`)

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET", nil)
		return
	}
	parties, ok := h.viewer(w, r)
	if !ok {
		return
	}
	id := r.PathValue("updateId")
	if h.UpdateID != nil {
		id = h.UpdateID(r)
	}
	if !updateIDPattern.MatchString(id) {
		writeError(w, http.StatusBadRequest, "INVALID_UPDATE_ID", "the update id is malformed", nil)
		return
	}
	t, err := h.Reader.Read(r.Context(), id, parties)
	if err != nil {
		writeLedgerError(w, err, h.LedgerMessages)
		return
	}
	if h.Decorate != nil {
		if err := h.Decorate(r, t); err != nil {
			writeError(w, http.StatusInternalServerError, "DECORATION_FAILED", "the trace could not be prepared for display", nil)
			return
		}
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *Handler) viewer(w http.ResponseWriter, r *http.Request) ([]string, bool) {
	parties, err := h.Viewer(r)
	switch {
	case errors.Is(err, ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "sign in to read traces", nil)
		return nil, false
	case errors.Is(err, ErrForbidden):
		writeError(w, http.StatusForbidden, "FORBIDDEN", "you may not read traces", nil)
		return nil, false
	case err != nil:
		writeError(w, http.StatusInternalServerError, "VIEWER_UNRESOLVED", "your ledger identity could not be resolved", nil)
		return nil, false
	case len(parties) == 0:
		writeError(w, http.StatusForbidden, "NO_LEDGER_PARTY", "you have no ledger party to read as", nil)
		return nil, false
	}
	return parties, true
}

// SimulateHandler serves POST simulate: it prepares a command and returns
// the transaction it would commit. Decoding the command is the
// application's: Prepare reads the request body into its own command type,
// checks the caller may submit it, and prepares it (for example with
// [cantontrace.Simulator]). Nothing is signed or submitted.
type SimulateHandler struct {
	Prepare        func(r *http.Request) (*cantontrace.Trace, error)
	Decorate       func(r *http.Request, t *cantontrace.Trace) error
	LedgerMessages bool
	// MaxBody bounds the request body; it defaults to 1 MiB.
	MaxBody int64
}

func (h *SimulateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST", nil)
		return
	}
	limit := h.MaxBody
	if limit <= 0 {
		limit = 1 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	t, err := h.Prepare(r)
	var bad *BadRequestError
	switch {
	case errors.Is(err, ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "sign in to simulate", nil)
		return
	case errors.Is(err, ErrForbidden):
		writeError(w, http.StatusForbidden, "FORBIDDEN", "you may not submit this command", nil)
		return
	case errors.As(err, &bad):
		writeError(w, http.StatusBadRequest, "INVALID_COMMAND", bad.Msg, nil)
		return
	case err != nil:
		writeLedgerError(w, err, h.LedgerMessages)
		return
	}
	if h.Decorate != nil {
		if err := h.Decorate(r, t); err != nil {
			writeError(w, http.StatusInternalServerError, "DECORATION_FAILED", "the trace could not be prepared for display", nil)
			return
		}
	}
	writeJSON(w, http.StatusOK, t)
}

// BadRequestError is a command Prepare couldn't read; Msg is shown to the
// caller.
type BadRequestError struct{ Msg string }

func (e *BadRequestError) Error() string { return e.Msg }

// ErrorBody is every error response's shape.
type ErrorBody struct {
	Code    string               `json:"code"`
	Message string               `json:"message"`
	Ledger  *cantontrace.Failure `json:"ledger,omitempty"`
}

func writeLedgerError(w http.ResponseWriter, err error, messages bool) {
	f := cantontrace.Explain(err)
	if errors.Is(err, cantontrace.ErrNotATransaction) {
		writeError(w, http.StatusNotFound, "NOT_A_TRANSACTION", "that update isn't a transaction", nil)
		return
	}
	if f == nil {
		writeError(w, http.StatusBadGateway, "LEDGER_UNREACHABLE", "the ledger could not be reached", nil)
		return
	}
	if !messages {
		f.Message, f.Metadata = "", nil
	}
	httpStatus, code, msg := http.StatusBadGateway, "LEDGER_ERROR", "the ledger refused the request"
	switch grpcCode(f) {
	case codes.NotFound:
		httpStatus, code, msg = http.StatusNotFound, "TRACE_NOT_FOUND", "no transaction with that id is visible to you"
	case codes.PermissionDenied, codes.Unauthenticated:
		httpStatus, code, msg = http.StatusForbidden, "LEDGER_FORBIDDEN", "the ledger refused your credentials"
	case codes.InvalidArgument, codes.FailedPrecondition, codes.AlreadyExists, codes.Aborted:
		httpStatus, code, msg = http.StatusUnprocessableEntity, "LEDGER_REJECTED", "the ledger rejected the command"
	case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted:
		httpStatus, code, msg = http.StatusServiceUnavailable, "LEDGER_UNAVAILABLE", "the ledger is busy or unavailable; try again"
	}
	writeError(w, httpStatus, code, msg, f)
}

func grpcCode(f *cantontrace.Failure) codes.Code {
	if c, ok := codeByName[f.GRPCCode]; ok {
		return c
	}
	switch f.HTTPStatus {
	case http.StatusNotFound:
		return codes.NotFound
	case http.StatusUnauthorized:
		return codes.Unauthenticated
	case http.StatusForbidden:
		return codes.PermissionDenied
	case http.StatusBadRequest:
		return codes.InvalidArgument
	case http.StatusServiceUnavailable:
		return codes.Unavailable
	}
	return codes.Unknown
}

// codeByName reads Failure.GRPCCode, which holds codes.Code's String form.
var codeByName = func() map[string]codes.Code {
	m := map[string]codes.Code{}
	for c := codes.OK; c <= codes.Unauthenticated; c++ {
		m[c.String()] = c
	}
	return m
}()

func noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func writeError(w http.ResponseWriter, httpStatus int, code, msg string, f *cantontrace.Failure) {
	writeJSON(w, httpStatus, ErrorBody{Code: code, Message: msg, Ledger: f})
}

func writeJSON(w http.ResponseWriter, httpStatus int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	_ = json.NewEncoder(w).Encode(v)
}
