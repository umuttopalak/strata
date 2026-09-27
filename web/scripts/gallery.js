// Downloads the gallery's images from the hosted service into
// public/gallery, so the site serves them as static files.
// Usage: npm run gallery [-- https://your-worker.workers.dev]

import { mkdir, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { GALLERY, HERO, galleryImage } from "../public/gallery.js";

const origin = process.argv[2] || "https://strata.umuttopalak-de4.workers.dev";
const publicDir = join(dirname(fileURLToPath(import.meta.url)), "..", "public");

let failed = false;
for (const repo of [HERO, ...GALLERY]) {
  const res = await fetch(`${origin}/${repo}.svg`);
  const status = res.headers.get("X-Strata-Status");
  if (!res.ok || status !== "ok") {
    // Keep the old file rather than replace it with a placeholder.
    console.error(`${repo}: ${status || res.status}, kept the previous image`);
    failed = true;
    continue;
  }
  const file = join(publicDir, galleryImage(repo));
  await mkdir(dirname(file), { recursive: true });
  await writeFile(file, await res.text());
  console.log(`${repo}: ok`);
}
process.exitCode = failed ? 1 : 0;
