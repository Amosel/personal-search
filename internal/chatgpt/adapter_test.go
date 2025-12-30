package chatgpt

import (
	"testing"
)

func TestToDocuments_ValidInput(t *testing.T) {
	exp := &Export{
		Conversations: []Conversation{
			{
				ID:    "conv1",
				Title: "Test Conversation",
				Mapping: map[string]Message{
					"msg1": {
						ID: "msg1",
						Message: &MsgBody{
							Author:     Author{Role: "user"},
							CreateTime: 1234567890.0,
							Content: Content{
								ContentType: "text",
								Parts:       []any{"Hello world"},
							},
						},
					},
					"msg2": {
						ID: "msg2",
						Message: &MsgBody{
							Author:     Author{Role: "assistant"},
							CreateTime: 1234567891.0,
							Content: Content{
								ContentType: "text",
								Parts:       []any{"Hi there"},
							},
						},
					},
				},
			},
		},
	}

	docs, err := ToDocuments(exp)
	if err != nil {
		t.Fatalf("ToDocuments failed: %v", err)
	}

	if len(docs) != 2 {
		t.Fatalf("expected 2 documents, got %d", len(docs))
	}

	// Check first document
	if docs[0].Source != "chatgpt" {
		t.Errorf("expected source 'chatgpt', got %s", docs[0].Source)
	}
	if docs[0].Type != "message" {
		t.Errorf("expected type 'message', got %s", docs[0].Type)
	}
	if docs[0].Text != "Hello world" {
		t.Errorf("unexpected text: %s", docs[0].Text)
	}
	if docs[0].Metadata["author"] != "user" {
		t.Errorf("expected author 'user', got %v", docs[0].Metadata["author"])
	}
	if docs[0].Metadata["thread_id"] != "conv1" {
		t.Errorf("expected thread_id 'conv1', got %v", docs[0].Metadata["thread_id"])
	}
	if docs[0].Metadata["title"] != "Test Conversation" {
		t.Errorf("expected title 'Test Conversation', got %v", docs[0].Metadata["title"])
	}
}

func TestToDocuments_SkipsSystemMessages(t *testing.T) {
	exp := &Export{
		Conversations: []Conversation{
			{
				ID: "conv1",
				Mapping: map[string]Message{
					"msg1": {
						ID: "msg1",
						Message: &MsgBody{
							Author:     Author{Role: "system"},
							CreateTime: 1234567890.0,
							Content:    Content{Parts: []any{"System message"}},
						},
					},
					"msg2": {
						ID: "msg2",
						Message: &MsgBody{
							Author:     Author{Role: "user"},
							CreateTime: 1234567891.0,
							Content:    Content{Parts: []any{"User message"}},
						},
					},
				},
			},
		},
	}

	docs, err := ToDocuments(exp)
	if err != nil {
		t.Fatalf("ToDocuments failed: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("expected 1 document (system skipped), got %d", len(docs))
	}

	if docs[0].Metadata["author"] == "system" {
		t.Error("system message was not skipped")
	}
}

func TestToDocuments_EmptyTextSkipped(t *testing.T) {
	exp := &Export{
		Conversations: []Conversation{
			{
				ID: "conv1",
				Mapping: map[string]Message{
					"msg1": {
						ID: "msg1",
						Message: &MsgBody{
							Author:     Author{Role: "user"},
							CreateTime: 1234567890.0,
							Content:    Content{Parts: []any{"   "}}, // Only whitespace
						},
					},
					"msg2": {
						ID: "msg2",
						Message: &MsgBody{
							Author:     Author{Role: "user"},
							CreateTime: 1234567891.0,
							Content:    Content{Parts: []any{"Valid text"}},
						},
					},
				},
			},
		},
	}

	docs, err := ToDocuments(exp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("expected 1 document (empty skipped), got %d", len(docs))
	}

	if docs[0].Text != "Valid text" {
		t.Errorf("unexpected text: %s", docs[0].Text)
	}
}

func TestToDocuments_InvalidTimestampSkipped(t *testing.T) {
	exp := &Export{
		Conversations: []Conversation{
			{
				ID: "conv1",
				Mapping: map[string]Message{
					"msg1": {
						ID: "msg1",
						Message: &MsgBody{
							Author:     Author{Role: "user"},
							CreateTime: 0.0, // Invalid timestamp - should be skipped
							Content:    Content{Parts: []any{"Test message"}},
						},
					},
				},
			},
		},
	}

	docs, err := ToDocuments(exp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(docs) != 0 {
		t.Fatalf("expected 0 documents (invalid timestamp skipped), got %d", len(docs))
	}
}

func TestToDocuments_Deterministic(t *testing.T) {
	exp := &Export{
		Conversations: []Conversation{
			{
				ID: "conv1",
				Mapping: map[string]Message{
					"msg1": {
						ID: "msg1",
						Message: &MsgBody{
							Author:     Author{Role: "user"},
							CreateTime: 1234567890.0,
							Content:    Content{Parts: []any{"First"}},
						},
					},
					"msg2": {
						ID: "msg2",
						Message: &MsgBody{
							Author:     Author{Role: "assistant"},
							CreateTime: 1234567891.0,
							Content:    Content{Parts: []any{"Second"}},
						},
					},
				},
			},
		},
	}

	// Run conversion twice
	docs1, err1 := ToDocuments(exp)
	if err1 != nil {
		t.Fatalf("first conversion failed: %v", err1)
	}

	docs2, err2 := ToDocuments(exp)
	if err2 != nil {
		t.Fatalf("second conversion failed: %v", err2)
	}

	if len(docs1) != len(docs2) {
		t.Fatalf("document count differs: %d vs %d", len(docs1), len(docs2))
	}

	for i := range docs1 {
		if docs1[i].ID != docs2[i].ID {
			t.Errorf("document %d ID differs: %s vs %s", i, docs1[i].ID, docs2[i].ID)
		}
		if docs1[i].Text != docs2[i].Text {
			t.Errorf("document %d text differs: %s vs %s", i, docs1[i].Text, docs2[i].Text)
		}
	}
}

func TestToDocuments_OrderedByTimestamp(t *testing.T) {
	exp := &Export{
		Conversations: []Conversation{
			{
				ID: "conv1",
				Mapping: map[string]Message{
					"msg_b": {
						ID: "msg_b",
						Message: &MsgBody{
							Author:     Author{Role: "user"},
							CreateTime: 1234567890.0,
							Content:    Content{Parts: []any{"Second by ID"}},
						},
					},
					"msg_a": {
						ID: "msg_a",
						Message: &MsgBody{
							Author:     Author{Role: "assistant"},
							CreateTime: 1234567890.0, // Same timestamp
							Content:    Content{Parts: []any{"First by ID"}},
						},
					},
				},
			},
		},
	}

	docs, err := ToDocuments(exp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(docs) != 2 {
		t.Fatalf("expected 2 documents, got %d", len(docs))
	}

	// When timestamps are equal, should sort by message ID lexicographically (msg_a before msg_b)
	// Run twice to verify determinism
	docs2, _ := ToDocuments(exp)

	if docs[0].Text != docs2[0].Text {
		t.Error("ordering is not deterministic for equal timestamps")
	}

	if docs[0].Text == "Second by ID" {
		t.Error("expected msg_a (First by ID) to come before msg_b (Second by ID) when timestamps are equal")
	}
}
