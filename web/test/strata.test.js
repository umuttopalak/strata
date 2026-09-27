import assert from "node:assert/strict";
import { test } from "node:test";
import {
  checkRepo,
  escapeXML,
  isStale,
  messageSVG,
  parseRequest,
  STALE_SECONDS,
  svgHeaders,
} from "../src/strata.js";

const req = (path) => parseRequest(new URL(`https://strata.example${path}`));

test("parseRequest matches strata-server keys", () => {
  assert.deepEqual(req("/Charm/LipGloss.svg"), {
    owner: "Charm", name: "LipGloss", labels: false, key: "charm/lipgloss",
  });
  assert.equal(req("/charm/lipgloss.svg?labels").key, "charm/lipgloss?labels");
  assert.equal(req("/a/b.c-d_e.svg").name, "b.c-d_e");
});

test("parseRequest leaves other paths to the site", () => {
  for (const p of ["/", "/index.html", "/charm/lipgloss", "/charm/lipgloss.png",
    "/a/b/c.svg", "/-bad/repo.svg", "/owner/.svg", "/owner/a%2Fb.svg"]) {
    assert.equal(req(p), null, p);
  }
  // URL parsing resolves dot segments first: what is left is a plain
  // owner/name pair, looked up on GitHub like any other.
  assert.equal(req("/../etc/passwd.svg").key, "etc/passwd");
});

test("isStale", () => {
  const now = 1_000_000;
  assert.equal(isStale(null, now), true);
  assert.equal(isStale({ renderedAt: now - 60 }, now), false);
  assert.equal(isStale({ renderedAt: now - STALE_SECONDS - 1 }, now), true);
});

test("svgHeaders", () => {
  assert.equal(svgHeaders(3600)["Cache-Control"], "public, max-age=3600");
  assert.match(svgHeaders(0)["Cache-Control"], /no-cache/);
  assert.match(svgHeaders(0)["Content-Security-Policy"], /default-src 'none'/);
});

test("messageSVG escapes its text", () => {
  const svg = messageSVG("o/<r>", ["a & b", `"quoted"`]);
  assert.match(svg, /^<svg /);
  assert.ok(svg.includes("o/&lt;r&gt;"));
  assert.ok(svg.includes("a &amp; b"));
  assert.ok(!svg.includes("<r>"));
  assert.equal(escapeXML(`<'&">`), "&lt;&#39;&amp;&#34;&gt;");
});

const fakeFetch = (status, body) => async (url, init) => {
  fakeFetch.last = { url, init };
  return new Response(JSON.stringify(body), { status });
};

test("checkRepo", async () => {
  const opts = { owner: "o", name: "r", token: "t", maxRepoMB: 300 };

  let r = await checkRepo(fakeFetch(200, { size: 1024 }), opts);
  assert.deepEqual(r, { ok: true });
  assert.equal(fakeFetch.last.url, "https://api.github.com/repos/o/r");
  assert.equal(fakeFetch.last.init.headers.Authorization, "Bearer t");

  r = await checkRepo(fakeFetch(404, {}), opts);
  assert.equal(r.ok, false);
  assert.match(r.lines[0], /not found/);

  r = await checkRepo(fakeFetch(200, { private: true }), opts);
  assert.match(r.lines[0], /not found/);

  r = await checkRepo(fakeFetch(200, { size: 400 * 1024 }), opts);
  assert.match(r.lines[0], /too large to draw here \(400 MB, limit 300 MB\)/);

  // GitHub trouble does not block: the server checks again.
  r = await checkRepo(fakeFetch(403, {}), opts);
  assert.deepEqual(r, { ok: true, unknown: true });
  r = await checkRepo(async () => { throw new Error("offline"); }, opts);
  assert.deepEqual(r, { ok: true, unknown: true });
});
