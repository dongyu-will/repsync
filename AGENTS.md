# repsync 项目差异

公共默认规范继承本机 Codex 的 AGENTS.md。当前命令、启动与验收入口以 README.md 为准。

- 技术栈：MyGo WebView + Go；React + TypeScript + Vite；Tailwind CSS。使用 MyGo 生成的类型化绑定，不引入 Wails 或另一套样式框架。
- Go 工作加载 golang-how-to 技能；采用简单 internal 包和构造函数，不引入 DI 容器。
- 设计基准见 PRODUCT.md 中的 Figma 链接。按实际窗口尺寸验证，保留键盘操作与错误状态。
- 浏览器预览数据与用户登记的真实项目隔离；呈现约定见 PRODUCT.md 第 6 节。不得为预览修改其他仓库。
- 涉及项目接入、配置或共享资源时，先读 PRODUCT.md 第 4.8 节；AI 配置操作约定见 README.md「AI 配置项目」；配置由现有 AI 通过 CLI 或配置文件完成，界面展示状态与结果。
- 普通验证使用针对性前端检查、Go 检查及受影响行为验收；具体入口见 README.md。
