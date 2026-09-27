# Hosting strata

The hosted service lets anyone embed a repository's mountain range without installing anything:

```markdown
![strata](https://<your-worker>.workers.dev/charmbracelet/lipgloss.svg)
```

It runs on free plans only:

```
README <img>  ──►  Cloudflare Worker ──► SVG in Workers KV?
                    (front door)          ├─ yes → served, cached
                                          └─ no  → "rendering…" image, and a render request ─┐
Render.com (free)   strata-server: clones, draws, writes the SVG into KV  ◄──────────────────┘
Cloudflare Pages    the gallery site
```

This file covers the server. The Worker and the site have their own sections once they exist.

## 1. Cloudflare: a KV namespace and a token

1. **KV namespace.** Dashboard → *Storage & Databases* → *Workers KV* → *Create* → name it `strata`.
   Copy its **namespace ID**.
2. **Account ID.** Shown in the dashboard's *Account home* (right-hand column), and in the URL.
3. **API token.** *My Profile* → *API Tokens* → *Create Token* → *Create Custom Token*:
   - Permissions: **Account · Workers KV Storage · Edit**
   - Account resources: include only your account

   Copy the token; Cloudflare shows it once.

## 2. GitHub: a read-only token

Without a token the server may call the GitHub API 60 times an hour; with one, 5,000.
*Settings* → *Developer settings* → *Fine-grained tokens* → *Generate new token*:
**Repository access: Public repositories (read-only)**, no extra permissions.

## 3. Render: deploy the server

1. *New* → *Blueprint* → pick this repository. Render reads [`render.yaml`](../render.yaml).
2. Fill in the values it asks for: `GITHUB_TOKEN`, `CF_ACCOUNT_ID`, `CF_KV_NAMESPACE_ID`, `CF_API_TOKEN`.
   `STRATA_SECRET` is generated for you; open it afterwards and copy it, the Worker needs the same
   value.
3. Deploy. The service gets a URL like `https://strata-server-xxxx.onrender.com`.

Check it (the first request can take a minute while a sleeping free instance wakes up):

```sh
curl https://strata-server-xxxx.onrender.com/healthz
curl -X POST https://strata-server-xxxx.onrender.com/render \
  -H "Authorization: Bearer $STRATA_SECRET" \
  -d '{"repo": "charmbracelet/lipgloss"}'
```

The second call answers `{"status":"queued"}` at once; about 15 seconds later the key
`charmbracelet/lipgloss` appears in the KV namespace (Dashboard → Workers KV → strata → *KV Pairs*).
Render's *Logs* tab shows one JSON line per render.

## Settings

| Variable | Default | |
| --- | --- | --- |
| `STRATA_SECRET` | *required* | shared with the Worker; requests without it get 401 |
| `CF_ACCOUNT_ID`, `CF_KV_NAMESPACE_ID`, `CF_API_TOKEN` | | where SVGs are written |
| `STRATA_STORE_DIR` | | write to a folder instead of KV (local use) |
| `GITHUB_TOKEN` | | raises the GitHub API limit |
| `STRATA_MAX_COMMITS` | `15000` | longer mainline histories are refused |
| `STRATA_MAX_REPO_MB` | `300` | larger repositories are refused before cloning |
| `STRATA_QUEUE` | `50` | pending renders beyond this get 503 |

Only public repositories are drawn. A repository that cannot be drawn gets an image saying why
(not found or private, too large, history too long), stored for an hour (a day for size limits) so
the Worker does not keep asking.

### What the free plan can do

Measured in Docker with Render's free limits (0.1 CPU, 512 MB):

| Repository | Commits | Render time | Peak memory |
| --- | --- | --- | --- |
| charmbracelet/lipgloss | 457 | 15 s | — |
| junegunn/fzf | 3,488 | 1 min 57 s | 129 MB |

Time grows with commits (about 30 a second), which is why `STRATA_MAX_COMMITS` exists. Renders run
one at a time.

## Run it locally

```sh
docker build -f deploy/server/Dockerfile -t strata-server .
docker run --rm -p 8080:8080 -e STRATA_SECRET=dev -e STRATA_STORE_DIR=/tmp/store strata-server

curl -X POST localhost:8080/render -H 'Authorization: Bearer dev' -d '{"repo":"charmbracelet/lipgloss"}'
curl -H 'Authorization: Bearer dev' localhost:8080/svg/charmbracelet/lipgloss.svg > lipgloss.svg
```

With `STRATA_STORE_DIR`, `GET /svg/{owner}/{repo}` serves what was rendered, so the whole flow can
be tried without Cloudflare.
