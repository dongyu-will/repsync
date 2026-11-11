import type { ProjectReview } from "./mygo";
import { gitPreview } from "./git-preview";

export function reviewPreview(path: string): ProjectReview {
  const git = gitPreview(path);
  const report: ProjectReview = {
    project: { name: path.split("/").at(-1) || "Project", path },
    checkedAt: "2026-11-10T05:30:00Z",
    configVersion: "preview",
    version: "preview",
    profileConfigured: true,
    profile: {
      kind: "web",
      areas: ["frontend", "backend", "database"],
      requirements: [
        "frontend-static",
        "frontend-behavior",
        "backend-api",
        "database-integration",
        "database-migrations",
        "database-consistency",
        "database-recovery",
      ],
      stages: ["push", "ci", "deploy", "acceptance"],
    },
    coverageOptions: [
      {
        id: "frontend-static",
        name: "Frontend type and static checks",
        area: "frontend",
        method: "static",
      },
      {
        id: "frontend-behavior",
        name: "Browser and interaction acceptance",
        area: "frontend",
        method: "behavior",
      },
      {
        id: "backend-static",
        name: "Backend static checks",
        area: "backend",
        method: "static",
      },
      {
        id: "backend-api",
        name: "Real HTTP / API Integration",
        area: "backend",
        method: "integration",
      },
      {
        id: "database-integration",
        name: "Real database integration",
        area: "database",
        method: "integration",
      },
      {
        id: "database-migrations",
        name: "Migration status and compatibility",
        area: "database",
        method: "migration",
      },
      {
        id: "database-consistency",
        name: "Data consistency and key business constraints",
        area: "database",
        method: "consistency",
      },
      {
        id: "database-recovery",
        name: "Backup and recovery drills",
        area: "database",
        method: "recovery",
      },
    ],
    coverage: [],
    acknowledged: [],
    taskWarnings: [],
    snapshot: {
      head: git.stashes.head,
      branch: git.stashes.branch,
      fingerprint: "current",
      fingerprintError: "",
      files: [
        {
          path: "src/App.tsx",
          index: " ",
          worktree: "M",
          untracked: false,
          conflict: false,
        },
        {
          path: "docs/notes.md",
          index: "?",
          worktree: "?",
          untracked: true,
          conflict: false,
        },
      ],
      staged: 0,
      unstaged: 1,
      untracked: 1,
      conflicts: 0,
      stashes: git.stashes.entries,
      upstream: "refs/remotes/origin/main",
      ahead: 2,
      behind: 0,
      upstreamKnown: true,
      fetchedAt: "2026-11-09T05:30:00Z",
      error: "",
    },
    issues: [
      {
        kind: "untracked",
        section: "changes",
        message: "1 untracked file not included in Git",
      },
      {
        kind: "ahead",
        section: "changes",
        message: "2  local commits need push verification",
      },
      {
        kind: "stash",
        section: "changes",
        message: `${git.stashes.entries.length} stashes need review`,
      },
      {
        kind: "checks",
        section: "checks",
        message: "File contents changed after checks",
      },
      {
        kind: "deploy",
        section: "delivery",
        message: "Deployment and live acceptance not verified",
      },
    ],
    checks: [
      {
        id: "preview-check",
        command: "npm run check",
        status: "success",
        scope: "Frontend TypeScript  and  ESLint",
        coverage: [{ area: "frontend", method: "static" }],
        environment: "local",
        target: "atlas-web",
        evidence: "Local check log: passed",
        source: "external",
        createdAt: "2026-11-10T04:30:00Z",
        commit: "previous",
        fingerprint: "previous",
        current: "stale",
        reason: "File contents changed after checks",
      },
    ],
    delivery: [
      {
        stage: "working",
        name: "Local changes",
        status: "observed",
        detail: "Uncommitted files remain",
        recordedAt: "",
      },
      {
        stage: "commit",
        name: "Commit",
        status: "observed",
        detail: "Current commit loaded",
        recordedAt: "",
      },
      {
        stage: "push",
        name: "Push",
        status: "pending",
        detail:
          "Ahead of local upstream ref by 2  commits; not verified online",
        recordedAt: "",
      },
      {
        stage: "ci",
        name: "CI / Check",
        status: "stale",
        detail: "File contents changed since the last check",
        recordedAt: "",
      },
      {
        stage: "deploy",
        name: "Deploy",
        status: "pending",
        detail: "Deployment results require separate verification",
        recordedAt: "",
      },
      {
        stage: "acceptance",
        name: "Live acceptance",
        status: "pending",
        detail: "Business behavior needs verification",
        recordedAt: "",
      },
    ],
    resources: [
      {
        state: {
          definition: {
            id: "database",
            name: "Local database and backups",
            kind: "manual",
            target: "",
            guide:
              "Verify the latest backup and recovery drill records against the project guide",
          },
          observed: "needs_confirmation",
          version: "preview",
          size: 0,
          modifiedAt: "",
          tracking: "not_applicable",
          error: "",
        },
        status: "pending",
        note: "",
        recordedAt: "",
      },
      {
        state: {
          definition: {
            id: "env",
            name: "Local environment configuration",
            kind: "file",
            target: ".env.local",
            guide:
              "Confirm an independent backup exists; configuration contents are not read or shown",
          },
          observed: "exists",
          version: "preview",
          size: 128,
          modifiedAt: "2026-11-10T02:30:00Z",
          tracking: "ignored",
          error: "",
        },
        status: "pending",
        note: "",
        recordedAt: "",
      },
    ],
    diagnosis: [],
    tasks: [
      {
        id: "preview-task",
        title: "Sidebar layout update",
        startedAt: "2026-11-10T03:00:00Z",
        endedAt: "2026-11-10T05:00:00Z",
        start: {
          head: "previous",
          branch: "feat/workspace-layout",
          fingerprint: "previous",
          staged: 0,
          unstaged: 0,
          untracked: [],
          stashes: [],
          known: true,
        },
        end: {
          head: git.stashes.head,
          branch: git.stashes.branch,
          fingerprint: "current",
          staged: 0,
          unstaged: 1,
          untracked: ["docs/notes.md"],
          stashes: [],
          known: true,
        },
        summary: "Align sidebar and title",
        checks:
          "Frontend checks passed, but files changed afterward. Recheck the affected scope.",
        blockers: "Review untracked notes and push status",
        newStashes: [],
        newUntracked: ["docs/notes.md"],
        handover:
          "# Sidebar layout update\n\n## Completed work\n\nAlign the sidebar and title.\n\n## Check\n\nFrontend checks passed, but files changed afterward. Recheck the affected scope.\n\n## Open issues\n\n- docs/notes.md not yet included in Git\n- Push, deployment, and live acceptance not verified\n",
      },
    ],
  };
  if (!git.git) {
    report.snapshot.error = "This folder is not a Git Worktrees";
    report.snapshot.files = [];
    report.snapshot.stashes = [];
    report.issues = [
      {
        kind: "git",
        section: "changes",
        message: "Git status unavailable; record data and handoff manually",
      },
    ];
    report.checks = [];
    report.profile = { kind: "notes", areas: [], requirements: [], stages: [] };
    report.resources = [];
    report.tasks = [];
    report.snapshot.head = "";
    report.snapshot.branch = "";
    report.snapshot.fingerprint = "";
    report.snapshot.upstreamKnown = false;
    report.snapshot.upstream = "";
    report.snapshot.ahead = report.snapshot.behind = 0;
    report.snapshot.staged =
      report.snapshot.unstaged =
      report.snapshot.untracked =
        0;
    for (const stage of report.delivery) {
      if (["working", "commit", "push", "ci"].includes(stage.stage)) {
        stage.status = "unknown";
        stage.detail = "Git status unavailable";
      }
    }
  }
  report.coverage = report.coverageOptions
    .filter((r) => report.profile.requirements.includes(r.id))
    .map((requirement) => ({
      requirement,
      status: requirement.id === "frontend-static" ? "stale" : "pending",
      reason:
        requirement.id === "frontend-static"
          ? "File contents changed after checks"
          : "No check records for this scope and method",
      checkId: "",
      source: "",
      evidence: "",
      environment: "",
      target: "",
      recordedAt: "",
    }));
  if (git.git) {
    report.issues = report.issues.filter((i) => i.section !== "checks");
    report.issues.push(
      ...report.coverage.map((item) => ({
        kind: `coverage_${item.requirement.id}`,
        section: "checks",
        message: `${item.requirement.name}Verification needed: ${item.reason}`,
      })),
    );
  }
  return report;
}
