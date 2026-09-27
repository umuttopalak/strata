// strata Worker: serves /owner/repo.svg from Workers KV and asks
// strata-server to draw what is missing or stale. Everything else is the
// static site in ../public.
//
// Bindings (wrangler.toml): SVGS (KV), ASSETS (static files),
// SERVER_URL and MAX_REPO_MB (vars), STRATA_SECRET and GITHUB_TOKEN (secrets).

import {
  RENDERING_LINES,
  checkRepo,
  isStale,
  messageSVG,
  parseRequest,
  svgHeaders,
} from "./strata.js";

// Browser and GitHub-proxy cache lifetimes.
const OK_MAX_AGE = 60 * 60;
const ERROR_MAX_AGE = 10 * 60;

// Keys this isolate asked the server about recently. Best effort only (the
// Cache API does not work on workers.dev); the server also merges repeats.
const asked = new Map();
const ASK_EVERY_MS = 60 * 1000;

export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);
    const job = parseRequest(url);
    if (!job) return env.ASSETS.fetch(request);
    if (request.method !== "GET" && request.method !== "HEAD") {
      return new Response("method not allowed", { status: 405, headers: { Allow: "GET, HEAD" } });
    }
    const title = `${job.owner}/${job.name}`;
    const reply = (svg, maxAge, status) =>
      new Response(request.method === "HEAD" ? null : svg, { headers: svgHeaders(maxAge, status) });

    const { value, metadata } = await env.SVGS.getWithMetadata(job.key, { type: "text" });
    if (value !== null) {
      if (isStale(metadata, Date.now() / 1000)) ctx.waitUntil(askServer(env, job));
      const ok = metadata?.status === "ok";
      return reply(value, ok ? OK_MAX_AGE : ERROR_MAX_AGE, ok ? "ok" : "error");
    }

    // Nothing stored yet. Refuse what cannot be drawn right away, so made-up
    // names cost neither a KV write nor a server wake-up.
    const check = await checkRepo(fetch, {
      owner: job.owner,
      name: job.name,
      token: env.GITHUB_TOKEN,
      maxRepoMB: Number(env.MAX_REPO_MB) || 300,
    });
    if (!check.ok) return reply(messageSVG(title, check.lines), ERROR_MAX_AGE, "refused");

    ctx.waitUntil(askServer(env, job));
    return reply(messageSVG(title, RENDERING_LINES), 0, "rendering");
  },
};

// askServer requests a render. A sleeping free server can take a minute to
// answer; the request is dropped after 25 seconds (the most waitUntil
// allows), but it has woken the server, and the next viewer asks again.
async function askServer(env, job) {
  const now = Date.now();
  if (now - (asked.get(job.key) ?? 0) < ASK_EVERY_MS) return;
  asked.set(job.key, now);
  if (asked.size > 1000) asked.clear();

  try {
    const res = await fetch(`${env.SERVER_URL}/render`, {
      method: "POST",
      headers: { Authorization: `Bearer ${env.STRATA_SECRET}`, "Content-Type": "application/json" },
      body: JSON.stringify({ repo: `${job.owner}/${job.name}`, labels: job.labels }),
      signal: AbortSignal.timeout(25_000),
    });
    if (res.status >= 500) asked.delete(job.key); // let the next viewer retry
  } catch {
    asked.delete(job.key);
  }
}
