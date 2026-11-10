import type { ProjectReview } from "./mygo";
import { gitPreview } from "./git-preview";

export function reviewPreview(path: string): ProjectReview {
  const git = gitPreview(path);
  const report: ProjectReview = {
    project: { name: path.split("/").at(-1) || "项目", path },
    checkedAt: "2026-11-10T05:30:00Z",
    configVersion: "preview",
    version: "preview",
    profile: { kind: "", areas: [], requirements: [], stages: [] },
    profileConfigured: false,
    coverageOptions: [],
    coverage: [],
    acknowledged: [],
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
        message: "1 个未跟踪文件尚未纳入 Git",
      },
      {
        kind: "ahead",
        section: "changes",
        message: "2 个本地提交需要核对是否已推送",
      },
      {
        kind: "stash",
        section: "changes",
        message: `${git.stashes.entries.length} 条 stash 待核对`,
      },
      {
        kind: "checks",
        section: "checks",
        message: "检查之后文件内容发生变化",
      },
      {
        kind: "deploy",
        section: "delivery",
        message: "部署及线上验收尚未核验",
      },
    ],
    checks: [
      {
        id: "preview-check",
        command: "npm run check",
        status: "success",
        scope: "前端 TypeScript 与 ESLint",
        evidence: "本地检查日志，已通过",
        source: "external",
        createdAt: "2026-11-10T04:30:00Z",
        commit: "previous",
        fingerprint: "previous",
        current: "stale",
        reason: "检查之后文件内容发生变化",
      },
    ],
    delivery: [
      {
        stage: "working",
        name: "本地改动",
        status: "observed",
        detail: "仍有未提交文件",
        recordedAt: "",
      },
      {
        stage: "commit",
        name: "提交",
        status: "observed",
        detail: "已读取当前提交",
        recordedAt: "",
      },
      {
        stage: "push",
        name: "推送",
        status: "pending",
        detail: "相对本地上游引用领先 2 个提交，未联网核验",
        recordedAt: "",
      },
      {
        stage: "ci",
        name: "CI / 检查",
        status: "stale",
        detail: "最近检查之后文件内容变化",
        recordedAt: "",
      },
      {
        stage: "deploy",
        name: "部署",
        status: "pending",
        detail: "需要独立核验部署结果",
        recordedAt: "",
      },
      {
        stage: "acceptance",
        name: "线上验收",
        status: "pending",
        detail: "需要核验业务行为",
        recordedAt: "",
      },
    ],
    resources: [
      {
        state: {
          definition: {
            id: "database",
            name: "本地数据库与备份",
            kind: "manual",
            target: "",
            guide: "按项目指南确认最近备份及恢复演练记录",
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
            name: "本地环境配置",
            kind: "file",
            target: ".env.local",
            guide: "确认有独立保留方式；不读取或展示配置内容",
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
        title: "侧栏布局调整",
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
        summary: "调整侧栏与标题对齐",
        checks: "前端专项检查通过，后续又修改了文件，需要按范围核对",
        blockers: "核对未跟踪笔记与推送状态",
        newStashes: [],
        newUntracked: ["docs/notes.md"],
        handover:
          "# 侧栏布局调整\n\n## 完成内容\n\n调整侧栏与标题对齐。\n\n## 检查\n\n前端专项检查通过，后续又修改了文件，需要按范围核对。\n\n## 遗留问题\n\n- docs/notes.md 尚未纳入 Git\n- 推送、部署与线上验收尚未核验\n",
      },
    ],
  };
  if (!git.git) {
    report.snapshot.error = "此目录不是 Git 工作树";
    report.snapshot.files = [];
    report.snapshot.stashes = [];
    report.issues = [
      {
        kind: "git",
        section: "changes",
        message: "Git 状态不可用，数据与交接可手动记录",
      },
    ];
    report.checks = [];
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
        stage.detail = "Git 状态不可用";
      }
    }
  }
  return report;
}
