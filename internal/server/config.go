// Package server renders strata SVGs for public GitHub repositories on
// request and stores them where the Cloudflare Worker serves them from.
package server

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config is read from the environment, as Render and similar hosts set it.
type Config struct {
	Addr   string // PORT, default 8080
	Secret string // STRATA_SECRET: shared with the Worker, required

	GitHubToken string // GITHUB_TOKEN: optional, raises the API limit from 60 to 5000 requests an hour
	MaxRepoMB   int    // STRATA_MAX_REPO_MB: repositories larger than this are refused (default 300)
	MaxCommits  int    // STRATA_MAX_COMMITS: longer mainline histories are refused (default 15000)
	QueueSize   int    // STRATA_QUEUE: pending renders beyond this are refused (default 50)
	JobTimeout  time.Duration

	// Where SVGs go: Cloudflare KV when these are set…
	CFAccountID   string // CF_ACCOUNT_ID
	CFNamespaceID string // CF_KV_NAMESPACE_ID
	CFAPIToken    string // CF_API_TOKEN: needs "Workers KV Storage: Edit"
	// …or a local folder, for development.
	StoreDir string // STRATA_STORE_DIR
}

// ConfigFromEnv reads and checks the configuration.
func ConfigFromEnv() (Config, error) {
	c := Config{
		Addr:          ":" + env("PORT", "8080"),
		Secret:        os.Getenv("STRATA_SECRET"),
		GitHubToken:   os.Getenv("GITHUB_TOKEN"),
		CFAccountID:   os.Getenv("CF_ACCOUNT_ID"),
		CFNamespaceID: os.Getenv("CF_KV_NAMESPACE_ID"),
		CFAPIToken:    os.Getenv("CF_API_TOKEN"),
		StoreDir:      os.Getenv("STRATA_STORE_DIR"),
		JobTimeout:    10 * time.Minute,
	}
	var err error
	if c.MaxRepoMB, err = envInt("STRATA_MAX_REPO_MB", 300); err != nil {
		return c, err
	}
	if c.MaxCommits, err = envInt("STRATA_MAX_COMMITS", 15000); err != nil {
		return c, err
	}
	if c.QueueSize, err = envInt("STRATA_QUEUE", 50); err != nil {
		return c, err
	}

	if c.Secret == "" {
		return c, errors.New("STRATA_SECRET must be set (the Worker sends it with every request)")
	}
	kv := c.CFAccountID != "" || c.CFNamespaceID != "" || c.CFAPIToken != ""
	switch {
	case kv && (c.CFAccountID == "" || c.CFNamespaceID == "" || c.CFAPIToken == ""):
		return c, errors.New("CF_ACCOUNT_ID, CF_KV_NAMESPACE_ID and CF_API_TOKEN must be set together")
	case !kv && c.StoreDir == "":
		return c, errors.New("set CF_ACCOUNT_ID, CF_KV_NAMESPACE_ID and CF_API_TOKEN, or STRATA_STORE_DIR for local use")
	}
	return c, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%s must be a positive number (got %q)", key, v)
	}
	return n, nil
}
