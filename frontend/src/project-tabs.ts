export const primaryProjectTabs = [
  "Overview",
  "Rules",
  "CI/CD",
  "Worktrees",
  "Review",
] as const;

export const utilityProjectTabs = ["Project map", "AI resources"] as const;
export const projectTabs = [
  ...primaryProjectTabs,
  ...utilityProjectTabs,
] as const;
export type ProjectTab = (typeof projectTabs)[number];

export function isUtilityProjectTab(tab: ProjectTab): boolean {
  return tab === "Project map" || tab === "AI resources";
}

export function projectTabLabel(tab: ProjectTab): string {
  if (tab === "Overview") return "Project startup";
  if (tab === "Rules") return "Project standards";
  if (tab === "Worktrees") return "Branches and workspaces";
  if (tab === "Review") return "Todos and handoff";
  return tab;
}

// Existing saved pages continue at the workspace that now owns their functions.
export function restoreProjectTab(
  value: unknown,
  fallback: ProjectTab,
): ProjectTab {
  const previous: Record<string, ProjectTab> = {
    Documents: "Rules",
    "Service tools": "Overview",
    "\u6982\u89c8": "Overview",
    "\u5de5\u4f5c\u6811": "Worktrees",
    "\u89c4\u8303": "Rules",
    "AI \u8d44\u6e90": "AI resources",
    "\u6587\u6863": "Rules",
  };
  if (typeof value !== "string") return fallback;
  return projectTabs.includes(value as ProjectTab)
    ? (value as ProjectTab)
    : Object.hasOwn(previous, value)
      ? previous[value]
      : fallback;
}

export function restorePrimaryProjectTab(
  value: unknown,
  fallback: ProjectTab,
): ProjectTab {
  const tab = restoreProjectTab(value, fallback);
  return isUtilityProjectTab(tab) ? fallback : tab;
}
