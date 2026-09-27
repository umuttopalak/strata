// Pure helpers for the strata Worker: no Workers-only APIs, so they can be
// tested with `node --test`.

// /owner/repo.svg, with GitHub's rules for owner and repository names.
const REPO_PATH = /^\/([A-Za-z0-9][A-Za-z0-9-]{0,38})\/([A-Za-z0-9._-]{1,100})\.svg$/;

/**
 * Reads a request URL. Returns null when it is not an SVG request.
 * The key matches strata-server's: lower case, "?labels" when labelled.
 */
export function parseRequest(url) {
  const m = REPO_PATH.exec(url.pathname);
  if (!m) return null;
  const [, owner, name] = m;
  const labels = url.searchParams.has("labels");
  let key = `${owner}/${name}`.toLowerCase();
  if (labels) key += "?labels";
  return { owner, name, labels, key };
}

/** How old a stored image may get before a new render is requested. */
export const STALE_SECONDS = 24 * 60 * 60;

export function isStale(metadata, nowSeconds) {
  return !metadata || !metadata.renderedAt || nowSeconds - metadata.renderedAt > STALE_SECONDS;
}

/**
 * Response headers for an SVG. maxAge 0 means "do not cache": GitHub's
 * image proxy then asks again, so a placeholder is replaced once the real
 * image exists.
 */
export function svgHeaders(maxAge) {
  return {
    "Content-Type": "image/svg+xml; charset=utf-8",
    "Cache-Control": maxAge > 0 ? `public, max-age=${maxAge}` : "no-cache, no-store, must-revalidate",
    // An SVG is a document; this one never needs scripts or outside resources.
    "Content-Security-Policy": "default-src 'none'; style-src 'unsafe-inline'",
    "X-Content-Type-Options": "nosniff",
    "Access-Control-Allow-Origin": "*",
  };
}

export function escapeXML(s) {
  return String(s).replace(/[&<>"']/g, (c) =>
    ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&#34;", "'": "&#39;" })[c],
  );
}

// Same look as strata's WriteMessageSVG (internal/render/svg.go).
const CELL_H = 16;
const PAD = 16;
const TITLE_BAR = 28;
const WIDTH = 2 * PAD + 100 * 8;

/** A small terminal-style image with a title and a few lines of text. */
export function messageSVG(title, lines) {
  const top = PAD + TITLE_BAR;
  const height = top + (lines.length + 2) * CELL_H + PAD;
  const baseline = (rowTop) => rowTop + (CELL_H * 3) / 4;
  const dots = ["#ff5f57", "#febc2e", "#28c840"]
    .map((c, i) => `<circle cx="${PAD + 6 + i * 20}" cy="${PAD + 6}" r="6" fill="${c}"/>`)
    .join("");
  const text = lines
    .map((l, i) => `<text x="${PAD}" y="${baseline(top + (i + 2) * CELL_H)}">${escapeXML(l)}</text>`)
    .join("");
  return (
    `<svg xmlns="http://www.w3.org/2000/svg" xml:space="preserve" width="${WIDTH}" height="${height}" viewBox="0 0 ${WIDTH} ${height}" role="img">` +
    `<title>strata · ${escapeXML(title)}</title>` +
    `<style>text{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,"Liberation Mono",monospace;font-size:13px;white-space:pre;fill:#8b949e}.b{font-weight:bold;fill:#e6edf3}</style>` +
    `<rect x="0.5" y="0.5" width="${WIDTH - 1}" height="${height - 1}" rx="8" fill="#0d1117" stroke="#30363d"/>` +
    dots +
    `<text x="${PAD}" y="${baseline(top)}">$ strata <tspan class="b">${escapeXML(title)}</tspan></text>` +
    text +
    `</svg>`
  );
}

export const RENDERING_LINES = [
  "drawing this repository's mountain range…",
  "it takes a minute or two the first time; refresh to see it",
];

/**
 * Asks GitHub whether a repository can be drawn, so the Worker can answer
 * "not found" or "too large" at once without waking the server.
 * Returns { ok: true } when it can, { ok: false, lines } when it cannot,
 * and { ok: true, unknown: true } when GitHub could not be asked (the
 * server checks again anyway).
 */
export async function checkRepo(fetchFn, { owner, name, token, maxRepoMB }) {
  const headers = { Accept: "application/vnd.github+json", "User-Agent": "strata-worker" };
  if (token) headers.Authorization = `Bearer ${token}`;
  let res;
  try {
    res = await fetchFn(
      `https://api.github.com/repos/${encodeURIComponent(owner)}/${encodeURIComponent(name)}`,
      { headers },
    );
  } catch {
    return { ok: true, unknown: true };
  }
  if (res.status === 404 || res.status === 451) return { ok: false, lines: NOT_FOUND };
  if (!res.ok) return { ok: true, unknown: true };
  const info = await res.json();
  if (info.private) return { ok: false, lines: NOT_FOUND };
  const mb = Math.floor((info.size || 0) / 1024);
  if (mb > maxRepoMB) {
    return {
      ok: false,
      lines: [
        `this repository is too large to draw here (${mb} MB, limit ${maxRepoMB} MB)`,
        "run strata locally or use the GitHub Action instead",
      ],
    };
  }
  return { ok: true };
}

const NOT_FOUND = [
  "repository not found, or it is private",
  "strata can only draw public GitHub repositories here",
];
