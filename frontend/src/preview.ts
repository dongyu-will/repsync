import type { Inspection } from "./mygo";

// Browser-only document content; never read from local repositories.
export const previewDocuments: Record<string, Record<string, string>> = {
  "/Projects/notebook": {
    "AGENTS.md": `# Notebook 项目规范

## 实现
- 优先复用已有组件和项目命令，保持改动范围清晰。
- 前端、后端和数据库分别验证，不以页面加载成功代替接口与数据检查。

## 工作区与交付
- 提交前核对修改范围，保留其他任务的改动。
- 不提交凭据、数据库文件和本地运行日志。
- 修改入口或行为时同步相关指南。
`,
    "CONTRIBUTING.md": `# Notebook 开发约定

- 启动前查阅 README 中的前端、后端和数据库入口。
- 日常改动运行受影响范围的检查。
- 数据库迁移、备份和恢复按项目已有流程处理。
`,
  },
  "/Projects/atlas": {
    "AGENTS.md": `# Atlas Project rules

## Development
- Use React, TypeScript  and  Tailwind CSS.
- Reuse existing components and keep changes focused.
- Desktop layouts handle long paths, empty states, and keyboard navigation.

## Verify
- For frontend changes, run npm run check.
- Before releasing, run npm run build.
- Include only task-related files in each commit.
`,
    "DESIGN.md": `# Atlas Interface design

- Use a compact sidebar and project tabs.
- Preserve keyboard focus, empty states, and read errors.
`,
    "PRODUCT.md": `# Atlas Product requirements

## Goal
Bring local project status, commands, and documents into one workspace.

## Current scope
- Select a project to view its context.
- Browse the project root's Markdown  documents.
`,
    "README.md": `# Atlas

## Local development
npm run dev

## Check
npm run check
`,
  },
  "/Projects/Work documents/Product design and engineering collaboration/Desktop workspace/Design assets and project handoff documents":
    {
      "AGENTS.md": `# Document folder conventions

- Preserve original designs, meeting notes, and handoff documents.
- Name files by date and topic for easy discovery.
- This folder does not use Git,  and has no package.json  commands.
`,
      "HANDOFF.md": "",
    },
};

// Browser-only examples. These paths are labels, never registered or inspected.
export const previewInspections: Inspection[] = [
  {
    project: { name: "Atlas", path: "/Projects/atlas" },
    git: true,
    branch: "feat/workspace-layout",
    statusKnown: true,
    stashKnown: true,
    stashCount: 2,
    status: " M src/App.tsx\n M src/style.css\n?? docs/design-notes.md",
    documents: Object.keys(previewDocuments["/Projects/atlas"]),
    scripts: {
      dev: "vite --host 127.0.0.1",
      check: "tsc -b && eslint .",
      build: "tsc -b && vite build",
      "format:check": "prettier --check src",
    },
    warnings: [],
  },
  {
    project: { name: "Notebook", path: "/Projects/notebook" },
    git: true,
    branch: "main",
    statusKnown: true,
    stashKnown: true,
    stashCount: 0,
    status: "",
    documents: Object.keys(previewDocuments["/Projects/notebook"]),
    scripts: { dev: "vite", build: "vite build" },
    warnings: [],
  },
  {
    project: {
      name: "Design assets and project handoff documents",
      path: "/Projects/Work documents/Product design and engineering collaboration/Desktop workspace/Design assets and project handoff documents",
    },
    git: false,
    branch: "",
    statusKnown: false,
    stashKnown: false,
    stashCount: 0,
    status: "",
    documents: Object.keys(
      previewDocuments[
        "/Projects/Work documents/Product design and engineering collaboration/Desktop workspace/Design assets and project handoff documents"
      ],
    ),
    scripts: {},
    warnings: [],
  },
];
