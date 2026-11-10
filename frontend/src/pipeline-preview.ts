import type { ActionsOverview, PipelineConfig, PipelineRun } from "./mygo";

// Isolated browser fixtures. No process execution or repository access.
export function pipelinePreview(path: string): {
  config: PipelineConfig;
  runs: PipelineRun[];
  log: string;
  actions: ActionsOverview;
} {
  const atlas = path === "/Projects/atlas";
  return {
    config: {
      source: ".repsync.json",
      version: "example",
      pipelines: atlas
        ? [
            {
              id: "ci",
              name: "项目检查",
              kind: "ci",
              steps: [["npm", "run", "check:ci"]],
              timeoutSeconds: 1800,
              cwd: "",
              env: {},
            },
            {
              id: "deploy",
              name: "发布预览环境",
              kind: "cd",
              steps: [["npm", "run", "deploy:preview"]],
              timeoutSeconds: 1800,
              cwd: "",
              env: {},
            },
          ]
        : [],
    },
    runs: atlas
      ? [
          {
            id: "example-ci",
            pipelineId: "ci",
            name: "项目检查",
            kind: "ci",
            status: "failure",
            startedAt: "2026-11-10T02:30:00Z",
            endedAt: "2026-11-10T02:30:24Z",
            commit: "c31b8e27d41a",
            worktree: "dirty",
            step: 1,
            exitCode: 1,
            error: "检查未通过",
          },
        ]
      : [],
    log: "[1/1] npm run check:ci\n\nsrc/App.tsx:18:7\nerror: 'unused' is declared but never used.\n\nProcess exited with code 1.\n",
    actions: {
      repository: "team/atlas",
      runs: atlas
        ? [
            {
              databaseId: 42,
              workflowName: "CI",
              displayTitle: "调整项目工作区布局",
              status: "completed",
              conclusion: "success",
              headBranch: "main",
              headSha: "a612fe9",
              createdAt: "2026-11-10T01:00:00Z",
              url: "",
            },
            {
              databaseId: 41,
              workflowName: "Preview",
              displayTitle: "更新预览环境",
              status: "completed",
              conclusion: "failure",
              headBranch: "feat/workspace-layout",
              headSha: "c31b8e2",
              createdAt: "2026-11-09T12:20:00Z",
              url: "",
            },
          ]
        : [],
    },
  };
}
