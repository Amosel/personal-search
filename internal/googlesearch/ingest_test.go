package googlesearch

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"personal-search/internal/embed"
	"personal-search/internal/model"
	"personal-search/internal/qdrant"
	"strings"
	"testing"
)

func doc() model.Document {
	d := model.Document{Source: "google_search", Type: "search_query", Timestamp: 1767278400000, Text: "café", Metadata: map[string]any{"account": "personal", "url": "https://www.google.com/search?q=cafe", "raw": map[string]any{"title": "Searched for café"}, "provenance": []any{map[string]any{"locator": "fixture.json", "sha256": strings.Repeat("a", 64), "record_index": float64(0)}}}}
	h := sha256.Sum256([]byte(`["google_search","personal",1767278400000,"café"]`))
	d.ID = hex.EncodeToString(h[:])
	return d
}
func TestLoadBoundary(t *testing.T) {
	for _, tc := range []struct {
		name    string
		archive Archive
		bad     bool
	}{
		{"valid", Archive{1, "google_search", []model.Document{doc()}}, false},
		{"wrong source", Archive{1, "chatgpt", []model.Document{doc()}}, true},
		{"empty", Archive{1, "google_search", nil}, true},
		{"duplicate", Archive{1, "google_search", []model.Document{doc(), doc()}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, _ := json.Marshal(tc.archive)
			path := filepath.Join(t.TempDir(), "archive.json")
			os.WriteFile(path, data, 0600)
			_, err := Load(path)
			if (err != nil) != tc.bad {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
func TestQdrantWireAndReingest(t *testing.T) {
	stored := map[string]json.RawMessage{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			fmt.Fprint(w, `{"result":{"config":{"params":{"vectors":{"size":8,"distance":"Cosine"}}}}}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/scroll") {
			fmt.Fprint(w, `{"result":{"points":[],"next_page_offset":null}}`)
			return
		}
		if r.URL.Path != "/collections/google_search/points" || r.URL.Query().Get("wait") != "true" {
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(400)
			return
		}
		var body struct {
			Points []struct {
				ID      json.RawMessage `json:"id"`
				Payload map[string]any  `json:"payload"`
				Vector  []float32       `json:"vector"`
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		for _, point := range body.Points {
			if point.Payload["source"] != "google_search" || point.Payload["text"] != "café" || len(point.Vector) != 8 {
				t.Errorf("invalid point: %+v", point)
			}
			stored[string(point.ID)] = point.ID
		}
		fmt.Fprint(w, `{"result":{"status":"completed"}}`)
	}))
	defer srv.Close()
	for i := 0; i < 2; i++ {
		n, err := Run(context.Background(), []model.Document{doc()}, "google_search", 1, &testEmbedder{embed.FakeEmbedder{DimVal: 8}}, qdrant.New(srv.URL))
		if err != nil || n != 1 {
			t.Fatalf("n=%d err=%v", n, err)
		}
	}
	if len(stored) != 1 {
		t.Fatal("reingest changed point IDs")
	}
}

type failedStore struct{ calls int }

func (s *failedStore) EnsureCollection(context.Context, string, int) error { return nil }
func (s *failedStore) Upsert(context.Context, string, []qdrant.Point) error {
	s.calls++
	if s.calls == 2 {
		return fmt.Errorf("unavailable")
	}
	return nil
}
func TestPartialFailureCount(t *testing.T) {
	second := doc()
	second.Metadata["account"] = "work"
	h := sha256.Sum256([]byte(`["google_search","work",1767278400000,"café"]`))
	second.ID = hex.EncodeToString(h[:])
	n, err := Run(context.Background(), []model.Document{doc(), second}, "google_search", 1, &testEmbedder{embed.FakeEmbedder{DimVal: 8}}, &failedStore{})
	if err == nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

type testEmbedder struct{ embed.FakeEmbedder }

func (*testEmbedder) EmbeddingIdentity() string                                   { return "fake:test:v1" }
func (*failedStore) ValidateEmbedding(context.Context, string, string, int) error { return nil }

func TestArchiveMetadataAndIdentity(t *testing.T) {
	for _, field := range []string{"account", "url", "raw", "provenance"} {
		d := doc()
		delete(d.Metadata, field)
		if err := ValidateDocuments([]model.Document{d}); err == nil {
			t.Fatalf("accepted missing %s", field)
		}
	}
	d := doc()
	d.Metadata["account"] = "another"
	if err := ValidateDocuments([]model.Document{d}); err == nil {
		t.Fatal("accepted wrong account identity")
	}
	d = doc()
	d.Metadata["embedding_identity"] = "foreign"
	if err := ValidateDocuments([]model.Document{d}); err == nil {
		t.Fatal("accepted reserved metadata")
	}
	if got := identityString("<&\u2028\\u2028"); got != "\"<&\u2028\\\\u2028\"" {
		t.Fatalf("canonical quoting: %q", got)
	}
}

func TestLoadTakeoutHTML(t *testing.T) {
	html := `<html><body><div class="mdl-grid">` +
		`<div class="outer-cell mdl-cell mdl-cell--12-col mdl-shadow--2dp"><div class="mdl-grid"><div class="header-cell mdl-cell mdl-cell--12-col"><p class="mdl-typography--title">Search<br></p></div><div class="content-cell mdl-cell mdl-cell--6-col mdl-typography--body-1">Searched for&nbsp;<a href="https://www.google.com/search?q=caf%C3%A9">café</a><br>Oct 11, 2026, 9:42:57&#8239;AM EDT<br></div><div class="content-cell mdl-cell mdl-cell--6-col mdl-typography--body-1 mdl-typography--text-right"></div><div class="content-cell mdl-cell mdl-cell--12-col mdl-typography--caption"><b>Products:</b><br>&emsp;Search<br></div></div></div>` +
		`<div class="outer-cell mdl-cell mdl-cell--12-col mdl-shadow--2dp"><div class="mdl-grid"><div class="header-cell mdl-cell mdl-cell--12-col"><p class="mdl-typography--title">Search<br></p></div><div class="content-cell mdl-cell mdl-cell--6-col mdl-typography--body-1">Visited&nbsp;<a href="https://www.google.com/url?q=https://example.com/a&amp;usg=x">Example A</a><br>Jan 5, 2022, 2:13:01&#8239;PM EST<br></div><div class="content-cell mdl-cell mdl-cell--6-col mdl-typography--body-1 mdl-typography--text-right"></div><div class="content-cell mdl-cell mdl-cell--12-col mdl-typography--caption"><b>Products:</b><br>&emsp;Search<br></div></div></div>` +
		`<div class="outer-cell mdl-cell mdl-cell--12-col mdl-shadow--2dp"><div class="mdl-grid"><div class="header-cell mdl-cell mdl-cell--12-col"><p class="mdl-typography--title">Search<br></p></div><div class="content-cell mdl-cell mdl-cell--6-col mdl-typography--body-1">Used&nbsp;Search<br>Jan 4, 2022, 9:09:19&#8239;AM EST<br></div><div class="content-cell mdl-cell mdl-cell--6-col mdl-typography--body-1 mdl-typography--text-right"></div><div class="content-cell mdl-cell mdl-cell--12-col mdl-typography--caption"><b>Products:</b><br>&emsp;Search<br></div></div></div>` +
		`<div class="outer-cell mdl-cell mdl-cell--12-col mdl-shadow--2dp"><div class="mdl-grid"><div class="header-cell mdl-cell mdl-cell--12-col"><p class="mdl-typography--title">Search<br></p></div><div class="content-cell mdl-cell mdl-cell--6-col mdl-typography--body-1">Ran internet speed test<br><a href="https://support.google.com/websearch?p=speedtest&amp;hl=en">Test run by Measurement Lab</a><br>Sep 17, 2026, 7:37:01&#8239;PM EDT<br></div><div class="content-cell mdl-cell mdl-cell--6-col mdl-typography--body-1 mdl-typography--text-right"></div><div class="content-cell mdl-cell mdl-cell--12-col mdl-typography--caption"><b>Products:</b><br>&emsp;Search<br></div></div></div>` +
		`<div class="outer-cell mdl-cell mdl-cell--12-col mdl-shadow--2dp"><div class="mdl-grid"><div class="header-cell mdl-cell mdl-cell--12-col"><p class="mdl-typography--title">Image Search<br></p></div><div class="content-cell mdl-cell mdl-cell--6-col mdl-typography--body-1">Searched for&nbsp;<a href="https://www.google.com/search?q=image">image</a><br>Jan 4, 2022, 9:09:19&#8239;AM EST<br></div></div></div>` +
		`</div></body></html>`
	path := filepath.Join(t.TempDir(), "MyActivity.html")
	if err := os.WriteFile(path, []byte(html), 0600); err != nil {
		t.Fatal(err)
	}
	docs, err := LoadTakeoutHTMLFile(path, "amosel", "Takeout/My Activity/Search/MyActivity.html")
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 4 {
		t.Fatalf("docs=%d", len(docs))
	}
	if docs[0].Text != "café" || docs[0].Timestamp != 1791726177000 {
		t.Fatalf("first doc text/time = %q/%d", docs[0].Text, docs[0].Timestamp)
	}
	if docs[1].Text != "Example A" || docs[1].Metadata["url"] != "https://www.google.com/url?q=https://example.com/a&usg=x" {
		t.Fatalf("visit doc = %+v", docs[1])
	}
	if docs[2].Text != "Used Search" || docs[2].Metadata["url"] != "" {
		t.Fatalf("used doc = %+v", docs[2])
	}
	if docs[3].Text != "Test run by Measurement Lab" {
		t.Fatalf("speed test doc = %+v", docs[3])
	}
}

func TestLoadTakeoutZIPInfersAccount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "takeout.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range map[string]string{
		"Takeout/Google Account/amosel.SubscriberInfo.html":             "account",
		"Takeout/My Activity/Search/MyActivity.html":                    `<html><body><div class="outer-cell mdl-cell mdl-cell--12-col mdl-shadow--2dp"><div class="mdl-grid"><div class="header-cell mdl-cell mdl-cell--12-col"><p class="mdl-typography--title">Search<br></p></div><div class="content-cell mdl-cell mdl-cell--6-col mdl-typography--body-1">Searched for&nbsp;<a href="https://www.google.com/search?q=test">test</a><br>Oct 11, 2026, 9:42:57&#8239;AM EDT<br></div></div></div></body></html>`,
		"Takeout/YouTube and YouTube Music/history/search-history.html": "ignored",
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	docs, err := LoadTakeoutZIP(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := docs[0].Metadata["account"]; got != "amosel" {
		t.Fatalf("account=%v", got)
	}
}
