---
version: 1
slug: "frontend-src-airesources-tsx"
primary_target: "frontend/src/AIResources.tsx"
related_targets: []
---

# AI 资源

范围：在已有项目工作台增加全局和项目资源视图，Operate 模式。用户已确认项目适配、可用性及使用证据的具体内容，继续沿用既有连续列表和按需展开交互。

## Direction contract

THESIS: 每项资源都能解释采用理由，并从统计展开到实际证据；安装与任务效果分开显示。

OWN-WORLD: 继承 DESIGN.md 的中性浅色、蓝色选择、紧凑字体、细分隔线、原生控件及 8px 连续列表，不新增视觉体系。

STORY: 选择项目，筛选推荐或全部资源，读取适配理由和缺失前提；按需读取使用记录，再展开会话、结果及来源。

FIRST VIEWPORT: 侧栏与项目标题不变；内容上方为资源/使用记录分段，随后搜索和类型筛选，主体为可展开的资源列表。读取记录按钮与覆盖范围同时出现。默认 960×640，最小 760×500，放大 1440×900，长来源可换行。

FORM: 已确认的局部扩展，以连续列表和 details 复用现有组成；无需新视觉方向或 comp。seed: none (established surface extension)。主要交互是从统计展开到具体执行证据；不增加装饰动画。

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance
