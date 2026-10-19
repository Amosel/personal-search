package googlesearch

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"math"
	"os"
	"path/filepath"
	"personal-search/internal/embed"
	"personal-search/internal/model"
	"personal-search/internal/qdrant"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Archive struct {
	SchemaVersion int              `json:"schema_version"`
	Source        string           `json:"source"`
	Documents     []model.Document `json:"documents"`
}

func Load(path string) ([]model.Document, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var a Archive
	dec := json.NewDecoder(f)
	dec.UseNumber()
	if err := dec.Decode(&a); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("trailing data in archive")
	}
	if a.SchemaVersion != 1 || a.Source != "google_search" {
		return nil, fmt.Errorf("expected google_search archive schema 1")
	}
	if len(a.Documents) == 0 {
		return nil, fmt.Errorf("no search documents")
	}
	if err := ValidateDocuments(a.Documents); err != nil {
		return nil, err
	}

	return a.Documents, nil
}

func LoadPath(path, account string) ([]model.Document, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".zip":
		return LoadTakeoutZIP(path, account)
	case ".html", ".htm":
		if strings.TrimSpace(account) == "" {
			account = "personal"
		}
		return LoadTakeoutHTMLFile(path, account, path)
	default:
		return Load(path)
	}
}

func LoadTakeoutZIP(path, account string) ([]model.Document, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	if strings.TrimSpace(account) == "" {
		account = inferGoogleAccount(zr.File)
	}
	if strings.TrimSpace(account) == "" {
		return nil, fmt.Errorf("account namespace required for raw Takeout zip")
	}
	var docs []model.Document
	for _, f := range zr.File {
		if f.Name != "Takeout/My Activity/Search/MyActivity.html" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(rc)
		closeErr := rc.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		docs, err = parseTakeoutSearchHTML(data, account, f.Name)
		if err != nil {
			return nil, err
		}
		break
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("Takeout/My Activity/Search/MyActivity.html not found or produced no documents")
	}
	if err := ValidateDocuments(docs); err != nil {
		return nil, err
	}
	return docs, nil
}

func LoadTakeoutHTMLFile(path, account, locator string) ([]model.Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	docs, err := parseTakeoutSearchHTML(data, account, locator)
	if err != nil {
		return nil, err
	}
	if err := ValidateDocuments(docs); err != nil {
		return nil, err
	}
	return docs, nil
}

// PointID uses a deterministic 128-bit UUID derived from the canonical event ID.
func PointID(id string) string {
	h := sha256.Sum256([]byte("google_search:" + id))
	v := hex.EncodeToString(h[:16])
	return v[:8] + "-" + v[8:12] + "-" + v[12:16] + "-" + v[16:20] + "-" + v[20:]
}

func ValidateDocuments(docs []model.Document) error {
	if len(docs) == 0 {
		return fmt.Errorf("no search documents")
	}
	seen := map[string]bool{}
	for i, d := range docs {
		if err := d.Validate(); err != nil {
			return fmt.Errorf("document %d: %w", i, err)
		}
		if d.Source != "google_search" || d.Type != "search_query" || seen[d.ID] {
			return fmt.Errorf("invalid source/type or duplicate ID at document %d", i)
		}
		seen[d.ID] = true
		account, ok := d.Metadata["account"].(string)
		if !ok || strings.TrimSpace(account) == "" {
			return fmt.Errorf("document %d: account namespace required", i)
		}
		if d.ID != documentID(account, d.Timestamp, d.Text) {
			return fmt.Errorf("document %d: ID does not match account/timestamp/query identity", i)
		}
		if _, ok := d.Metadata["url"].(string); !ok {
			return fmt.Errorf("document %d: original URL field required", i)
		}
		if raw, ok := d.Metadata["raw"].(map[string]any); !ok || raw == nil {
			return fmt.Errorf("document %d: raw record required", i)
		}
		refs, ok := d.Metadata["provenance"].([]any)
		if !ok || len(refs) == 0 {
			return fmt.Errorf("document %d: provenance required", i)
		}
		for _, ref := range refs {
			r, ok := ref.(map[string]any)
			if !ok {
				return fmt.Errorf("document %d: invalid provenance", i)
			}
			locator, _ := r["locator"].(string)
			digest, _ := r["sha256"].(string)
			index, ok := recordIndex(r["record_index"])
			digestBytes, err := hex.DecodeString(digest)
			if locator == "" || err != nil || len(digestBytes) != 32 || !ok || index < 0 || index != float64(int64(index)) {
				return fmt.Errorf("document %d: invalid provenance locator/hash/index", i)
			}
		}
		for _, key := range []string{"doc_id", "source", "type", "timestamp_unix_ms", "text", "embedding_identity"} {
			if _, exists := d.Metadata[key]; exists {
				return fmt.Errorf("reserved metadata field: %s", key)
			}
		}
	}
	return nil
}

var (
	cardStartRE = regexp.MustCompile(`<div class="outer-cell mdl-cell mdl-cell--12-col mdl-shadow--2dp">`)
	titleRE     = regexp.MustCompile(`(?s)<p class="mdl-typography--title">(.*?)<br></p>`)
	contentRE   = regexp.MustCompile(`(?s)<div class="content-cell mdl-cell mdl-cell--6-col mdl-typography--body-1">(.*?)</div>`)
	linkRE      = regexp.MustCompile(`(?s)<a href="([^"]*)">(.*?)</a>`)
	tagRE       = regexp.MustCompile(`(?s)<[^>]+>`)
	spaceRE     = regexp.MustCompile(`[ \t\r\n]+`)
)

func parseTakeoutSearchHTML(data []byte, account, locator string) ([]model.Document, error) {
	parts := cardStartRE.Split(string(data), -1)
	if len(parts) < 2 {
		return nil, fmt.Errorf("no Google Takeout activity cards found")
	}
	docs := make([]model.Document, 0, len(parts)-1)
	seen := map[string]bool{}
	for i, part := range parts[1:] {
		card := `<div class="outer-cell mdl-cell mdl-cell--12-col mdl-shadow--2dp">` + part
		title := firstClean(titleRE, card)
		if title != "Search" {
			continue
		}
		content := contentRE.FindStringSubmatch(card)
		if len(content) != 2 {
			return nil, fmt.Errorf("%s card %d: content shape not recognized", locator, i)
		}
		actionHTML, timestampText, err := splitTakeoutContent(content[1])
		if err != nil {
			return nil, fmt.Errorf("%s card %d: %w", locator, i, err)
		}
		t, err := parseTakeoutTime(timestampText)
		if err != nil {
			return nil, fmt.Errorf("%s card %d: %w", locator, i, err)
		}
		action, href, text := parseTakeoutAction(actionHTML)
		if text == "" {
			text = action
		}
		if text == "" {
			return nil, fmt.Errorf("%s card %d: empty activity text", locator, i)
		}
		h := sha256.Sum256([]byte(card))
		rawHash := hex.EncodeToString(h[:])
		raw := map[string]any{
			"title":          title,
			"action":         action,
			"text":           text,
			"url":            href,
			"time":           timestampText,
			"source_format":  "google_takeout_my_activity_html",
			"source_locator": locator,
		}
		d := model.Document{
			Source:    "google_search",
			Type:      "search_query",
			Timestamp: t.UnixMilli(),
			Text:      text,
			Metadata: map[string]any{
				"account": account,
				"url":     href,
				"raw":     raw,
				"provenance": []any{map[string]any{
					"locator":      locator,
					"sha256":       rawHash,
					"record_index": float64(i),
				}},
			},
		}
		d.ID = documentID(account, d.Timestamp, d.Text)
		if seen[d.ID] {
			continue
		}
		seen[d.ID] = true
		docs = append(docs, d)
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("no Search activity documents parsed from %s", locator)
	}
	return docs, nil
}

func parseTakeoutAction(s string) (action, href, text string) {
	if m := linkRE.FindStringSubmatch(s); len(m) == 3 {
		href = html.UnescapeString(m[1])
		text = cleanHTML(m[2])
		action = cleanHTML(s[:strings.Index(s, m[0])])
		return action, href, text
	}
	action = cleanHTML(s)
	return action, "", action
}

func splitTakeoutContent(s string) (actionHTML, timestampText string, err error) {
	lines := strings.Split(s, "<br>")
	nonEmpty := make([]string, 0, len(lines))
	for _, line := range lines {
		if cleanHTML(line) != "" {
			nonEmpty = append(nonEmpty, line)
		}
	}
	if len(nonEmpty) < 2 {
		return "", "", fmt.Errorf("content has fewer than two nonempty lines")
	}
	timestampText = cleanHTML(nonEmpty[len(nonEmpty)-1])
	actionHTML = strings.Join(nonEmpty[:len(nonEmpty)-1], " ")
	return actionHTML, timestampText, nil
}

func parseTakeoutTime(s string) (time.Time, error) {
	cleaned := normalizeSpace(s)
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return time.Time{}, err
	}
	t, err := time.ParseInLocation("Jan 2, 2006, 3:04:05 PM MST", cleaned, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time %q: %w", s, err)
	}
	return t, nil
}

func firstClean(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return cleanHTML(m[1])
}

func cleanHTML(s string) string {
	s = strings.ReplaceAll(s, "<br>", "\n")
	s = tagRE.ReplaceAllString(s, "")
	return normalizeSpace(html.UnescapeString(s))
}

func normalizeSpace(s string) string {
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = strings.ReplaceAll(s, "\u202f", " ")
	s = strings.ReplaceAll(s, "\u200e", "")
	s = strings.ReplaceAll(s, "\u200f", "")
	return strings.TrimSpace(spaceRE.ReplaceAllString(s, " "))
}

func inferGoogleAccount(files []*zip.File) string {
	for _, f := range files {
		if !strings.HasPrefix(f.Name, "Takeout/Google Account/") || !strings.HasSuffix(f.Name, ".SubscriberInfo.html") {
			continue
		}
		base := filepath.Base(f.Name)
		return strings.TrimSuffix(base, ".SubscriberInfo.html")
	}
	return ""
}

func documentID(account string, timestamp int64, text string) string {
	identity := "[\"google_search\"," + identityString(account) + "," + strconv.FormatInt(timestamp, 10) + "," + identityString(text) + "]"
	hash := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(hash[:])
}

type Store interface {
	EnsureCollection(context.Context, string, int) error
	ValidateEmbedding(context.Context, string, string, int) error
	Upsert(context.Context, string, []qdrant.Point) error
}
type IdentifiedEmbedder interface {
	embed.Embedder
	EmbeddingIdentity() string
}

// Run returns only the count acknowledged by Qdrant, including on partial failure.
func Run(ctx context.Context, docs []model.Document, collection string, batch int, emb IdentifiedEmbedder, store Store) (int, error) {
	if batch <= 0 || collection == "" || emb == nil || store == nil {
		return 0, fmt.Errorf("collection, positive batch, embedder and store required")
	}
	if err := ValidateDocuments(docs); err != nil {
		return 0, err
	}
	if emb.EmbeddingIdentity() == "" {
		return 0, fmt.Errorf("embedding identity required")
	}
	if err := ctx.Err(); err != nil {
		return 0, &model.IngestError{Stage: "canceled", Progress: model.IngestProgress{Classified: len(docs), Selected: len(docs), NotAttempted: len(docs)}, Err: err}
	}
	if emb.Dim() <= 0 {
		return 0, fmt.Errorf("embedding dimension must be positive")
	}
	if err := store.EnsureCollection(ctx, collection, emb.Dim()); err != nil {
		return 0, err
	}
	if err := store.ValidateEmbedding(ctx, collection, emb.EmbeddingIdentity(), emb.Dim()); err != nil {
		return 0, err
	}
	completed := 0
	progress := model.IngestProgress{Classified: len(docs), Selected: len(docs)}
	fail := func(stage string, start, end int, err error) (int, error) {
		progress.Acknowledged = completed
		progress.Recount()
		return completed, &model.IngestError{Stage: stage, Batch: &model.BatchRange{Start: start, End: end}, Progress: progress, Err: err}
	}
	for i := 0; i < len(docs); i += batch {
		end := i + batch
		if end > len(docs) {
			end = len(docs)
		}
		if err := ctx.Err(); err != nil {
			return fail("canceled", i, end, err)
		}
		texts := make([]string, end-i)
		for j := i; j < end; j++ {
			texts[j-i] = docs[j].Text
		}
		vectors, err := emb.Embed(ctx, texts)
		if err != nil {
			return fail("embed", i, end, err)
		}
		if len(vectors) != len(texts) {
			return fail("embed", i, end, fmt.Errorf("embedding count mismatch"))
		}
		points := make([]qdrant.Point, len(texts))
		for j := i; j < end; j++ {
			d := docs[j]
			vector := vectors[j-i]
			if len(vector) != emb.Dim() {
				return fail("embed", i, end, fmt.Errorf("embedding dimension mismatch"))
			}
			for _, x := range vector {
				if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
					return fail("embed", i, end, fmt.Errorf("nonfinite embedding value"))
				}
			}
			payload := map[string]any{}
			for k, v := range d.Metadata {
				payload[k] = v
			}
			payload["embedding_identity"] = emb.EmbeddingIdentity()
			payload["doc_id"] = d.ID
			payload["source"] = d.Source
			payload["type"] = d.Type
			payload["text"] = d.Text
			payload["timestamp_unix_ms"] = d.Timestamp
			id, err := qdrant.UUIDID(PointID(d.ID))
			if err != nil {
				return fail("point_id", i, end, err)
			}
			points[j-i] = qdrant.Point{ID: id, Vector: vector, Payload: payload}
		}
		if err := ctx.Err(); err != nil {
			return fail("canceled", i, end, err)
		}
		if err := store.Upsert(ctx, collection, points); err != nil {
			progress.Uncertain = end - i
			progress.UncertainBatches = []model.BatchRange{{Start: i, End: end}}
			return fail("upsert", i, end, fmt.Errorf("completion unknown; may have been applied: %w", err))
		}
		completed += len(points)
	}
	if err := ctx.Err(); err != nil {
		return fail("canceled", len(docs), len(docs), err)
	}
	return completed, nil
}

// Match the crawler's Python ensure_ascii=False canonical JSON, including U+2028.
func identityString(value string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range value {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 32 {
				fmt.Fprintf(&b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func recordIndex(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		i, err := n.Int64()
		return float64(i), err == nil
	default:
		return 0, false
	}
}
