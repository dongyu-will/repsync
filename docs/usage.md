# Using repsync

See the [README](../README.md) for installation, development, and validation commands.

## Navigation

The app opens a specific project directly. **Projects** in the sidebar expands or collapses the project list; select a project or use search to switch. There is no separate projects overview.

Projects have five main tabs: **Project startup**, **Project standards**, **CI/CD**, **Branches and workspaces**, and **Todos and handoff**. They remain directly visible and wrap in narrow windows. Expand **Utilities** to open **Project map** or project **AI resources**; expanding the group does not load either page. Selecting a main tab or switching projects collapses Utilities. Collapsing Utilities while viewing a utility page returns to Project startup. Documents are part of Project standards; startup command details are part of Project startup. Shared updates and shared files are managed once from the sidebar's **Shared rules**. Saved Documents preferences open Project standards, and saved Startup configuration preferences open Project startup. Setup still uses **Ask AI to configure**.

By default, startup restores the last project and its remembered main tab. Utilities start collapsed, are excluded from the default-page options, and never replace the remembered main tab. Older saved utility pages fall back to the configured main default page. Disable **Reopen last project** in Settings to open the first project in the sidebar instead.

## Project standards

**Project standards → Guidelines and documents** lists all root `.md` and `.markdown` files. Guideline names AGENTS, CLAUDE, GEMINI, CONTRIBUTING, RULES, STYLEGUIDE, DEVELOPMENT, DESIGN, and CODE_OF_CONDUCT are prioritized, with AGENTS first. Select a file to read saved content, search it, or refresh after external edits. **Edit document** restores any session draft in the same reader; save with the existing protected operation, or choose **Back to saved file** to read saved content while retaining the draft. Empty files, missing files, and read errors have separate states.

The reader covers the project root; it does not expand inherited parent instructions, nested directory rules, or non-Markdown tool configuration, and does not claim to resolve all effective rules. **Project checks** discovers existing tool configurations and check commands; see [Project checks](#project-checks). The sidebar's **Shared rules** manages common sources, project adoption, diff preview, apply, recovery, and shared files; those views are not duplicated inside each project.

## Project checks

**Project standards → Project checks** discovers the project's existing configuration sources and runnable commands. It has no built-in code restrictions. Rules remain in project-owned tools such as ESLint, TypeScript, Ruff, golangci-lint, or other existing checkers. Finding a configuration does not prove a command uses it, establish effective inherited rules, or mean a check has passed. Written guidelines are listed separately; their automatic coverage remains unverified until reviewed against those tools.

Discovery reads root and nested package.json scripts, Make/GNUmakefile targets, explicit CI pipelines, and Moon tasks declared as `test`. Package and Make command names begin with `ci`, `check`, `lint`, `typecheck`, `test`, `verify`, or `validate`. Known fix/write/watch/update/release variants are excluded. Script and target names are conventions for finding candidates, not a guarantee that arbitrary project commands are read-only; inspect the actual command and project guide before running it. Moon task mappings and inheritance come from the installed `moon query tasks --type test` CLI. Discovery does not evaluate ESLint JavaScript, source shell files, run Make, install tools, or generate commands from prose/configuration alone.

Each command retains its source and working directory. Nested packages inherit the root package-manager selection unless they declare their own. Explicit CI pipeline environment and timeout settings are retained. Configurations include ESLint, TypeScript, Biome, Prettier, Ruff, Python tool tables, Go modules, golangci-lint, mypy, pytest, tox, EditorConfig, and SwiftLint. These identify configuration sources; dynamic imports, external shared configs, and rules mentioned only in prose are not resolved as effective rules by repsync.

Git projects use tracked and non-ignored untracked file names. Other folders scan at most six directory levels and 12,000 filesystem entries, excluding dependency/build directories. Individual files retain the existing 256 KiB read limit; Moon output is bounded to 1 MiB and five seconds. At most 128 commands are listed, with explicit CI pipelines and Moon tasks first. Missing tools, invalid sources, scan limits, and retired `policies` declarations are reported. Discovery never writes project files or removes existing policies, generated guideline blocks, backups, or history.

Running a selected command uses the existing local pipeline runner, with configuration-version checks, project concurrency protection, timeout, process-group stop, exit code, bounded logs, and history. The command's tools decide whether it passes. No extra repsync rule gate is prepended to CI or CD. History shows the last execution; it does not certify later source changes. The browser preview is isolated and disables execution.

```sh
repsync checks /absolute/path/to/project
repsync check --wait --timeout 5m /absolute/path/to/project CHECK_ID VERSION
repsync pipeline-logs /absolute/path/to/project RUN_ID
repsync pipeline-stop /absolute/path/to/project RUN_ID
```

The IDs and version come from `checks`; `check --wait` returns the real command result and exits nonzero on failure. Unknown naming conventions can be connected through existing project scripts or explicit CI pipelines; configuration remains with the project and its existing AI workflow.

## CI/CD discovery

The **CI/CD** page displays local entries and repository GitHub Actions workflows together, with an execution-location label on each entry. A shared search filters both kinds of definitions, and refresh updates both. Local execution controls and GitHub log links keep their respective behavior; local and GitHub histories and results remain separate.

Local entries use existing project definitions. An explicit `.repsync.json` `pipelines` array remains an override; `[]` disables local discovery. With the field absent or null, repsync discovers root and nested package scripts, Make/GNUmakefile targets, and Moon tasks. Package and Make names follow the check conventions above, plus `build`, `cd`, `deploy`, `release`, and `publish` prefixes. Moon tasks declared as `test` or `build` are discovered by type, alongside delivery-named tasks; internal, persistent, and known fix/write/watch/update/dev entries are excluded. There is no repository-specific CI template and no discovery-time execution.

Each entry shows its source, working directory, executable wrapper, and original script/recipe/task declaration. Compound scripts stay one invocation; repsync does not fabricate their internal steps, dependencies, or results. CI/CD labels categorize discovered entries; they do not establish release authorization or safety. Existing explicit pipelines keep their declared ordered steps. Runtime limits, logs, cancellation, version protection, and exit codes use the same runner. At most 128 automatic entries or 32 explicitly configured pipelines are supported; scan/query limits follow Project checks. Missing tools, invalid sources, and truncation remain visible warnings. Historical results are shown separately; changed declarations require fresh evidence.

**GitHub Actions** also reads current-worktree `.github/workflows/*.yml` and `*.yaml` files, independently of GitHub authentication and run history. It shows trigger declarations, job IDs/names, dependencies, conditions, runners, matrices, reusable-workflow references, steps, and original YAML. Working-directory defaults are inherited for display. Expressions, matrices, reusable jobs, action implementations, and scripts are not executed or expanded. These definitions describe the current files, while remote results describe GitHub runs; their revisions may differ.

Workflow reading is owned by repsync's backend and uses `go.yaml.in/yaml/v3`; the dependency is only a YAML parser, with no external process or new service. Reads remain within the project root, limited to 64 workflow files, 256 KiB per file, 1 MiB combined source, 64 jobs per workflow, and 256 steps per job. Invalid, inaccessible, or oversized definitions are reported individually and do not become successful or empty remote-run results.

```sh
repsync pipelines /absolute/path/to/project
repsync workflows /absolute/path/to/project
```

These commands only discover/read definitions. Run a selected local entry through the existing `run PATH PIPELINE_ID VERSION` operation. Remote execution remains with GitHub; the app does not replay workflow YAML locally or publish automatically.

## Project configuration

Use **Ask AI to configure** on services, checks, rules, AI resources, shared files, cleanup policy, or review applicability. It opens a project-scoped instruction that you copy into your existing AI chat; it does not send or execute the task. The assistant reads current guides and configuration, preserves unrelated changes, and uses `.repsync.json` or existing version-protected CLI operations. Refresh the relevant view after it finishes. Project registration, personal preferences, review decisions, and optional result records remain interactive.

```json
{
  "services": [
    {
      "id": "web",
      "name": "Frontend",
      "command": ["npm", "run", "dev", "--", "--port", "{port}"],
      "port": 3000,
      "autoPort": true,
      "url": "http://127.0.0.1:{port}"
    }
  ],
  "pipelines": [
    {
      "id": "check",
      "name": "Project checks",
      "kind": "ci",
      "timeoutSeconds": 600,
      "steps": [["npm", "run", "check"]]
    }
  ]
}
```

Commands are argument arrays and run from the project root unless `cwd` is set. The example assumes the `dev` script accepts `--port`; adapt it to your tool. `autoPort` uses `{port}` to pass the allocated port to the command. Use `fixedPort` for services that require a specific port. `envFrom` can reference declared service endpoints.

## Automatic runtime discovery

An explicit `services` array in `.repsync.json` takes priority, including `[]` to disable discovery. When the field is absent or null, Moon workspaces use the installed `moon query tasks` interface; other projects start from their root package.json `dev` / `start` entry. No repository name or absolute directory is built into discovery, and discovery never writes project files.

For package scripts, literal `concurrently`, `run-p`, and `npm-run-all --parallel` groups are expanded into the existing referenced child commands. Simple root script aliases and declared npm workspace / pnpm workspace selectors are followed for metadata; unrelated development scripts are not added. A Node wrapper is retained for every child only when its source forwards `process.argv.slice(2)` to a single `spawn` and has no recognized mutating subprocess/file calls. Node environment-file flags are retained too; environment values are not copied into service definitions or diagnostics. Aggregate pre/post lifecycle hooks, complex shell expressions, unknown options and opaque wrappers retain the original entry and report that it was not expanded. Repeated child references require an explicit override.

Preparation commands must precede that root entry in the same README shell fence. Directory changes are tracked, and only references to root package scripts are selected. A detached `docker compose up` is recognized directly or through a Node helper with a literal `spawnSync` argument list. The original documented command is used as a one-shot; installs, migrations, reset/down commands and arbitrary setup names are not automatically selected. Published TCP ports of its selected Compose services become observed dependencies, with profiled/unselected services excluded. Both short and long port declarations are supported; dynamic mappings and remote bind addresses remain unresolved. Repsync waits for successful preparation and dependency connectivity before launching applications. Stopping a project does not stop detached containers.

Application ports are read from literal `--port` arguments, literal Vite `server.port` values (or Vite’s default when unambiguous), and simple Go `net/http.Server.Addr` literals or an `os.Getenv` value with a literal empty-value fallback in `main`. Referenced script aliases, working directories and workspace packages are resolved without evaluating source code. Group ports stay fixed to preserve proxy and environment relationships. Portless workers remain without a TCP check, and dynamic expressions or unsupported frameworks require explicit service metadata. Each input retains its existing 256 KiB bound; runtime metadata reads at most 128 files, aliases at most 12 levels, and service count retains the existing limit. Relevant source and environment-file fingerprints invalidate stale startup plans and flag running instances after changes.

For Moon, standalone `moon run PROJECT:dev`, `PROJECT:start`, and `PROJECT:dev-infra` lines in README.md shell fences select the default runtime. Inline examples are excluded, as are installation, migration, build, test, and alternate development tasks. Without a documented selection, only exact persistent `dev` tasks with an identifiable listener are selected. The installed Moon CLI resolves workspace mappings and inherited task configuration. Missing Moon or failed queries remain visible errors.

Listener metadata comes from explicit `--port` arguments or simple numeric Makefile variables used by the task's Make recipe. Make is never evaluated during discovery. Inferred ports remain fixed because application proxies may depend on them. Complex expressions and hidden script ports require an explicit override; repsync does not guess them. Local PostgreSQL and Redis URLs in the root `.env` contribute only deduplicated loopback ports for observation; credentials and complete URLs are never saved or shown. The file is parsed as data, never sourced.

Documented `dev-infra` tasks are finite preparation commands (`oneShot`): startup waits for a successful exit before checking observed dependencies and launching applications in documented order, waiting on known application ports. Their logs and exit results remain available. They retain their existing data/initialization guards and may start detached infrastructure; stopping the project stops owned application processes, not those containers. Discovery and `--dry-run` execute no startup commands.

The same runner supports explicit `dependsOn: ["service-id"]` readiness dependencies without injecting environment variables, and `oneShot: true` completion dependencies. A dependency must expose a port or successful one-shot completion. Startup failure blocks remaining consumers; already started components retain their stop controls. TCP connectivity remains distinct from application health.

## Project runtime

**Project startup** starts the configured project components together and shows each component's process state, port connectivity, address, and recent logs. Startup reuses running app-owned instances and checks all prerequisites before launching new processes. **Stop project** stops only instances owned by repsync for this project; cross-project dependencies retain their own lifecycle. Each managed component has **Command details**. Startup omits the package.json script list and workspace, stash, and handoff shortcuts; use Branches and workspaces and Todos and handoff for those records. Configuration stays with the existing AI workflow. **Monitoring details** expands the connectivity and resource measurement limitations; component start buttons remain secondary to project startup.

CPU and memory refresh with the component status, roughly every two seconds while this view is open. Managed components sum their owned process group, including ordinary children such as npm, Node, and build tools. Observed dependencies sum only the host processes listening on the declared port, labeled **Host listener**. These are host measurements: a container port proxy does not expose the database process inside the container. No container API, database credentials, or monitoring agent is added. Unavailable measurements are shown as unavailable; stopped components retain no live metrics.

CPU uses the operating system's `ps %CPU` snapshot (100% represents one core and values can exceed 100%); it is not a precise two-second utilization sample. Memory is summed resident set size (RSS), displayed in MiB; shared resident pages can be counted by more than one process. Metrics are read on demand and are not persisted as history.

Declare shared databases, caches, or other existing local dependencies as `observeOnly`, with an explicit port and no command:

```json
{"id": "db", "name": "PostgreSQL", "observeOnly": true, "port": 5432}
```

These entries are checked for TCP connectivity and never started, stopped, or assigned a run ID. They cannot declare `command`, `cwd`, `env`, `envFrom`, or `autoPort`. An unreachable observed dependency blocks project startup before any new process launches. Database credentials and database URLs are not needed for this check. Observed dependencies have no repsync process log; inspect them through their existing database or container tools. For project-owned databases, reuse the project's existing foreground service command; detached container commands do not establish process ownership.

Use `envFrom` on a managed component to declare a real endpoint dependency, for example `"envFrom": {"DATABASE_PORT": {"service": "db", "field": "port"}}`. The consumer waits for the prerequisite port before launch. This injects only the referenced endpoint; keep credentials and complete database connection strings in the project's existing environment setup. TCP connectivity does not verify authentication, migrations, application responses, or database business health.

## CLI

Build the development CLI and connect it to the desktop app:

```sh
go build -o bin/repsync ./cmd/repsync
export REPSYNC_APP="$PWD/build/darwin-arm64/repsync.app/Contents/MacOS/repsync"
./bin/repsync open
./bin/repsync add /absolute/path/to/project
./bin/repsync projects
./bin/repsync doctor /absolute/path/to/project
./bin/repsync start --dry-run /absolute/path/to/project
./bin/repsync start --wait --timeout 30s /absolute/path/to/project
./bin/repsync status /absolute/path/to/project
```

The desktop app owns the local service. `REPSYNC_APP` points a development CLI to the packaged executable; it is unnecessary when the desktop app is already running. `--help` lists all commands.

CLI responses use `ok`, `data`, and an `error` on failure. Exit codes are 0 for success, 1 for operational failure, and 2 for invalid arguments. `start --wait` checks TCP readiness for declared ports; business health checks remain project-specific.

Use the returned `runId` to read logs or stop a service. Read pipeline IDs and configuration versions before running checks:

```sh
repsync logs /absolute/path/to/project web RUN_ID
repsync stop /absolute/path/to/project web RUN_ID
repsync stop-projects /absolute/path/to/project

repsync pipelines /absolute/path/to/project
repsync run --wait --timeout 10m /absolute/path/to/project CHECK_ID CONFIG_VERSION
repsync runs /absolute/path/to/project
```

## Branches and workspaces

The **Branches and workspaces** tab contains worktrees, local branches, and Stash. Its saved page ID remains `Worktrees`. Opening, refreshing, or refocusing the page uses `git-summary`: it reads current lists and file-state summaries, with a single merged-branch query, and does not wait for cleanup's global mutation lock, inspect active processes or per-ref reflogs, or update retention observations. The response has `cleanupChecked: false`, no eligible cleanup candidates, and a version distinct from a full inspection.

**Manual cleanup** runs the existing full `git-workspace` inspection before opening the selection dialog, with a visible checking state and errors. The actual cleanup still rechecks version, retained/ignored files, branch reachability, retention, registrations, Stash, and active use before removing selected entries. Automatic cleanup and CLI `git-workspace` retain the full inspection path. A listing snapshot cannot authorize deletion. Retention observation starts or updates during full inspection, rather than during the lightweight listing.

```sh
repsync git-summary /absolute/path/to/project
repsync git-workspace /absolute/path/to/project
```

## Automatic cleanup configuration

Read `repsync git-workspace PATH` for `policyVersion` and the current policy. AI can write policy JSON and call `repsync git-policy PATH POLICY_VERSION FILE`. The JSON contains `enabled`, `baseBranch`, and `retentionDays` (1–365); the base must be an existing local branch. Stale versions are rejected. This changes app-owned settings only and does not run immediate cleanup or change `.repsync.json`. Enabling automatic cleanup requires an explicit user request. Existing worktree, stash, process, and recovery protections still apply.

## Settings

Settings saves preferences automatically for the current desktop app or browser. Desktop and browser preferences are separate.

- **Startup:** reopen the last project (default), remember each project's tab, and choose a default page. The default applies when there is no remembered tab, or whenever tab restoration is disabled.
- **Appearance:** choose the interface language (Follow system, 简体中文, or English), standard or larger text, and system or reduced motion. Language changes apply immediately and are remembered after reopening. Follow system uses the system/WebView language (Chinese maps to Simplified Chinese; other languages use English) and responds to language-change events. Project names, paths, commands, logs, diffs, and document or memory content keep their original text.
- **Project status:** choose how often to refresh the project overview. Focus and manual refresh remain available. Other views retain their own update schedules.
- **About:** view the current app version and open documentation, issues, releases, or source code.

Reset preferences restores these defaults without changing project registrations, document drafts, or history. A storage error leaves changes active for the current window and shows an error in Settings.

## Project map

Open **Project map** in a project to group implementations by user-facing feature. Open a module to see its implementations and entry references. **Ask AI to organize** copies an instruction for your existing AI chat; refresh after it writes `.repsync-map.json` in the project root. The app does not call a model or change project code.

```json
{
  "version": 1,
  "modules": [
    {
      "id": "corrections",
      "name": "Essay correction",
      "summary": "Submit an essay and read feedback",
      "overlap": "Both pages submit essays; check whether the older page is still needed.",
      "implementations": [
        {
          "name": "Correction page",
          "source": { "path": "src/pages/Correction.tsx", "find": "function Correction(" },
          "entry": { "path": "src/App.tsx", "find": "<Correction" }
        },
        {
          "name": "Earlier correction page",
          "source": { "path": "src/legacy/Essay.tsx", "find": "function Essay(" }
        }
      ]
    }
  ]
}
```

Names, summaries, and `overlap` are authored project content. `overlap` is optional: provide a specific reason only for suspected duplication. Multiple implementations can be intentional. `source` identifies the primary implementation; `entry` is optional and should cite an actual mount, route, or navigation connection. An import alone does not establish user reachability. An omitted entry means **not recorded**, not confirmed unreachable or unused.

Use project-relative file paths and a short single-line `find` that matches exactly one current line. `find` is optional for a source file and required for an entry. On each read, the app resolves line numbers and marks missing, unreadable, ambiguous, or changed references. It recomputes `line` and `issue`; supplied values are not evidence. References stay within the registered project, including symbolic links. This locates declared references; it neither discovers all modules nor verifies runtime behavior, permissions, or complete user flows. AI should report its inspection scope and omissions separately.

The map is limited to 256 KiB, 64 modules, 1–8 implementations per module, and 256 references. Each referenced file uses the existing 256 KiB text limit; at most roughly 8 MiB of unique reference content is read per request. Read limits and unresolved references are shown without hiding other modules. Names are limited to 256 bytes, module IDs to 128 bytes, summaries and duplicate notes to 2048 bytes, paths to 2048 bytes, and `find` to 256 bytes.

Read the same map through the desktop control service with `repsync project-map PATH`. This returns modules and current reference locations without running project commands. Regenerate maps through the project's existing AI workflow when features or entry points change. The repsync repository includes its own map; other registered projects are not modified automatically.

## Shared rules and files

Both sections start empty. Add your own sources through **Ask AI to configure**; no rules or files are preloaded, including in browser preview. Existing registrations are preserved.

Shared rules use a managed block inside `AGENTS.md`; surrounding project content is preserved. Shared files have explicit source and destination registrations. Preview changes before applying them. Backups support restore as long as later edits have not replaced the update.

Worktree cleanup retains modified, ignored, active, protected, and uncertain entries. It does not delete remote branches. Stash apply preserves the original stash and leaves conflicts for review.

## AI resource updates and manuals

Before adopting a Skill, choose **Assess suitability and conflicts** in **Ask AI to maintain** and provide the original source in your existing AI chat. AI compares project rules, tasks and existing resources, then records a scoped recommendation with separate overlap, conflict, prerequisite and unknown findings. Candidates appear in the existing resource list without installation. Use `repsync ai-resource-assess [PATH] FILE` to write reports; existing resource and project snapshots protect freshness. Global reports remain references when viewed inside a project. See [the assessment format and limits](guide.zh-CN.md#安装前适配与冲突评估).

Open an existing AI resource to see AI-authored update findings, new capabilities, a practical manual, check time, and sources. Use **Ask AI to maintain** to copy a scoped instruction into your existing AI chat. Choose checking, updating through the existing installation channel, or writing a manual. Rescan after completion; rescanning itself only inventories local resources.

AI reads `repsync ai-resources [PATH]` and saves each report through `repsync ai-resource-record [PATH] FILE`. Omit PATH for global records. Reports protect resource snapshots and existing records; project reports can override global guidance. Local Skill text or MCP configuration changes mark guidance as stale. Remote tool changes and companion documents still require a fresh AI check. The app does not run an updater, call a model, or schedule checks. See [the report format and limits](guide.zh-CN.md#资源更新与说明书).

## Desktop updates

Packaged builds from 0.1.2 use MyGo’s signed updater with the public key in `mygo.json`. The desktop app checks shortly after launch and daily; **Settings → Check for updates** and the app menu open the update window. It supports release notes, progress, skip, remind later, and optional automatic downloads. Downloaded updates take effect after quitting; relaunch uses the same app path. Project data is stored outside the bundle. Development builds and browser preview do not install updates. The install directory must be writable by the current user.

Only published stable GitHub releases are offered by default. Prereleases, drafts, unsigned archives, and older versions are not offered or installed. Network and signature errors leave the original app intact. The first upgrade from an older build needs one manual replacement because it has no updater.

The release owner maintains the Ed25519 key pair outside the repository. The public key is pinned in `mygo.json`; `MYGO_UPDATER_PRIVATE_KEY` is stored in GitHub repository secrets. Keep a secure backup of the private key; do not regenerate it for each release. The local key is under the system configuration directory in `repsync/update-keys/`, with owner-only access to the secret. For a local signed build, pass its contents through `MYGO_UPDATER_PRIVATE_KEY`; ordinary builds without the secret do not generate update artifacts.

For a release, update `mygo.json` and `CHANGELOG.md`, commit the reviewed changes, and push the matching `v<version>` tag. `.github/workflows/release.yml` runs the project checks, builds the signed macOS Apple Silicon archive, verifies its size, version, download URL and Ed25519 signature, uploads the installer, archive, manifest and checksums to a draft, then publishes the complete release. Published releases are never overwritten by rerunning the workflow. A prerelease tag stays outside the stable feed. Do not publish a standalone ZIP without update artifacts.

macOS application signing remains ad hoc; Ed25519 update verification does not provide Apple notarization. Other operating systems are outside the verified release scope. Local builds and tests do not establish that a public update is available; verify the published manifest and a real upgrade separately.

## Local data

On macOS, project registration, logs, backups, and history are stored under `~/Library/Application Support/repsync/`. Removing a project registration keeps its directory and history. Browser preview data is separate from desktop projects.

Commands run locally with your account's permissions. Keep credentials in environment variables or existing credential stores. Memory extraction sends selected user messages to the chosen AI CLI and its configured provider; extracted memories await confirmation.

The [detailed Chinese guide](guide.zh-CN.md) covers configuration fields, operational limits, recovery, and review declarations.
