import type { GitOverview } from "./mygo";

export function gitPreview(path: string): GitOverview {
  const git = path === "/Projects/atlas" || path === "/Projects/notebook";
  const commit = "e2a7c43f597fd9fa52eafef9a5b335df571d21ab";
  const current = path === "/Projects/atlas" ? "feat/workspace-layout" : "main";
  const view: GitOverview = {
    cleanupChecked: true,
    git,
    policy: { enabled: false, baseBranch: "main", retentionDays: 7 },
    policyVersion: "preview",
    version: "preview",
    history: [],
    warnings: [],
    stashError: "",
    stashes: {
      entries:
        path === "/Projects/atlas"
          ? [
              {
                oid: "a2c4e68da2c4e68da2c4e68da2c4e68da2c4e68d",
                selector: "stash@{0}",
                createdAt: "2026-11-10T02:30:00Z",
                subject:
                  "On feat/workspace-layout: before push — settings changes",
              },
              {
                oid: "b3d5f79ab3d5f79ab3d5f79ab3d5f79ab3d5f79a",
                selector: "stash@{1}",
                createdAt: "2026-11-06T02:30:00Z",
                subject: "WIP on main: e2a7c43 update sidebar",
              },
            ]
          : [],
      head: commit,
      branch: current,
      version: "preview",
    },
    worktrees: git
      ? [
          {
            path,
            branch: current,
            commit,
            primary: true,
            locked: false,
            missing: false,
            dirty: path === "/Projects/atlas",
            statusKnown: true,
            eligible: false,
            reasons: ["Main worktree", "Registered project"],
          },
          {
            path: `${path}-worktrees/settings`,
            branch: "feature/settings",
            commit: "94f63ab14912c8544188dd120bbcab760ec14fcb",
            primary: false,
            locked: false,
            missing: false,
            dirty: true,
            statusKnown: true,
            eligible: false,
            reasons: [
              "Modified, untracked, or ignored files",
              "Not merged into the base branch",
            ],
          },
          {
            path: `${path}-worktrees/sidebar`,
            branch: "fix/sidebar",
            commit,
            primary: false,
            locked: false,
            missing: false,
            dirty: false,
            statusKnown: true,
            eligible: true,
            reasons: [],
          },
        ]
      : [],
    branches: git
      ? [
          {
            name: "main",
            commit,
            upstream: "origin/main",
            updatedAt: "2026-11-10T02:30:00Z",
            merged: true,
            worktrees: current === "main" ? [path] : [],
            eligible: false,
            reasons: ["Protected branch", "Linked worktree must be kept"],
          },
          ...(current !== "main"
            ? [
                {
                  name: current,
                  commit,
                  upstream: "",
                  updatedAt: "2026-11-10T02:30:00Z",
                  merged: true,
                  worktrees: [path],
                  eligible: false,
                  reasons: ["Linked worktree must be kept"],
                },
              ]
            : []),
          {
            name: "feature/settings",
            commit: "94f63ab14912c8544188dd120bbcab760ec14fcb",
            upstream: "origin/feature/settings",
            updatedAt: "2026-11-09T02:30:00Z",
            merged: false,
            worktrees: [`${path}-worktrees/settings`],
            eligible: false,
            reasons: ["Not merged into the base branch"],
          },
          {
            name: "fix/sidebar",
            commit,
            upstream: "",
            updatedAt: "2026-11-01T02:30:00Z",
            merged: true,
            worktrees: [`${path}-worktrees/sidebar`],
            eligible: true,
            reasons: [],
          },
          {
            name: "fix/empty-state",
            commit,
            upstream: "",
            updatedAt: "2026-11-01T02:30:00Z",
            merged: true,
            worktrees: [],
            eligible: true,
            reasons: [],
          },
        ]
      : [],
  };
  if (view.stashes.entries.length) {
    const reason = `${view.stashes.entries.length} pending stashes; cleanup paused`;
    view.warnings.push(reason);
    for (const item of [...view.worktrees, ...view.branches]) {
      item.eligible = false;
      item.reasons.push(reason);
    }
  }
  return view;
}
