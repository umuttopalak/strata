package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/umuttopalak/strata/internal/testutil"
)

const secret = "s3cret"

// fakeGitHub answers /repos/{owner}/{name} for a few known repositories,
// ignoring case like GitHub does.
func fakeGitHub(t *testing.T) *httptest.Server {
	repos := map[string]RepoInfo{
		"/repos/o/good":   {Name: "good", FullName: "o/good", SizeKB: 10},
		"/repos/o/big":    {Name: "big", FullName: "o/big", SizeKB: 900 * 1024},
		"/repos/o/secret": {Name: "secret", FullName: "o/secret", Private: true},
		"/repos/o/empty":  {Name: "empty", FullName: "o/empty", SizeKB: 1},
		"/repos/o/long":   {Name: "long", FullName: "o/long", SizeKB: 1},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, ok := repos[strings.ToLower(r.URL.Path)]
		if !ok {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(info)
	}))
	t.Cleanup(srv.Close)
	return srv
}

type fixture struct {
	srv   *Server
	http  *httptest.Server
	store DirStore
}

func newFixture(t *testing.T, queue int) fixture {
	t.Helper()
	good := testutil.NewRepo(t)
	good.Write("src/a.go", testutil.Lines(40))
	good.Write("docs/b.md", testutil.Lines(10))
	good.Commit("first", testutil.Day(2020, 1, 1))
	good.Write("src/a.go", testutil.Lines(60))
	good.Commit("second", testutil.Day(2020, 2, 1))
	empty := testutil.NewRepo(t)

	long := testutil.NewRepo(t)
	for i := range 3 {
		long.Write("a", testutil.Lines(i+1))
		long.Commit("c", testutil.Day(2020, 1, 1+i))
	}

	// good has 2 commits, long has 3: one over the limit.
	cfg := Config{Secret: secret, MaxRepoMB: 300, MaxCommits: 2, QueueSize: queue, JobTimeout: time.Minute}
	store := DirStore{Dir: t.TempDir()}
	gh := &GitHub{BaseURL: fakeGitHub(t).URL}
	s := New(cfg, gh, store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.cloneURL = func(owner, name string) string {
		switch name {
		case "empty":
			return "file://" + empty.Dir
		case "long":
			return "file://" + long.Dir
		}
		return "file://" + good.Dir
	}
	h := httptest.NewServer(s.Handler())
	t.Cleanup(h.Close)
	return fixture{srv: s, http: h, store: store}
}

func (f fixture) post(t *testing.T, body, auth string) (int, map[string]string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, f.http.URL+"/render", strings.NewReader(body))
	if auth != "" {
		req.Header.Set("Authorization", "Bearer "+auth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]string
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// waitFor polls the store until key has been written.
func (f fixture) waitFor(t *testing.T, key string) ([]byte, Meta) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if svg, meta, err := f.store.Get(key); err == nil {
			return svg, meta
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s was never stored", key)
	return nil, Meta{}
}

func TestRenderEndToEnd(t *testing.T) {
	f := newFixture(t, 10)
	code, out := f.post(t, `{"repo":"O/Good"}`, secret)
	if code != http.StatusAccepted || out["key"] != "o/good" {
		t.Fatalf("POST = %d %v", code, out)
	}
	// Until the worker picks it up, a repeat request is deduplicated.
	if code, out := f.post(t, `{"repo":"o/good"}`, secret); code != http.StatusOK || out["status"] != "already queued" {
		t.Fatalf("repeat POST = %d %v", code, out)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go f.srv.Run(ctx)

	svg, meta := f.waitFor(t, "o/good")
	if meta.Status != "ok" || meta.Commits != 2 || meta.RenderedAt == 0 {
		t.Errorf("meta = %+v", meta)
	}
	if !strings.Contains(string(svg), "<animateTransform") || !strings.Contains(string(svg), ">good<") {
		t.Errorf("unexpected SVG: %.300s", svg)
	}

	// The same image is served back by the local-only endpoint.
	req, _ := http.NewRequest(http.MethodGet, f.http.URL+"/svg/o/good.svg", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/svg+xml" {
		t.Errorf("GET /svg = %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
}

func TestRenderErrorsBecomeImages(t *testing.T) {
	f := newFixture(t, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go f.srv.Run(ctx)

	tests := []struct{ repo, want string }{
		{"o/missing", "not found"},
		{"o/secret", "not found"},
		{"o/big", "too large"},
		{"o/empty", "no commits"},
		{"o/long", "too long"},
	}
	for _, tt := range tests {
		if code, _ := f.post(t, `{"repo":"`+tt.repo+`"}`, secret); code != http.StatusAccepted {
			t.Fatalf("%s: POST = %d", tt.repo, code)
		}
		svg, meta := f.waitFor(t, tt.repo)
		if meta.Status != "error" || !strings.Contains(meta.Message, tt.want) || !strings.Contains(string(svg), tt.want) {
			t.Errorf("%s: meta %+v", tt.repo, meta)
		}
	}
}

func TestRenderRequestValidation(t *testing.T) {
	f := newFixture(t, 1)
	if code, _ := f.post(t, `{"repo":"o/good"}`, ""); code != http.StatusUnauthorized {
		t.Errorf("no secret: %d", code)
	}
	if code, _ := f.post(t, `{"repo":"o/good"}`, "wrong"); code != http.StatusUnauthorized {
		t.Errorf("wrong secret: %d", code)
	}
	for _, body := range []string{`nope`, `{"repo":"no-slash"}`, `{"repo":"../../etc/passwd"}`, `{"repo":"a/b/c"}`} {
		if code, _ := f.post(t, body, secret); code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", body, code)
		}
	}
	// Queue of one, nothing running: the second distinct job is refused.
	if code, _ := f.post(t, `{"repo":"o/good"}`, secret); code != http.StatusAccepted {
		t.Errorf("first: %d", code)
	}
	if code, _ := f.post(t, `{"repo":"o/other"}`, secret); code != http.StatusServiceUnavailable {
		t.Errorf("queue full: %d", code)
	}
}

func TestJobKey(t *testing.T) {
	if k := (Job{Owner: "Charm", Name: "LipGloss", Labels: true}).Key(); k != "charm/lipgloss?labels" {
		t.Errorf("key = %q", k)
	}
}

func TestKVStorePut(t *testing.T) {
	var got struct {
		method, path, query, auth string
		value, meta               string
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path, got.query, got.auth = r.Method, r.URL.EscapedPath(), r.URL.RawQuery, r.Header.Get("Authorization")
		_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			p, err := mr.NextPart()
			if err != nil {
				break
			}
			b, _ := io.ReadAll(p)
			switch p.FormName() {
			case "value":
				got.value = string(b)
			case "metadata":
				got.meta = string(b)
			}
		}
		w.Write([]byte(`{"success":true}`))
	}))
	defer api.Close()

	s := &KVStore{BaseURL: api.URL, AccountID: "acc", NamespaceID: "ns", Token: "tok"}
	err := s.Put(context.Background(), "charm/lipgloss?labels", []byte("<svg/>"),
		Meta{Status: "error", RenderedAt: 42, Message: "x"}, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got.method != http.MethodPut ||
		got.path != "/accounts/acc/storage/kv/namespaces/ns/values/charm%2Flipgloss%3Flabels" ||
		got.query != "expiration_ttl=60" || got.auth != "Bearer tok" ||
		got.value != "<svg/>" || got.meta != `{"status":"error","renderedAt":42,"message":"x"}` {
		t.Errorf("request = %+v", got)
	}

	api.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"success":false}`, http.StatusForbidden)
	})
	if err := s.Put(context.Background(), "k", nil, Meta{}, 0); err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %v", err)
	}
}

func TestConfigFromEnv(t *testing.T) {
	clear := func() {
		for _, k := range []string{"STRATA_SECRET", "CF_ACCOUNT_ID", "CF_KV_NAMESPACE_ID", "CF_API_TOKEN",
			"STRATA_STORE_DIR", "STRATA_MAX_REPO_MB", "STRATA_QUEUE", "PORT"} {
			t.Setenv(k, "")
		}
	}
	clear()
	if _, err := ConfigFromEnv(); err == nil {
		t.Error("missing secret should fail")
	}
	t.Setenv("STRATA_SECRET", "x")
	if _, err := ConfigFromEnv(); err == nil {
		t.Error("missing store should fail")
	}
	t.Setenv("CF_API_TOKEN", "t")
	if _, err := ConfigFromEnv(); err == nil {
		t.Error("partial Cloudflare settings should fail")
	}
	t.Setenv("CF_ACCOUNT_ID", "a")
	t.Setenv("CF_KV_NAMESPACE_ID", "n")
	t.Setenv("PORT", "9000")
	c, err := ConfigFromEnv()
	if err != nil || c.Addr != ":9000" || c.MaxRepoMB != 300 {
		t.Errorf("config = %+v, %v", c, err)
	}
	t.Setenv("STRATA_MAX_REPO_MB", "lots")
	if _, err := ConfigFromEnv(); err == nil {
		t.Error("bad number should fail")
	}
}
