import { useEffect, useRef, useState } from "react";
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
  id,
  onSaved,
}: {
  path: string;
  section: Section;
  id?: string;
  onSaved: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [notice, setNotice] = useState("");
  const title = `${id ? "编辑" : "添加"}${section === "services" ? "服务" : "流水线"}`;
  return (
    <>
      <Button
        type="button"
        aria-label={id ? `${title} ${id}` : title}
        onClick={() => {
          setNotice("");
          setOpen(true);
        }}
      >
        {id ? "编辑" : title}
      </Button>
      {notice && (
        <span role="status" className="text-xs text-muted">
          {notice}
        </span>
      )}
      {open && (
        <CommandDialog
          key={path + section + id}
          path={path}
          section={section}
          id={id}
          title={title}
          onClose={() => setOpen(false)}
          onSaved={() => {
            setNotice("已保存");
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
  id,
  title,
  onClose,
  onSaved,
}: {
  path: string;
  section: Section;
  id?: string;
  title: string;
  onClose: () => void;
  onSaved: () => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
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
        const item = value[section].find((item) => item.id === id);
        if (id && !item) throw new Error("这条配置已被移除，请关闭后刷新。");
        setConfig(value);
        setName(item?.name || "");
        setCommand(item ? stepsText(item) : "");
        setCwd(item?.cwd || "");
        setUrl(item && "url" in item ? item.url : "");
        setKind(item && "kind" in item ? item.kind : "ci");
        setError("");
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
  }, [path, section, id, revision]);
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
        if (busy) event.preventDefault();
        else onClose();
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
              正在读取…
            </p>
          ) : (
            config && (
              <fieldset disabled={busy} className="min-w-0 space-y-4">
                <label className="block space-y-1.5 text-sm font-medium">
                  名称
                  <input
                    autoFocus
                    required
                    className={inputClass}
                    placeholder={services ? "例如 前端开发" : "例如 项目检查"}
                    value={name}
                    onChange={(event) => setName(event.target.value)}
                  />
                </label>
                <label className="block space-y-1.5 text-sm font-medium">
                  命令
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
                <details>
                  <summary className="w-fit cursor-pointer text-sm text-muted">
                    更多选项
                  </summary>
                  <div className="mt-3 space-y-3">
                    <label className="block space-y-1.5 text-sm font-medium">
                      工作目录
                      <input
                        className={inputClass}
                        placeholder="留空使用项目根目录"
                        value={cwd}
                        onChange={(event) => setCwd(event.target.value)}
                      />
                    </label>
                    {services ? (
                      <label className="block space-y-1.5 text-sm font-medium">
                        页面地址
                        <input
                          type={
                            config?.services.find((entry) => entry.id === id)
                              ?.autoPort
                              ? "text"
                              : "url"
                          }
                          className={inputClass}
                          placeholder="可选，例如 http://localhost:5173"
                          value={url}
                          onChange={(event) => setUrl(event.target.value)}
                        />
                      </label>
                    ) : (
                      <label className="block space-y-1.5 text-sm font-medium">
                        用途
                        <Select
                          className="w-full"
                          value={kind}
                          onChange={(event) => setKind(event.target.value)}
                        >
                          <option value="ci">检查</option>
                          <option value="cd">发布</option>
                        </Select>
                      </label>
                    )}
                  </div>
                </details>
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
                重新载入
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
                移除
              </button>
            )}
          </div>
          <div className="flex gap-2">
            <Button type="button" disabled={busy} onClick={onClose}>
              取消
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
              {busy ? "保存中…" : "保存"}
            </Button>
          </div>
        </div>
      </form>
    </dialog>
  );
}
