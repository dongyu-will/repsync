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
        stash 读取失败：{error}。请刷新后重试。
      </p>
    );
  if (!list.entries.length)
    return (
      <Empty icon="file" title="没有待处理的 stash">
        这里显示同一仓库所有工作树共享的 stash。
      </Empty>
    );
  return (
    <div className="space-y-4">
      <p className="text-xs leading-5 text-muted">
        同一仓库的工作树共享这些 stash。查看内容后可恢复到当前工作树；原 stash
        会继续保留。
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
                {new Date(stash.createdAt).toLocaleString("zh-CN", {
                  hour12: false,
                })}{" "}
                · {stash.oid.slice(0, 8)}
              </p>
            </div>
            <Button
              disabled={state.pending || state.loading}
              onClick={() => show(stash)}
            >
              查看内容
            </Button>
          </div>
        ))}
      </div>
      {state.error && (
        <p
          role="alert"
          className="break-words rounded-md bg-red-50 p-3 text-sm text-red-800"
        >
          {state.error}
        </p>
      )}
      {state.loading && (
        <p role="status" className="text-sm text-muted">
          正在读取 stash 改动…
        </p>
      )}
      {state.notice && (
        <p
          role={state.conflict ? "alert" : "status"}
          className={`rounded-md p-3 text-sm leading-6 ${state.conflict ? "bg-amber-50 text-amber-900" : "bg-sidebar text-ink"}`}
        >
          {state.notice}
        </p>
      )}
      {state.preview && (
        <section
          aria-label="stash 内容"
          className="space-y-3 border-t border-line pt-4"
        >
          <div className="flex flex-wrap items-center justify-between gap-3">
            <h2 className="text-sm font-semibold">
              {state.preview.stash.selector} · 改动内容
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
              {state.pending ? "正在恢复…" : "恢复到当前工作树"}
            </Button>
          </div>
          <p className="break-all text-xs leading-5 text-muted">
            恢复目标：{path} ·{" "}
            {state.preview.branch || "分离 HEAD，需先切换分支"}
            。当前工作树需干净；出现冲突时保留现场和原 stash，请在项目中处理。
          </p>
          {state.preview.truncated && (
            <p role="status" className="text-xs text-removed">
              改动超过 1 MiB，仅展示前部内容；请使用 Git 查看完整改动后再恢复。
            </p>
          )}
          <pre
            tabIndex={0}
            aria-label="stash 文件差异"
            className="source-panel max-h-80"
          >
            {state.preview.patch}
          </pre>
        </section>
      )}
    </div>
  );
}
