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
    source: ".repsync.json",
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
                name: "Frontend development",
                command: ["npm", "run", "dev", "--", "--port", "5173"],
                port: 5173,
                url: "http://127.0.0.1:5173",
              },
              status: "running",
              portState: "listening",
              pid: 43210,
              runId: "example-web",
              startedAt: "2026-11-10T02:30:00Z",
              usage: {
                cpuPercent: 3.6,
                memoryBytes: 194615705,
                processCount: 3,
                scope: "process-group",
                sampledAt: "2026-11-12T02:00:00Z",
              },
            },
            {
              ...base,
              definition: {
                autoPort: false,
                cwd: "",
                env: {},
                id: "api",
                name: "API Service",
                command: ["go", "run", "./cmd/server"],
                port: 8100,
                url: "http://127.0.0.1:8100",
              },
              status: "stopped",
              portState: "occupied",
            },
            {
              ...base,
              definition: {
                id: "db",
                name: "PostgreSQL",
                command: [],
                port: 5432,
                autoPort: false,
                observeOnly: true,
                cwd: "",
                env: {},
                url: "",
              },
              status: "reachable",
              portState: "listening",
              usage: {
                cpuPercent: 0.4,
                memoryBytes: 73400320,
                processCount: 1,
                scope: "listener",
                sampledAt: "2026-11-12T02:00:00Z",
              },
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
                  id: "web",
                  name: "Frontend",
                  command: ["npm", "run", "dev", "--", "--port", "3000"],
                  port: 3000,
                  url: "http://127.0.0.1:3000",
                },
                status: "stopped",
                portState: "available",
              },
              {
                ...base,
                definition: {
                  id: "api",
                  name: "Backend API",
                  command: ["npm", "run", "dev:api"],
                  port: 8080,
                  autoPort: false,
                  cwd: "",
                  env: {},
                  url: "http://127.0.0.1:8080",
                },
                status: "stopped",
                portState: "available",
              },
              {
                ...base,
                definition: {
                  id: "db",
                  name: "PostgreSQL",
                  command: [],
                  port: 5432,
                  autoPort: false,
                  observeOnly: true,
                  cwd: "",
                  env: {},
                  url: "",
                },
                status: "reachable",
                portState: "listening",
                usage: {
                  cpuPercent: 0.2,
                  memoryBytes: 62914560,
                  processCount: 1,
                  scope: "listener",
                  sampledAt: "2026-11-12T02:00:00Z",
                },
              },
            ]
          : [],
  };
}

export const servicePreviewLog =
  "$ npm run dev -- --port 5173\n\nVITE ready in 246 ms\nLocal: http://127.0.0.1:5173/\n\n10:32:18 [vite] page reload src/App.tsx\n";
