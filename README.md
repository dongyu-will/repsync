<div align="center">
  <img src="frontend/public/brand.svg" width="72" alt="repsync" />
  <h1>repsync</h1>
  <p><a href="README.zh-CN.md">简体中文</a> · <a href="https://dongyu-will.github.io/repsync/">Live demo</a> · <a href="https://github.com/dongyu-will/repsync/releases">Download</a></p>
</div>

Manage local repositories, run existing development commands, and keep shared rules and AI resources in one desktop app. Built with **Go, MyGo WebView, React, TypeScript, and Tailwind CSS**.

![repsync workspace](docs/images/workspace.png)

## Features

- **Projects:** add local folders, clone repositories, and switch projects with ⌘K / Ctrl+K.
- **Services and pipelines:** manage development servers, ports, local CI/CD, logs, and GitHub Actions history.
- **Git workspace:** inspect worktrees, branches, and stashes; preview cleanup and restore previous entries.
- **Documents:** preview, edit, and search project Markdown files.
- **Project map:** browse features and locate their implementations and entries. See the [map format](docs/usage.md#project-map).
- **Shared resources:** maintain shared rules, reusable files, Skills, and Codex MCP configuration.
- **Project context:** keep sourced memories, check records, and task handoffs alongside your projects.

## Install

Download the [macOS Apple Silicon preview](https://github.com/dongyu-will/repsync/releases), open the DMG, and drag `repsync.app` to Applications. The preview is ad-hoc signed and not notarized.

From 0.1.2, the desktop app checks for signed updates shortly after launch and daily. Use **Settings → Check for updates** or the app menu to check now. Install from the update window; it replaces the same app after quitting and can relaunch it. Automatic downloads are optional in that window. Project registrations, backups, and history are preserved. Development builds never update themselves.

Versions before 0.1.2 need one manual replacement of `/Applications/repsync.app` to enable the updater. Keep one installed copy; archive installers rather than extracted apps. Releases must include the signed update manifest and archive. See [Desktop updates](docs/usage.md#desktop-updates) for release setup and limits.

Or build from source with **Go 1.27.4+**, **Node.js 22.13+** (Node 24 recommended), npm, and Git:

```sh
git clone https://github.com/dongyu-will/repsync.git
cd repsync
npm --prefix frontend ci
go mod download
go tool mygo dev
```

Add a project from the sidebar to get started. Project tabs cover **Project startup**, **Project standards**, **CI/CD**, **Branches and workspaces**, and **Todos and handoff**. **Project map** and project **AI resources** are grouped under **Utilities**, collapsed by default and opened only when selected. Documents and editing live in Project standards; command details live in Project startup. Shared updates and shared files are managed once from the sidebar's Shared rules. To package the app, run `go tool mygo build`; output goes to `build/darwin-arm64/`.

### Browser preview

```sh
npm --prefix frontend run dev
```

Open [localhost:5178](http://127.0.0.1:5178), or try the [online demo](https://dongyu-will.github.io/repsync/). The browser uses sample projects; local file and process operations use the desktop app.

## AI project configuration (AI 配置项目)

Use **Ask AI to configure** to copy the current project and task into your existing AI chat. The assistant reads project guides and maintains `.repsync.json` or uses the protected CLI operations. Technical setup forms have been removed; the app displays configuration and results. Copying instructions does not send or execute them. The desktop app and JSON CLI share the same local operations. See the [configuration and CLI guide](docs/usage.md) for examples.

Optional tools: `gh` for GitHub Actions, `rg` for document review, and Codex or Claude Code for session memory extraction.

## Development and validation

```sh
go tool mygo generate
sh tools/shared/check-go-format.sh cmd internal
npm --prefix frontend run check
go vet ./...
go test -tags=integration -race ./internal/workspace ./internal/control
npm --prefix frontend run build
go tool mygo build
```

Format changed files with the project’s Prettier or `gofmt`. Regenerate `frontend/src/mygo.ts` through MyGo. For interface changes, check 760×500, 960×640, and 1440×900, including keyboard navigation and error states.

## Documentation

- [Usage, configuration, and CLI](docs/usage.md)
- [Detailed Chinese guide](docs/guide.zh-CN.md)
- [Contributing](CONTRIBUTING.md) · [Security](SECURITY.md) · [Changelog](CHANGELOG.md)
- [Product decisions](PRODUCT.md) · [Interface conventions](DESIGN.md) · [Agent instructions](AGENTS.md)

## License

[MIT](LICENSE). See [third-party notices](THIRD_PARTY_NOTICES.md) for dependency licenses.
