import { formatMessage, getLocale, t } from "./i18n";
import ActionDialog from "./ActionDialog";
import { useEffect, useReducer, useRef, type ReactNode } from "react";
import { isMyGo } from "mygo-runtime";
import { Service, type GitOverview, type GitCleanupTarget } from "./mygo";
import { Button, Empty, Icon } from "./components";
import { gitPreview } from "./git-preview";
import ProjectStashes from "./ProjectStashes";
import AISetup from "./AISetup";

const native = isMyGo();
const message = (error: unknown) =>
  error instanceof Error ? error.message : String(error);
const date = (value: string) =>
  new Date(value).toLocaleString(getLocale(), { hour12: false });

export type GitSection = "worktrees" | "branches" | "stashes";
type State = {
  view: GitOverview | null;
  section: GitSection;
  error: string;
  notice: string;
  pending: boolean;
  inspectingCleanup: boolean;
  revision: number;
  preview: GitOverview | null;
};

export default function ProjectGit({
  path,
  initialSection = "worktrees",
  onUpdated,
}: {
  path: string;
  initialSection?: GitSection;
  onUpdated: () => void;
}) {
  const [state, update] = useReducer(
    (s: State, patch: Partial<State>) => ({ ...s, ...patch }),
    {
      view: native ? null : gitPreview(path),
      section: initialSection,
      error: "",
      notice: "",
      pending: false,
      inspectingCleanup: false,
      revision: 0,
      preview: null,
    },
  );
  const { view, section, error, notice, pending, preview, inspectingCleanup } =
    state;
  useEffect(() => {
    if (!native) return;
    let active = true;
    let request = 0;
    const load = () => {
      const current = ++request;
      void Service.gitWorkspaceSummary(path)
        .then((view) => {
          if (active && current === request) update({ view, error: "" });
        })
        .catch((error) => {
          if (active && current === request) update({ error: message(error) });
        });
    };
    load();
    const timer = window.setInterval(load, 60000);
    window.addEventListener("focus", load);
    return () => {
      active = false;
      window.clearInterval(timer);
      window.removeEventListener("focus", load);
    };
  }, [path, state.revision]);

  async function perform(operation: () => Promise<unknown>, success: string) {
    if (!native || pending) return;
    update({ pending: true, error: "", notice: "" });
    try {
      await operation();
      update({ preview: null, ...(success ? { notice: success } : {}) });
      const next = await Service.gitWorkspaceSummary(path);
      update({ view: next });
    } catch (error) {
      update({ error: message(error) });
    } finally {
      update({ pending: false });
    }
  }

  async function inspectCleanup() {
    if (!view || pending) return;
    if (!native) {
      update({ preview: view, error: "", notice: "" });
      return;
    }
    update({ pending: true, inspectingCleanup: true, error: "", notice: "" });
    try {
      const inspected = await Service.gitWorkspace(path);
      update({ view: inspected, preview: inspected });
    } catch (error) {
      update({ error: message(error) });
    } finally {
      update({ pending: false, inspectingCleanup: false });
    }
  }

  const candidates = (data: GitOverview): GitCleanupTarget[] => [
    ...data.worktrees
      .filter((w) => w.eligible)
      .map((w) => ({ kind: "worktree", target: w.path })),
    ...data.branches
      .filter((b) => b.eligible)
      .map((b) => ({ kind: "branch", target: b.name })),
  ];

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div
          className="flex gap-1 rounded-lg bg-sidebar p-1"
          role="group"
          aria-label={t("Git View scope")}
        >
          {(
            [
              ["worktrees", "Worktrees"],
              ["branches", "Local branches"],
              ["stashes", "Stash"],
            ] as const
          ).map(([id, label]) => (
            <button
              key={id}
              aria-pressed={section === id}
              onClick={() => update({ section: id })}
              className={`min-h-8 rounded-md px-3 text-sm ${section === id ? "bg-white font-medium" : "text-muted hover:text-ink"}`}
            >
              {t(label)}
              {view?.git
                ? ` · ${id === "worktrees" ? view.worktrees.length : id === "branches" ? view.branches.length : view.stashError ? "?" : view.stashes.entries.length}`
                : ""}
            </button>
          ))}
        </div>
        <div className="flex items-center gap-2">
          <Button
            disabled={!view?.git || pending}
            onClick={() => void inspectCleanup()}
          >
            {t(inspectingCleanup ? "Inspecting cleanup…" : "Manual cleanup")}
          </Button>
          <button
            className="icon-button text-muted disabled:opacity-45"
            aria-label={t("Refresh worktrees and branches")}
            title={t("Refresh worktrees and branches")}
            disabled={!native || pending}
            onClick={() => update({ revision: state.revision + 1, error: "" })}
          >
            <Icon name="refresh" size={16} />
          </button>
        </div>
      </div>
      {view?.git && !view.cleanupChecked && (
        <p className="text-xs text-muted">
          {t("Cleanup checks run when you open Manual cleanup.")}
        </p>
      )}
      {error && (
        <p
          role="alert"
          className="break-words rounded-md bg-red-50 p-3 text-sm text-red-800"
        >
          {t(error)}
          <button
            className="ml-2 underline"
            disabled={pending}
            onClick={() =>
              update({ revision: state.revision + 1, preview: null })
            }
          >
            {t("Reload")}
          </button>
        </p>
      )}
      {notice && (
        <p role="status" className="text-sm text-muted">
          {t(notice)}
        </p>
      )}
      {!view ? (
        <p role="status" className="py-8 text-sm text-muted">
          {t("Loading worktrees and branches…")}
        </p>
      ) : !view.git ? (
        <Empty title={t("This folder is not a Git Repository")}>
          {t("Worktrees and branches require Git project.")}
        </Empty>
      ) : (
        <>
          {view.warnings.map((w) => (
            <p key={w} role="status" className="text-sm text-removed">
              {t(w)}
            </p>
          ))}
          {section === "stashes" ? (
            <ProjectStashes
              path={path}
              list={view.stashes}
              error={view.stashError}
              onChanged={() => {
                onUpdated();
                update({ revision: state.revision + 1 });
              }}
            />
          ) : (
            <div className="grouped-list">
              {section === "worktrees" ? (
                view.worktrees.map((w) => (
                  <GitRow
                    key={w.path}
                    name={w.branch || t("Detached HEAD")}
                    marker={
                      w.primary ? "Main worktree" : w.locked ? "Locked" : ""
                    }
                    status={
                      w.eligible
                        ? "Eligible for cleanup"
                        : w.missing
                          ? "Folder missing"
                          : !w.statusKnown
                            ? "Status unknown"
                            : w.dirty
                              ? "Contains local files"
                              : "Clean"
                    }
                    eligible={w.eligible}
                  >
                    <p className="break-all">{w.path}</p>
                    {w.locked && w.primary && <p>{t("Locked")}</p>}
                    <p>
                      {w.eligible
                        ? t("Merged and past retention")
                        : w.primary
                          ? t("Main worktree is always kept")
                          : w.missing
                            ? t("Folder missing. Registration kept for repair.")
                            : w.reasons
                                .filter(
                                  (reason) => !view.warnings.includes(reason),
                                )
                                .map((reason) => t(reason))
                                .join("; ")}
                    </p>
                  </GitRow>
                ))
              ) : view.branches.length ? (
                view.branches.map((b) => (
                  <GitRow
                    key={b.name}
                    name={b.name}
                    status={
                      b.eligible
                        ? "Eligible for cleanup"
                        : b.merged
                          ? "Merged"
                          : "Unmerged"
                    }
                    eligible={b.eligible}
                  >
                    <p className="break-all">
                      {b.commit.slice(0, 8)}
                      {b.upstream
                        ? t(" · Track {0}", b.upstream)
                        : t(" · No upstream")}
                      {b.updatedAt ? ` · ${date(b.updatedAt)}` : ""}
                    </p>
                    {b.worktrees.length > 0 && (
                      <p className="break-all">
                        {t("Checked out at")} {b.worktrees.join(", ")}
                      </p>
                    )}
                    <p>
                      {b.eligible
                        ? t("Merged and past retention")
                        : b.reasons
                            .filter((reason) => !view.warnings.includes(reason))
                            .map((reason) => t(reason))
                            .join("; ")}
                    </p>
                  </GitRow>
                ))
              ) : (
                <p className="p-3 text-sm text-muted">
                  {t("No local branches yet. Create the first commit.")}
                </p>
              )}
            </div>
          )}
          <div className="flex flex-wrap items-center gap-2">
            <ActionDialog
              title={
                <>
                  {t("Automatic cleanup")}
                  <span className="ml-2 text-xs font-normal text-muted">
                    {view.policy.enabled
                      ? t("Enabled · Keep {0}  days", view.policy.retentionDays)
                      : t("Disabled")}
                  </span>
                </>
              }
              busy={pending}
              error={error}
              notice={notice}
            >
              <div className="space-y-3 text-sm">
                <p>
                  {t("Merged into base branch")}:{" "}
                  {view.policy.baseBranch || t("Not provided")}
                </p>
                <p>
                  {t("Retention days")}: {view.policy.retentionDays}
                </p>
                <AISetup path={path} task="cleanup" />
              </div>
            </ActionDialog>
            <ActionDialog
              title={
                <>
                  {t("Cleanup history and restore")}
                  {view.history.length ? ` · ${view.history.length}` : ""}
                </>
              }
              busy={pending}
              error={error}
              notice={notice}
            >
              <div className="mt-3 space-y-3">
                {view.history.length ? (
                  view.history.map((r) => (
                    <div
                      key={r.id}
                      className="rounded-lg border border-line p-3 text-sm"
                    >
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <span>
                          {date(r.createdAt)} ·{" "}
                          {r.automatic
                            ? t("Automatic cleanup")
                            : t("Manual cleanup")}
                        </span>
                        <Button
                          disabled={
                            !native ||
                            pending ||
                            !!r.restoredAt ||
                            !r.items.some(
                              (i) =>
                                i.status === "removed" ||
                                i.status === "pending",
                            )
                          }
                          onClick={() =>
                            perform(
                              () => Service.restoreGitCleanup(path, r.id),
                              "Restored. Retention restarts for restored content.",
                            )
                          }
                        >
                          {r.restoredAt ? t("Restored") : t("Restore")}
                        </Button>
                      </div>
                      <ul className="mt-2 space-y-1 text-xs text-muted">
                        {r.items.map((item, i) => (
                          <li key={i} className="break-all">
                            {item.kind === "worktree"
                              ? t("Worktrees")
                              : t("Branch")}{" "}
                            · {item.target} ·{" "}
                            {t(
                              (
                                {
                                  removed: "Removed",
                                  restored: "Restored",
                                  skipped: "Kept",
                                  pending: "Needs review",
                                } as Record<string, string>
                              )[item.status] || item.status,
                            )}
                            {item.error ? `: ${t(item.error)}` : ""}
                          </li>
                        ))}
                      </ul>
                    </div>
                  ))
                ) : (
                  <p className="text-sm text-muted">
                    {t("No cleanup history yet.")}
                  </p>
                )}
              </div>
            </ActionDialog>
          </div>
        </>
      )}
      {preview && (
        <CleanupPreview
          pending={pending}
          items={candidates(preview)}
          branches={preview.branches}
          error={error}
          onClose={() => update({ preview: null })}
          onApply={(targets) =>
            perform(async () => {
              const result = await Service.cleanupGitSelection(
                path,
                preview.version,
                targets,
              );
              const removed = result.items.filter(
                (i) => i.status === "removed",
              ).length;
              const skipped = result.items.filter(
                (i) => i.status !== "removed",
              ).length;
              update({
                notice: formatMessage(
                  "Removed {0} items; kept {1}. See cleanup history for details.",
                  removed,
                  skipped,
                ),
              });
            }, "")
          }
        >
          <p className="text-sm leading-6 text-muted">
            {t(
              "Choose the worktrees and local branches to remove. Only selected eligible items will be cleaned up; retention and workspace protections still apply. Recovery refs preserve commits.",
            )}
          </p>
          {preview.warnings.map((warning) => (
            <p key={warning} className="text-sm text-removed">
              {t(warning)}
            </p>
          ))}
          {!candidates(preview).length && (
            <p className="py-3 text-sm">
              {t("Nothing to clean up. See the list for retention reasons.")}
            </p>
          )}
          {!candidates(preview).length && (
            <ul className="max-h-[40dvh] space-y-3 overflow-auto text-xs text-muted">
              {preview.worktrees
                .filter((item) => !item.eligible)
                .map((item) => (
                  <li key={item.path}>
                    <p className="break-all font-medium">{item.path}</p>
                    <p className="mt-1 break-words">
                      {item.reasons.map((reason) => t(reason)).join("; ")}
                    </p>
                  </li>
                ))}
              {preview.branches
                .filter((item) => !item.eligible)
                .map((item) => (
                  <li key={`branch:${item.name}`}>
                    <p className="break-all font-medium">
                      {t("Branch")} · {item.name}
                    </p>
                    <p className="mt-1 break-words">
                      {item.reasons.map((reason) => t(reason)).join("; ")}
                    </p>
                  </li>
                ))}
            </ul>
          )}
        </CleanupPreview>
      )}
    </div>
  );
}

function CleanupPreview({
  children,
  items,
  branches,
  pending,
  error,
  onClose,
  onApply,
}: {
  children: React.ReactNode;
  items: GitCleanupTarget[];
  branches: GitOverview["branches"];
  pending: boolean;
  error: string;
  onClose: () => void;
  onApply: (targets: GitCleanupTarget[]) => void;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const [selected, select] = useReducer(
    (_current: string[], next: string[]) => next,
    [],
  );
  const key = (item: GitCleanupTarget) =>
    JSON.stringify([item.kind, item.target]);
  const targets = items.filter((item) => selected.includes(key(item)));
  const missingWorktrees = branches
    .filter((branch) =>
      targets.some(
        (item) => item.kind === "branch" && item.target === branch.name,
      ),
    )
    .flatMap((branch) => branch.worktrees)
    .filter(
      (path) =>
        !targets.some(
          (item) => item.kind === "worktree" && item.target === path,
        ),
    );
  useEffect(() => {
    const dialog = ref.current;
    dialog
      ?.querySelector<HTMLButtonElement>("[data-cleanup-cancel]")
      ?.setAttribute("autofocus", "");
    dialog?.showModal();
  }, []);
  const close = () => {
    if (pending) return;
    ref.current?.close();
    onClose();
  };
  return (
    <dialog
      ref={ref}
      aria-labelledby="git-cleanup-title"
      onCancel={(event) => {
        event.preventDefault();
        close();
      }}
      className="m-auto max-h-[calc(100dvh-32px)] w-[520px] max-w-[calc(100vw-32px)] overflow-auto overscroll-contain rounded-xl bg-white p-4 text-ink shadow-xl backdrop:overflow-hidden backdrop:overscroll-contain backdrop:bg-black/20"
    >
      <div className="mb-3 flex items-center gap-3">
        <h2 id="git-cleanup-title" className="flex-1 text-lg font-semibold">
          {t("Manual cleanup")}
        </h2>
        <button
          className="icon-button"
          aria-label={t("Close cleanup preview")}
          disabled={pending}
          onClick={close}
        >
          <Icon name="close" />
        </button>
      </div>
      {children}
      {items.length > 0 && (
        <>
          <div className="mt-4 flex flex-wrap items-center justify-between gap-2 text-sm">
            <span>
              {t(
                "Selected {0} of {1} eligible items",
                targets.length,
                items.length,
              )}
            </span>
            <Button
              disabled={pending}
              onClick={() =>
                select(targets.length === items.length ? [] : items.map(key))
              }
            >
              {t(
                targets.length === items.length
                  ? "Clear selection"
                  : "Select all eligible items",
              )}
            </Button>
          </div>
          <ul className="grouped-list mt-3 max-h-[40dvh] overflow-auto">
            {items.map((item) => (
              <li key={key(item)}>
                <label className="flex min-h-11 cursor-pointer items-start gap-3 px-3 py-3 text-sm">
                  <input
                    type="checkbox"
                    aria-label={t(
                      item.kind === "worktree"
                        ? "Select worktree {0}"
                        : "Select branch {0}",
                      item.target,
                    )}
                    className="mt-0.5 size-4 shrink-0 accent-accent"
                    disabled={pending}
                    checked={selected.includes(key(item))}
                    onChange={(event) =>
                      select(
                        event.target.checked
                          ? [...selected, key(item)]
                          : selected.filter((value) => value !== key(item)),
                      )
                    }
                  />
                  <span className="min-w-0 break-all">
                    <span className="mr-2 text-muted">
                      {t(item.kind === "worktree" ? "Worktrees" : "Branch")}
                    </span>
                    {item.target}
                  </span>
                </label>
              </li>
            ))}
          </ul>
        </>
      )}
      {missingWorktrees.length > 0 && (
        <p className="mt-3 break-all text-sm text-removed">
          {t(
            "Also select the linked worktrees: {0}",
            missingWorktrees.join(", "),
          )}
        </p>
      )}
      {error && (
        <p role="alert" className="mt-3 break-words text-sm text-removed">
          {t(error)}
        </p>
      )}
      {!native && targets.length > 0 && (
        <p className="mt-3 text-xs text-muted">
          {t("Open the desktop app to execute this cleanup.")}
        </p>
      )}
      <div className="mt-4 flex justify-end gap-2">
        <Button data-cleanup-cancel="" disabled={pending} onClick={close}>
          {t("Cancel")}
        </Button>
        <Button
          primary
          disabled={
            !native ||
            pending ||
            targets.length === 0 ||
            missingWorktrees.length > 0
          }
          onClick={() => onApply(targets)}
        >
          {pending
            ? t("Cleaning up…")
            : t("Clean up {0}  items", targets.length)}
        </Button>
      </div>
    </dialog>
  );
}

function GitRow({
  name,
  status,
  marker,
  eligible,
  children,
}: {
  name: string;
  status: string;
  marker?: string;
  eligible: boolean;
  children: ReactNode;
}) {
  return (
    <section className="border-b border-line-subtle last:border-0">
      <h3 className="flex min-h-11 items-center gap-3 px-3 py-2">
        <span className="flex min-w-0 flex-1 flex-wrap items-baseline gap-x-2">
          <span className="font-medium [overflow-wrap:anywhere]">{name}</span>
          {marker && <span className="text-xs text-muted">{t(marker)}</span>}
        </span>
        <span
          className={`shrink-0 text-xs ${eligible ? "text-green-700" : "text-muted"}`}
        >
          {t(status)}
        </span>
      </h3>
      <div className="space-y-1 border-t border-line-subtle bg-editor px-3 py-2 text-xs leading-5 text-muted">
        {children}
      </div>
    </section>
  );
}
