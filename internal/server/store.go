package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Meta travels with each stored SVG. The Worker reads it to decide whether
// an image is fresh, stale (serve it, but ask for a new one) or an error.
type Meta struct {
	Status     string `json:"status"` // "ok" or "error"
	RenderedAt int64  `json:"renderedAt"`
	Commits    int    `json:"commits,omitempty"`
	Message    string `json:"message,omitempty"`
}

// Store keeps rendered SVGs. ttl 0 means keep until overwritten.
type Store interface {
	Put(ctx context.Context, key string, svg []byte, meta Meta, ttl time.Duration) error
}

// KVStore writes to a Cloudflare Workers KV namespace over the REST API.
type KVStore struct {
	BaseURL     string // https://api.cloudflare.com/client/v4
	AccountID   string
	NamespaceID string
	Token       string
	Client      *http.Client
}

// Put stores the SVG with its metadata as JSON, as the Worker's
// getWithMetadata expects.
func (s *KVStore) Put(ctx context.Context, key string, svg []byte, meta Meta, ttl time.Duration) error {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("metadata", mustJSON(meta)); err != nil {
		return err
	}
	fw, err := mw.CreateFormFile("value", "value")
	if err != nil {
		return err
	}
	fw.Write(svg)
	if err := mw.Close(); err != nil {
		return err
	}

	u := fmt.Sprintf("%s/accounts/%s/storage/kv/namespaces/%s/values/%s",
		s.BaseURL, s.AccountID, s.NamespaceID, url.PathEscape(key))
	if ttl > 0 {
		// KV refuses expirations shorter than a minute.
		u += "?expiration_ttl=" + strconv.Itoa(int(max(ttl, time.Minute).Seconds()))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.Token)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("kv: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("kv: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	return nil
}

// DirStore writes key.svg and key.json files into a folder, for running
// the server locally without Cloudflare. It ignores ttl.
type DirStore struct{ Dir string }

func (s DirStore) Put(_ context.Context, key string, svg []byte, meta Meta, _ time.Duration) error {
	path := s.path(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path+".svg", svg, 0o644); err != nil {
		return err
	}
	return os.WriteFile(path+".json", []byte(mustJSON(meta)), 0o644)
}

// Get reads back what Put wrote.
func (s DirStore) Get(key string) ([]byte, Meta, error) {
	path := s.path(key)
	svg, err := os.ReadFile(path + ".svg")
	if err != nil {
		return nil, Meta{}, err
	}
	var meta Meta
	raw, err := os.ReadFile(path + ".json")
	if err == nil {
		err = json.Unmarshal(raw, &meta)
	}
	return svg, meta, err
}

// path maps "owner/repo?labels" to Dir/owner/repo_labels; keys are
// validated before they get here, so they cannot escape Dir.
func (s DirStore) path(key string) string {
	return filepath.Join(s.Dir, filepath.FromSlash(strings.ReplaceAll(key, "?", "_")))
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
