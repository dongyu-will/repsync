import { useCallback, useEffect, useRef, useState } from "react";
import { isMyGo } from "mygo-runtime";
import {
  Service,
  type ProjectConfig,
  type Pipeline,
  type LocalServiceDefinition,
} from "./mygo";
import { Button, Select } from "./components";
import { servicePreview } from "./service-preview";
import { pipelinePreview } from "./pipeline-preview";

type Section = "services" | "pipelines";
const native = isMyGo();
const inputClass =
  "min-h-9 w-full min-w-0 rounded-md border border-control bg-white px-3 py-1.5 text-sm";
const message = (error: unknown) =>
  error instanceof Error ? error.message : String(error);

function commandText(args: string[]) {
  if (!args?.length) return "";
  if (
    args.length === 3 &&
    ["sh", "/bin/sh"].includes(args[0]) &&
    ["-c", "-ec"].includes(args[1])
  )
    return args[2];
  return args
    .map((arg) =>
      /^[A-Za-z0-9_@%+=:,./-]+$/.test(arg)
        ? arg
        : "'" + arg.replaceAll("'", "'\"'\"'") + "'",
    )
    .join(" ");
}
function stepsText(item: Pipeline | LocalServiceDefinition) {
  return ("steps" in item ? item.steps || [] : [item.command || []])
    .map(commandText)
    .join(" &&\n");
}

export default function ProjectConfigEditor({
  path,
  section,
  onSaved,
}: {
  path: string;
  section: Section;
  onSaved: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [notice, setNotice] = useState("");
  const title = `Configure ${section === "services" ? "services" : "pipelines"}`;
  return (
    <>
      <Button
        type="button"
        aria-label={title}
        onClick={() => {
          setNotice("");
          setOpen(true);
        }}
      >
        {title}
      </Button>
      {notice && (
        <span role="status" className="text-xs text-muted">
          {notice}
        </span>
      )}
      {open && (
        <CommandDialog
          key={path + section}
          path={path}
          section={section}
          title={title}
          onClose={() => setOpen(false)}
          onSaved={() => {
            setNotice("Saved");
            onSaved();
            setOpen(false);
          }}
        />
      )}
    </>
  );
}

function CommandDialog({
  path,
  section,
  title,
  onClose,
  onSaved,
}: {
  path: string;
  section: Section;
  title: string;
  onClose: () => void;
  onSaved: () => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [id, setID] = useState("");
  const [config, setConfig] = useState<ProjectConfig | null>(null);
  const [name, setName] = useState("");
  const [command, setCommand] = useState("");
  const [cwd, setCwd] = useState("");
  const [url, setUrl] = useState("");
  const [kind, setKind] = useState("ci");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  const [loading, setLoading] = useState(true);
  const services = section === "services";
  const selectItem = useCallback(
    (nextID: string, current: ProjectConfig) => {
      const item = current[section].find((entry) => entry.id === nextID);
      setID(nextID);
      setName(item?.name || "");
      setCommand(item ? stepsText(item) : "");
      setCwd(item?.cwd || "");
      setUrl(item && "url" in item ? item.url : "");
      setKind(item && "kind" in item ? item.kind : "ci");
      setError("");
    },
    [section],
  );
  useEffect(() => {
    dialog.current?.showModal();
  }, []);
  useEffect(() => {
    let active = true;
    const load = native
      ? Service.projectConfiguration(path)
      : Promise.resolve({
          version: "example",
          warnings: [],
          servicesAutomatic: false,
          pipelinesAutomatic: false,
          services: servicePreview(path).services.map(
            (state) => state.definition,
          ),
          pipelines: pipelinePreview(path).config.pipelines,
        });
    load
      .then((value) => {
        if (!active) return;
        setConfig(value);
        selectItem(value[section][0]?.id || "", value);
      })
      .catch((error) => {
        if (active) {
          setConfig(null);
          setError(message(error));
        }
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [path, section, revision, selectItem]);
  async function save(remove = false) {
    if (!native || !config) return;
    setBusy(true);
    setError("");
    try {
      const original = config[section].find((item) => item.id === id);
      let nextID = id;
      if (!nextID) {
        let index = 1;
        const prefix = services ? "service" : "pipeline";
        while (config[section].some((item) => item.id === `${prefix}-${index}`))
          index++;
        nextID = `${prefix}-${index}`;
      }
      const unchanged = original && command === stepsText(original);
      if (services) {
        let entries = config.services.filter((item) => item.id !== id);
        if (!remove) {
          const previous = original as LocalServiceDefinition | undefined;
          // Retain argv and environment unless the user edits the command itself.
          const item: LocalServiceDefinition = {
            autoPort: previous?.autoPort || false,
            ...previous,
            id: nextID,
            name: name.trim(),
            cwd,
            env: previous?.env || {},
            command:
              unchanged && previous
                ? previous.command
                : ["/bin/sh", "-c", command],
            url,
            port: previous && url === previous.url ? previous.port : 0,
          };
          entries = id
            ? config.services.map((entry) => (entry.id === id ? item : entry))
            : [...entries, item];
        }
        await Service.saveServices(path, config.version, false, entries);
      } else {
        let entries = config.pipelines.filter((item) => item.id !== id);
        if (!remove) {
          const previous = original as Pipeline | undefined;
          const item: Pipeline = {
            ...previous,
            id: nextID,
            name: name.trim(),
            cwd,
            env: previous?.env || {},
            steps:
              unchanged && previous
                ? previous.steps
                : [["/bin/sh", "-ec", command]],
            kind,
            timeoutSeconds: previous?.timeoutSeconds || 1800,
          };
          entries = id
            ? config.pipelines.map((entry) => (entry.id === id ? item : entry))
            : [...entries, item];
        }
        await Service.savePipelines(path, config.version, false, entries);
      }
      dialog.current?.close();
      onSaved();
    } catch (error) {
      setError(message(error));
    } finally {
      setBusy(false);
    }
  }
  return (
    <dialog
      ref={dialog}
      aria-labelledby="command-dialog-title"
      onCancel={(event) => {
        event.preventDefault();
        if (!busy) dialog.current?.close();
      }}
      onClose={onClose}
      className="m-auto max-h-[calc(100dvh-32px)] w-[480px] max-w-[calc(100vw-32px)] overflow-hidden rounded-xl bg-white p-0 text-ink shadow-xl backdrop:bg-black/20"
    >
      <form
        onSubmit={(event) => {
          event.preventDefault();
          void save();
        }}
        className="flex max-h-[calc(100dvh-32px)] flex-col p-5"
      >
        <div>
          <h2 id="command-dialog-title" className="text-xl font-semibold">
            {title}
          </h2>
          <p className="mt-1 truncate text-xs text-muted" title={path}>
            {path}
          </p>
        </div>
        <div className="-mx-1 min-h-0 space-y-4 overflow-y-auto px-1 pb-1 pt-4">
          {loading ? (
            <p role="status" className="text-sm text-muted">
              Loading…
            </p>
          ) : (
            config && (
              <fieldset disabled={busy} className="min-w-0 space-y-4">
                <p className="text-sm leading-5 text-muted">
                  Ask AI to configure existing{" "}
                  {services ? "start" : "check or release"} commands, or edit
                  them here.
                </p>
                <label className="block space-y-1.5 text-sm font-medium">
                  {services ? "Service" : "Pipelines"}
                  <Select
                    autoFocus
                    className="w-full"
                    value={id}
                    onChange={(event) => {
                      const item = config[section].find(
                        (entry) => entry.id === id,
                      );
                      const changed =
                        name !== (item?.name || "") ||
                        command !== (item ? stepsText(item) : "") ||
                        cwd !== (item?.cwd || "") ||
                        url !== (item && "url" in item ? item.url : "") ||
                        kind !== (item && "kind" in item ? item.kind : "ci");
                      if (
                        !changed ||
                        window.confirm("Discard unsaved changes and switch?")
                      )
                        selectItem(event.target.value, config);
                    }}
                  >
                    {config[section].map((item) => (
                      <option key={item.id} value={item.id}>
                        {item.name}
                      </option>
                    ))}
                    <option value="">
                      Add{services ? "Service" : "Pipelines"}
                    </option>
                  </Select>
                </label>
                <label className="block space-y-1.5 text-sm font-medium">
                  Name
                  <input
                    required
                    className={inputClass}
                    placeholder={
                      services
                        ? "e.g. Frontend development"
                        : "e.g. Project checks"
                    }
                    value={name}
                    onChange={(event) => setName(event.target.value)}
                  />
                </label>
                <label className="block space-y-1.5 text-sm font-medium">
                  Command
                  <textarea
                    required
                    rows={3}
                    spellCheck={false}
                    className={`${inputClass} resize-y font-mono`}
                    placeholder={services ? "npm run dev" : "npm run check"}
                    value={command}
                    onChange={(event) => setCommand(event.target.value)}
                  />
                </label>
                <div className="mt-3 space-y-3">
                  <label className="block space-y-1.5 text-sm font-medium">
                    Working directory
                    <input
                      className={inputClass}
                      placeholder="Leave blank for the project root"
                      value={cwd}
                      onChange={(event) => setCwd(event.target.value)}
                    />
                  </label>
                  {services ? (
                    <label className="block space-y-1.5 text-sm font-medium">
                      Page URL
                      <input
                        type={
                          config?.services.find((entry) => entry.id === id)
                            ?.autoPort
                            ? "text"
                            : "url"
                        }
                        className={inputClass}
                        placeholder="Optional, e.g. http://localhost:5173"
                        value={url}
                        onChange={(event) => setUrl(event.target.value)}
                      />
                    </label>
                  ) : (
                    <label className="block space-y-1.5 text-sm font-medium">
                      Purpose
                      <Select
                        className="w-full"
                        value={kind}
                        onChange={(event) => setKind(event.target.value)}
                      >
                        <option value="ci">Check</option>
                        <option value="cd">Release</option>
                      </Select>
                    </label>
                  )}
                </div>
              </fieldset>
            )
          )}
          {error && (
            <div className="space-y-2">
              <p role="alert" className="break-words text-sm text-removed">
                {error}
              </p>
              <button
                type="button"
                disabled={busy || loading}
                className="min-h-7 text-sm text-accent"
                onClick={() => {
                  setLoading(true);
                  setRevision((value) => value + 1);
                }}
              >
                Reload
              </button>
            </div>
          )}
        </div>
        <div className="flex shrink-0 items-center justify-between gap-2 pt-4">
          <div>
            {id && (
              <button
                type="button"
                className="min-h-9 text-sm text-removed"
                disabled={!native || !config || busy || loading}
                onClick={() => void save(true)}
              >
                Remove
              </button>
            )}
          </div>
          <div className="flex gap-2">
            <Button
              type="button"
              disabled={busy}
              onClick={() => dialog.current?.close()}
            >
              Cancel
            </Button>
            <Button
              primary
              type="submit"
              disabled={
                !native ||
                !config ||
                busy ||
                loading ||
                !name.trim() ||
                !command.trim()
              }
            >
              {busy ? "Saving…" : "Save"}
            </Button>
          </div>
        </div>
      </form>
    </dialog>
  );
}
