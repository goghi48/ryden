export type Theme = "light" | "dark";

export function getInitialTheme(): Theme {
  const savedTheme = localStorage.getItem("ryden-theme");
  if (savedTheme === "light" || savedTheme === "dark") {
    return savedTheme;
  }
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}
