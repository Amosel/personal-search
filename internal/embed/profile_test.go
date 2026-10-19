package embed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestProfileFingerprintsEveryIdentityField(t *testing.T) {
	p := Profile{Version: 1, Provider: "ollama", ModelReference: "test:latest", ModelRevision: "digest-a", RevisionKind: "content_digest", Dimensions: 3, DocumentTransform: "headtail2000-v1", QueryTransform: "headtail2000-v1"}
	base, err := p.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if base != "sha256:d8a6425b1f49a2c76ea19be96accfd9c4f3f000b1cca38c9dc86928f76f90710" {
		t.Fatalf("v1 fingerprint contract changed: %s", base)
	}
	var reordered Profile
	if err := json.Unmarshal([]byte(`{"query_transform":"headtail2000-v1","document_transform":"headtail2000-v1","dimensions":3,"revision_kind":"content_digest","model_revision":"digest-a","model_reference":"test:latest","provider":"ollama","version":1}`), &reordered); err != nil {
		t.Fatal(err)
	}
	same, err := reordered.Fingerprint()
	if err != nil || base != same {
		t.Fatal("construction order changed fingerprint")
	}
	for _, field := range []string{"ModelReference", "ModelRevision", "DocumentTransform", "QueryTransform", "Dimensions"} {
		changed := p
		v := reflect.ValueOf(&changed).Elem().FieldByName(field)
		if v.Kind() == reflect.Int {
			v.SetInt(4)
		} else {
			v.SetString("other")
		}
		h, err := changed.Fingerprint()
		if err != nil || h == base {
			t.Fatalf("%s not distinguished: hash=%s err=%v", field, h, err)
		}
	}
	for _, field := range []string{"Provider", "ModelReference", "ModelRevision", "RevisionKind", "DocumentTransform", "QueryTransform", "Dimensions", "Version"} {
		invalid := p
		v := reflect.ValueOf(&invalid).Elem().FieldByName(field)
		v.SetZero()
		if _, err := invalid.Fingerprint(); err == nil {
			t.Fatalf("accepted missing %s", field)
		}
	}
}

func TestProviderAssuranceAndMalformedVectors(t *testing.T) {
	fake := (&FakeEmbedder{DimVal: 3}).Profile()
	openai := (&OpenAIEmbedder{Model: "example", DimVal: 3}).Profile()
	if fake.RevisionKind != "test_algorithm" || openai.RevisionKind != "provider_reference" {
		t.Fatal("incorrect assurance")
	}
	for _, p := range []Profile{fake, openai} {
		if _, err := p.Fingerprint(); err != nil {
			t.Fatal(err)
		}
	}
	for _, vectors := range [][][]float32{nil, {{1, 2}}, {{1, 2, 3}, {1, 2, 3}}, {{1, float32(math.NaN()), 3}}, {{1, 2, float32(math.Inf(1))}}} {
		var boundary *VectorError
		if err := ValidateVectors(vectors, 1, 3); !errors.As(err, &boundary) {
			t.Fatalf("accepted malformed vectors: %v, %v", vectors, err)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestPinnedOllamaRejectsTagChangesAtEachBoundary(t *testing.T) {
	for _, mode := range []string{"unchanged", "probe", "before_batch", "after_batch", "missing", "dimension"} {
		t.Run(mode, func(t *testing.T) {
			tags, embeds := 0, 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				body := ""
				switch r.URL.Path {
				case "/api/tags":
					tags++
					digest := "A"
					if mode == "probe" && tags == 2 || mode == "before_batch" && tags == 3 || mode == "after_batch" && tags == 4 {
						digest = "B"
					}
					body = fmt.Sprintf(`{"models":[{"name":"test:latest","digest":%q}]}`, digest)
					if mode == "missing" {
						body = `{"models":[]}`
					}
				case "/api/embed":
					embeds++
					var req struct {
						Input []string `json:"input"`
					}
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						t.Fatal(err)
					}
					if embeds == 2 && req.Input[0] != strings.Repeat("h", 1500)+"\n...\n"+strings.Repeat("t", 500) {
						t.Fatal("legacy transform changed")
					}
					body = `{"embeddings":[[1,0,0]]}`
				default:
					t.Fatalf("unexpected provider mutation: %s", r.URL)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}
			dim := 0
			if mode == "dimension" {
				dim = 4
			}
			e, err := NewPinnedOllama(context.Background(), "http://provider.invalid", "test", dim, client)
			if mode == "probe" || mode == "missing" || mode == "dimension" {
				if err == nil {
					t.Fatal("accepted missing/changed probe")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if e.Profile().RevisionKind != "content_digest" || e.EmbeddingIdentity() != "ollama:A:clip2000:v1" {
				t.Fatalf("profile=%+v", e.Profile())
			}
			_, err = e.Embed(context.Background(), []string{strings.Repeat("h", 1500) + "MIDDLE" + strings.Repeat("t", 500)})
			if (err != nil) != (mode != "unchanged") {
				t.Fatalf("err=%v", err)
			}
			if mode == "before_batch" && embeds != 1 {
				t.Fatal("embedded after tag changed")
			}
		})
	}
}
