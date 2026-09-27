import assert from "node:assert/strict";
import { test } from "node:test";
import { parseRepoInput } from "../public/repo.js";

test("parseRepoInput accepts what people paste", () => {
  const cases = {
    "junegunn/fzf": "junegunn/fzf",
    "  junegunn/fzf  ": "junegunn/fzf",
    "github.com/junegunn/fzf": "junegunn/fzf",
    "https://github.com/junegunn/fzf": "junegunn/fzf",
    "https://www.github.com/junegunn/fzf/": "junegunn/fzf",
    "https://github.com/junegunn/fzf.git": "junegunn/fzf",
    "https://github.com/junegunn/fzf/tree/master/src": "junegunn/fzf",
    "https://github.com/junegunn/fzf?tab=readme": "junegunn/fzf",
    "git@github.com:junegunn/fzf.git": "junegunn/fzf",
    "Some-Org/my.repo_name": "Some-Org/my.repo_name",
  };
  for (const [input, want] of Object.entries(cases)) {
    assert.equal(parseRepoInput(input), want, input);
  }
});

test("parseRepoInput rejects everything else", () => {
  for (const input of ["", "fzf", "/fzf", "junegunn/", "https://gitlab.com/a/b",
    "-bad/repo", "a/..", "a/b c", "<script>/x"]) {
    assert.equal(parseRepoInput(input), null, input);
  }
});
