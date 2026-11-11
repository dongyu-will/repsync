import { useEffect, useReducer, useRef, type FormEvent } from "react";
import { isMyGo } from "mygo-runtime";
import { Button, Select } from "./components";
import {
  Service,
  type AIResource,
  type CodexResourceChange,
  type CodexResourcePlacement,
  type CodexResourcePreview,
} from "./mygo";

const native = isMyGo();
const inputClass = "min-h-9 w-full rounded-md border border-line bg-white px-2";
const initialChange: CodexResourceChange = {
  kind: "mcp",
  name: "",
  action: "add",
  source: "",
  url: "",
  tokenEnv: "",
  command: "",
  args: [],
};
const placementLabels: Record<string, string> = {
  applied: "Enabled for project",
  changed: "Project configuration changed externally",
  missing: "Project link removed",
  source_missing: "Skill Source unavailable",
  disabled: "Disabled for project",
};

type State = {
  transport: string;
  selected: string | null;
  preview: (CodexResourcePreview & { change: CodexResourceChange }) | null;
  error: string;
  notice: string;
  pending: boolean;
};

export default function ResourceSettings({
  path,
  resource,
  placement,
  onChanged,
}: {
  path: string;
  resource?: AIResource;
  placement?: CodexResourcePlacement;
  onChanged: () => void;
}) {
  const [state, update] = useReducer(
    (current: State, next: Partial<State>) => ({ ...current, ...next }),
    {
      transport: "http",
      selected: null,
      preview: null,
      error: "",
      notice: "",
      pending: false,
    },
  );
  const previewRef = useRef<HTMLDivElement>(null);
  const { preview, pending } = state;
  const editing = !!resource;
  const userMCP =
    resource?.kind === "mcp" &&
    resource.source === "Codex CLI Current configuration" &&
    ["configured", "disabled"].includes(resource.availability);
  const override = placement?.source === "user";
  const globalDefault =
    resource?.availability === "disabled" ? "disable" : "enable";
  const current = path
    ? placement
      ? placement.state === "disabled"
        ? "disable"
        : "enable"
      : "remove"
    : globalDefault;
  const selected = state.selected ?? current;
  const message = (error: unknown) =>
    error instanceof Error ? error.message : String(error);

  useEffect(() => {
    if (preview) previewRef.current?.focus();
  }, [preview]);

  async function plan(change: CodexResourceChange) {
    if (pending) return;
    update({ pending: true, preview: null, error: "", notice: "" });
    try {
      const next = native
        ? await Service.previewCodexResource(path, change)
        : previewResourceChange(path, change, current, placement);
      update({ preview: { ...next, change } });
    } catch (error) {
      update({ error: message(error) });
    } finally {
      update({ pending: false });
    }
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const fields = new FormData(event.currentTarget);
    const value = (key: string) => String(fields.get(key) || "").trim();
    await plan({
      ...initialChange,
      name: value("name"),
      url: value("url"),
      tokenEnv: value("tokenEnv"),
      command: value("command"),
      args: String(fields.get("args") || "")
        .split(/\r?\n/)
        .filter(Boolean),
    });
  }

  function planResource(action: string) {
    if (!resource) return;
    void plan({
      ...initialChange,
      kind: resource.kind,
      name: resource.name,
      source:
        resource.kind === "skill" && action === "add" ? resource.location : "",
      action,
    });
  }

  async function apply() {
    if (!native || !preview || pending) return;
    update({ pending: true, error: "" });
    try {
      await Service.applyCodexResource(path, preview.change, preview.version);
      update({
        preview: null,
        selected: null,
        notice: "Resource settings saved",
      });
      onChanged();
    } catch (error) {
      update({ error: message(error) });
    } finally {
      update({ pending: false });
    }
  }

  const readOnly = placement?.state === "changed";
  const changeSelection = (value: string) =>
    update({ selected: value, preview: null, error: "", notice: "" });

  return (
    <div className="space-y-3">
      {editing && (
        <div className="space-y-2">
          {placement && (
            <p className="text-xs text-muted">
              {placementLabels[placement.state]}
            </p>
          )}
          {!readOnly && ((userMCP && (!placement || override)) || override) ? (
            <div className="flex flex-wrap items-end gap-2">
              <label className="grid gap-1">
                <span className="text-xs text-muted">
                  {path ? "Project settings" : "User defaults"}
                </span>
                <Select
                  value={selected}
                  disabled={pending}
                  onChange={(e) => changeSelection(e.target.value)}
                >
                  {path && (
                    <option value="remove">
                      Inherit defaults
                      {userMCP
                        ? globalDefault === "enable"
                          ? "(enabled)"
                          : "(disabled)"
                        : ""}
                    </option>
                  )}
                  {(userMCP || current === "enable") && (
                    <option value="enable">Enable</option>
                  )}
                  {(userMCP || current === "disable") && (
                    <option value="disable">Disable</option>
                  )}
                </Select>
              </label>
              <Button
                type="button"
                disabled={pending || selected === current}
                onClick={() => planResource(selected)}
              >
                Preview changes
              </Button>
            </div>
          ) : !readOnly && placement ? (
            <Button
              type="button"
              disabled={pending}
              onClick={() => planResource("remove")}
            >
              {resource.kind === "skill"
                ? "Preview removing project link"
                : "Preview removing project configuration"}
            </Button>
          ) : !readOnly && path && resource.kind === "skill" ? (
            <Button
              type="button"
              disabled={pending}
              onClick={() => planResource("add")}
            >
              Preview project connection
            </Button>
          ) : null}
        </div>
      )}
      {!editing && (
        <form
          onSubmit={submit}
          className="space-y-3"
          onChange={() => changeSelection("")}
        >
          <div className="flex flex-wrap gap-3">
            <label className="grid flex-1 gap-1">
              <span className="text-xs text-muted">Server name</span>
              <input
                className={inputClass}
                required
                name="name"
                pattern={String.raw`[A-Za-z0-9_\-]{1,64}`}
                disabled={pending}
              />
            </label>
            <label className="grid gap-1">
              <span className="text-xs text-muted">Transport</span>
              <Select
                value={state.transport}
                disabled={pending}
                onChange={(e) => update({ transport: e.target.value })}
              >
                <option value="http">Streamable HTTP</option>
                <option value="stdio">Standard input/output</option>
              </Select>
            </label>
          </div>
          {state.transport === "http" ? (
            <>
              <label className="grid gap-1">
                <span className="text-xs text-muted">Server URL</span>
                <input
                  className={inputClass}
                  required
                  name="url"
                  type="url"
                  disabled={pending}
                />
              </label>
              <label className="grid gap-1">
                <span className="text-xs text-muted">
                  Token environment variable (optional)
                </span>
                <input
                  className={inputClass}
                  name="tokenEnv"
                  disabled={pending}
                />
              </label>
            </>
          ) : (
            <>
              <label className="grid gap-1">
                <span className="text-xs text-muted">Executable</span>
                <input
                  className={inputClass}
                  required
                  name="command"
                  disabled={pending}
                />
              </label>
              <label className="grid gap-1">
                <span className="text-xs text-muted">
                  Arguments (one per line)
                </span>
                <textarea
                  className={inputClass}
                  name="args"
                  rows={3}
                  disabled={pending}
                />
              </label>
            </>
          )}
          <Button type="submit" disabled={pending}>
            Preview addition
          </Button>
        </form>
      )}
      {state.error && (
        <p role="alert" className="break-words text-removed">
          {state.error}
        </p>
      )}
      {state.notice && (
        <p role="status" className="text-muted">
          {state.notice}
        </p>
      )}
      {preview && (
        <div
          ref={previewRef}
          tabIndex={-1}
          aria-label="Resource change preview"
          className="space-y-2 border-t border-line-subtle pt-3"
        >
          <p className="break-all text-xs text-muted">
            Codex · {preview.target}
          </p>
          <pre className="max-h-56 overflow-auto whitespace-pre-wrap break-all rounded-md bg-white p-2 font-mono text-xs">
            {preview.before && (
              <span className="block text-removed">{preview.before}</span>
            )}
            {preview.after && (
              <span className="block text-added">{preview.after}</span>
            )}
          </pre>
          <div className="flex flex-wrap justify-end gap-2">
            <Button
              disabled={pending}
              onClick={() => update({ preview: null })}
            >
              Cancel
            </Button>
            <Button
              primary
              disabled={!native || pending}
              onClick={() => void apply()}
            >
              Apply changes
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

function previewResourceChange(
  path: string,
  change: CodexResourceChange,
  current: string,
  placement?: CodexResourcePlacement,
): CodexResourcePreview {
  const enabled = (action: string) =>
    action === "remove" ? "" : "enabled = " + (action === "enable");
  if (change.kind === "skill") {
    return {
      version: "preview",
      target: ".agents/skills/" + change.name,
      before: placement?.source || "",
      after: change.source,
    };
  }
  return {
    version: "preview",
    target: path ? ".codex/config.toml" : "~/.codex/config.toml",
    before: enabled(current),
    after:
      change.action === "add"
        ? `[mcp_servers.${change.name}]\n` +
          (change.url
            ? `url = ${JSON.stringify(change.url)}`
            : `command = ${JSON.stringify(change.command)}\nargs = ${JSON.stringify(change.args)}`)
        : enabled(change.action),
  };
}
