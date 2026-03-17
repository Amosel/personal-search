package embed

import "testing"

func TestNew_OllamaRejectsNegativeDim(t *testing.T) {
	_, err := New("ollama", "", "", DefaultOllamaURL, DefaultOllamaModel, -1)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNew_OpenAIRequiresKey(t *testing.T) {
	_, err := New("openai", "", DefaultOpenAIModel, DefaultOllamaURL, DefaultOllamaModel, 1536)
	if err == nil {
		t.Fatal("expected error")
	}
}
