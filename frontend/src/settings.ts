export interface Theme {
  id: string;
  label: string;
  swatch: [bg: string, accent: string];
}

export const THEMES: Theme[] = [
  { id: "paper", label: "Paper", swatch: ["#fafaf8", "#2f6f5e"] },
  { id: "midnight", label: "Midnight", swatch: ["#151514", "#5fb59f"] },
  { id: "blossom", label: "Blossom", swatch: ["#fff0f5", "#d6336c"] },
  { id: "ocean", label: "Ocean", swatch: ["#eef6fc", "#1f6fb2"] },
  { id: "forest", label: "Forest", swatch: ["#12201a", "#7bc47f"] },
];

const STORE = "fh.theme";

export function savedTheme(): string | null {
  try {
    const t = localStorage.getItem(STORE);
    return THEMES.some((x) => x.id === t) ? t : null;
  } catch {
    return null;
  }
}

/** Without a saved choice the theme follows the system's light/dark setting. */
export function currentTheme(): string {
  return savedTheme() ?? (matchMedia("(prefers-color-scheme: dark)").matches ? "midnight" : "paper");
}

export function applyTheme(id: string | null = savedTheme()): void {
  if (id) document.documentElement.dataset.theme = id;
}

export function setTheme(id: string): void {
  try {
    localStorage.setItem(STORE, id);
  } catch {
    /* not persisted */
  }
  applyTheme(id);
}
