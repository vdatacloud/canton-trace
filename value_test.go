package cantontrace

import (
	"testing"

	v2 "github.com/digital-asset/dazl-client/v8/go/api/com/daml/ledger/api/v2"
	"google.golang.org/protobuf/types/known/emptypb"
)

func rec(fields ...*v2.RecordField) *v2.Value {
	return &v2.Value{Sum: &v2.Value_Record{Record: &v2.Record{Fields: fields}}}
}
func field(label string, v *v2.Value) *v2.RecordField { return &v2.RecordField{Label: label, Value: v} }
func text(s string) *v2.Value                         { return &v2.Value{Sum: &v2.Value_Text{Text: s}} }
func some(v *v2.Value) *v2.Value {
	return &v2.Value{Sum: &v2.Value_Optional{Optional: &v2.Optional{Value: v}}}
}
func none() *v2.Value { return &v2.Value{Sum: &v2.Value_Optional{Optional: &v2.Optional{}}} }

func TestValueJSON_MatchesTheJSONLedgerAPI(t *testing.T) {
	v := rec(
		field("owner", &v2.Value{Sum: &v2.Value_Party{Party: "alice::1220"}}),
		field("amount", &v2.Value{Sum: &v2.Value_Numeric{Numeric: "25.0000000000"}}),
		field("count", &v2.Value{Sum: &v2.Value_Int64{Int64: -7}}),
		field("due", &v2.Value{Sum: &v2.Value_Date{Date: 20000}}),
		field("at", &v2.Value{Sum: &v2.Value_Timestamp{Timestamp: 1_700_000_000_123_456}}),
		field("ok", &v2.Value{Sum: &v2.Value_Bool{Bool: true}}),
		field("unit", &v2.Value{Sum: &v2.Value_Unit{Unit: &emptypb.Empty{}}}),
		field("cid", &v2.Value{Sum: &v2.Value_ContractId{ContractId: "00ab"}}),
		field("memo", some(text("hi"))),
		field("nothing", none()),
		field("nested", some(some(text("x")))),
		field("nestedNone", some(none())),
		field("tags", &v2.Value{Sum: &v2.Value_List{List: &v2.List{Elements: []*v2.Value{text("a"), text("b")}}}}),
		field("meta", &v2.Value{Sum: &v2.Value_TextMap{TextMap: &v2.TextMap{Entries: []*v2.TextMap_Entry{{Key: "k", Value: text("v")}}}}}),
		field("gm", &v2.Value{Sum: &v2.Value_GenMap{GenMap: &v2.GenMap{Entries: []*v2.GenMap_Entry{{Key: text("k"), Value: text("v")}}}}}),
		field("status", &v2.Value{Sum: &v2.Value_Enum{Enum: &v2.Enum{Constructor: "Open"}}}),
		field("action", &v2.Value{Sum: &v2.Value_Variant{Variant: &v2.Variant{Constructor: "Accept", Value: rec()}}}),
		field("", text("positional")),
	)
	want := `{"owner":"alice::1220","amount":"25.0000000000","count":"-7","due":"2024-10-04",` +
		`"at":"2023-11-14T22:13:20.123456Z","ok":true,"unit":{},"cid":"00ab","memo":"hi","nothing":null,` +
		`"nested":["x"],"nestedNone":[],"tags":["a","b"],"meta":{"k":"v"},"gm":[["k","v"]],"status":"Open",` +
		`"action":{"tag":"Accept","value":{}},"_18":"positional"}`
	if got := string(ValueJSON(v)); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestValueJSON_NilAndIdentifiers(t *testing.T) {
	if got := string(ValueJSON(nil)); got != "null" {
		t.Fatalf("nil value = %s", got)
	}
	if RecordJSON(nil) != nil {
		t.Fatal("nil record encodes as absent")
	}
	if got := IdentifierString(&v2.Identifier{PackageId: "pkg", ModuleName: "A.B", EntityName: "C"}); got != "pkg:A.B:C" {
		t.Fatalf("identifier = %s", got)
	}
	if IdentifierString(nil) != "" {
		t.Fatal("nil identifier is empty")
	}
}
