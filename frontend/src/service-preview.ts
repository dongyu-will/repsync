import type { LocalServicesOverview } from "./mygo";

// Browser-only examples; these PIDs and ports do not refer to actual processes.
export function servicePreview(path: string): LocalServicesOverview {
  const base = {
    pid: 0,
    runId: "",
    startedAt: "",
    endedAt: "",
    exitCode: -1,
    error: "",
    configChanged: false,
  };
  return {
    source: path === "/Projects/atlas" ? ".repsync.json" : "package.json",
    version: "example",
    error: "",
    services:
      path === "/Projects/atlas"
        ? [
            {
              ...base,
              definition: {
                autoPort: false,
                cwd: "",
                env: {},
                id: "web",
                name: "前端开发",
                command: ["npm", "run", "dev", "--", "--port", "5173"],
                port: 5173,
                url: "http://127.0.0.1:5173",
              },
              status: "running",
              portState: "listening",
              pid: 43210,
              runId: "example-web",
              startedAt: "2026-11-10T02:30:00Z",
            },
            {
              ...base,
              definition: {
                autoPort: false,
                cwd: "",
                env: {},
                id: "api",
                name: "API 服务",
                command: ["go", "run", "./cmd/server"],
                port: 8100,
                url: "http://127.0.0.1:8100",
              },
              status: "stopped",
              portState: "occupied",
            },
          ]
        : path === "/Projects/notebook"
          ? [
              {
                ...base,
                definition: {
                  autoPort: false,
                  cwd: "",
                  env: {},
                  id: "dev",
                  name: "dev",
                  command: ["npm", "run", "dev"],
                  port: 0,
                  url: "",
                },
                status: "stopped",
                portState: "none",
              },
            ]
          : [],
  };
}

export const servicePreviewLog =
  "$ npm run dev -- --port 5173\n\nVITE ready in 246 ms\nLocal: http://127.0.0.1:5173/\n\n10:32:18 [vite] page reload src/App.tsx\n";
