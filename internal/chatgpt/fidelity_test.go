package chatgpt

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"testing"
)

func TestExtractedOriginalIsIndependentOfSearchText(t *testing.T) {
	parts := []any{"def f():\r\n    return 'שלום 🌍'  \r\n\r\n", map[string]any{"asset_pointer": "image"}, "root:\n  child: value\n"}
	want := parts[0].(string) + "\n" + parts[2].(string)
	exp := &Export{Conversations: []Conversation{{ID: "conv", Mapping: map[string]Message{"node": {ID: "node", Message: &MsgBody{ID: "body", Author: Author{Role: "assistant"}, CreateTime: 1234567890, Content: Content{Parts: parts}}}}}}}
	docs, err := ToDocuments(exp)
	if err != nil || len(docs) != 1 {
		t.Fatalf("docs=%v err=%v", docs, err)
	}
	b, _ := json.Marshal(docs[0])
	var d map[string]any
	json.Unmarshal(b, &d)
	if d["original_text"] != want {
		t.Errorf("original=%q want=%q", d["original_text"], want)
	}
	if d["search_text"] != CanonicalizeText(want) {
		t.Errorf("search=%q", d["search_text"])
	}
	h := sha256.Sum256([]byte(want))
	if d["text_sha256"] != hex.EncodeToString(h[:]) || d["text_fidelity"] != "extracted_original" {
		t.Errorf("missing fidelity/hash: %s", b)
	}
	if !reflect.DeepEqual(docs[0].Metadata["source_parts"], parts) {
		t.Error("source parts lost")
	}
	if docs[0].Metadata["source_node_id"] != "node" || docs[0].Metadata["source_message_id"] != "body" {
		t.Error("source identity lost")
	}
}

func TestNonTextContentAndEmptyTextClassification(t *testing.T) {
	report := NewIngestReport("synthetic")
	e := &Export{Conversations: []Conversation{{ID: "c", Mapping: map[string]Message{
		"image":    {ID: "image", Message: &MsgBody{Author: Author{Role: "user"}, CreateTime: 1, Content: Content{Parts: []any{map[string]any{"image": "x"}}}}},
		"spaces":   {ID: "spaces", Message: &MsgBody{Author: Author{Role: "user"}, CreateTime: 2, Content: Content{Parts: []any{" \r\n\t"}}}},
		"fallback": {ID: "fallback", Message: &MsgBody{Author: Author{Role: "assistant"}, CreateTime: 3, Content: Content{Text: "  fallback\r\n"}}},
	}}}}
	docs, err := ToDocumentsWithReport(e, report)
	if err != nil || len(docs) != 1 || *docs[0].OriginalText != "  fallback\r\n" {
		t.Fatalf("docs=%+v err=%v", docs, err)
	}
	if report.Summary.ByReason[string(ReasonSkipUnsupportedContent)] != 1 || report.Summary.ByReason[string(ReasonSkipEmptyTextAfterCanon)] != 1 {
		t.Fatalf("report=%+v", report.Summary)
	}
}
