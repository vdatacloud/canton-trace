package cantontrace

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Failure is a ledger refusal read into its parts: the Canton error code
// (e.g. CONTRACT_NOT_FOUND, DAML_AUTHORIZATION_ERROR), its category, the
// correlation id to find it in the participant's logs, and the metadata the
// participant attached. Message is the ledger's own text, which can name
// parties and contract ids: an application decides whether to show it.
type Failure struct {
	GRPCCode      string            `json:"grpcCode,omitempty"`
	HTTPStatus    int               `json:"httpStatus,omitempty"`
	Code          string            `json:"code,omitempty"`
	Category      string            `json:"category,omitempty"`
	CorrelationID string            `json:"correlationId,omitempty"`
	Retryable     bool              `json:"retryable,omitempty"`
	Message       string            `json:"message,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// Explain reads a ledger error from either transport, or returns nil when
// err isn't one (a network failure, a cancelled context).
func Explain(err error) *Failure {
	if err == nil {
		return nil
	}
	var je *JSONError
	if errors.As(err, &je) {
		return explainJSON(je)
	}
	st, ok := status.FromError(unwrapToStatus(err))
	if !ok || st.Code() == codes.OK || st.Code() == codes.Unknown && len(st.Details()) == 0 {
		return nil
	}
	f := &Failure{GRPCCode: st.Code().String(), Message: st.Message(), Metadata: map[string]string{}}
	for _, d := range st.Details() {
		switch v := d.(type) {
		case *errdetails.ErrorInfo:
			f.Code = v.GetReason()
			for k, val := range v.GetMetadata() {
				switch k {
				case "category":
					f.Category = val
				default:
					f.Metadata[k] = val
				}
			}
		case *errdetails.RequestInfo:
			f.CorrelationID = v.GetRequestId()
		case *errdetails.RetryInfo:
			f.Retryable = true
		}
	}
	if f.Code == "" {
		f.Code = codeFromMessage(st.Message())
	}
	if len(f.Metadata) == 0 {
		f.Metadata = nil
	}
	return f
}

// unwrapToStatus finds the gRPC status error inside a wrapped error;
// status.FromError only looks at the outermost error's GRPCStatus.
func unwrapToStatus(err error) error {
	for e := err; e != nil; e = errors.Unwrap(e) {
		if _, ok := e.(interface{ GRPCStatus() *status.Status }); ok {
			return e
		}
	}
	return err
}

// codeFromMessage reads the code a Canton message starts with:
// "CONTRACT_NOT_FOUND(11,abc123): ...".
func codeFromMessage(msg string) string {
	i := strings.IndexByte(msg, '(')
	if i <= 0 {
		return ""
	}
	code := msg[:i]
	for _, r := range code {
		if (r < 'A' || r > 'Z') && r != '_' && (r < '0' || r > '9') {
			return ""
		}
	}
	return code
}

func explainJSON(e *JSONError) *Failure {
	f := &Failure{HTTPStatus: e.Status}
	var body struct {
		Code          string            `json:"code"`
		Cause         string            `json:"cause"`
		CorrelationID string            `json:"correlationId"`
		Context       map[string]string `json:"context"`
		ErrorCategory int               `json:"errorCategory"`
		GRPCCodeValue int               `json:"grpcCodeValue"`
		RetryInfo     json.RawMessage   `json:"retryInfo"`
	}
	if json.Unmarshal(e.Body, &body) != nil {
		f.Message = http.StatusText(e.Status)
		return f
	}
	f.Code, f.Message, f.CorrelationID, f.Metadata = body.Code, body.Cause, body.CorrelationID, body.Context
	if body.GRPCCodeValue != 0 {
		f.GRPCCode = codes.Code(uint32(body.GRPCCodeValue)).String()
	}
	f.Retryable = len(body.RetryInfo) > 0 && string(body.RetryInfo) != "null"
	return f
}
