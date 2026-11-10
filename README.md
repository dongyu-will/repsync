# repsync

本地优先的多项目桌面工作台。以项目作为日常入口，以公共规范同步作为核心特色，让 Git、命令运行与规则更新围绕同一个项目衔接起来。

## 文档入口

- [PRODUCT.md](PRODUCT.md)：当前需求、已确认决策、设计基准、验收行为和待决事项的权威记录。
- [AGENTS.md](AGENTS.md)：本项目的 Agent 工作规则。
- [HANDOFF.md](HANDOFF.md)：早期研究和历史交接；当前需求以 PRODUCT.md 为准。
- [Figma：规范更新页面](https://www.figma.com/design/dVq8rmWWIe4lTebO5gEW11?node-id=2-143)：已认可的可编辑设计稿。

## 当前状态

首轮实现采用 **MyGo WebView + Go、React + TypeScript + Vite、Tailwind CSS**。

已接通桌面项目登记、只读 Git 状态、根目录 Markdown 文档和 package.json 读取。桌面首页只展示用户登记的项目；浏览器预览使用明确标记的示例数据，与真实登记隔离。已接通本地公共规则源、单项目 AGENTS.md 公共区块的差异预览、应用与恢复。命令执行、Git 写操作、检查调度和跨项目批量同步尚未接通。

## 安装与启动

macOS：Go 1.27.3+、Node.js 22.13+（建议 Node 24 LTS）、npm。Go 模块和前端 runtime 固定 MyGo v0.8.3；Go 可自动下载模块要求的工具链。当前仅在 macOS 验证。

```sh
npm --prefix frontend ci
go mod download
go tool mygo dev
```

`go tool mygo dev` 生成绑定、启动 Vite 与桌面应用，退出应用或 Ctrl+C 停止。界面开发地址为 [http://127.0.0.1:5178](http://127.0.0.1:5178)。端口由 mygo.json 与 frontend/package.json 共同配置；占用时先核对进程归属，不停止其他项目服务。

仅浏览器预览：`npm --prefix frontend run dev`，然后打开上述地址。浏览器展示三个示例项目，可切换概览、命令和文档；数据源为 `frontend/src/preview.ts`，不调用桌面服务、不登记或读取本地目录。公共规范与更新在浏览器提示使用桌面应用，不模拟写入或成功结果。不要同时启动第二个 Vite 实例。

## 检查与构建

```sh
go tool mygo generate
npm --prefix frontend run check
go vet ./...
go test -tags=integration -race ./internal/workspace
npm --prefix frontend run build
go tool mygo build
```

前端 `check` 包含 TypeScript 与 ESLint；`build` 生成静态资源。Go 集成测试只使用临时目录和临时 Git 仓库，检查登记持久化、真实读取、规范更新/恢复、已有改动保护与错误状态。`go tool mygo build` 生成 `build/darwin-arm64/` 下的应用与镜像；本地 ad-hoc 签名不代表完成分发签名或公证。

前端锁文件包含 `minimatch ^11.0.1` override，供 ESLint / typescript-eslint 的开发期 glob 匹配使用，避开上游仍引用的已知拒绝服务问题。项目负责在升级这两个工具时复核并移除此 override；不进入应用运行时。

格式化：`gofmt -w <本次修改的 Go 文件>`；前端使用 `frontend/node_modules/.bin/prettier --write <本次修改的文件>`。`frontend/src/mygo.ts` 由 MyGo 生成，不手动修改。

## 手动验收

1. 桌面首页只显示已登记的本地项目；未登记时提供添加入口。公共规范可接入实际本地规则文件；浏览器不调用同步服务或显示虚构执行结果。
2. 添加本地项目绝对路径；重开应用后登记仍在。通过概览查看分支与工作区摘要，通过页签读取项目文档和命令，确认内容对应同一目录。文档页可切换 AGENTS.md、PRODUCT.md 等根目录 Markdown 文件；空文件和读取失败有明确提示。
3. 侧栏不显示「所有项目」按钮或常驻搜索框。点击「项目」展开或收起列表；收起时确认当前项目名仍可见，隐藏子项不进入 Tab 顺序。顶部搜索按钮或 ⌘K 打开搜索弹窗，输入名称或路径后可点击结果或按 Enter 打开，Escape 关闭；方向键切换顶部页签，Tab 访问文件和按钮。
4. 桌面默认打开为 960×640，可缩至 760×500。在默认、最小和 1440×900 窗口检查自适应布局、导航、键盘滚动、长路径与错误状态。
5. 浏览器预览明确显示“示例”，可查看有改动、干净工作区和非 Git 目录；切换项目与页签不调用本地项目服务。

文档页列出项目根目录的 `.md` / `.markdown` 文件（扩展名不区分大小写），按文件名排序，只读并按需加载正文，不递归扫描子目录。单个文件限制 256 KiB；指向项目目录外的符号链接不可读取。浏览器预览使用独立的示例文档。

项目登记保存在系统用户配置目录（macOS 为 `~/Library/Application Support/repsync/projects.json`）。项目页签偏好保存在 WebView localStorage；不自动登记当前目录或其他真实仓库。

## 公共规范同步

1. 在桌面「公共规范」中输入公共规则 Markdown 文件的绝对路径，接入后查看真实内容与 SHA-256 内容版本。
2. 在「更新」选择一个已登记项目，点击「预览差异」。首版只同步根目录 `AGENTS.md` 中 `repsync:shared:start` / `repsync:shared:end` 标记之间的公共区块；首次在原文前添加区块，区块外的项目内容逐字保留。`PRODUCT.md` 等文档不自动同步。
3. 确认差异后「应用更新」。服务端预览有效期为 15 分钟，最多保留 32 份；公共源、目标内容、存在状态或权限发生变化时需重新预览。通过校验后先保存备份，再以临时文件 + rename 更新目标文件。符号链接和异常区块标记会阻止更新。
4. 在「更新记录」恢复更新前内容。当前文件须与该次更新结果完全一致；有后续改动时停止恢复。原先不存在的文件会恢复为不存在，已有文件恢复原文和权限。

配置位于系统用户配置目录的 `repsync/sync/source.json`，备份和操作记录位于 `repsync/sync/history/`，不写入项目仓库。应用中断后按实际文件内容判断记录状态，重开应用仍可恢复。备份包含更新前后全文，请按用户数据保留；暂不自动清理。

当前仅提供单个本地规则源与逐项目手动更新，不执行项目脚本或 Git 提交，不自动应用或自动合并项目内容。不要同时通过多个 repsync 实例更新同一项目；对外部编辑器采用写入前重新读取检查，不提供文件系统层面的原子比较交换。

## 图标

应用保留原来的蓝色双斜线标志，唯一可编辑源文件为 `resources/mark.svg`。`npm --prefix frontend run icons` 生成桌面图标（`resources/icon.svg`、1024px `resources/icon.png`）、侧栏标志与 favicon；前端构建会运行同一入口，内容未变化时不会重写文件或触发桌面重启。

界面图标使用 `lucide-react` 的具名导入，由 `frontend/src/components.tsx` 统一尺寸和线宽；只打包实际使用的图标。`@resvg/resvg-js` 仅用于开发期把标志渲染成桌面 PNG，不进入应用运行时。图标生成脚本与依赖锁文件由本项目维护。
