# Using repsync

See the [README](../README.md) for installation, development, and validation commands.

## Navigation

The app opens a specific project directly. **Projects** in the sidebar expands or collapses the project list; select a project or use search to switch. There is no separate projects overview.

Projects have five main tabs: **Project startup**, **Project standards**, **CI/CD**, **Worktrees**, and **Todos and handoff**. They remain directly visible and wrap in narrow windows. Expand **Utilities** to open **Project map** or project **AI resources**; expanding the group does not load either page. Selecting a main tab or switching projects collapses Utilities. Collapsing Utilities while viewing a utility page returns to Project startup. Documents are part of Project standards; startup command details and package.json scripts are part of Project startup. Shared updates and shared files are managed once from the sidebar's **Shared rules**. Saved Documents preferences open Project standards, and saved Startup configuration preferences open Project startup.

By default, startup restores the last project and its remembered main tab. Utilities start collapsed, are excluded from the default-page options, and never replace the remembered main tab. Older saved utility pages fall back to the configured main default page. Disable **Reopen last project** in Settings to open the first project in the sidebar instead.

## Project standards

**Project standards → Guidelines and documents** lists all root `.md` and `.markdown` files. Guideline names AGENTS, CLAUDE, GEMINI, CONTRIBUTING, RULES, STYLEGUIDE, DEVELOPMENT, DESIGN, and CODE_OF_CONDUCT are prioritized, with AGENTS first. Select a file to read saved content, search it, or refresh after external edits. **Edit document** restores any session draft in the same reader; save with the existing protected operation, or choose **Back to saved file** to read saved content while retaining the draft. Empty files, missing files, and read errors have separate states.

The reader covers the project root; it does not expand inherited parent instructions, nested directory rules, or non-Markdown tool configuration, and does not claim to resolve all effective rules. **Rule checks** retains policy declarations and checks. The sidebar's **Shared rules** manages common sources, project adoption, diff preview, apply, recovery, and shared files; those views are not duplicated inside each project.

## Project configuration

Ask your AI assistant to read the project's guides and connect its existing commands through `.repsync.json`. Manual configuration forms are also available.

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

## Project runtime

**Project startup** starts the configured project components together and shows each component's process state, port connectivity, address, and recent logs. Startup reuses running app-owned instances and checks all prerequisites before launching new processes. **Stop project** stops only instances owned by repsync for this project; cross-project dependencies retain their own lifecycle. Each managed component has **Command details**, and package.json scripts are available on the same page. Configuration stays with the existing AI workflow. **Monitoring details** expands the connectivity and resource measurement limitations; component start buttons remain secondary to project startup.

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

Shared rules use a managed block inside `AGENTS.md`; surrounding project content is preserved. Shared files have explicit source and destination registrations. Preview changes before applying them. Backups support restore as long as later edits have not replaced the update.

Worktree cleanup retains modified, ignored, active, protected, and uncertain entries. It does not delete remote branches. Stash apply preserves the original stash and leaves conflicts for review.

## AI resource updates and manuals

Before adopting a Skill, choose **Assess suitability and conflicts** in **Ask AI to maintain** and provide the original source in your existing AI chat. AI compares project rules, tasks and existing resources, then records a scoped recommendation with separate overlap, conflict, prerequisite and unknown findings. Candidates appear in the existing resource list without installation. Use `repsync ai-resource-assess [PATH] FILE` to write reports; existing resource and project snapshots protect freshness. Global reports remain references when viewed inside a project. See [the assessment format and limits](guide.zh-CN.md#安装前适配与冲突评估).

Open an existing AI resource to see AI-authored update findings, new capabilities, a practical manual, check time, and sources. Use **Ask AI to maintain** to copy a scoped instruction into your existing AI chat. Choose checking, updating through the existing installation channel, or writing a manual. Rescan after completion; rescanning itself only inventories local resources.

AI reads `repsync ai-resources [PATH]` and saves each report through `repsync ai-resource-record [PATH] FILE`. Omit PATH for global records. Reports protect resource snapshots and existing records; project reports can override global guidance. Local Skill text or MCP configuration changes mark guidance as stale. Remote tool changes and companion documents still require a fresh AI check. The app does not run an updater, call a model, or schedule checks. See [the report format and limits](guide.zh-CN.md#资源更新与说明书).

## Local data

On macOS, project registration, logs, backups, and history are stored under `~/Library/Application Support/repsync/`. Removing a project registration keeps its directory and history. Browser preview data is separate from desktop projects.

Commands run locally with your account's permissions. Keep credentials in environment variables or existing credential stores. Memory extraction sends selected user messages to the chosen AI CLI and its configured provider; extracted memories await confirmation.

The [detailed Chinese guide](guide.zh-CN.md) covers configuration fields, operational limits, recovery, and review declarations.
