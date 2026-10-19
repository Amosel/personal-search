package embed

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// Profile identifies both the model and transformations used to produce vectors.
// Field order is part of the v1 canonical JSON fingerprint contract.
type Profile struct {
	Version           int    `json:"version"`
	Provider          string `json:"provider"`
	ModelReference    string `json:"model_reference"`
	ModelRevision     string `json:"model_revision"`
	RevisionKind      string `json:"revision_kind"`
	Dimensions        int    `json:"dimensions"`
	DocumentTransform string `json:"document_transform"`
	QueryTransform    string `json:"query_transform"`
}
type IdentifiedEmbedder interface {
	Embedder
	Profile() Profile
}

func (p Profile) Validate() error {
	if p.Version != 1 || p.Dimensions <= 0 {
		return fmt.Errorf("invalid profile version or dimension")
	}
	for _, v := range []string{p.Provider, p.ModelReference, p.ModelRevision, p.RevisionKind, p.DocumentTransform, p.QueryTransform} {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("profile fields must not be empty")
		}
	}
	switch p.Provider {
	case "ollama":
		if p.RevisionKind != "content_digest" {
			return fmt.Errorf("Ollama requires content digest")
		}
	case "openai":
		if p.RevisionKind != "provider_reference" {
			return fmt.Errorf("OpenAI identity is only a provider reference")
		}
	case "fake":
		if p.RevisionKind != "test_algorithm" {
			return fmt.Errorf("fake identity must name test algorithm")
		}
	default:
		return fmt.Errorf("unsupported profile provider %q", p.Provider)
	}
	return nil
}
func (p Profile) Fingerprint() (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	b, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(b)), nil
}

// VectorError classifies malformed provider output before any storage call.
type VectorError struct{ Reason string }

func (e *VectorError) Error() string { return "embedding boundary: " + e.Reason }
func ValidateVectors(vectors [][]float32, count, dim int) error {
	if dim <= 0 || len(vectors) != count {
		return &VectorError{Reason: fmt.Sprintf("count/dimension mismatch: got %d vectors, want %d of dimension %d", len(vectors), count, dim)}
	}
	for i, v := range vectors {
		if len(v) != dim {
			return &VectorError{Reason: fmt.Sprintf("vector %d dimension %d, want %d", i, len(v), dim)}
		}
		for _, x := range v {
			if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
				return &VectorError{Reason: fmt.Sprintf("vector %d contains nonfinite value", i)}
			}
		}
	}
	return nil
}
