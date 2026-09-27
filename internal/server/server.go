package server

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/umuttopalak/strata/internal/gitlog"
	"github.com/umuttopalak/strata/internal/render"
	"github.com/umuttopalak/strata/internal/timeline"
)

// How long error images live before the Worker asks again.
const (
	errorTTL    = time.Hour
	tooLargeTTL = 24 * time.Hour
)

// repoPattern accepts GitHub owner and repository names.
var repoPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})/[A-Za-z0-9._-]{1,100}$`)

// Job is one render request.
type Job struct {
	Owner, Name string
	Labels      bool
}

// tooLong explains a history the server will not draw; n is 0 when the
// count is unknown (the render timed out).
func tooLong(n, limit int) []string {
	first := "this repository's history is too long to draw here"
	if n > 0 {
		first = fmt.Sprintf("this repository's history is too long to draw here (%d commits, limit %d)", n, limit)
	}
	return []string{first, "run strata locally or use the GitHub Action instead"}
}

// Key is where the job's SVG is stored. GitHub names are case-insensitive,
// so keys are lower case; the Worker builds the same key.
func (j Job) Key() string {
	k := strings.ToLower(j.Owner + "/" + j.Name)
	if j.Labels {
		k += "?labels"
	}
	return k
}

// Server accepts render requests and works through them one at a time:
// the free hosts it targets have a fraction of a CPU.
type Server struct {
	cfg    Config
	github *GitHub
	store  Store
	log    *slog.Logger

	// cloneURL is where a repository is fetched from; tests point it at
	// local fixtures.
	cloneURL func(owner, name string) string

	mu      sync.Mutex
	pending map[string]bool
	jobs    chan Job
}

// New builds a server; call Run to start working through the queue.
func New(cfg Config, gh *GitHub, store Store, log *slog.Logger) *Server {
	return &Server{
		cfg: cfg, github: gh, store: store, log: log,
		cloneURL: func(owner, name string) string {
			return fmt.Sprintf("https://github.com/%s/%s.git", owner, name)
		},
		pending: map[string]bool{},
		jobs:    make(chan Job, cfg.QueueSize),
	}
}

// Run renders queued jobs until ctx is cancelled.
func (s *Server) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-s.jobs:
			s.process(ctx, j)
			s.mu.Lock()
			delete(s.pending, j.Key())
			s.mu.Unlock()
		}
	}
}

// Handler routes:
//
//	GET  /healthz              liveness, also used to wake a sleeping host
//	POST /render               {"repo": "owner/name", "labels": false}; needs the secret
//	GET  /svg/{owner}/{name}   the stored SVG, only with a local STRATA_STORE_DIR
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("POST /render", s.authorized(s.handleRender))
	mux.HandleFunc("GET /svg/{owner}/{name}", s.authorized(s.handleSVG))
	return mux
}

func (s *Server) authorized(next http.HandlerFunc) http.HandlerFunc {
	want := []byte("Bearer " + s.cfg.Secret)
	return func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), want) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) handleRender(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Repo   string `json:"repo"`
		Labels bool   `json:"labels"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil ||
		!repoPattern.MatchString(req.Repo) {
		http.Error(w, `body must be {"repo": "owner/name"}`, http.StatusBadRequest)
		return
	}
	owner, name, _ := strings.Cut(req.Repo, "/")
	j := Job{Owner: owner, Name: name, Labels: req.Labels}

	status, code := s.enqueue(j)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"status": status, "key": j.Key()})
}

// enqueue adds a job unless the same one is already waiting or running.
func (s *Server) enqueue(j Job) (status string, code int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending[j.Key()] {
		return "already queued", http.StatusOK
	}
	select {
	case s.jobs <- j:
		s.pending[j.Key()] = true
		return "queued", http.StatusAccepted
	default:
		return "queue full, try again later", http.StatusServiceUnavailable
	}
}

func (s *Server) handleSVG(w http.ResponseWriter, r *http.Request) {
	dir, ok := s.store.(DirStore)
	if !ok {
		http.Error(w, "only available with STRATA_STORE_DIR", http.StatusNotFound)
		return
	}
	owner, name := r.PathValue("owner"), strings.TrimSuffix(r.PathValue("name"), ".svg")
	if !repoPattern.MatchString(owner + "/" + name) {
		http.Error(w, "bad repository name", http.StatusBadRequest)
		return
	}
	j := Job{Owner: owner, Name: name, Labels: r.URL.Query().Has("labels")}
	svg, meta, err := dir.Get(j.Key())
	if err != nil {
		http.Error(w, "not rendered yet", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("X-Strata-Status", meta.Status)
	w.Write(svg)
}

// process renders one job and stores the result, or an image explaining
// why there is none.
func (s *Server) process(ctx context.Context, j Job) {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.JobTimeout)
	defer cancel()
	start := time.Now()
	log := s.log.With("repo", j.Owner+"/"+j.Name, "labels", j.Labels)

	svg, meta, ttl, err := s.render(ctx, j)
	if err != nil {
		log.Warn("render failed", "err", err, "took", time.Since(start).Round(time.Millisecond).String())
	}
	if err := s.store.Put(ctx, j.Key(), svg, meta, ttl); err != nil {
		log.Error("store failed", "err", err)
		return
	}
	log.Info("stored", "status", meta.Status, "commits", meta.Commits,
		"bytes", len(svg), "took", time.Since(start).Round(time.Millisecond).String())
}

// render returns the SVG to store. On failure it returns an error image,
// its metadata and a short ttl, together with the error for logging.
func (s *Server) render(ctx context.Context, j Job) ([]byte, Meta, time.Duration, error) {
	title := j.Owner + "/" + j.Name
	fail := func(ttl time.Duration, err error, lines ...string) ([]byte, Meta, time.Duration, error) {
		var b bytes.Buffer
		render.WriteMessageSVG(&b, title, lines...)
		return b.Bytes(), Meta{Status: "error", RenderedAt: time.Now().Unix(), Message: lines[0]}, ttl, err
	}

	info, err := s.github.Lookup(ctx, j.Owner, j.Name)
	switch {
	case errors.Is(err, ErrNotFound):
		return fail(errorTTL, err, "repository not found, or it is private",
			"strata can only draw public GitHub repositories here")
	case err != nil:
		return fail(errorTTL, err, "could not reach GitHub, trying again later")
	case info.SizeKB > s.cfg.MaxRepoMB*1024:
		return fail(tooLargeTTL, fmt.Errorf("%d MB is over the limit", info.SizeKB/1024),
			fmt.Sprintf("this repository is too large to draw here (%d MB, limit %d MB)", info.SizeKB/1024, s.cfg.MaxRepoMB),
			"run strata locally or use the GitHub Action instead")
	}

	tmp, err := os.MkdirTemp("", "strata-server-")
	if err != nil {
		return fail(errorTTL, err, "could not draw this repository, trying again later")
	}
	defer gitlog.RemoveClone(tmp)
	dir := filepath.Join(tmp, "repo.git")
	if err := gitlog.Clone(ctx, s.cloneURL(j.Owner, j.Name), dir, nil); err != nil {
		return fail(errorTTL, err, "could not clone this repository, trying again later")
	}
	repo, err := gitlog.OpenRepo(ctx, dir)
	if errors.Is(err, gitlog.ErrEmptyRepo) {
		return fail(errorTTL, err, "this repository has no commits yet")
	}
	if err != nil {
		return fail(errorTTL, err, "could not read this repository")
	}
	// Reading history is what takes time on a small CPU, and it grows with
	// commits, not size: refuse long histories before spending minutes.
	if n, err := repo.CountCommits(ctx); err == nil && n > s.cfg.MaxCommits {
		return fail(tooLargeTTL, fmt.Errorf("%d commits is over the limit", n), tooLong(n, s.cfg.MaxCommits)...)
	}
	tl, err := timeline.Build(ctx, repo, gitlog.LogOptions{}, timeline.Options{})
	if errors.Is(err, context.DeadlineExceeded) {
		return fail(tooLargeTTL, err, tooLong(0, s.cfg.MaxCommits)...)
	}
	if err != nil {
		return fail(errorTTL, err, "could not read this repository's history")
	}

	layout := render.NewLayout(tl)
	layout.Labels = j.Labels
	opts := render.DefaultSVGOptions
	opts.Repo = info.Name
	var b bytes.Buffer
	if err := render.WriteSVG(&b, tl, layout, opts); err != nil {
		return fail(errorTTL, err, "could not draw this repository")
	}
	return b.Bytes(), Meta{Status: "ok", RenderedAt: time.Now().Unix(), Commits: tl.Commits}, 0, nil
}
