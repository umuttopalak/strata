// Turns what people paste into "owner/repo": the short form, or any GitHub
// URL (https, www, .git, a path to a file or branch, git@github.com:…).
// Returns null for anything that is not a GitHub repository.

const OWNER = /^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$/;
const NAME = /^[A-Za-z0-9._-]{1,100}$/;

export function parseRepoInput(input) {
  let s = String(input).trim();
  s = s.replace(/^git@github\.com:/i, "");
  s = s.replace(/^(?:https?:\/\/)?(?:www\.)?github\.com\//i, "");
  const [owner, rawName] = s.split(/[/?#]/);
  const name = (rawName || "").replace(/\.git$/i, "");
  if (!OWNER.test(owner || "") || !NAME.test(name) || name === "." || name === "..") return null;
  return `${owner}/${name}`;
}
