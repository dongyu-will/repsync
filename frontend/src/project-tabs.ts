export const projectTabs = [
  "Overview",
  "Worktrees",
  "CI/CD",
  "Rules",
  "AI resources",
  "Documents",
] as const;

export type ProjectTab = (typeof projectTabs)[number];
