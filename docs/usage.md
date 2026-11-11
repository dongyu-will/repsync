# Using repsync

See the [README](../README.md) for installation, development, and validation commands.

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

- **Startup:** reopen the last project, remember each project's tab, and choose a default page. The default applies when there is no remembered tab, or whenever tab restoration is disabled.
- **Appearance:** choose standard or larger text, and follow system motion preferences or reduce motion explicitly.
- **Project status:** choose how often to refresh the project overview. Focus and manual refresh remain available. Other views retain their own update schedules.
- **About:** view the current app version and open documentation, issues, releases, or source code.

Reset preferences restores these defaults without changing project registrations, document drafts, or history. A storage error leaves changes active for the current window and shows an error in Settings.

## Shared rules and files

Shared rules use a managed block inside `AGENTS.md`; surrounding project content is preserved. Shared files have explicit source and destination registrations. Preview changes before applying them. Backups support restore as long as later edits have not replaced the update.

Worktree cleanup retains modified, ignored, active, protected, and uncertain entries. It does not delete remote branches. Stash apply preserves the original stash and leaves conflicts for review.

## Local data

On macOS, project registration, logs, backups, and history are stored under `~/Library/Application Support/repsync/`. Removing a project registration keeps its directory and history. Browser preview data is separate from desktop projects.

Commands run locally with your account's permissions. Keep credentials in environment variables or existing credential stores. Memory extraction sends selected user messages to the chosen AI CLI and its configured provider; extracted memories await confirmation.

The [detailed Chinese guide](guide.zh-CN.md) covers configuration fields, operational limits, recovery, and review declarations.
