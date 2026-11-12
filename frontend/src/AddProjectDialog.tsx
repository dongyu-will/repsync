import { t } from "./i18n";
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
        t("Choose project folder"),
        t("Choose folder"),
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
          {t("Add project")}
        </h2>
        <div
          role="group"
          aria-label={t("Project source")}
          className="flex gap-1 rounded-md bg-surface p-1"
        >
          {(
            [
              ["local", "Local folder"],
              ["remote", "Remote repository"],
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
              {t(label)}
            </button>
          ))}
        </div>
        {mode === "local" ? (
          <>
            <p className="text-sm text-muted">
              {t("Choose an existing project folder to add and open it.")}
            </p>
            <label className="block text-sm font-medium" htmlFor="project-path">
              {t("Project folder")}
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
                {t("Choose folder")}
              </Button>
            </div>
          </>
        ) : (
          <>
            <p className="text-sm text-muted">
              {t(
                "Clone and register a repository using your existing Git credentials or SSH keys.",
              )}
            </p>
            <label className="block text-sm font-medium" htmlFor="clone-remote">
              {t("Repository URL")}
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
              placeholder={t("https://github.com/owner/repo.git or git@…")}
              className="h-9 w-full rounded-md border border-control px-3 text-sm"
            />
            <label className="block text-sm font-medium" htmlFor="clone-parent">
              {t("Save to")}
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
                {t("Choose folder")}
              </Button>
            </div>
            <label className="block text-sm font-medium" htmlFor="clone-name">
              {t("Folder name")}
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
            {t(error || clone?.error)}
          </p>
        )}
        {clone && (
          <section aria-label={t("Clone progress")} className="space-y-2">
            <p role="status" className="text-sm">
              {running
                ? stopping
                  ? t("Stopping clone…")
                  : t("Cloning repository…")
                : clone.status === "success"
                  ? t("Clone complete")
                  : t("Clone incomplete")}
            </p>
            {clone.log && (
              <pre
                className="source-panel max-h-40"
                tabIndex={0}
                role="region"
                aria-label={t("Clone output")}
              >
                {clone.log.replace(/\r/g, "\n")}
              </pre>
            )}
            {!running && (
              <p className="break-all text-xs text-muted">
                {t("Destination:")} {clone.destination}
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
                {t("Register cloned folder")}
              </Button>
            )}
          </section>
        )}
        <div className="flex justify-end gap-2">
          <Button type="button" disabled={busy} onClick={onClose}>
            {t("Cancel")}
          </Button>
          {running ? (
            <Button type="button" disabled={stopping} onClick={stop}>
              {stopping ? t("Stopping…") : t("Stop cloning")}
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
                ? t("Working…")
                : mode === "local"
                  ? t("Add project")
                  : t("Clone and add")}
            </Button>
          )}
        </div>
      </form>
    </dialog>
  );
}
