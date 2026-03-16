package chatgpt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadExport_ObjectFormat(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "export.json")

	content := `{
		"conversations": [
			{
				"id": "conv1",
				"title": "Test Conversation",
				"mapping": {
					"msg1": {
						"id": "msg1",
						"create_time": 1234567890.0,
						"message": {
							"author": {"role": "user"},
							"content": {"content_type": "text", "parts": ["Hello"]}
						}
					}
				}
			}
		]
	}`

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	exp, err := LoadExport(path)
	if err != nil {
		t.Fatalf("LoadExport failed: %v", err)
	}

	if len(exp.Conversations) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(exp.Conversations))
	}

	if exp.Conversations[0].ID != "conv1" {
		t.Errorf("unexpected conversation ID: %s", exp.Conversations[0].ID)
	}
}

func TestLoadExport_ArrayFormat(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "export.json")

	content := `[
		{
			"id": "conv1",
			"title": "Test",
			"mapping": {}
		}
	]`

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	exp, err := LoadExport(path)
	if err != nil {
		t.Fatalf("LoadExport failed: %v", err)
	}

	if len(exp.Conversations) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(exp.Conversations))
	}
}

func TestLoadExport_MalformedJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "invalid.json")

	if err := os.WriteFile(path, []byte("{invalid json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadExport(path)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestLoadExport_MissingFile(t *testing.T) {
	_, err := LoadExport("/nonexistent/path/file.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadExport_EmptyArray(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "empty.json")

	if err := os.WriteFile(path, []byte("[]"), 0644); err != nil {
		t.Fatal(err)
	}

	exp, err := LoadExport(path)
	if err != nil {
		t.Fatalf("LoadExport failed: %v", err)
	}

	if exp.Conversations == nil {
		t.Fatal("Conversations should not be nil")
	}
}

func TestLoadExport_ObjectFormatWithExtraFields(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "export.json")

	content := `{
		"metadata": {"exported_at":"2026-03-16"},
		"conversations": [
			{
				"id": "conv1",
				"title": "Test",
				"mapping": {}
			}
		],
		"extra": ["ignored"]
	}`

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	exp, err := LoadExport(path)
	if err != nil {
		t.Fatalf("LoadExport failed: %v", err)
	}

	if len(exp.Conversations) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(exp.Conversations))
	}
}
