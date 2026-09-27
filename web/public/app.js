import { parseRepoInput } from "./repo.js";

// Repositories in the gallery: well known, varied, and within the hosted
// service's limits (300 MB, 15,000 mainline commits).
const GALLERY = [
  "BurntSushi/ripgrep",
  "charmbracelet/bubbletea",
  "pallets/flask",
  "expressjs/express",
  "jqlang/jq",
  "ohmyzsh/ohmyzsh",
  "vitejs/vite",
  "tailwindlabs/tailwindcss",
  "charmbracelet/lipgloss",
];

const $ = (id) => document.getElementById(id);
const imageURL = (repo, labels) => `${location.origin}/${repo}.svg${labels ? "?labels" : ""}`;

function renderGallery() {
  const grid = $("gallery-grid");
  for (const repo of GALLERY) {
    const a = document.createElement("a");
    a.className = "card";
    a.href = `https://github.com/${repo}`;
    const img = document.createElement("img");
    img.src = imageURL(repo, false);
    img.alt = `The history of ${repo} as a mountain range`;
    img.loading = "lazy";
    img.width = 832;
    img.height = 540;
    const name = document.createElement("span");
    name.className = "name";
    name.textContent = repo;
    a.append(img, name);
    grid.append(a);
  }
}

// Try it: show the image, and while the service is still drawing it, ask
// again every few seconds until the real one is there.
let polling = null;

async function draw(repo, labels) {
  clearTimeout(polling);
  const url = imageURL(repo, labels);
  const markdown = `![strata](${url})`;
  $("snippet").textContent = markdown;
  $("result").hidden = false;
  $("preview").alt = `The history of ${repo} as a mountain range`;
  history.replaceState(null, "", `#${repo}${labels ? "?labels" : ""}`);

  const started = Date.now();
  const check = async () => {
    let status = "error";
    try {
      const res = await fetch(url, { cache: "no-store" });
      status = res.headers.get("X-Strata-Status") || (res.ok ? "ok" : "error");
    } catch {
      status = "offline";
    }
    // A cache-busting query shows the newest image, not the placeholder
    // the browser already has.
    $("preview").src = `${url}${labels ? "&" : "?"}t=${Date.now()}`;

    const seconds = Math.round((Date.now() - started) / 1000);
    const el = $("status");
    el.classList.toggle("ok", status === "ok");
    if (status === "rendering" && seconds < 300) {
      el.textContent = `drawing ${repo}… ${seconds}s (the first time can take a couple of minutes)`;
      polling = setTimeout(check, 8000);
    } else if (status === "rendering") {
      el.textContent = "still drawing; come back in a few minutes, the link already works";
    } else if (status === "ok") {
      el.textContent = `${repo} is ready. Copy the line below into your README.`;
    } else if (status === "offline") {
      el.textContent = "could not reach strata; check your connection";
    } else {
      el.textContent = "strata cannot draw this repository here; the image says why";
    }
  };
  await check();
}

function onSubmit(event) {
  event.preventDefault();
  const repo = parseRepoInput($("repo").value);
  const error = $("try-error");
  if (!repo) {
    error.textContent = "Enter a GitHub repository as owner/repo or paste its URL.";
    error.hidden = false;
    return;
  }
  error.hidden = true;
  draw(repo, $("labels").checked);
}

async function copySnippet() {
  const text = $("snippet").textContent;
  try {
    await navigator.clipboard.writeText(text);
  } catch {
    // Older browsers or insecure contexts: select it for a manual copy.
    const range = document.createRange();
    range.selectNodeContents($("snippet"));
    getSelection().removeAllRanges();
    getSelection().addRange(range);
    return;
  }
  $("copy").textContent = "Copied";
  setTimeout(() => ($("copy").textContent = "Copy"), 1500);
}

// A shared link like /#owner/repo?labels opens straight to that repository.
function fromHash() {
  const [path, query] = decodeURIComponent(location.hash.slice(1)).split("?");
  const repo = parseRepoInput(path || "");
  if (!repo) return;
  $("repo").value = repo;
  $("labels").checked = query === "labels";
  draw(repo, $("labels").checked);
  $("try").scrollIntoView();
}

renderGallery();
$("try-form").addEventListener("submit", onSubmit);
$("copy").addEventListener("click", copySnippet);
fromHash();
