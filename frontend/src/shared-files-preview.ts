import type { Project, ResourceOverview, UpdatePreview } from "./mygo";

export function sharedFilesPreview(projects: Project[]): ResourceOverview {
  const resources = [
    {
      id: "http-health",
      path: "/Shared/engineering/http-health.sh",
      target: "tools/http-health.sh",
      version:
        "1900865792a6dd90a34b83e3bbe90b441d9db6bb29b78e4eaa5208eb0061a42a",
      error: "",
    },
    {
      id: "browser-checks",
      path: "/Shared/engineering/browser-checks.mjs",
      target: "tools/browser-checks.mjs",
      version:
        "bde5614838acd6d7c4d09c430fe6bac92f2f428ec16693406f7c15ed897b4511",
      error: "",
    },
    {
      id: "eslint-base",
      path: "/Shared/engineering/eslint-base.mjs",
      target: "tools/eslint-base.mjs",
      version:
        "63001318190931ce239803a406e53ebaf33ad2fb1cb28db34485fc72b537b7bc",
      error: "",
    },
  ];
  return {
    version: "preview",
    resources,
    projects: projects.map((project) => ({
      project,
      configVersion: "preview",
      error: "",
      adoptions:
        project.path === "/Projects/atlas"
          ? [
              {
                ...resources[0],
                status: "outdated",
                adoptedVersion:
                  "c40f84371799bf45ced0d5d0095cbdd0074cdcbbea27aa72444c1fabfb7cd30b",
              },
              {
                ...resources[1],
                status: "modified",
                adoptedVersion:
                  "f4e0a9a353ead606078758c900719baff61838a4d7aaf705773c03761da48c04",
              },
            ]
          : project.path === "/Projects/notebook"
            ? [
                {
                  ...resources[0],
                  status: "current",
                  adoptedVersion: resources[0].version,
                },
              ]
            : [],
    })),
  };
}

export function sharedFilePreview(
  report: ResourceOverview,
  project: Project,
  id: string,
): UpdatePreview {
  const source = report.resources.find((item) => item.id === id)!;
  const adoption = report.projects
    .find((item) => item.project.path === project.path)
    ?.adoptions.find((item) => item.id === id);
  const diff =
    id === "http-health"
      ? ' #!/bin/sh\n-curl --fail "$1"\n+curl --fail --retry 2 --connect-timeout 5 "$1"'
      : "-export const timeout = 5000;\n+export const timeout = 10000;";
  return {
    token: "preview",
    project,
    target: source.target,
    sourcePath: source.path,
    version: source.version,
    changed: adoption?.status !== "current",
    conflict: adoption?.status === "modified",
    projectContent: "",
    diff: `--- a/${source.target}\n+++ b/${source.target}\n${id === "http-health" ? "@@ -1,2 +1,2 @@" : "@@ -1 +1 @@"}\n${diff}`,
  };
}
