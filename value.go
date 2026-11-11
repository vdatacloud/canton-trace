package cantontrace

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	v2 "github.com/digital-asset/dazl-client/v8/go/api/com/daml/ledger/api/v2"
)

// ValueJSON encodes a Ledger API value the way the JSON Ledger API v2 does,
// so a trace reads the same from either transport:
//
//   - records are objects keyed by field label (read with verbose: true;
//     unlabelled fields are keyed "_1", "_2", ...)
//   - variants are {"tag": constructor, "value": ...}; enums the constructor
//   - Int64 is a string, Numeric its decimal string, Date "YYYY-MM-DD",
//     Timestamp RFC 3339 in UTC with microseconds
//   - Optional is null or the value; a nested Optional's Some is [value]
//     and its None []
//   - TextMap is an object; GenMap a list of [key, value] pairs
func ValueJSON(v *v2.Value) json.RawMessage {
	out, err := json.Marshal(valueAny(v, false))
	if err != nil {
		return json.RawMessage(`null`)
	}
	return out
}

// RecordJSON encodes a record as ValueJSON does.
func RecordJSON(r *v2.Record) json.RawMessage {
	if r == nil {
		return nil
	}
	return ValueJSON(&v2.Value{Sum: &v2.Value_Record{Record: r}})
}

var epoch = time.Unix(0, 0).UTC()

func valueAny(v *v2.Value, insideOptional bool) any {
	if v == nil {
		return nil
	}
	switch s := v.Sum.(type) {
	case *v2.Value_Unit:
		return struct{}{}
	case *v2.Value_Bool:
		return s.Bool
	case *v2.Value_Int64:
		return strconv.FormatInt(s.Int64, 10)
	case *v2.Value_Date:
		return epoch.AddDate(0, 0, int(s.Date)).Format("2006-01-02")
	case *v2.Value_Timestamp:
		return time.UnixMicro(s.Timestamp).UTC().Format("2006-01-02T15:04:05.000000Z")
	case *v2.Value_Numeric:
		return s.Numeric
	case *v2.Value_Party:
		return s.Party
	case *v2.Value_Text:
		return s.Text
	case *v2.Value_ContractId:
		return s.ContractId
	case *v2.Value_Optional:
		inner := s.Optional.GetValue()
		if insideOptional {
			if inner == nil {
				return []any{}
			}
			return []any{valueAny(inner, true)}
		}
		if inner == nil {
			return nil
		}
		return valueAny(inner, true)
	case *v2.Value_List:
		out := make([]any, 0, len(s.List.GetElements()))
		for _, e := range s.List.GetElements() {
			out = append(out, valueAny(e, false))
		}
		return out
	case *v2.Value_TextMap:
		out := make(map[string]any, len(s.TextMap.GetEntries()))
		for _, e := range s.TextMap.GetEntries() {
			out[e.GetKey()] = valueAny(e.GetValue(), false)
		}
		return out
	case *v2.Value_GenMap:
		out := make([]any, 0, len(s.GenMap.GetEntries()))
		for _, e := range s.GenMap.GetEntries() {
			out = append(out, []any{valueAny(e.GetKey(), false), valueAny(e.GetValue(), false)})
		}
		return out
	case *v2.Value_Record:
		return recordAny(s.Record)
	case *v2.Value_Variant:
		return map[string]any{"tag": s.Variant.GetConstructor(), "value": valueAny(s.Variant.GetValue(), false)}
	case *v2.Value_Enum:
		return s.Enum.GetConstructor()
	default:
		return nil
	}
}

func recordAny(r *v2.Record) any {
	out := orderedObject{}
	for i, f := range r.GetFields() {
		label := f.GetLabel()
		if label == "" {
			label = fmt.Sprintf("_%d", i+1)
		}
		out = append(out, kv{label, valueAny(f.GetValue(), false)})
	}
	return out
}

// orderedObject keeps a record's fields in declaration order, as the
// template declares them, rather than sorted by name.
type orderedObject []kv

type kv struct {
	k string
	v any
}

func (o orderedObject) MarshalJSON() ([]byte, error) {
	buf := []byte{'{'}
	for i, e := range o {
		if i > 0 {
			buf = append(buf, ',')
		}
		k, err := json.Marshal(e.k)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(e.v)
		if err != nil {
			return nil, err
		}
		buf = append(append(append(buf, k...), ':'), v...)
	}
	return append(buf, '}'), nil
}

// IdentifierString renders a template or interface id as
// "package:Module.Name:Entity", the JSON Ledger API's form.
func IdentifierString(id *v2.Identifier) string {
	if id == nil {
		return ""
	}
	return id.GetPackageId() + ":" + id.GetModuleName() + ":" + id.GetEntityName()
}
