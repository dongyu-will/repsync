# repsync

[English README](../README.md) · [在线演示](https://dongyu-will.github.io/repsync/) · [下载预览版](https://github.com/dongyu-will/repsync/releases)

应用默认界面为英文。本文保留中文详细操作说明；快速上手、开发命令、开源贡献与发布入口以英文 README 为准。浏览器演示使用隔离数据，不能操作本机项目。

由 AI 配置的本地多项目管理器。集中启动、检查和查看项目，复用公共规范与工程资源，减少跨项目切换和手动配置。

## 文档入口

- [PRODUCT.md](../PRODUCT.md)：当前需求、已确认决策、设计基准、验收行为和待决事项的权威记录。
- [AGENTS.md](../AGENTS.md)：本项目的 Agent 工作规则。
- [DESIGN.md](../DESIGN.md)：当前界面基线、组件与交互约定。
- [Figma：规范更新页面](https://www.figma.com/design/dVq8rmWWIe4lTebO5gEW11?node-id=2-143)：已认可的可编辑设计稿。

## 当前状态

首轮实现采用 **MyGo WebView + Go、React + TypeScript + Vite、Tailwind CSS**。

已接通桌面项目登记、只读 Git 状态、根目录 Markdown 文档预览与编辑，以及 package.json 读取。启动时恢复上次打开的项目和项目页签；没有已登记项目时直接提供添加入口。浏览器预览使用独立数据并按正常使用界面呈现，与真实登记隔离。已接通本地公共规则源、单项目 AGENTS.md 公共区块的差异预览、应用与恢复。项目「CI/CD」支持本地 CI/CD 命令执行、停止、超时、日志和历史，GitHub Actions 支持读取最近运行与打开日志。概览的本地服务支持启动、停止、重启、端口占用查看和日志。工作树和分支支持查看、受保护的清理与恢复，以及按仓库开启的定时清理；提交、拉取、推送、流水线自动调度和跨项目批量同步尚未接通。

## 安装与启动

macOS：Go 1.27.3+、Node.js 22.13+（建议 Node 24 LTS）、npm。Go 模块和前端 runtime 固定 MyGo v0.8.3；Go 可自动下载模块要求的工具链。当前仅在 macOS 验证。

```sh
npm --prefix frontend ci
go mod download
go tool mygo dev
```

`go tool mygo dev` 生成绑定、启动 Vite 与桌面应用，退出应用或 Ctrl+C 停止。界面开发地址为 [http://127.0.0.1:5178](http://127.0.0.1:5178)。端口由 mygo.json 与 frontend/package.json 共同配置；占用时先核对进程归属，不停止其他项目服务。

仅浏览器预览：`npm --prefix frontend run dev`，然后打开上述地址。浏览器恢复上次查看的示例项目，首次默认打开第一个示例。三个示例项目可切换概览、项目地图、工作树、CI/CD、规范、AI 资源和文档；数据源为 `frontend/src/preview.ts`，不调用桌面服务、不登记或读取本地目录。公共规范在浏览器提示使用桌面应用，不模拟写入或成功结果。不要同时启动第二个 Vite 实例。

## 检查与构建

```sh
go tool mygo generate
sh tools/shared/check-go-format.sh cmd internal
npm --prefix frontend run check
go vet ./...
go test -tags=integration -race ./internal/workspace ./internal/control
npm --prefix frontend run build
go tool mygo build
```

前端 `check` 包含 TypeScript 与 ESLint；`build` 生成静态资源。Go 集成测试只使用临时目录和临时 Git 仓库，检查登记持久化、真实读取、规范更新/恢复、已有改动保护与错误状态。`go tool mygo build` 生成 `build/darwin-arm64/` 下的应用与镜像；本地 ad-hoc 签名不代表完成分发签名或公证。

前端锁文件包含 `minimatch ^11.0.1` override，供 ESLint / typescript-eslint 的开发期 glob 匹配使用，避开上游仍引用的已知拒绝服务问题。项目负责在升级这两个工具时复核并移除此 override；不进入应用运行时。

格式化：`gofmt -w <本次修改的 Go 文件>`；前端使用 `frontend/node_modules/.bin/prettier --write <本次修改的文件>`。`frontend/src/mygo.ts` 由 MyGo 生成，不手动修改。

## 手动验收

1. 启动时恢复上次打开的已登记项目及页签；已保存项目不在列表时打开第一个项目，没有项目时直接显示添加入口，登记读取失败时可重试。通过侧栏或搜索切换项目。公共规范可接入实际本地规则文件；浏览器不调用同步服务或显示虚构执行结果。
2. 添加项目支持「本地目录」和「远程仓库」；本地可通过系统目录选择器登记，远程选择仓库地址、上级目录和新目录名称，克隆成功后自动登记并打开。重开应用后登记仍在。通过概览查看分支与工作区摘要，通过页签读取项目文档，在概览的「本地服务」区域查看启动、停止与日志，确认内容对应同一目录；「CI/CD」本地页按项目已有配置运行，远程页读取 origin 对应的 GitHub Actions。文档页可切换 AGENTS.md、PRODUCT.md 等根目录 Markdown 文件，默认预览，切换编辑后可保存；空文件和读取失败有明确提示。
3. 侧栏不显示「所有项目」按钮或常驻搜索框。点击「项目」展开或收起列表；收起时确认当前项目名仍可见，隐藏子项不进入 Tab 顺序。顶部搜索按钮或 ⌘K 打开搜索弹窗，输入名称或路径后可点击结果或按 Enter 打开，Escape 关闭；方向键切换顶部页签，Tab 访问文件和按钮。「规范」页签分为「项目规则 / 公共规范」，切换分区保留当前编辑状态；AI 规范说明在项目规则中通过按钮打开。概览只保留项目状态、本地服务和必要提醒。
4. 桌面默认打开为 960×640，可缩至 760×500。在默认、最小和 1440×900 窗口检查自适应布局、导航、键盘滚动、长路径与错误状态。
5. 浏览器预览不显示示例标签或模式说明，可查看有改动、干净工作区和非 Git 目录；切换项目与页签不调用本地项目服务。

文档页列出项目根目录的 `.md` / `.markdown` 文件（扩展名不区分大小写），按文件名排序，按需加载正文，不递归扫描子目录。默认显示 Markdown 预览，支持标题、列表、代码块、表格和任务列表；切换「编辑」修改原文，通过「保存」或 ⌘S / Ctrl+S 写入已有文件。编辑草稿保留在当前会话，切换文件或页签后可继续编辑，返回文件时默认预览；「重新载入」可放弃草稿并读取磁盘版本。

在文档预览中点击放大镜或按 ⌘F / Ctrl+F，查找当前文档正文。匹配处会高亮，Enter 跳到下一处，Shift+Enter 返回上一处，Esc 关闭查找。

单个文件限制 256 KiB；保存保留原权限，以临时文件 + rename 更新。原文件被修改、删除、替换或改为符号链接时拒绝保存并保留草稿；只读文件不可保存。不支持多个应用实例同时修改同一文件，外部编辑器采用保存前重新读取核对。符号链接仅支持读取项目内目标，不能编辑。浏览器示例可编辑和预览会话草稿，不写入任何真实项目。

文档预览使用按需加载的 `react-markdown` 和 `remark-gfm`，由项目维护其版本及前端锁文件；不执行原始 HTML。根目录 Markdown 文件间的链接可以切换文档，外部链接和远程图片链接由用户点击后打开；本地图片暂不展示。

项目登记保存在系统用户配置目录（macOS 为 `~/Library/Application Support/repsync/projects.json`）。最近项目与页签偏好保存在 WebView localStorage（桌面最近项目使用 `repsync:last-project`，浏览器示例使用独立的 `repsync:preview:last-project`）；不自动登记当前目录或其他真实仓库。

## 项目地图

项目页签中的「项目地图」以功能模块为主。列表只显示功能、简短用途与必要提示；点开模块查看对应实现和入口。多份实现不自动判重，疑似重复需要具体理由；未记录入口保留待核对，不能据此判定代码已废弃。

点击「让 AI 整理」将说明复制到已有 AI 聊天。AI 检查项目后维护根目录 `.repsync-map.json`，回到地图刷新即可。应用只读取地图并核对引用，不内置 AI，不改动业务代码，也不自动维护其他项目。引用定位与流程验收分别判断；地图的覆盖范围由整理者说明。

通过 `repsync project-map PATH` 可读取相同结果。完整字段、示例与读取限制见 [Project map](usage.md#project-map)，统一在该处维护。

## 设置

设置中可控制最近项目与页签恢复、默认项目页、概览刷新频率、文字大小与减少动态效果。偏好自动保存，桌面与浏览器分别保存；修改启动偏好不切走当前项目。概览刷新可选每 30 秒、每分钟、每 5 分钟或仅重新聚焦，其他页面沿用自己的刷新周期。

「外观 → 语言」提供跟随系统、简体中文和 English，默认跟随系统；切换即时生效，重新打开后保留选择。跟随系统读取系统 / WebView 的语言，中文使用简体中文，其他语言使用英文，并响应语言变更事件。项目名、路径、命令、日志、差异以及文档和记忆原文保留原始文本。

重置只恢复上述偏好，保留项目登记、文档草稿与历史。保存失败时当前窗口仍采用修改，并显示错误；版本与文档、问题反馈、下载入口位于设置底部。英文说明见 [Settings](usage.md#settings)。

## 工作树、分支与自动清理

项目「工作树」按「工作树 / 本地分支」查看路径、检出分支、提交、上游、合并状态及保留原因；点击顶部的分支标记可直接进入。只读取当前本地 Git 状态，不 fetch、不切换分支、不展示整仓 diff。未提交过的仓库显示主工作树与空分支列表。

「自动清理」按仓库单独开启，默认关闭、保留 7 天，可选本地基准分支及 1–365 天保留期。同仓库的多个登记项目共享设置。桌面启动 15 秒后检查一次，此后应用运行期间每小时检查；退出停止计时，不安装后台任务。保留期取首次观察、分支 reflog 最近变更和工作树 HEAD 最近检出的较新时间，缺少时间依据时保留。仅按 Git 祖先关系判断已合并，squash 合并不会被推定为已合并。

主工作树、基准分支、main/master/develop/development/production/staging/release、release/* 与 hotfix/* 始终保留。已登记项目及与其目录重叠的工作树、锁定/缺失/分离 HEAD 工作树、有修改、未跟踪或忽略文件的工作树均保留。通过 lsof 检查目录内打开文件和进程工作目录，无法确认时保留；Git 操作未完成时保留。Codex 管理的 `~/.codex/worktrees/` 工作树由对应聊天管理，展示但不在此清理。不推定未检出的本地分支无用途，保留期与基准分支应按仓库实际工作流设置。

「预览清理」列出本次候选，确认后重新核对版本及状态，再通过不带 force 的 Git 命令逐项移除；分支仍被检出或 Git 自身拒绝删除时保留。只处理本地分支和干净的附加工作树，不删除远程分支、忽略文件或其他未登记仓库。外部 Git/编辑器操作不参与应用锁，清理时应避免同时改变候选目录和分支；执行前再次检查和 Git 自身保护不能提供跨进程的文件系统事务。

清理记录与设置保存于用户配置目录 `repsync/git/<仓库哈希>/`；每项清理前保存 `refs/repsync/cleanup/<记录编号>/<序号>` 恢复引用，防止提交被垃圾回收。记录展示逐项成功、保留及原因；中断的 pending 项需结合实际 Git 状态核对。恢复重建分支及干净工作树，保留已有同名分支的新提交和已有目录；恢复的内容重新计算保留期。恢复不重建已删除的工作树 reflog 或其他 Git 临时状态。恢复引用、配置及历史不自动删除，界面显示最近 20 条；它们属于本机项目数据。

CLI 与桌面使用相同检查和恢复入口：

```sh
repsync git-workspace /path/project
repsync git-cleanup /path/project VERSION
repsync git-restore /path/project RECORD_ID
```

VERSION 来自当前 `git-workspace` 的回执；状态变化后旧回执不能执行。浏览器工作树数据来自 `frontend/src/git-preview.ts`，清理、保存设置和恢复按钮禁用，不读取或修改真实项目。单次检查限制 64 个工作树、256 个本地分支及约 25 秒；超过限制不清理，单个 Git/lsof 命令另有 5 秒超时。自动检查失败写入桌面运行日志，后续周期重试。

专项验收：`go test -tags=integration -race ./internal/workspace -run TestGitWorkspace`，使用临时 Git 仓库、工作树和进程，验证清理、恢复、保留策略、进程保护、过期预览和退出收尾。

### Stash 待处理内容

「工作树」中的 Stash 分区列出仓库全部 stash，包括序号、原始说明、创建时间和提交标识。同一仓库的工作树共享 stash，Git 通常不能可靠确定原工作树、AI 会话或是否已经恢复，不按说明猜测所有者或处理状态。项目概览在有 stash 时持续提示并提供入口；当前项目和工作树页在窗口重新获得焦点、每 60 秒及手动刷新时更新，避免外部 AI 提交推送后留下的 stash 被忽略。

「查看内容」展示文件统计、文件差异及 stash 中包含的未跟踪文件；超过 1 MiB 显示截断提示。恢复使用完整对象 ID，不依赖会随新 stash 移动的 `stash@{N}` 序号，并核对预览时的列表、目标分支和 HEAD。只向已登记工作树根目录恢复，要求工作树无修改及未跟踪文件、本应用的该项目服务/流水线/规则检查已结束，并且没有未完成的 Git 操作；外部编辑器和 Git 操作不参与应用锁，请勿在恢复期间并行改动目标。

恢复等同 `git stash apply --index OID`，保留原 stash，不执行 pop、drop、clear 或自动重置。冲突或部分恢复会明确提示并保留现场，需在原项目中处理；原 stash 的备份仍在。确认恢复结果后，应通过项目原有 Git 工作流处理保留的 stash，计数不会仅因 apply 成功而减少。有任何 stash 待处理、或 stash 状态无法确认时，暂停该仓库的分支和工作树清理；stash 不纳入自动删除。

```sh
repsync git-stashes /path/project
repsync git-stash-show /path/project OID
repsync git-stash-apply /path/project OID VERSION
```

VERSION 取自本次 `git-stash-show` 回执。AI 在提交推送前后可通过该入口核对并报告 stash；工作区干净不能作为 stash 已恢复的证据。CLI 恢复冲突时返回失败，并保留具体回执及原 stash。浏览器只展示隔离内容，恢复按钮禁用。

专项验收：`go test -tags=integration -race ./internal/workspace ./internal/control -run 'TestGitStash|TestControlStash'`，通过临时真实仓库、工作树及 CLI 检查共享列表、暂存区与未跟踪文件恢复、序号变化、过期预览、冲突和忽略文件保护。

## 手动收尾检查与交接

项目概览的「待处理」提供「检查当前状态」，按改动、检查、交付、数据与环境、交接及适用项查看。此入口只读取本地状态，不 fetch、不运行测试、不提交、推送、部署或清理，也不创建定时任务。

- **改动**：列出已暂存、未暂存、未跟踪和冲突文件、stash，以及相对本地上游引用的领先/落后提交数。显示 FETCH_HEAD 的最近时间；缺少上游或信息读取失败时显示未知，不用本地引用证明远程当前状态。界面列前 100 个改动，CLI 返回完整的已读取列表。
- **检查**：本地 CI 流水线记录开始/结束文件内容摘要、命令配置版本、提交和结果。外部 AI/终端检查可手动填报命令、覆盖范围、结果和日志位置，明确标记未由应用执行。通过与内容一致性分别显示；文件或命令变化、运行期间文件变化、旧记录缺少内容依据时，提示变化或未知。提交相同文件内容不单独导致内容比较过期；依赖 Git 元数据的检查仍需另行判断。不自动要求或执行全量检查。
- **交付**：本地改动、提交、推送、CI/检查、部署、线上验收逐项展示；本地命令成功不证明部署或业务验收。推送、CI、部署和验收可记录手工核验结果、说明或链接，绑定当时内容和提交；状态变化后保留旧说明并提示过期。手工记录不等同应用已执行该动作。
- **数据与环境**：仅检查项目声明的对象。文件和目录只读取存在性、大小、时间及 Git 跟踪/忽略状态；环境变量只读取是否定义，不展示值；数据库、迁移、备份与恢复演练通过手工项目记录。文件存在不证明备份有效或可恢复。复用 doctor 显示工具、配置、端口和服务引用诊断，不执行项目命令。
- **交接**：手动记录任务开始、结束和说明，保存两端状态，列出期间新增的 stash 对象和未跟踪路径，不推定创建者。记录完成内容、检查、阻塞和交付阶段，支持查看、复制 Markdown 交接。没有完整开始/结束依据时明确说明不能可靠计算差异；交接完成不推定推送、部署或验收完成。

文件内容比较覆盖 Git 工作树已跟踪及未忽略的未跟踪文件，不读取忽略的依赖、外部环境或其他服务数据。单次限制 5000 个文件、40 MiB 内容、约 10 秒文件读取；符号链接只比较链接目标，不读取项目外目标；子模块、权限失败或读取中内容变化报告未知。此比较不冻结文件系统，不能证明检查运行期间从未发生过并发改动。

检查、核验、任务和交接记录保存在系统用户配置目录 `repsync/review/<登记路径哈希>/`，不写入目标源码。界面显示最近 20 条检查及已完成任务，进行中任务全部保留；检查、核验、任务和提醒处理记录每类最多读取 1000 条用于判断，超过上限明确报错。记录不会自动删除。旧路径的记录保留，不自动转移到重新登记的新目录。

### 项目记忆

在项目内「AI 资源 → 项目记忆」添加和查看记录。每条必须填写来源，可附适用环境和文档关键词。手动录入可直接保存为已确认，AI 通过 CLI 提交的推断进入「待确认」。待确认的记录可确认或归档。归档后的记录可撤销归档，恢复到之前的状态。

记忆保存在 repsync 的本机配置目录，按登记目录隔离。文档核对使用同项目已确认记录的关键词匹配段落，并展示记录内容、来源与适用环境。关键词可以填旧名称，例如 `MySQL`。已确认记录变化后，使用旧依据的扫描需要重做。

```sh
repsync project-memory PATH
repsync project-memory-propose PATH JSON_FILE
```

读取命令返回已确认记录。提交文件的示例为 `{"statement":"生产环境已切换到 PostgreSQL","source":"部署记录 #42","environment":"生产","keywords":["MySQL"]}`。

打开「从 AI 会话提炼」，分别选择「会话来源」和「提炼工具」，当前支持 Codex 与 Claude Code，可交叉选择。列表只读取最近 30 天内修改、起始目录属于当前登记项目的本地主会话。Codex 从 `$CODEX_HOME/sessions`（默认 `~/.codex/sessions`）读取，Claude Code 从 `$CLAUDE_CONFIG_DIR/projects`（默认 `~/.claude/projects`）读取；配置目录须为绝对路径。不读取子 Agent 会话或符号链接。

点击「提炼并加入待确认」后，将所选会话中归属当前项目且已结束轮次的用户文字发送给所选 CLI 的默认模型；需自行安装并登录该工具。Codex 使用已标记的用户文字或旧版用户消息事件，Claude Code 使用用户消息及 assistant 的 `end_turn` / `stop_sequence` 完成标记，排除工具结果、系统注入、压缩摘要和未完成轮次。无法识别完成标记的消息不进入提炼。

每次扫描最多 8000 个目录项、256 个近期文件、16 MiB 元数据并列出 20 个会话；选定文件最多读取 32 MiB，各阶段约 5 秒。提炼最多发送最近 48 条消息或 24 KiB 用户文字，生成 5 条候选，CLI 超时 90 秒。CLI 在临时目录中运行，关闭工具、MCP、项目规则和会话持久化；Claude Code 沿用本机版本的 `--safe-mode`、`--tools ""`、`--strict-mcp-config` 和结构化输出入口（参见 [Claude Code CLI 文档](https://code.claude.com/docs/en/cli-reference)）。

每条候选的引文必须逐字出现在对应的用户消息中。通过校验的记录保存为待确认，「原会话与用户原话」列出原文件、行号、引文及提炼工具。同一来源工具、会话行上的相同引文或陈述会复用已有记录，包括已归档的记录；更换提炼工具不会重复添加。旧版没有来源工具字段的引用按 Codex 识别。浏览器仅展示界面，不扫描或提炼本机会话。

专项验收：`go test -tags=integration -race ./internal/workspace ./internal/control -run 'Test.*Memory'`。使用临时会话、临时登记项目和 CLI 进程夹具，验证项目隔离、已结束用户原话、路径保护、提炼来源、跨工具去重与失败回执。

### 文档核对

在项目概览「待处理 → 文档核对」选择未提交改动或最近一次提交。应用从 Git diff 提取移除的技术名称、符号和路径，用 `rg` 匹配项目根目录和 `docs/` 下的 Markdown。可以补充旧功能名或别名。结果列出原文位置、变更依据和改写建议。历史、归档和迁移目录中的文档不在扫描范围。

需要本机安装 `rg`。如果项目已有 [CodeGraph](https://github.com/colbymchenry/codegraph) 索引，扫描会先同步索引，再补充符号定义或调用方。可在项目目录运行 `codegraph init` 建立索引。索引或工具不可用时显示原因，保留 Git 和 `rg` 的结果。代码证据只覆盖本地可读范围，线上退役状态及备份恢复等保留用途需要另行核实。

逐段编辑或预览删除，核对前后内容后点「应用修改」。写入时校验文档及 Git 变更，保护外部编辑，保留其他段落和文件权限。应用后可以重新扫描同一文档。浏览器使用隔离预览，实际扫描和写入由桌面版提供。

单次最多读取 128 个文档、合计 4 MiB，显示 50 个候选段落。未提交改动以 HEAD 和工作区比较，不含未跟踪的源码。专项验收使用 `go test -tags=integration -race ./internal/workspace -run 'TestDocumentReview|TestSaveDocument'`。

### 前端、后端与数据库分别核验

`.repsync.json` 的 `reviewProfile` 声明项目类型、适用范围、必需检查和交付阶段。优先由 AI 按项目指南配置；概览「待处理 → 适用项」提供兜底表单。未配置时提示缺少声明，不推定所有项目都需要部署或数据库。保存复用配置版本保护，只更新此字段、保留其他配置与文件权限，不执行任何检查。

```json
{
  "reviewProfile": {
    "kind": "web",
    "areas": ["frontend", "backend", "database"],
    "requirements": ["frontend-static", "frontend-behavior", "backend-static", "backend-api", "database-integration", "database-migrations", "database-consistency", "database-recovery"],
    "stages": ["push", "ci", "deploy", "acceptance"]
  }
}
```

kind 支持 web/backend/desktop/library/notes/custom；areas 只声明实际存在的前端、后端和数据库，requirements 必须属于已声明范围。库、笔记或仅本地工具可省略不适用的交付阶段；空 stages 不要求远程交付。各必需检查分别显示状态，不用最近一条成功记录代表全部检查通过。

| requirement | coverage 的 area / method | 需要记录的实际范围 |
| --- | --- | --- |
| frontend-static | frontend / static | 类型、lint 等静态检查 |
| frontend-behavior | frontend / behavior | 浏览器、键盘和交互验收 |
| backend-static | backend / static | 后端静态检查 |
| backend-api | backend / integration | 真实 HTTP / API 的认证、权限、错误及受影响行为 |
| database-integration | database / integration | 真实数据库中的读写、事务及回滚 |
| database-migrations | database / migration | 当前迁移状态、新旧代码兼容及既有回滚方案 |
| database-consistency | database / consistency | 关键约束、关联及受影响业务数据 |
| database-recovery | database / recovery | 备份对象、恢复演练及恢复结果 |

外部检查填报增加 `coverage`、`environment`、`target`；数据库类别必须填环境和目标别名，禁止在这两个字段放连接串或密码。数据库脚本可填 `database / static`，但不抵扣任何真实数据库检查。仅填旧版 scope 文本不能推断类别。每条记录展示来源、声明范围、环境、目标及结果依据；外部成功标为「已记录核验」，由应用执行的命令标为「命令通过」。应用不解析测试内部逻辑，仍须确认声明类别是否确实执行。

```json
{
  "command": "项目指南规定的集成测试入口",
  "scope": "真实测试数据库中的事务回滚；未覆盖生产库",
  "status": "success",
  "evidence": "已有日志位置、运行时间及结果",
  "coverage": [{ "area": "database", "method": "integration" }],
  "environment": "local",
  "target": "test-db"
}
```

显式 CI 流水线也可声明相同 `coverage`，环境和目标字段使用 `reviewEnvironment` / `reviewTarget`；运行回执保存当次配置。自动发现的命令不推断覆盖类别。后续同类失败覆盖旧成功，文件内容或命令配置变化显示过期；未读到对应记录时显示待核验。内容比较不观察外部数据库或环境变化，历史核验不能证明外部状态仍一致；本地目标记录不能作为生产库的核验依据。

### 提醒处理与并行任务

每项提醒可手动标为「已核对，保留」或「稍后处理」，必须记录理由，也可重新提醒。原事实和检查结果仍在：保留 stash 不删除 stash，也不解除工作树清理保护。相关内容变化后再次提示；stash 按对象 ID 识别，其他文件变化不会反复唤起同一条保留提醒。不按时间自动唤起或处理。

任务可额外声明负责人/AI 会话别名、涉及范围、项目内路径以及环境和目标。已登记项目中同时进行的任务若声明相同数据库环境与目标，或修改路径重叠，展示协调提示；缺少路径时明确无法排除重叠。此提示仅依据手工声明，不是进程监控、归因或数据库锁。任务完成后保留声明、实际检查类别及保留理由到 Markdown 交接。

### 声明数据与环境对象

优先由 AI 阅读项目指南后维护 `.repsync.json` 的 `reviewResources` 数组；概览也提供添加/移除声明的兜底入口。只修改该数组，保留服务、流水线、其他字段及文件权限；版本不一致时拒绝写入。移除声明不删除数据、备份或历史。

```json
{
  "reviewResources": [
    { "id": "local-env", "name": "本地配置", "kind": "file", "target": ".env.local", "guide": "确认独立保留方式，不读取配置内容" },
    { "id": "uploads", "name": "上传目录", "kind": "directory", "target": "uploads", "guide": "按项目指南核对备份和恢复演练" },
    { "id": "db-env", "name": "数据库连接配置", "kind": "environment", "target": "DATABASE_URL", "guide": "只检查定义，连接和数据状态另行核验" },
    { "id": "db-backup", "name": "数据库备份与迁移", "kind": "manual", "target": "", "guide": "按项目运行指南检查，不自行拼接数据库命令" }
  ]
}
```

最多 32 项；file/directory 使用项目内相对路径，环境变量使用名称，不填写实际秘密值。项目外备份、数据库与迁移采用 manual，并在核验说明中记录已有检查方法和结果。不新增驱动、数据库或备份服务。

### AI 手动记录入口

```sh
repsync review /path/project
repsync review-profile /path/project CONFIG_VERSION /absolute/profile.json
repsync review-issue /path/project REVIEW_VERSION /absolute/issue.json
repsync review-task-context /path/project REVIEW_VERSION /absolute/task.json
repsync review-task-start /path/project REVIEW_VERSION "任务名称"
repsync review-check /path/project REVIEW_VERSION /absolute/check.json
repsync review-evidence /path/project REVIEW_VERSION /absolute/evidence.json
repsync review-resources /path/project CONFIG_VERSION /absolute/resources.json
repsync review-task-finish /path/project TASK_ID REVIEW_VERSION /absolute/handover.json
repsync review-handover /path/project TASK_ID
```

REVIEW_VERSION 来自刚读取的 review；CONFIG_VERSION 使用其中 configVersion。状态或记录变化后需重新读取，不覆盖旧视图。JSON 文件分别为：

- check：`{"command":"npm run check","status":"success","scope":"frontend","evidence":"日志位置及结果说明"}`；status 支持 success/failure。
- evidence：`{"stage":"deploy","target":"","status":"success","note":"核验提交、环境和结果链接"}`；stage 支持 push/ci/deploy/acceptance/resource；resource 的 target 为检查对象 id；status 还支持 not_applicable。
- profile：与 reviewProfile 相同的对象；CONFIG_VERSION 使用 configVersion。
- issue：`{"key":"review 中对应提醒的 key","state":"retained","reason":"保留理由"}`；state 支持 retained/later/reopened，重新提醒可以不填理由。
- task：`{"title":"任务名称","owner":"AI 会话别名","areas":["backend","database"],"paths":["internal/api","migrations"],"environment":"staging","target":"test-db"}`；数据库范围必须声明环境与目标。旧版 review-task-start 仍支持仅记录名称。
- resources：与 reviewResources 相同的数组，空数组表示移除全部声明。
- handover：`{"summary":"完成内容","checks":"检查说明","blockers":"遗留问题与下一步"}`。

先记录任务开始再工作，结束时重新检查并填报；外部检查记录是填报当时的内容依据，不能用事后填报证明先前未记录的环境或代码。输入文件只用于本次手动调用，不自动执行其中命令。浏览器使用隔离预览，写入和运行入口禁用。

专项验收：`go test -tags=integration -race ./internal/workspace ./internal/control -run 'TestProjectReview|TestControlManualReview|TestLocalPipeline'`，使用临时真实 Git 仓库、本地远程、文件和进程验证，不访问其他真实项目。

## 添加与克隆项目

「添加项目」提供两种方式：登记已有目录，或克隆远程仓库。两者均支持系统目录选择器；远程目录名称根据仓库地址建议，也可手动修改。

远程支持 HTTP(S)、SSH URL 和 `git@host:owner/repo.git` 形式的 SSH 地址，也接受 Git 协议。使用本机 Git、凭据助手和 SSH agent/config；不存储新的密码或令牌，地址内不接受凭据、查询参数或片段。没有配置自定义 SSH 命令时使用 BatchMode；不提供终端交互，首次 SSH 主机信任或账号认证请先在终端完成。

目标上级目录必须已存在，目标文件夹必须不存在（包括空目录、文件或符号链接）；服务先保留新目录，避免覆盖已有内容。克隆不递归初始化子模块、不执行项目脚本，并禁用该次 Git 调用的 hooks。界面显示真实 Git 输出，可停止克隆；一次运行一个克隆，最长 30 分钟，退出应用会结束所属克隆进程组。克隆并登记成功后直接进入项目。

失败或取消不登记项目；如 Git 留下部分文件，会保留目标目录并显示路径，重试需使用新目录或自行检查后处理。克隆成功但登记失败时，可选择「登记已克隆目录」，无需重复下载。最近一条克隆的进度保存在应用会话内，日志保存在系统用户配置目录的 `repsync/clones/`，每条最多 256 KiB，保留最新输出；重启不续传，日志不自动清理。

专项验收：`go test -tags=integration -race ./internal/workspace -run TestClone`。测试通过本机临时 HTTP 仓库完成真实 Git 克隆和登记，检查目录保护、非法地址、失败、取消与退出收尾，不访问或修改其他真实仓库。

## 项目 CI/CD

项目内「CI/CD」同时支持本机执行和 GitHub Actions，与项目是否开源无关。复用现有检查或发布脚本；repsync 自身的根目录 `.repsync.json` 已配置「本地 CI」与「桌面打包」，打包不代表发布或部署。

本地流水线按 `steps` 显示串行流程图，以最近一次运行的配置版本与步骤序号标示状态。正在执行的节点旋转、进入该节点的连线流动，完成或停止后静止；系统启用减少动态效果时使用静态提示。长流程在步骤变化时只滚动流程区域，露出当前节点，保留页面位置与键盘焦点。

桌面打开本地页期间，每秒读取运行中的记录及配置，空闲或读取失败时每 5 秒重试，以发现 AI 通过 CLI 发起的运行和配置修改；离开页面停止读取。配置读取失败时禁止启动并保留兜底配置入口，运行状态读取失败时保留上次记录、停止流程动画并将当前步骤标为待核对；恢复读取后清除对应错误。首次读取失败不显示为没有运行记录。

点击节点查看完整命令、工作目录、超时与环境变量名称，并可打开整次运行日志，焦点随之进入日志区域。运行中的日志每秒更新，结束后停止轮询；读取失败可单独重试。配置版本不匹配、旧记录缺少版本或步骤依据异常时显示待核对，不将历史结果套到当前步骤；一个脚本作为一个节点，不拆解脚本内部流程。GitHub Actions 当前仍显示运行摘要并链接远程日志，尚未读取远程 jobs 与步骤图。

本地配置放在项目根目录 `.repsync.json`：

```json
{
  "pipelines": [
    {
      "id": "ci",
      "name": "本地 CI",
      "kind": "ci",
      "timeoutSeconds": 1800,
      "steps": [["make", "ci"]]
    },
    {
      "id": "deploy",
      "name": "发布",
      "kind": "cd",
      "steps": [["npm", "run", "deploy"]]
    }
  ]
}
```

`steps` 是可执行文件与参数组成的数组，默认在登记项目根目录逐步运行，可通过 `cwd` 指定项目内子目录，非零退出即停止后续步骤。只有点击「运行」才执行，CD 运行的是配置的真实命令。支持 Go、npm、pnpm、yarn、bun、Make 和现有脚本；需要 Shell 语法时显式配置 `["/bin/sh", "-c", "你的命令"]`。命令继承桌面进程环境与 PATH，可通过 `env` 覆盖自定义变量，设置 `CI=true`、`NO_COLOR=1`，不载入交互式 Shell、不提供终端输入；缺少工具时配置可执行文件绝对路径或修正应用启动环境。账号及凭据沿用项目现有环境或脚本配置。

未声明 `pipelines`（或值为 `null`）时，仅按顺序识别根目录 package.json 已有的 `ci`、`check:ci` 或 `check` 脚本，选择首个入口，复用 `packageManager` / 锁文件识别包管理器。不会自动拼装 lint/test/build 或猜测发布命令；使用 `pipelines: []` 关闭自动识别。单仓多包项目可通过 `cwd` 指定子目录。每个项目最多 32 条流水线，每条 1–32 步，默认 30 分钟，支持 1–7200 秒超时。配置变化需刷新后再运行。

每项目同时运行一条，应用最多同时运行四个项目；切换页面继续运行，停止或退出应用会结束普通子进程组。当前仅在 macOS 验证，不适合交互式任务或脱离进程组的后台服务。运行记录保存起始提交及工作区是否有改动；检查针对运行时的本地工作区，记录不代表工作区已冻结。重启后未完成的记录标记为「已中断」，不会推定成功。

运行元数据和日志保存在用户配置目录 `repsync/pipelines/<项目路径哈希>/`，界面展示最近 20 条，每条日志最多保留 256 KiB，超出后任务仍继续。历史不自动删除，日志可能包含命令输出的敏感内容，按本机项目数据保留。

GitHub Actions 读取 Git `origin` 的 github.com 地址，通过本机 [GitHub CLI](https://cli.github.com/manual/gh_run_list) 获取最近 20 条运行；沿用 `gh auth login` 的账号和私有仓库权限，repsync 不另存 token。缺少 gh、未登录、权限不足或无远程仓库时分别提示，本地流水线仍可使用。远程日志、触发、重跑与审批通过 GitHub 原有界面完成，不自动触发远程发布。浏览器使用 `frontend/src/pipeline-preview.ts` 的隔离示例，运行按钮禁用。

专项验收：`go test -tags=integration -race ./internal/workspace -run 'TestLocalPipeline|TestGitHubActions'`，使用临时仓库和 CLI fixture，不访问其他真实仓库、不部署。

项目不设置独立「命令」页签。日常服务操作位于概览的「本地服务」区域，启动命令与 package.json 脚本通过按钮查看；CI/CD 的原始命令点击流程节点查看。旧的「命令」页签偏好会回到概览。

## 本地服务

在项目概览的「本地服务」管理开发服务，切换项目后继续运行，不自动随应用启动。`.repsync.json` 中使用 `services`，与已有 `pipelines` 并列：

```json
{
  "services": [
    {
      "id": "web",
      "name": "前端开发",
      "command": ["npm", "run", "dev", "--", "--port", "{port}", "--strictPort"],
      "port": 5173,
      "autoPort": true,
      "url": "http://127.0.0.1:{port}"
    }
  ]
}
```

每个服务使用唯一 id、名称和命令参数数组，默认工作目录为登记项目根目录，可用 `cwd` 指定子目录、`env` 配置环境变量；支持 Go、npm、Make 或现有脚本。`url` 可选，只接受本机 http/https 地址（127.0.0.1、localhost、::1）；端口从 URL 读取，也可仅配置 `port`。两者同时填写时必须一致；没有页面地址的服务仍可启动、停止并查看日志。页面入口需要本应用拥有运行进程且端口可连接；TCP 可连接不代表业务健康检查已通过。

未声明 `services` 时，按 `dev`、`start` 顺序识别根目录 package.json 首个已有脚本，沿用 CI/CD 的包管理器识别。服务读取后统一准备自动端口：命令参数 `--port N` / `--port=N`、配置环境变量 `PORT`，以及 npm/pnpm/yarn/bun 的单条 Vite 开发脚本均可自动接入，无需逐项目打开开关。Vite 优先沿用声明的端口或脚本中的端口，缺省使用其开发端口 5173；其他框架不猜测默认端口或页面地址。使用 `services: []` 可关闭自动识别，子目录项目可配置 `cwd`（如 `frontend`）。repsync 自身的前端预览默认自动分配端口：首选 5178，被开发模式的 MyGo/Vite 或其他进程占用时自动选择后续空闲端口，启动命令与页面地址使用同一个实际端口。

启动前核对配置版本与端口；能够自动接入的服务在占用时选择空闲端口，命令、环境变量与页面地址一起使用分配结果。显式 `fixedPort: true` 的服务保持固定端口，冲突时不执行命令；可按需通过本机 lsof 查看监听进程名称与 PID。停止仅接受本应用的服务运行编号，不接受外部 PID；重启需先停止，旧视图不能停止新实例。运行期间配置改变时保留原命令与停止入口，停止后核对新配置再启动；配置损坏或删除不会丢失正在运行服务的停止入口。

每项目最多配置 8 个服务，应用最多同时运行 16 个。服务不设置 CI 超时，不设 `CI=true`，沿用启动应用的 PATH 和环境，不提供交互式终端。停止或正常退出时先发送 SIGTERM，最多等待约 2 秒，再清理普通进程组；主动脱离进程组的后台服务不在此管理范围。崩溃后不自动接管遗留进程或重新启动，记录标为中断，需先检查端口。

最新一次状态与最近 256 KiB 日志保存到用户配置目录 `repsync/services/<项目路径哈希>/`，重新启动同一服务会替换其上次日志；长时间运行保留最新输出，不无限增长。使用临时进程的专项检查：`go test -tags=integration -race ./internal/workspace -run TestLocalService`。

## AI 控制与多项目启动

CLI 与桌面应用共用同一个服务实例、登记表、端口检查和运行日志；桌面应用持有进程，CLI 退出后服务继续运行。使用 `repsync open` 启动控制端，已经运行时复用。项目内开发构建：

```sh
go build -o bin/repsync ./cmd/repsync
./bin/repsync --help
# 开发 CLI 不含桌面资源。先打包，再指定桌面程序；已运行开发桌面时直接复用。
go tool mygo build
export REPSYNC_APP="/absolute/path/repsync/build/darwin-arm64/repsync.app/Contents/MacOS/repsync"
./bin/repsync open
./bin/repsync projects
./bin/repsync add /path/project-a
./bin/repsync add /path/project-b
./bin/repsync start --dry-run /path/project-a /path/project-b
./bin/repsync start --wait --timeout 30s /path/project-a /path/project-b
./bin/repsync status /path/project-a /path/project-b
```

可将 `bin/repsync` 或打包应用内的 `Contents/MacOS/repsync` 链接到已在 PATH 中的用户命令目录供其他项目的 AI 调用；已有同名命令应先核对归属。打包程序的 `open` 可直接启动自身，开发 CLI 使用 `REPSYNC_APP` 指定打包程序绝对路径；开发窗口继续使用 `go tool mygo dev`。启动等待最长 15 秒，失败返回启动日志路径；不在每个操作中隐式打开应用。

路径支持绝对或相对路径，开始前必须已登记；不根据名称猜测目录。`start` 启动所选项目声明的全部服务，一次最多 32 个项目，应用仍最多同时管理 16 个服务；进程启动后同时运行。CLI 统一输出 JSON：`ok`、`data` 和失败时的 `error`。退出码为 0（成功）、1（操作失败）、2（参数错误）；帮助文本除外。

整组先检查配置、可执行文件、数量和端口；任一预检失败均不启动新服务。端口在预检期间暂时保留，先为固定端口的服务分配，再安排自动端口的服务。重复请求会复用配置一致的已有运行实例，返回 `already_running`；配置已变化或正在停止时拒绝替换。

每项返回 `path`、`serviceId`、`status`、`state`、`portChecked`；`state` 含实际端口、页面 URL、PID 与 `runId`。`portChecked: false` 表示没有声明端口，该项无法保证端口无冲突，应由 AI 补齐网络服务的端口配置。`started` 仅表示进程已启动；用 `status` 检查端口是否可连接。`start --wait` 轮询本次回执的运行编号，直到端口可连接（`ready`）；没有声明端口时返回 `running_unchecked`，不能推定已就绪。等待超时、连接中断或服务退出返回失败和已有回执，保留已启动任务。默认等待 30 秒，`--timeout` 支持大于 0、最多 2h 的时长；`--dry-run` 不能与 `--wait` 同用，所有选项放在路径之前。TCP 可连接不代表业务健康检查通过，业务验证仍由项目负责。

```sh
./bin/repsync logs /path/project-a web RUN_ID
./bin/repsync stop /path/project-a web RUN_ID
./bin/repsync stop-projects /path/project-a /path/project-b
```

`stop-projects` 必须显式指定项目，先核对所有登记，再同时取消这些项目当前所属服务并等待停止；不停止流水线，不要求配置仍有效，不影响未选择项目和外部进程。返回每个捕获实例的运行编号，重复调用没有活动实例时返回空列表。

日志及单服务停止使用启动结果中的服务 id 和 `runId`，旧运行编号不能停止后来启动的实例，也不能用外部 PID 停止其他进程。CLI 退出不停止已启动服务，桌面应用正常退出会清理所属进程。实际启动过程中仍可能失败，结果逐项区分 `started`、`failed`、`not_started`；已启动服务保留，AI 根据回执继续处理。

控制使用用户配置目录内权限为 0600 的 Unix socket（macOS：`~/Library/Application Support/repsync/control.sock`），不开放网络监听端口；`REPSYNC_SOCKET` 可在桌面与 CLI 两端指定同一个绝对路径。文件锁保证单个控制入口只有一个桌面实例，重新打开时清理遗留 socket，保留非 socket 文件。MCP 尚未接入；后续可复用同一控制能力。

### 项目登记维护

项目标题旁的「管理」可更换目录或移除登记；即使旧目录失效，入口仍可使用。CLI 使用同一操作：

```sh
repsync relocate /old/project /new/project
repsync remove /new/project
```

更换目录仅修改登记，不搬动文件；新目录须可访问且没有重复登记。移除保留实际目录、日志和备份。服务、流水线或规则检查仍在运行时拒绝变更，先停止或等待完成；旧预览失效。历史仍按原路径保存，重新登记原路径后可以访问，不把旧备份自动应用到新目录。桌面定期刷新登记，并在窗口重新获得焦点时刷新，能够反映 CLI 的登记操作。

### AI 执行检查与规范更新

先读取已有流水线定义及 `version`，再明确选择入口执行。`run` 不猜测或自动选择 CD；版本不一致时拒绝运行：

```sh
repsync pipelines /path/project
repsync run --wait --timeout 10m /path/project CHECK_ID CONFIG_VERSION
repsync runs /path/project
repsync pipeline-logs /path/project RUN_ID
repsync pipeline-stop /path/project RUN_ID
```

不加 `--wait` 时仅表示已接受运行；加上后返回最终状态及日志，非成功结果退出码为 1。等待超时或中断保留任务，流水线自身的配置超时继续生效。运行、停止、历史、规则门禁和并发限制与桌面共用；`check-rules PATH` 仍可独立检查规则。

规范更新复用桌面的预览、备份与过期保护：

```sh
repsync sync-source /absolute/path/COMMON.md
repsync sync-status
repsync sync-preview /path/project
repsync sync-apply PREVIEW_TOKEN
repsync sync-history /path/project
repsync sync-restore RECORD_ID
```

`sync-source` 不传路径时只读取来源。`policy-preview PATH` 预览内置规则派生的 AI 说明，同样用 `sync-apply TOKEN` 应用。预览不写项目，AI 须审查返回的 `diff` 后再应用；不自动提交或执行后续检查。`sync-preview` 还返回公共区块外会保留的 `projectContent`，供核对项目差异。

公共规范页直接列出各项目采用状态；`sync-status` 返回相同数据。状态来自实际文件与已有备份：`current`（与当前来源一致）、`outdated`（匹配已知旧版本）、`modified`（公共区块有本地改动）、`untracked`（区块存在但版本未知）、`not_adopted`（无公共区块）、`no_source` 和 `error`。无法读取或标记损坏不算已采用。项目内容是否存在单独标出，预览中可点击「保留的项目内容」核对；这些状态不会自动升级或覆盖文件。AGENTS.md 继续使用公共区块同步；其他文本配置和共享脚本可通过下方「共享文件资源」接入。

专项验收：`go test -tags=integration -race ./internal/control ./internal/workspace -run 'TestControl|TestProjectRemoval|TestProjectRegistrationChanges|TestSharedAdoption|TestServiceBatch'`。只使用临时登记、目录、进程和端口。

### 自动避让端口

项目服务默认对可识别的端口输入自动避让，适用于界面单服务启动与 CLI 多项目启动。已有命令的 `--port N` / `--port=N`、配置 `env.PORT` 会在内存中转换为端口模板；npm/pnpm/yarn/bun 的 `run` 入口若对应一条简单 Vite 开发命令，会自动追加端口覆盖与严格端口选项，也支持项目内的 `cwd` 和 `--prefix` 目录。人工表单保存的简单 `/bin/sh -c` 命令同样适用。原配置文件和项目脚本保持原文，分配结果通过状态、页面链接和 AI 回执返回。

复杂 Shell、其他框架的隐藏脚本或无法确认端口传入方式的命令，由 AI 按项目实际入口接入 `autoPort` 与 `{port}`；不会只改页面地址而让进程继续监听原端口。确实依赖固定端口时配置 `fixedPort: true`，它优先保留原命令，不得与 `autoPort: true` 同时使用。`autoPort: false` 不禁止可识别入口的自动适配。外部进程始终由其原有所有者管理。

以下以已有 Vite 启动脚本为例：

```json
{
  "services": [
    {
      "id": "web",
      "name": "前端开发",
      "command": ["npm", "run", "dev", "--", "--host", "127.0.0.1", "--port", "{port}", "--strictPort"],
      "port": 5173,
      "autoPort": true,
      "url": "http://127.0.0.1:{port}"
    }
  ]
}
```

`port` 为首选值；被占用时向后最多尝试 100 个候选端口。省略 `port` 时由系统选择空闲端口。`command` 和 `env` 的 `{port}` 在启动时替换；页面 `url` 如有填写，也必须包含 `{port}`，实际 URL 随启动结果返回。不同框架使用项目自己支持的参数或变量，应用不猜测，也不持久改写配置或其他项目的连接地址。AI 应使用回执中的实际地址配置联调。

占用检查复用本机 `lsof`，覆盖 IPv4、IPv6 和不同监听地址；缺少该工具或读取失败时拒绝将端口视为空闲。预检期间保留 IPv4/IPv6 回环端口，子进程绑定前释放。外部进程仍可能在交接瞬间抢占，服务也可能忽略配置的端口；不能将预检通过视为业务就绪。应使用框架的严格端口模式，并结合启动日志和状态核验。

专项验收（使用临时项目、所属测试进程及短路径测试 socket）：

```sh
go test -tags=integration -race ./internal/control ./internal/workspace -run 'TestControl|TestServiceBatch|TestLocalService|TestProjectConfig'
```

### 服务地址联动

服务可通过 `envFrom` 引用另一服务实际使用的 URL 或端口。由 AI 按应用已有环境变量接入，例如前端读取后端地址：

```json
{
  "services": [
    {
      "id": "api", "name": "后端",
      "command": ["python", "-m", "uvicorn", "app:app", "--port", "{port}"],
      "port": 8100, "autoPort": true,
      "url": "http://127.0.0.1:{port}"
    },
    {
      "id": "web", "name": "前端",
      "command": ["npm", "run", "dev", "--", "--port", "{port}", "--strictPort"],
      "port": 5173, "autoPort": true,
      "url": "http://127.0.0.1:{port}",
      "envFrom": { "VITE_API_URL": { "service": "api", "field": "url" } }
    }
  ]
}
```

`field` 默认 `url`，也支持 `port`；引用端口时返回十进制字符串。变量名称以目标项目实际读取的名称为准，不统一改写应用代码。`envFrom` 与 `env` 不能定义同名变量；PATH 和当前服务的 PORT 继续由该服务自身配置提供。

引用默认属于同一项目。跨项目时，在引用中加 `"project": "../backend"` 或已登记项目的绝对路径；相对路径基于当前项目根目录。只有已登记项目可以作为依赖。点击单个服务「启动」或执行 `start`，都会自动包含所需依赖服务，先做整组预检与端口分配，再按依赖顺序启动。循环、缺失服务、缺失端口或 URL 在预检时阻止新服务启动。依赖启动后，等待 TCP 端口可连接并核对监听进程属于该依赖的所属进程组，再启动消费者；整组依赖等待上限 20 秒。失败保留逐项回执与已启动实例。

实际地址只注入本次运行环境，不改项目文件。回执包含自动纳入的依赖项目；`stop-projects` 仍只停止明确选择项目的所属服务。外部独立重启后端后，已有前端进程不会热更新环境；再次 `start` 会拒绝复用地址过期的消费者，`doctor` 返回 `dependency_address_changed`。AI 应按已有运行编号停止消费者，再启动它以读取新地址。TCP 可连接不替代业务健康检查。

## AI 配置项目

项目接入与日常操作优先使用已有 AI 编程工具，操作边界以 [PRODUCT.md 第 4.8 节](../PRODUCT.md#48-ai-配置与通用接入) 为准。当前 AI 可按下列约定维护文件，并通过上方 CLI 同时启动和检查多个项目；共享文本配置和测试脚本可登记后逐项目预览更新。现有人工表单仍可使用。应用内自动调用 AI、共享包依赖安装和跨项目批量升级尚未实现。

1. 读取目标项目的 AGENTS.md、README 及其指向的当前指南，确认已有启动、检查和发布命令，以及项目差异。读取已有配置后再修改。
2. 保留已能自动识别的入口。需要补充时，按「项目 CI/CD」「本地服务」和「编辑项目运行配置」约定维护目标项目根目录 `.repsync.json`；使用已有脚本、工作目录和本机环境，保留其他配置字段。网络开发服务优先使用运行层自动适配的端口参数、环境变量或 Vite 入口；不能自动适配时，确认项目支持的端口输入，再接入 `autoPort`，将实际监听端口与页面地址一起绑定到 `{port}`。确实不能切换的服务明确设置 `fixedPort: true`。只写确认存在的入口，缺少依据时报告缺少的信息。
3. 复用公共规则时，按下方「公共规范同步」流程接入来源，保留项目区块。测试相关配置和工具优先使用目标项目已有共享包或脚本，并保留其测试及发布门禁；需要复用独立文本文件时使用「共享文件资源」，项目特有内容通过原有配置或包装脚本保留。
4. 运行 `repsync doctor PATH` 做不执行命令的接入检查；它返回工具可用性、端口适配、服务引用和共享资源状态。有阻塞项时退出码为 1 并保留完整诊断；警告项不阻止后续操作。`ready` 表示配置与已检查工具可用，不代表依赖安装完整、服务健康或业务验收通过。先核对配置和受影响文件，再按目标项目允许的专项入口及本次授权验证。配置完成、服务启动、检查通过和发布成功分别报告；配置接入不自动执行任务。

用户可直接对当前 AI 说：“把这个项目接入 repsync，读取项目指南，复用已有启动和检查命令，保留项目差异。”涉及其他真实仓库时，以用户指定的目标和授权范围为准。

日常也可以直接指挥 AI，例如“启动已登记的 A、B 项目，等待端口可连接并报告实际地址”“读取 A 项目的服务日志，定位启动失败原因”或“检查 A 项目的收尾状态，列出 stash 和尚未核验的交付阶段”。AI 按上方 CLI 入口操作，诊断与后续修复分别依据授权处理，执行结果可在应用中查看；无需再手工填写同一份状态。

## AI 资源与使用证据

全局侧栏与项目页签提供「AI 资源」，按「资源 / 使用记录」查看。资源可按名称、来源及适配理由搜索。「筛选」可调整资源类型和项目适配范围。项目默认显示推荐与已采用的项，列表优先显示需要处理的状态；全部资源中保留尚未关联及技术栈候选，不把无关 CLI 尚未安装当作项目故障。点击资源行查看来源、当前位置的内容版本、任务条件、采用项目和可用性。资源多时分页，不把文件存在或配置启用显示成当前 AI 会话已加载。

项目采用主要由 AI 读取项目指南后维护 `.repsync.json` 的 `aiResources`，保留其他配置字段。类型为 `skill`、`mcp`、`cli`；`name` 对应 Skill 元数据名称、Codex MCP 服务器名称或不含参数的 CLI 名称。`reason` 解释适配依据，`tasks` 描述触发条件；同名 Skill 用可选的绝对 `source` 文件路径明确选择来源。最多 128 项，保存声明不启用、安装或调用任何资源。

```json
{
  "aiResources": [
    {
      "kind": "skill",
      "name": "golang-how-to",
      "reason": "项目指南要求 Go 工作使用对应开发技能",
      "tasks": "编写、检查或排查 Go 代码时使用"
    },
    {
      "kind": "cli",
      "name": "gh",
      "reason": "项目远程检查使用 GitHub Actions",
      "tasks": "读取远程运行及 PR 状态"
    }
  ]
}
```

盘点读取用户及项目的 `.agents/skills`、`.codex/skills`、`.claude/skills`，以及 Codex 插件缓存中的 Skill 文件；`CODEX_HOME` 如已设置用于定位 Codex 目录。插件缓存可能包含未启用或旧版资源，文件发现不证明已安装或可用于当前会话。符号链接按实际来源去重，同名不同来源保留。只解析常用 name/description 前置元数据，不作为完整 YAML 校验器。单文件上限 256 KiB，Skill 扫描限制 600 项、8000 个目录项和约 4 秒；全局最多读取前 64 个登记项目。达到限制、配置无效和读取失败明确显示缺口。

MCP 当前通过本机 `codex mcp list --json` 读取 CLI 配置，约 6 秒超时；不启动服务器、不连接远端、不展示令牌、环境变量或请求参数。CLI 配置与桌面插件或已有会话可能不同，连接、认证及模型可见性保持未核验。CLI 只按 RepSync 的执行环境定位常用命令及项目显式声明的命令，不运行版本探测或项目命令，不推定 AI 进程拥有相同 PATH。`doctor` 只检查项目显式声明及已知文件/命令前提，不执行 MCP 配置读取命令或扫描会话。

「使用记录」需点击读取，按最近 30 天展示本地证据，不创建后台任务，也不复制或持久化完整聊天。当前支持 Codex `sessions` JSONL 中的 `event_msg/item_completed`：命令事件读取 cwd、状态和退出结果；MCP 事件读取服务器、工具、状态及 isError。以会话/事件标识去重，真实重试另计；拒绝单列不计调用请求数，取消不推定是否执行。记录缺少结果、目录或支持字段时保持未知或报告缺口，不靠自然语言完成说明推断。

Skill 仅识别成功的字面量 `cat …/SKILL.md` 和 `rtk read …/SKILL.md` 读取命令；支持安全识别的引号与 `&&` 顺序，不执行日志中的命令，不推断管道、重定向、变量展开或任意脚本。重复读取主要按不同会话数汇总，正文完整性、注入事件、当时版本及要求落实未核验。CLI 的探测操作单列；记录有绝对路径时按来源区分，只有名称时保留独立的历史分组，不与当前安装来源合并。复合 Shell 命令仅确认入口，不猜测内部工具。

点击使用记录可查看会话、事件编号、记录目录及源文件行号，返回内容不含聊天正文、命令参数或工具结果正文。项目视图按记录 cwd 的目录边界筛选，工作树分别归属，不凭相同仓库推定同一项目；全局可查看尚未登记目录的原始事件上下文，不自动登记目录。仅支持本地已有记录，归档、云端及其他 AI 工具未接入；文件读取可能与当前会话写入并发。扫描限制为最近修改的 256 个文件、8000 个目录项、64 MiB 总读取量、单文件 16 MiB、单行 2 MiB、约 6 秒和 4000 条返回事件；资源列表及使用证据各保留约 3 MiB 返回预算，超过时截断并提示，以适应控制端响应限制。覆盖范围、记录版本、文件及关联会话数量、读取失败和超限均随回执返回，未知不显示成零使用。

RepSync 自己执行的流水线另列为应用入口回执，每项目读取最近 20 条；不归因于 AI、不推定内部 CLI 或业务验收结果。服务历史、其他外部工具、实时采集与跨来源关联尚未接通。浏览器预览仅用 `frontend/src/ai-preview.ts` 的隔离数据，不读取本机资源或会话。

CLI 复用桌面同一个服务实例；省略路径查看全局，指定路径须已登记：

```sh
repsync ai-resources [PATH]
repsync ai-usage [PATH]
```

专项验收：`go test -tags=integration -race ./internal/workspace ./internal/control -run 'TestAIResources|TestAIUsage|TestDoctor|TestControlAI'`，使用临时资源、项目与 JSONL，验证来源区分、符号链接去重、项目筛选、读取失败、探测/拒绝/取消、记录去重、隐私投影和大小限制。

### 安装前适配与冲突评估

「AI 资源」支持先评估、后接入。将候选 Skill 的链接或仓库交给现有 AI，或在资源详情的「交给 AI 处理」中选择「评估适配与冲突」。AI 读取来源及项目指南，对比任务、项目规则、已有 Skill/MCP/CLI，写回报告；候选无需先安装，即可作为「未接入」资源出现在现有列表。全局与项目列表均保留候选，项目报告优先；筛选可查看「尚未接入」或「评估缺失或已过期」。现有资源也能重新评估。

结论分建议接入、按条件接入、不建议接入、信息不足。详情先显示一句话结论，再列适合任务、独有收益、接入成本与撤回方式、已检查范围和缺口。发现项分别标为功能重复、明确冲突、前提不足、待确认；每条包含具体依据、处理建议和来源。明确冲突需要双方要求的不同引用；存在未解决冲突、前提或未知项时不能无条件推荐。未发现具体问题仅针对报告声明的范围，不代表完全兼容。保存评估不安装、不启用资源，也不修改项目规则。

AI 读取清单的 `contextVersion` 后，用同一范围写回，省略 PATH 为全局：

```sh
repsync ai-resources [PATH]
repsync ai-resource-assess [PATH] /absolute/path/assessment.json
```

候选报告格式：

```json
{
  "resourceId": "",
  "resourceRevision": "",
  "contextVersion": "从当前清单 data.contextVersion 复制",
  "expectedVersion": "新报告为空；已有报告取对应 assessment.version",
  "candidate": {
    "name": "release-helper",
    "source": "https://example.invalid/release-helper/SKILL.md",
    "sourceVersion": "已检查的版本、提交或内容摘要",
    "description": "候选用途"
  },
  "verdict": "conditional",
  "summary": "建议按条件接入：适合发布准备，但需保留项目验证门禁",
  "fit": "Markdown：适合哪些任务、独有收益及已有能力覆盖",
  "adoptionCost": "Markdown：准备条件、配置改动、维护方和撤回方式",
  "coverage": "Markdown：实际检查的任务、文件、项目和资源，及尚未覆盖的范围",
  "findings": [
    {
      "kind": "conflict",
      "resource": "项目验证规则",
      "evidence": "候选要求每次完整 CI，项目规则允许日常改动采用针对性验证",
      "recommendation": "限定发布任务触发，并遵循项目门禁",
      "sources": ["候选 SKILL.md 中的要求及位置", "项目 AGENTS.md 中的要求及位置"]
    }
  ],
  "sources": ["实际读取的原始来源"],
  "checkedAt": "2026-11-12T00:00:00Z"
}
```

`verdict` 为 `recommended`、`conditional`、`not_recommended`、`unknown`；`findings[].kind` 为 `overlap`、`conflict`、`prerequisite`、`unknown`。没有具体发现时使用空数组。来源不可读或未核对候选版本时保留 `unknown`。说明使用中文，保留原始来源及实际检查时间，不编造兼容性。

未安装候选填写 `candidate`，`source` 必须是无凭据的 HTTP(S) URL 或绝对本地来源路径；ID 按来源生成，快照按候选说明生成，不以名称判断同一资源。`sourceVersion` 是 AI 核对的来源版本，不是应用实时检查结果。评估现有资源时省略 `candidate`，从清单复制 `resourceId` 和 `resourceRevision`。候选经另外要求的安装流程接入后，AI 比对实际来源，再在候选中填写 `installedResourceId` 为确切的已盘点 Skill ID，并提供该安装项的当前 `resourceRevision`；同名不会自动关联。有效关联后报告出现在安装项详情，候选不重复显示。目标未盘点到时保留候选和过期提示，不宣称已安装。

写回同时校验项目上下文及已有报告版本。上下文包括本次盘点资源的身份/快照，以及登记项目根目录 `AGENTS.md`、`README.md`、`PRODUCT.md`、`DESIGN.md`、`.repsync.json`、`.codex/config.toml`、`package.json`、`go.mod`，和 Codex 用户目录的 `AGENTS.md`、`config.toml`；只保留内容摘要，不返回配置正文。采用已有 256 KiB 文件读取限制，缺失/读取错误也进入快照。全局最多 64 个项目，沿用清单扫描限制；未被盘点的工具、外链/被引用文档、其他 AI 工具配置、远端来源变化、CLI 二进制及聊天目标变化需要 AI 重新核对。报告的 coverage 应明确这些缺口，不把有界快照当作完整环境检查。

项目规则或已盘点资源变化后，报告保留旧结论并提示重新评估。全局报告在项目中只作参考，始终需要按本项目重新评估；项目写回不会覆盖全局。报告存于本地数据目录 `ai-resource-assessments/`，每范围最多 128 条、整个文件最多 1 MiB；单份最多 64 KiB、32 条发现、根来源及每条发现来源各最多 16 个，每个引用最多 2048 字节。读取失败单列缺口，不转成兼容或无问题。

专项验收复用 `go test -tags=integration -race ./internal/workspace ./internal/control -run 'TestAIResource|TestControlAI'`，覆盖未安装候选、分类与双方依据、项目/资源快照、并发保护、全局与项目范围、明确安装关联、来源损坏及 CLI 写回读取。

### 资源更新与说明书

资源详情集中显示更新状态、当前与上游版本、新能力摘要、中文说明书、AI 检查时间及原始来源。列表显示已有报告的状态；筛选中可选择「有可用更新」或「说明缺失或已过期」。未检查、版本未知、检查失败及报告过期分别呈现。「重新扫描」只盘点本地资源，不检查上游或调用 AI。

点击全局或资源详情的「交给 AI 处理」，可复制检查更新并补齐说明、执行更新并刷新说明、仅编写使用说明三类指令，交给已有 AI 聊天执行。完成后重新扫描查看结果。检查模式不升级；更新模式沿用资源已有安装渠道，插件内 Skill 随所属插件维护，保留本地修改。MCP 区分本地程序升级与远程服务工具变化；没有版本或上游证据时保留未知。应用本身不调用模型、不执行升级命令、不创建定期检查任务。

AI 先读取当前范围的清单，再逐项写回；全局省略 PATH，项目范围使用已登记目录：

```sh
repsync ai-resources [PATH]
repsync ai-resource-record [PATH] /absolute/path/report.json
```

JSON 字段约定：

```json
{
  "resourceId": "从清单复制 resource.id",
  "resourceRevision": "从清单复制 resource.revision",
  "expectedVersion": "从清单复制 resource.note.version；没有 note 时为空字符串",
  "installedVersion": "已核对的安装版本；未知时为空字符串",
  "latestVersion": "已核对的上游版本；未知时为空字符串",
  "updateStatus": "unknown",
  "checkedAt": "2026-11-12T00:00:00Z",
  "summary": "一句话说明更新结论及对相关项目的用途",
  "changes": "Markdown：新增、改变、升级后才能使用的能力及配置要求",
  "manual": "Markdown：能做什么、何时使用、准备条件、可直接对 AI 说的话、结果与限制",
  "sources": ["原仓库发布记录 URL", "本地 SKILL.md 或实际引用的文档路径"]
}
```

`updateStatus` 仅允许 `current`（检查时为最新）、`available`（有更新）、`unknown`（无法判断）和 `failed`（检查失败）。前两种必须提供安装版本、上游版本及来源；`checkedAt` 是实际检查时间，使用 RFC3339，不以保存说明的时间冒充上游检查时间。仅补说明时，只有原报告资源快照仍匹配才能保留原更新结论与检查时间；否则标为未知并说明未检查上游。说明书描述当前安装版，新版专有能力单列。报告属于附来源的 AI 整理结果，不视为独立核验或升级执行回执。资源安装、配置启用、会话可见及调用成功继续分别核验。

报告保存在 repsync 本地数据目录的 `ai-resource-notes/`，按范围与资源 ID 区分；不修改 Skill 源码、MCP 配置或项目仓库。单份输入最多 64 KiB、最多 16 个来源，每个来源最多 2048 字节；资源读取沿用清单总预算。写回前重新盘点并核对 `resourceRevision`，已有报告的 `expectedVersion` 不匹配时拒绝覆盖。全局报告可供项目引用，项目报告优先且不覆盖全局；同名不同来源 Skill 不混用。同名 MCP 仅在配置快照一致时继承全局报告。

当前自动过期判断覆盖 Skill 的 `SKILL.md` 正文和 Codex 返回的 MCP 配置；不是完整目录版本检查。附属文档、CLI 二进制、远程 MCP 的实际工具列表和版本变化需 AI 重新核对。报告保留检查时间，不将旧报告显示为实时状态；本地变更后保留旧内容并提示刷新。没有记录或读取失败不显示成最新。浏览器只读取隔离预览数据，复制指令不触发本机更新。

专项验收：`go test -tags=integration -race ./internal/workspace ./internal/control -run 'TestAIResource|TestControlAI'`，覆盖写回/读取、持久化、同名隔离、范围继承、旧快照与并发覆盖保护，以及失败和损坏记录。

### 调整 AI 资源

在全局「AI 资源」点击已有 MCP，可预览并调整用户默认启停。项目内点击同一资源，可选择继承默认、启用或停用。项目例外只保存 `enabled`，沿用用户级连接参数。「添加项目 MCP」用于添加项目专用 HTTP 或 stdio 服务器。

在项目资源列表点击已发现的 Skill，可预览并链接到 `.agents/skills/<name>`。链接跟随来源，撤回前核对链接身份，来源目录保留。当前 MCP 配置由 Codex 读取，预览显示配置目标。

预览显示目标路径与新增或移除内容。应用前复核来源和目标，拒绝同名项目配置以及过期预览。MCP 使用 `.codex/config.toml` 的标记区块，撤回仅移除内容仍一致的区块，保留其他配置。HTTP 可填写令牌环境变量名称。配置由 Codex 在信任项目后读取，启停例外和新增配置可在新会话核验。

`go-toml/v2` 由 repsync 维护，用于检查 Codex 配置及同名表，写入仅追加或移除所属区块。

```sh
repsync codex-resources PATH
repsync codex-resource-preview PATH JSON_FILE
repsync codex-resource-apply PATH VERSION JSON_FILE
```

输入文件示例为 `{"kind":"mcp","name":"docs","action":"add","url":"https://example.com/mcp"}`。stdio 使用 `command` 和 `args`，HTTP 可加 `tokenEnv`。Skill 使用 `{"kind":"skill","name":"doc-audit","action":"add","source":"/absolute/path/SKILL.md"}`。用户级 MCP 项目例外用 `enable` 或 `disable`，撤回用 `remove`。应用时传入预览返回的 `version` 和同一份输入文件。

## 共享文件资源

当前已接入 `go-format-check`：RepSync 维护权威来源 `shared/testing/check-go-format.sh`，RepSync 与英语项目都采用到 `tools/shared/check-go-format.sh`。RepSync 的本地 CI 检查 `cmd internal`，英语项目的既有 `scripts/check-local-ci.sh` 检查 `apps/api`；各项目继续维护自己的检查目录和其他门禁。脚本只检查、不格式化文件，要求本机已有 Go，无额外依赖。

修改该工具时只编辑公共来源，再按下方流程分别预览和更新采用项目，运行各项目的格式检查命令。采用副本随项目保留，运行检查不依赖 RepSync 正在启动；公共来源位置变更后需更新本机资源登记。更新、备份和受保护恢复由 RepSync 负责，项目差异保留在调用参数中。

AI 可登记可复用的测试脚本、检查配置等独立 UTF-8 文本文件。文件资源沿用现有差异预览、短期令牌、备份和受保护恢复，不引入依赖安装或配置合并语言。资源定义保存在用户配置目录的 `repsync/sync/resources.json`，当前上限 64 项：

```sh
repsync resource-add test-helper /absolute/path/shared/test-helper.sh tools/test-helper.sh
repsync resources
```

在采用项目的 `.repsync.json` 中维护 `"resources": ["test-helper"]`，保留其他字段。目标父目录须已存在，由 AI 接入时创建；目标为项目内相对路径，拒绝符号链接、`.git`、`AGENTS.md` 和 `.repsync.json`。AGENTS.md 仍走区块同步，项目自身配置保留在原有文件中，按各工具已有机制引用共享文件。

```sh
repsync resource-status /path/project
repsync resource-preview /path/project test-helper
repsync sync-apply PREVIEW_TOKEN
repsync resource-history /path/project
repsync sync-restore RECORD_ID
repsync resource-remove test-helper
```

桌面全局「公共规范 → 共享文件」登记或编辑公共来源，并查看内容版本、采用项目及各项目状态；项目「规范 → 共享文件」固定当前项目。人工登记提供标识、来源文件的绝对路径与项目内目标位置，AI 接入仍优先复用已有共享包或脚本。

点击「采用此资源」只更新该项目 `.repsync.json` 的 `resources`，保留其他配置和权限；随后单独「预览差异」，在审查窗口中「应用更新」才写入共享文件。采用前要求目标父目录已准备好，拒绝同一项目内多个资源写入同一目标。修改公共登记或采用配置时核对读取版本，过期操作需重新读取。公共来源变化后，各采用项目分别显示可更新状态，不自动批量覆盖。

每个项目的「更新记录」列出其共享文件更新，可恢复更新前内容。取消采用保留项目文件和历史；移除公共登记也保留已采用文件和历史，仍有采用声明的项目显示来源缺失，可重新登记或取消采用。恢复原先不存在的文件会将其移除，后续有改动时拒绝恢复。浏览器使用隔离资源数据，可查看状态与差异，登记、采用、应用及恢复禁用。

来源、采用状态和目标差异也通过 CLI 返回，供 AI 操作。`resource-status` 区分 `current`、`outdated`、`missing`、`modified` 和 `error`。目标与当前来源一致，或仍与上次受管理快照一致时，可预览后更新；已有项目文件不同于来源、或共享文件有本地修改时，预览返回 `conflict: true`，应用会拒绝覆盖。AI 可调整为独立共享文件，再让项目配置引用它，不能用整个公共文件覆盖项目差异。

预览不写文件。应用再次核对来源内容、来源权限、采用登记和目标快照，备份后原子替换。已有目标权限保留，新文件沿用来源权限；单文件上限 256 KiB。恢复仅接受没有后续改动的更新结果，原先不存在的目标恢复为不存在。取消采用从项目的 `resources` 移除 id；`resource-remove` 仅移除公共登记，不删除已采用文件或历史。资源更新历史与 AGENTS.md 更新分别查询，恢复共用同一入口。

专项验收：`go test -tags=integration -race ./internal/workspace ./internal/control -run 'TestServiceLinks|TestResource|TestDoctor|TestControlResource|TestProjectConfigSaving|TestProjectConfigConcurrent'`。只使用临时项目、来源文件、端口与所属进程。

## 项目规则

「公共规范」中的公共项目规则提供默认值，项目「规范」→「项目规则」可单独覆盖或恢复继承。当前支持「禁止新增单元测试」和「React 禁用 useState，使用 useReducer」。规则初始均关闭，不自动改变已登记项目的政策。

执行强度为关闭、仅提示 AI、检查警告、检查失败。规则配置是唯一来源：代码检查直接读取配置，AI 说明从同一配置生成。启用后点击「检查规则」，结果按新增、历史和待判定展示路径、行列及原因；没有启用代码检查时不会显示为检查通过。规则改动保存后需重新检查。

公共默认保存在系统用户配置目录 `repsync/policies/defaults.json`，项目例外位于 `.repsync.json` 的 `policies` 数组。数组中没有对应 id 即继承默认，`level: "off"` 显式关闭；保存保留服务、流水线和其他配置。下面仅示意配置格式，实际路径应由 AI 按目标项目的测试约定填写：

```json
{
  "policies": [
    {
      "id": "no-unit-tests",
      "level": "error",
      "scope": "all",
      "include": ["**/*.unit.test.ts", "**/*_test.go"],
      "exclude": ["**/integration/**"],
      "excludeMarkers": ["//go:build integration"]
    },
    {
      "id": "react-no-use-state",
      "cwd": "frontend",
      "level": "error",
      "scope": "react",
      "include": ["frontend/src/**"],
      "exclude": [],
      "excludeMarkers": []
    }
  ]
}
```

`level` 为 `off` / `advice` / `warn` / `error`；`scope` 为 `all` 或 `react`，React 自动识别依据检查工作目录的 package.json 的 dependencies、devDependencies、peerDependencies。React 规则的可选 `cwd` 指定项目内已有的 ESLint 工作目录（例如 frontend），留空使用项目根目录；不依赖 package.json 识别时可明确使用 `all`，规则仍只识别来自 React 的 useState。路径模式相对登记目录，支持 `*`、`**`、`?`；例外标记按文件中的整行文本匹配。

单元测试规则只检查项目明确声明的路径，例外用于保留集成测试或其他允许的测试；不会仅凭文件名猜测测试语义。未声明 include 时，代码检查报告未完成。匹配路径中的新文件或新增行算本次违规，未增加内容的历史文件单独展示；这是路径约束，不分析任意语言的测试函数语义。

React 检查复用目标项目可解析的 Node.js、ESLint 9+ flat config 及其 JS/TS/TSX parser，通过内置 ESLint 规则检查 React 导入、命名空间、解构和别名引用；普通注释、字符串、同名局部变量不报错。不安装或改写目标项目的 ESLint 配置。项目 ESLint 忽略的文件列为跳过；解析器、依赖或配置缺失均报告未完成，不伪装成通过。该检查脚本随应用维护，不新增 npm 依赖；用户项目继续负责自己的 ESLint 及 parser 版本。

基线使用 Git HEAD，包含暂存、未暂存及未跟踪文件，遵守 Git 忽略规则；仅新增违规使 error 级别失败，历史违规继续显示。没有 HEAD 的项目标记待判定，error 级别的待判定违规也会使检查失败。单次检查上限 30 秒、5000 个文件、40 MiB 源码，单文件 256 KiB；超限明确报告。检查不生成、修改或删除项目源码。结果对应本次读取的工作区内容，后续代码或规则变化后需重新检查；应用退出会结束其拥有的规则检查进程。

repsync 本地 CI 在执行项目已有命令前运行规则检查，error 级别的新增违规或检查未完成会停止后续命令，warn 级别记录诊断后继续；CD 不自动接入该门禁。外部脚本可调用应用二进制的 `check-rules PATH` 子命令，无需桌面应用运行；输出 JSON，退出码 0 为允许继续、1 为规则失败或执行失败、2 为参数错误。已将应用二进制加入 PATH 时，例如：

```sh
repsync check-rules .
```

此命令读取当前用户的公共默认和目标项目例外。外部 CI 若需要与本机一致，应显式维护相同默认，或在项目中固定规则；应用不自动改写已有 CI、pre-push 或远程工作流。

「AI 规范说明」可预览并应用 AGENTS.md 的独立 `repsync:policies:start/end` 区块。此操作复用现有备份、恢复和过期预览保护，保留公共规范区块及项目原文；仅保存规则不会改写 AGENTS.md。规则或目标文件变化后需重新预览。关闭全部规则后，更新会移除对应规则区块。

专项验收：`go test -tags=integration -race ./internal/workspace -run TestPolicy`，使用临时仓库及已安装的前端 ESLint 验证 AST 检查、继承、项目例外、历史分类、规范预览恢复及 CI 拦截，不改动其他真实仓库。

## 编辑项目运行配置

配置主要由 AI 按上述接入约定维护；需要临时调整时，概览的「配置服务」及 CI/CD 本地页的「配置流水线」集中提供添加、编辑和移除入口，运行列表不再逐行显示编辑按钮。打开后默认选择第一条配置，也可选择其他条目或添加新项；切换前有未保存修改时提示是否放弃。表单直接显示名称、命令，以及工作目录、服务页面地址或流水线用途。标识自动生成，流水线默认作为检查运行、超时 30 分钟；环境变量及其他低频配置继续在 `.repsync.json` 中维护。保存不执行命令，浏览器表单仅供示例预览，不能保存。

命令按终端习惯填写，例如 `npm run dev`、`make ci`，支持参数、引号与 Shell 语法。新增或修改后的命令由 `/bin/sh` 执行；流水线启用 `-e`，命令失败时停止后续语句。编辑时若未改动命令文本，保留原来的参数数组、多步骤及既有环境配置。

两类配置均独立采用相同规则：字段未声明或为 `null` 时自动识别根目录 package.json 已有入口；显式空数组关闭该类自动识别。在界面中编辑或添加条目后保存该类明确配置；移除最后一条会保存空列表。需要恢复自动识别时从配置文件移除对应字段。配置另一类不会阻止自动识别。

服务及流水线均支持可选 `cwd` 与 `env`：

```json
{
  "services": [
    {
      "id": "web",
      "name": "前端开发",
      "cwd": "frontend",
      "env": { "NODE_ENV": "development" },
      "command": ["npm", "run", "dev"],
      "url": "http://127.0.0.1:5178"
    }
  ]
}
```

`cwd` 是项目内已存在的相对目录，留空使用项目根目录；拒绝绝对目录、越界路径和指向外部的符号链接。流水线的全部步骤使用该流水线的 `cwd` / `env`。`env` 覆盖继承环境中的同名变量，值原样传入，不展开 `$PATH`、`~` 等；配置 `PATH` 时应填写完整搜索路径，并用于查找可执行文件。相对可执行文件及相对 PATH 项均基于工作目录解析；可用 `.venv/bin/python` 等已有入口，不自动激活交互式 Shell 或版本管理器。运行器保留管理 PWD、CI、NO_COLOR 和 Git 工作树重定向变量，不接受这些变量的覆盖。

环境变量明文保存在项目 `.repsync.json`，运行记录也可能包含服务配置，请勿填写密码或令牌；敏感配置继续使用项目已有的本机环境或凭据工具。

保存只替换对应配置段，保留其他顶层字段、另一类配置、同 id 条目的扩展字段及已有文件权限；新文件权限为 0600。外部修改后旧表单保存会失败，可「重新载入」后重试；无效 JSON、符号链接配置及超过 256 KiB 的文件不写入。写入前再次核对内容和权限，再临时文件 + rename；对外部进程不提供文件系统层面的原子比较交换。

运行中的任务保留启动时配置；服务工作目录或环境变化会标记「配置已变化」，仍可停止原实例，停止后再使用新配置。专项验收：`go test -tags=integration -race ./internal/workspace -run 'TestProjectConfig|TestLocalPipeline|TestLocalService'`。

## 公共规范同步

1. 在桌面「公共规范」中输入公共规则 Markdown 文件的绝对路径，接入后查看真实内容与 SHA-256 内容版本。
2. 打开对应项目的「规范」→「公共规范」，可点击「查看公共规范原文」，也可直接预览当前项目的差异；也可在全局「公共规范」页选择一个已登记项目。首版只同步根目录 `AGENTS.md` 中 `repsync:shared:start` / `repsync:shared:end` 标记之间的公共区块；首次在原文前添加区块，区块外的项目内容逐字保留。`PRODUCT.md` 等文档不自动同步。
3. 确认差异后「应用更新」。服务端预览有效期为 15 分钟，最多保留 32 份；公共源、目标内容、存在状态或权限发生变化时需重新预览。通过校验后先保存备份，再以临时文件 + rename 更新目标文件。符号链接和异常区块标记会阻止更新。
4. 在项目「规范」→「公共规范」或全局公共规范页点击「更新记录与恢复」，恢复更新前内容。当前文件须与该次更新结果完全一致；有后续改动时停止恢复。原先不存在的文件会恢复为不存在，已有文件恢复原文和权限。

配置位于系统用户配置目录的 `repsync/sync/source.json`，备份和操作记录位于 `repsync/sync/history/`，不写入项目仓库。应用中断后按实际文件内容判断记录状态，重开应用仍可恢复。备份包含更新前后全文，请按用户数据保留；暂不自动清理。

当前仅提供单个本地规则源与逐项目手动更新，不执行项目脚本或 Git 提交，不自动应用或自动合并项目内容。不要同时通过多个 repsync 实例更新同一项目；对外部编辑器采用写入前重新读取检查，不提供文件系统层面的原子比较交换。

## 图标

应用保留原来的蓝色双斜线标志，唯一可编辑源文件为 `resources/mark.svg`。`npm --prefix frontend run icons` 生成桌面图标（`resources/icon.svg`、1024px `resources/icon.png`）、侧栏标志与 favicon；前端构建会运行同一入口，内容未变化时不会重写文件或触发桌面重启。

界面图标使用 `lucide-react` 的具名导入，由 `frontend/src/components.tsx` 统一尺寸和线宽；只打包实际使用的图标。`@resvg/resvg-js` 仅用于开发期把标志渲染成桌面 PNG，不进入应用运行时。图标生成脚本与依赖锁文件由本项目维护。
