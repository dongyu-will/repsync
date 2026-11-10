import type { Inspection } from "./mygo";

// Browser-only document content; never read from local repositories.
export const previewDocuments: Record<string, Record<string, string>> = {
  "/示例项目/atlas": {
    "AGENTS.md": `# Atlas 项目规范（示例）

## 开发
- 使用 React、TypeScript 和 Tailwind CSS。
- 优先复用已有组件，保持修改范围清晰。
- 桌面布局兼顾长路径、空状态与键盘操作。

## 验证
- 前端修改运行 npm run check。
- 发布前运行 npm run build。
- 提交前确认仅包含本次任务的文件。
`,
    "DESIGN.md": `# Atlas 界面设计（示例）

- 使用紧凑侧栏与项目页签。
- 保留键盘焦点、空状态和读取错误提示。
`,
    "PRODUCT.md": `# Atlas 产品需求（示例）

## 目标
将多个本地项目的状态、命令和项目文档集中到一个工作台。

## 当前范围
- 选择项目并查看项目上下文。
- 查看项目根目录的 Markdown 文档。
`,
    "README.md": `# Atlas（示例）

## 本地开发
npm run dev

## 检查
npm run check
`,
  },
  "/示例项目/工作资料/产品设计与工程协作/桌面工作台/设计资料与项目交接文档": {
    "AGENTS.md": `# 资料目录规范（示例）

- 保留设计稿、会议记录和交接文档的原始文件。
- 文件命名使用日期与主题，便于查找。
- 此目录不使用 Git，也没有 package.json 命令。
`,
    "HANDOFF.md": "",
  },
};

// Browser-only examples. These paths are labels, never registered or inspected.
export const previewInspections: Inspection[] = [
  {
    project: { name: "Atlas", path: "/示例项目/atlas" },
    git: true,
    branch: "feat/workspace-layout",
    statusKnown: true,
    status: " M src/App.tsx\n M src/style.css\n?? docs/design-notes.md",
    documents: Object.keys(previewDocuments["/示例项目/atlas"]),
    scripts: {
      dev: "vite --host 127.0.0.1",
      check: "tsc -b && eslint .",
      build: "tsc -b && vite build",
      "format:check": "prettier --check src",
    },
    warnings: [],
  },
  {
    project: { name: "Notebook", path: "/示例项目/notebook" },
    git: true,
    branch: "main",
    statusKnown: true,
    status: "",
    documents: [],
    scripts: { dev: "vite", build: "vite build" },
    warnings: [],
  },
  {
    project: {
      name: "设计资料与项目交接文档",
      path: "/示例项目/工作资料/产品设计与工程协作/桌面工作台/设计资料与项目交接文档",
    },
    git: false,
    branch: "",
    statusKnown: false,
    status: "",
    documents: Object.keys(
      previewDocuments[
        "/示例项目/工作资料/产品设计与工程协作/桌面工作台/设计资料与项目交接文档"
      ],
    ),
    scripts: {},
    warnings: [],
  },
];
