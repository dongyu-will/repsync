import { useEffect, useRef, useState } from "react";
import { Button } from "./components";
import { Desktop, Service, type CloneState, type Project } from "./mygo";

const errorMessage = (error: unknown) =>
  error instanceof Error ? error.message : String(error);

function suggestedName(remote: string) {
  let path = remote.trim().replace(/\/+$/, "");
  try {
    path = new URL(path).pathname;
  } catch {
    /* SSH scp-style address. */
  }
  return (path.split(/[/:]/).pop() || "")
    .replace(/\.git$/i, "")
    .replace(/[^\p{L}\p{N}._-]+/gu, "-");
}

export default function AddProjectDialog({
  onClose,
  onAdded,
}: {
  onClose: () => void;
  onAdded: (project: Project) => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [mode, setMode] = useState<"local" | "remote">("local");
  const [path, setPath] = useState("");
  const [remote, setRemote] = useState("");
  const [parent, setParent] = useState("");
  const [name, setName] = useState<string | null>(null);
  const folder = name ?? suggestedName(remote);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const [picking, setPicking] = useState(false);
  const [stopping, setStopping] = useState(false);
  const [clone, setClone] = useState<CloneState | null>(null);
  const running = clone?.status === "running";
  const busy = saving || picking || running;
  const cloneID = clone?.id;

  useEffect(() => {
    dialog.current?.showModal();
  }, []);
  useEffect(() => {
    if (!cloneID || !running) return;
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      try {
        const state = await Service.cloneStatus(cloneID!);
        if (!active) return;
        setClone(state);
        if (state.status === "success") {
          onAdded(state.project);
          return;
        }
        if (state.status !== "running") {
          setStopping(false);
          return;
        }
      } catch (error) {
        if (active) setError(errorMessage(error));
      }
      if (active) timer = setTimeout(poll, 750);
    }
    void poll();
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [cloneID, running, onAdded]);

  async function browse() {
    setPicking(true);
    setError("");
    try {
      const selected = await Desktop.chooseDirectory(
        mode === "local" ? path : parent,
      );
      if (selected) {
        if (mode === "local") setPath(selected);
        else setParent(selected);
      }
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setPicking(false);
    }
  }

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    if (busy) return;
    setSaving(true);
    setError("");
    setClone(null);
    try {
      if (mode === "local") onAdded(await Service.add(path.trim()));
      else
        setClone(
          await Service.startClone(remote.trim(), parent.trim(), folder.trim()),
        );
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setSaving(false);
    }
  }

  async function stop() {
    if (!clone) return;
    setStopping(true);
    setError("");
    try {
      await Service.cancelClone(clone.id);
    } catch (error) {
      setError(errorMessage(error));
      setStopping(false);
    }
  }

  return (
    <dialog
      ref={dialog}
      aria-labelledby="add-project-title"
      onCancel={(event) => {
        if (busy) event.preventDefault();
        else onClose();
      }}
      onClose={onClose}
      className="m-auto max-h-[calc(100dvh-32px)] w-[480px] max-w-[calc(100vw-32px)] overflow-y-auto rounded-xl bg-white p-0 text-ink shadow-xl backdrop:bg-black/20"
    >
      <form onSubmit={submit} className="space-y-4 p-5">
        <h2 id="add-project-title" className="text-xl font-semibold">
          添加项目
        </h2>
        <div
          role="group"
          aria-label="项目来源"
          className="flex gap-1 rounded-md bg-surface p-1"
        >
          {(
            [
              ["local", "本地目录"],
              ["remote", "远程仓库"],
            ] as const
          ).map(([value, label]) => (
            <button
              key={value}
              type="button"
              aria-pressed={mode === value}
              disabled={busy}
              onClick={() => {
                setMode(value);
                setError("");
                setClone(null);
              }}
              className={`min-h-8 flex-1 rounded px-3 text-sm disabled:opacity-50 ${mode === value ? "bg-white font-medium text-ink" : "text-muted"}`}
            >
              {label}
            </button>
          ))}
        </div>
        {mode === "local" ? (
          <>
            <p className="text-sm text-muted">
              选择已有的项目目录，添加后直接打开。
            </p>
            <label className="block text-sm font-medium" htmlFor="project-path">
              项目目录
            </label>
            <div className="flex gap-2">
              <input
                id="project-path"
                autoFocus
                autoCapitalize="none"
                autoCorrect="off"
                spellCheck={false}
                required
                disabled={busy}
                value={path}
                onChange={(event) => setPath(event.target.value)}
                placeholder="/Users/you/Projects/my-project"
                className="h-9 min-w-0 flex-1 rounded-md border border-control px-3 text-sm"
              />
              <Button type="button" disabled={busy} onClick={browse}>
                选择目录
              </Button>
            </div>
          </>
        ) : (
          <>
            <p className="text-sm text-muted">
              克隆到本机后自动添加，使用本机已有的 Git 凭据或 SSH 密钥。
            </p>
            <label className="block text-sm font-medium" htmlFor="clone-remote">
              仓库地址
            </label>
            <input
              id="clone-remote"
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
              required
              disabled={busy}
              value={remote}
              onChange={(event) => setRemote(event.target.value)}
              placeholder="https://github.com/owner/repo.git 或 git@…"
              className="h-9 w-full rounded-md border border-control px-3 text-sm"
            />
            <label className="block text-sm font-medium" htmlFor="clone-parent">
              保存到
            </label>
            <div className="flex gap-2">
              <input
                id="clone-parent"
                autoCapitalize="none"
                autoCorrect="off"
                spellCheck={false}
                required
                disabled={busy}
                value={parent}
                onChange={(event) => setParent(event.target.value)}
                placeholder="/Users/you/Projects"
                className="h-9 min-w-0 flex-1 rounded-md border border-control px-3 text-sm"
              />
              <Button type="button" disabled={busy} onClick={browse}>
                选择目录
              </Button>
            </div>
            <label className="block text-sm font-medium" htmlFor="clone-name">
              目录名称
            </label>
            <input
              id="clone-name"
              required
              disabled={busy}
              value={folder}
              onChange={(event) => setName(event.target.value)}
              placeholder="my-project"
              className="h-9 w-full rounded-md border border-control px-3 text-sm"
            />
            {parent.trim() && folder.trim() && (
              <p className="break-all text-xs text-muted">
                {parent.trim().replace(/\/+$/, "")}/{folder.trim()}
              </p>
            )}
          </>
        )}
        {(error || clone?.error) && (
          <p
            role="alert"
            className="break-words text-sm leading-5 text-red-800"
          >
            {error || clone?.error}
          </p>
        )}
        {clone && (
          <section aria-label="仓库克隆进度" className="space-y-2">
            <p role="status" className="text-sm">
              {running
                ? stopping
                  ? "正在停止克隆…"
                  : "正在克隆仓库…"
                : clone.status === "success"
                  ? "克隆完成"
                  : "克隆未完成"}
            </p>
            {clone.log && (
              <pre
                className="source-panel max-h-40"
                tabIndex={0}
                role="region"
                aria-label="克隆输出"
              >
                {clone.log.replace(/\r/g, "\n")}
              </pre>
            )}
            {!running && (
              <p className="break-all text-xs text-muted">
                目标目录：{clone.destination}
              </p>
            )}
            {!running && clone.cloned && (
              <Button
                type="button"
                onClick={() => {
                  setPath(clone.destination);
                  setMode("local");
                  setClone(null);
                  setError("");
                }}
              >
                登记已克隆目录
              </Button>
            )}
          </section>
        )}
        <div className="flex justify-end gap-2">
          <Button type="button" disabled={busy} onClick={onClose}>
            取消
          </Button>
          {running ? (
            <Button type="button" disabled={stopping} onClick={stop}>
              {stopping ? "正在停止…" : "停止克隆"}
            </Button>
          ) : (
            <Button
              primary
              type="submit"
              disabled={
                busy ||
                (mode === "local"
                  ? !path.trim()
                  : !remote.trim() || !parent.trim() || !folder.trim())
              }
            >
              {saving
                ? "正在处理…"
                : mode === "local"
                  ? "添加项目"
                  : "克隆并添加"}
            </Button>
          )}
        </div>
      </form>
    </dialog>
  );
}
