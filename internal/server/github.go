package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// ErrNotFound means the repository does not exist or is private: GitHub
// answers both the same way to anyone without access.
var ErrNotFound = errors.New("repository not found or private")

// RepoInfo is what the server needs to know before cloning.
type RepoInfo struct {
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Private  bool   `json:"private"`
	SizeKB   int    `json:"size"` // as GitHub reports it, roughly the packed repository
}

// GitHub looks repositories up through the REST API.
type GitHub struct {
	BaseURL string // https://api.github.com
	Token   string
	Client  *http.Client
}

// Lookup returns a public repository's details.
func (g *GitHub) Lookup(ctx context.Context, owner, name string) (RepoInfo, error) {
	u := fmt.Sprintf("%s/repos/%s/%s", g.BaseURL, url.PathEscape(owner), url.PathEscape(name))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return RepoInfo{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "strata-server")
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}
	client := g.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return RepoInfo{}, fmt.Errorf("github: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound, http.StatusUnavailableForLegalReasons:
		return RepoInfo{}, ErrNotFound
	default:
		return RepoInfo{}, fmt.Errorf("github: %s", resp.Status)
	}
	var info RepoInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return RepoInfo{}, fmt.Errorf("github: %w", err)
	}
	if info.Private {
		return RepoInfo{}, ErrNotFound // only reachable with a token that can see it
	}
	return info, nil
}
