import { getLocale, t } from "./i18n";
import { useEffect, useReducer, useRef } from "react";
import { isMyGo } from "mygo-runtime";
import {
  Service,
  type GitStash,
  type GitStashList,
  type GitStashPreview,
} from "./mygo";
import { Button, Empty } from "./components";

const native = isMyGo();
type State = {
  preview: GitStashPreview | null;
  loading: boolean;
  pending: boolean;
  error: string;
  notice: string;
  conflict: boolean;
};

export default function ProjectStashes({
  path,
  list,
  error,
  onChanged,
}: {
  path: string;
  list: GitStashList;
  error: string;
  onChanged: () => void;
}) {
  const [state, update] = useReducer(
    (s: State, p: Partial<State>) => ({ ...s, ...p }),
    {
      preview: null,
      loading: false,
      pending: false,
      error: "",
      notice: "",
      conflict: false,
    },
  );
  const request = useRef(0);
  useEffect(
    () => () => {
      request.current++;
    },
    [path],
  );

  async function show(stash: GitStash) {
    const id = ++request.current;
    update({
      loading: true,
      error: "",
      preview: null,
      notice: "",
      conflict: false,
    });
    try {
      const preview = native
        ? await Service.gitStashChanges(path, stash.oid)
        : {
            stash,
            version: list.version,
            branch: list.branch,
            truncated: false,
            patch:
              " src/settings.ts | 2 +-\n\ndiff --git a/src/settings.ts b/src/settings.ts\n--- a/src/settings.ts\n+++ b/src/settings.ts\n@@ -1 +1 @@\n-export const sidebarWidth = 176;\n+export const sidebarWidth = 192;\n",
          };
      if (id === request.current) update({ preview });
    } catch (error) {
      if (id === request.current)
        update({
          error: error instanceof Error ? error.message : String(error),
        });
    } finally {
      if (id === request.current) update({ loading: false });
    }
  }

  async function apply() {
    if (!native || !state.preview) return;
    const id = ++request.current;
    update({ pending: true, error: "", notice: "" });
    try {
      const result = await Service.applyGitStash(
        path,
        state.preview.stash.oid,
        state.preview.version,
      );
      if (id === request.current)
        update({ notice: result.message, conflict: !result.applied });
      onChanged();
    } catch (error) {
      if (id === request.current)
        update({
          error: error instanceof Error ? error.message : String(error),
        });
    } finally {
      if (id === request.current) update({ pending: false });
    }
  }

  if (error)
    return (
      <p role="alert" className="p-3 text-sm text-removed">
        {t("stash Read failed:")} {t(error)}
        {t(".Refresh and try again.")}
      </p>
    );
  if (!list.entries.length)
    return (
      <Empty icon="file" title={t("No pending stash")}>
        {t(
          "These entries are shared by all worktrees in this repository: stash.",
        )}
      </Empty>
    );
  return (
    <div className="space-y-4">
      <p className="text-xs leading-5 text-muted">
        {t(
          "All worktrees in the repository share these stash.Review the contents before applying to this worktree. The original stash is preserved.",
        )}
      </p>
      <div className="grouped-list">
        {list.entries.map((stash) => (
          <div
            key={`${stash.selector}:${stash.oid}`}
            className="flex min-w-0 items-start gap-3 border-b border-line-subtle p-3 last:border-0"
          >
            <div className="min-w-0 flex-1">
              <p className="break-words text-sm font-medium">{stash.subject}</p>
              <p className="mt-1 break-all text-xs text-muted">
                {stash.selector} ·{" "}
                {new Date(stash.createdAt).toLocaleString(getLocale(), {
                  hour12: false,
                })}{" "}
                · {stash.oid.slice(0, 8)}
              </p>
            </div>
            <Button
              disabled={state.pending || state.loading}
              onClick={() => show(stash)}
            >
              {t("View contents")}
            </Button>
          </div>
        ))}
      </div>
      {state.error && (
        <p
          role="alert"
          className="break-words rounded-md bg-red-50 p-3 text-sm text-red-800"
        >
          {t(state.error)}
        </p>
      )}
      {state.loading && (
        <p role="status" className="text-sm text-muted">
          {t("Loading stash changes…")}
        </p>
      )}
      {state.notice && (
        <p
          role={state.conflict ? "alert" : "status"}
          className={`rounded-md p-3 text-sm leading-6 ${state.conflict ? "bg-amber-50 text-amber-900" : "bg-sidebar text-ink"}`}
        >
          {t(state.notice)}
        </p>
      )}
      {state.preview && (
        <section
          aria-label={t("stash Content")}
          className="space-y-3 border-t border-line pt-4"
        >
          <div className="flex flex-wrap items-center justify-between gap-3">
            <h2 className="text-sm font-semibold">
              {state.preview.stash.selector} {t("· Changes")}
            </h2>
            <Button
              disabled={
                !native ||
                state.pending ||
                !state.preview.branch ||
                !!state.notice
              }
              onClick={apply}
            >
              {state.pending ? t("Restoring…") : t("Apply to this worktree")}
            </Button>
          </div>
          <p className="break-all text-xs leading-5 text-muted">
            {t("Apply to:")} {path} ·{" "}
            {state.preview.branch || t("Detached HEAD, Switch branches first")}
            {t(
              ".The current worktree must be clean. Conflicts preserve the worktree and original stash, Resolve this in the project.",
            )}
          </p>
          {state.preview.truncated && (
            <p role="status" className="text-xs text-removed">
              {t(
                "Changes exceed 1 MiB, Only the beginning is shown. Use Git to review the full diff before applying.",
              )}
            </p>
          )}
          <pre
            tabIndex={0}
            aria-label={t("stash File diff")}
            className="source-panel max-h-80"
          >
            {state.preview.patch}
          </pre>
        </section>
      )}
    </div>
  );
}
