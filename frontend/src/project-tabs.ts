export const projectTabs = [
  "Overview",
  "Project map",
  "Worktrees",
  "CI/CD",
  "Rules",
  "AI resources",
  "Documents",
] as const;

export type ProjectTab = (typeof projectTabs)[number];
