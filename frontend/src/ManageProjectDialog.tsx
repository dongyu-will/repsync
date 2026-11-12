import { t } from "./i18n";
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
      const selected = await Desktop.chooseDirectory(
        path,
        t("Choose project folder"),
        t("Choose folder"),
      );
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
          {t("Manage")} {project.name}
        </h2>
        <p className="text-sm leading-6 text-muted">
          {t(
            "Update the path when a project moves. Removing a project keeps its files, logs, and backups.",
          )}
        </p>
        <label htmlFor="relocate-path" className="block text-sm font-medium">
          {t("Project folder")}
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
            {t("Choose folder")}
          </Button>
        </div>
        <p className="text-xs leading-5 text-muted">
          {t(
            "Updating the path does not move files. History stays with the original path.",
          )}
        </p>
        {error && (
          <p
            role="alert"
            className="break-words text-sm leading-6 text-red-800"
          >
            {t(error)}
          </p>
        )}
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            className="mr-auto text-removed"
            disabled={busy}
            onClick={() => void change(true)}
          >
            {t("Remove registration")}
          </Button>
          <Button type="button" disabled={busy} onClick={onClose}>
            {t("Cancel")}
          </Button>
          <Button
            primary
            type="submit"
            disabled={busy || !path.trim() || path.trim() === project.path}
          >
            {busy ? t("Working…") : t("Save path")}
          </Button>
        </div>
      </form>
    </dialog>
  );
}
