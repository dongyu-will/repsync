import { useEffect, useRef, useState } from "react";
import { Button } from "./components";
import { Desktop, Service, type Project } from "./mygo";

export default function ManageProjectDialog({
  project,
  onClose,
  onChanged,
}: {
  project: Project;
  onClose: () => void;
  onChanged: (next: Project | null) => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [path, setPath] = useState(project.path);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    dialog.current?.showModal();
  }, []);

  async function change(remove: boolean) {
    setBusy(true);
    setError("");
    try {
      if (remove) {
        await Service.removeProject(project.path);
        onChanged(null);
      } else
        onChanged(await Service.relocateProject(project.path, path.trim()));
    } catch (error) {
      setError(String(error));
    } finally {
      setBusy(false);
    }
  }

  async function browse() {
    setBusy(true);
    setError("");
    try {
      const selected = await Desktop.chooseDirectory(path);
      if (selected) setPath(selected);
    } catch (error) {
      setError(String(error));
    } finally {
      setBusy(false);
    }
  }

  return (
    <dialog
      ref={dialog}
      aria-labelledby="manage-project-title"
      onCancel={(event) => {
        if (busy) event.preventDefault();
        else onClose();
      }}
      onClose={onClose}
      className="m-auto max-h-[calc(100dvh-32px)] w-[480px] max-w-[calc(100vw-32px)] overflow-y-auto rounded-xl bg-white p-0 text-ink shadow-xl backdrop:bg-black/20"
    >
      <form
        className="space-y-4 p-5"
        onSubmit={(event) => {
          event.preventDefault();
          void change(false);
        }}
      >
        <h2 id="manage-project-title" className="text-xl font-semibold">
          管理 {project.name}
        </h2>
        <p className="text-sm leading-6 text-muted">
          更换目录用于修复已移动的项目。移除仅取消登记，项目文件、日志和备份都会保留。
        </p>
        <label htmlFor="relocate-path" className="block text-sm font-medium">
          项目目录
        </label>
        <div className="flex gap-2">
          <input
            id="relocate-path"
            autoFocus
            required
            value={path}
            disabled={busy}
            onChange={(event) => setPath(event.target.value)}
            autoCapitalize="none"
            autoCorrect="off"
            spellCheck={false}
            className="h-9 min-w-0 flex-1 rounded-md border border-control px-3 text-sm"
          />
          <Button type="button" disabled={busy} onClick={browse}>
            选择目录
          </Button>
        </div>
        <p className="text-xs leading-5 text-muted">
          更换目录不搬动文件；历史按原路径保留。
        </p>
        {error && (
          <p
            role="alert"
            className="break-words text-sm leading-6 text-red-800"
          >
            {error}
          </p>
        )}
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            className="mr-auto text-removed"
            disabled={busy}
            onClick={() => void change(true)}
          >
            移除登记
          </Button>
          <Button type="button" disabled={busy} onClick={onClose}>
            取消
          </Button>
          <Button
            primary
            type="submit"
            disabled={busy || !path.trim() || path.trim() === project.path}
          >
            {busy ? "正在处理…" : "保存目录"}
          </Button>
        </div>
      </form>
    </dialog>
  );
}
