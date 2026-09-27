// Repositories in the gallery: well known, varied, and within the hosted
// service's limits (300 MB, 15,000 mainline commits). Their images are
// static files in /gallery, so a busy day on the site costs no Worker
// requests; refresh them with `npm run gallery`.
export const HERO = "junegunn/fzf";

export const GALLERY = [
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

export const galleryImage = (repo) => `/gallery/${repo}.svg`;
