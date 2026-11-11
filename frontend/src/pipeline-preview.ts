import type { ActionsOverview, PipelineConfig, PipelineRun } from "./mygo";

// Isolated browser fixtures. No process execution or repository access.
export function pipelinePreview(path: string): {
  config: PipelineConfig;
  runs: PipelineRun[];
  logs: Record<string, string>;
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
              name: "Project checks",
              kind: "ci",
              steps: [
                ["npm", "run", "lint"],
                ["npm", "run", "typecheck"],
                ["npm", "run", "test:integration"],
                ["npm", "run", "build"],
              ],
              timeoutSeconds: 1800,
              cwd: "",
              env: {},
            },
            {
              id: "deploy",
              name: "Release preview environment",
              kind: "cd",
              steps: [
                ["npm", "run", "build"],
                ["npm", "run", "deploy:preview"],
                ["npm", "run", "verify:preview"],
              ],
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
            name: "Project checks",
            kind: "ci",
            status: "failure",
            startedAt: "2026-11-10T02:30:00Z",
            endedAt: "2026-11-10T02:30:24Z",
            commit: "c31b8e27d41a",
            worktree: "dirty",
            step: 3,
            configVersion: "example",
            exitCode: 1,
            error: "Checks failed",
          },
          {
            id: "example-deploy",
            pipelineId: "deploy",
            name: "Release preview environment",
            kind: "cd",
            status: "running",
            startedAt: "2026-11-11T02:30:00Z",
            endedAt: "",
            commit: "c31b8e27d41a",
            worktree: "dirty",
            step: 2,
            exitCode: -1,
            error: "",
            configVersion: "example",
          },
        ]
      : [],
    logs: {
      "example-ci":
        "[1/4] npm run lint\nPassed\n\n[2/4] npm run typecheck\nPassed\n\n[3/4] npm run test:integration\nAPI integration check failed: expected 200, received 503\n\nProcess exited with code 1.\n",
      "example-deploy":
        "[1/3] npm run build\nBuild complete\n\n[2/3] npm run deploy:preview\nUploading assets…\n",
    },
    actions: {
      repository: "team/atlas",
      runs: atlas
        ? [
            {
              databaseId: 42,
              workflowName: "CI",
              displayTitle: "Update project workspace layout",
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
              displayTitle: "Update preview environment",
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
