package model

import "testing"

func TestPayloadProtectsSourceEvidence(t *testing.T) {
	d := Document{ID: "real", Source: "chatgpt", Type: "message", Timestamp: 1, Metadata: map[string]any{"doc_id": "forged", "original_text": "forged", "text_fidelity": "forged", "text_sha256": "forged", "embedding_identity": "forged"}}
	d.SetExtractedText("  real\r\n", "real", "v1")
	p := d.Payload()
	if p["doc_id"] != "real" || p["original_text"] != "  real\r\n" || p["text_fidelity"] != "extracted_original" || p["text_sha256"] == "forged" {
		t.Fatalf("%v", p)
	}
	if _, exists := p["embedding_identity"]; exists {
		t.Fatal("metadata forged embedding identity")
	}
}
