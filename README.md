<div align="center">
  <img src="frontend/public/brand.svg" width="72" alt="repsync" />
  <h1>repsync</h1>
  <p>A local workspace for your projects, development services, and AI tools.</p>
  <p><a href="README.zh-CN.md">简体中文</a> · <a href="https://dongyu-will.github.io/repsync/">Live demo</a> · <a href="https://github.com/dongyu-will/repsync/releases">Download</a></p>
</div>

Manage local repositories, run existing development commands, and keep shared rules and AI resources in one desktop app. Built with **Go, MyGo WebView, React, TypeScript, and Tailwind CSS**.

![repsync workspace](docs/images/workspace.png)

## Features

- **Projects:** add local folders, clone repositories, and switch projects with ⌘K / Ctrl+K.
- **Services and pipelines:** manage development servers, ports, local CI/CD, logs, and GitHub Actions history.
- **Git workspace:** inspect worktrees, branches, and stashes; preview cleanup and restore previous entries.
- **Documents:** preview, edit, and search project Markdown files.
- **Shared resources:** maintain shared rules, reusable files, Skills, and Codex MCP configuration.
- **Project context:** keep sourced memories, check records, and task handoffs alongside your projects.

## Install

Download the [macOS Apple Silicon preview](https://github.com/dongyu-will/repsync/releases), extract the ZIP, and move `repsync.app` to Applications. The preview is ad-hoc signed and not notarized.

Or build from source with **Go 1.27.3+**, **Node.js 22.13+** (Node 24 recommended), npm, and Git:

```sh
git clone https://github.com/dongyu-will/repsync.git
cd repsync
npm --prefix frontend ci
go mod download
go tool mygo dev
```

Add a project from the sidebar to get started. To package the app, run `go tool mygo build`; output goes to `build/darwin-arm64/`.

### Browser preview

```sh
npm --prefix frontend run dev
```

Open [localhost:5178](http://127.0.0.1:5178), or try the [online demo](https://dongyu-will.github.io/repsync/). The browser uses sample projects; local file and process operations use the desktop app.

## AI project configuration (AI 配置项目)

Ask your AI assistant to connect a project's existing commands through `.repsync.json`. The desktop app and JSON CLI share the same local operations. See the [configuration and CLI guide](docs/usage.md) for examples.

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
