import { useEffect, useReducer, useRef } from "react";
import { isMyGo } from "mygo-runtime";
import { Service, type GitCleanupPolicy, type GitOverview } from "./mygo";
import { Button, Empty, Icon, Select } from "./components";
import { gitPreview } from "./git-preview";
import ProjectStashes from "./ProjectStashes";

const native = isMyGo();
const message = (error: unknown) =>
  error instanceof Error ? error.message : String(error);
const date = (value: string) =>
  new Date(value).toLocaleString("zh-CN", { hour12: false });

export type GitSection = "worktrees" | "branches" | "stashes";
type State = {
  view: GitOverview | null;
  section: GitSection;
  error: string;
  notice: string;
  pending: boolean;
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
      revision: 0,
      preview: null,
    },
  );
  const { view, section, error, notice, pending, preview } = state;
  useEffect(() => {
    if (!native) return;
    let active = true;
    let request = 0;
    const load = () => {
      const current = ++request;
      void Service.gitWorkspace(path)
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
    update({ pending: true, error: "", notice: "" });
    try {
      await operation();
      update({ preview: null, ...(success ? { notice: success } : {}) });
      const next = await Service.gitWorkspace(path);
      update({ view: next });
    } catch (error) {
      update({ error: message(error) });
    } finally {
      update({ pending: false });
    }
  }

  const candidates = (data: GitOverview) => [
    ...data.worktrees
      .filter((w) => w.eligible)
      .map((w) => ({ kind: "工作树", name: w.path })),
    ...data.branches
      .filter((b) => b.eligible)
      .map((b) => ({ kind: "本地分支", name: b.name })),
  ];

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div
          className="flex gap-1 rounded-lg bg-sidebar p-1"
          role="group"
          aria-label="Git 查看范围"
        >
          {(
            [
              ["worktrees", "工作树"],
              ["branches", "本地分支"],
              ["stashes", "Stash"],
            ] as const
          ).map(([id, label]) => (
            <button
              key={id}
              aria-pressed={section === id}
              onClick={() => update({ section: id })}
              className={`min-h-8 rounded-md px-3 text-sm ${section === id ? "bg-white font-medium" : "text-muted hover:text-ink"}`}
            >
              {label}
              {view?.git
                ? ` · ${id === "worktrees" ? view.worktrees.length : id === "branches" ? view.branches.length : view.stashError ? "?" : view.stashes.entries.length}`
                : ""}
            </button>
          ))}
        </div>
        <div className="flex items-center gap-2">
          <Button
            disabled={!native || !view?.git || pending}
            onClick={() =>
              view && update({ preview: view, error: "", notice: "" })
            }
          >
            预览清理
          </Button>
          <button
            className="icon-button text-muted disabled:opacity-45"
            aria-label="刷新工作树和分支"
            title="刷新工作树和分支"
            disabled={!native || pending}
            onClick={() => update({ revision: state.revision + 1, error: "" })}
          >
            <Icon name="refresh" size={16} />
          </button>
        </div>
      </div>
      {error && (
        <p
          role="alert"
          className="break-words rounded-md bg-red-50 p-3 text-sm text-red-800"
        >
          {error}
          <button
            className="ml-2 underline"
            disabled={pending}
            onClick={() =>
              update({ revision: state.revision + 1, preview: null })
            }
          >
            重新读取
          </button>
        </p>
      )}
      {notice && (
        <p role="status" className="text-sm text-muted">
          {notice}
        </p>
      )}
      {!view ? (
        <p role="status" className="py-8 text-sm text-muted">
          正在读取工作树和分支…
        </p>
      ) : !view.git ? (
        <Empty title="此目录不是 Git 仓库">
          工作树和分支仅适用于 Git 项目。
        </Empty>
      ) : (
        <>
          {view.warnings.map((w) => (
            <p key={w} role="status" className="text-sm text-removed">
              {w}
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
                  <div
                    key={w.path}
                    className="min-w-0 space-y-1 border-b border-line-subtle px-3 py-3 last:border-0"
                  >
                    <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
                      <span className="inline-flex min-w-0 items-center gap-2 font-medium">
                        <Icon name="branch" size={16} />
                        <span className="break-all">
                          {w.branch || "分离 HEAD"}
                        </span>
                      </span>
                      {w.primary && (
                        <span className="text-xs text-muted">主工作树</span>
                      )}
                      {w.locked && (
                        <span className="text-xs text-muted">已锁定</span>
                      )}
                      <span
                        className={`ml-auto text-xs ${w.eligible ? "text-green-700" : "text-muted"}`}
                      >
                        {w.eligible
                          ? "可清理"
                          : w.missing
                            ? "目录缺失"
                            : !w.statusKnown
                              ? "状态未知"
                              : w.dirty
                                ? "有本地文件"
                                : "干净"}
                      </span>
                    </div>
                    <p className="break-all text-xs text-muted">{w.path}</p>
                    <p className="text-xs leading-5 text-muted">
                      {w.eligible
                        ? "已合并且达到保留期"
                        : w.primary
                          ? "主工作树始终保留"
                          : w.missing
                            ? "目录缺失，保留登记供修复"
                            : w.reasons.join("；")}
                    </p>
                  </div>
                ))
              ) : view.branches.length ? (
                view.branches.map((b) => (
                  <div
                    key={b.name}
                    className="min-w-0 space-y-1 border-b border-line-subtle px-3 py-3 last:border-0"
                  >
                    <div className="flex flex-wrap items-center gap-2 text-sm">
                      <span className="break-all font-medium">{b.name}</span>
                      <span className="ml-auto text-xs text-muted">
                        {b.eligible ? "可清理" : b.merged ? "已合并" : "未合并"}
                      </span>
                    </div>
                    <p className="break-all text-xs text-muted">
                      {b.commit.slice(0, 8)}
                      {b.upstream ? ` · 跟踪 ${b.upstream}` : " · 无上游"}
                      {b.updatedAt ? ` · ${date(b.updatedAt)}` : ""}
                    </p>
                    {b.worktrees.length > 0 && (
                      <p className="break-all text-xs text-muted">
                        检出于 {b.worktrees.join("、")}
                      </p>
                    )}
                    <p className="text-xs leading-5 text-muted">
                      {b.eligible ? "已合并且达到保留期" : b.reasons.join("；")}
                    </p>
                  </div>
                ))
              ) : (
                <p className="p-3 text-sm text-muted">
                  还没有本地分支，请先完成首次提交。
                </p>
              )}
            </div>
          )}
          <details className="border-t border-line pt-4">
            <summary className="cursor-pointer text-sm font-medium">
              自动清理
              <span className="ml-2 text-xs font-normal text-muted">
                {view.policy.enabled
                  ? `已开启 · 保留 ${view.policy.retentionDays} 天`
                  : "未开启"}
              </span>
            </summary>
            <CleanupSettings
              key={view.policyVersion}
              policy={view.policy}
              branches={view.branches.map((b) => b.name)}
              pending={pending}
              onSave={(policy) =>
                perform(
                  () =>
                    Service.saveGitCleanupPolicy(
                      path,
                      view.policyVersion,
                      policy,
                    ),
                  "清理设置已保存",
                )
              }
            />
          </details>
          <details className="border-t border-line pt-4">
            <summary className="cursor-pointer text-sm font-medium">
              清理记录与恢复
              {view.history.length ? ` · ${view.history.length}` : ""}
            </summary>
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
                        {r.automatic ? "自动清理" : "手动清理"}
                      </span>
                      <Button
                        disabled={
                          !native ||
                          pending ||
                          !!r.restoredAt ||
                          !r.items.some(
                            (i) =>
                              i.status === "removed" || i.status === "pending",
                          )
                        }
                        onClick={() =>
                          perform(
                            () => Service.restoreGitCleanup(path, r.id),
                            "恢复完成；恢复的内容会重新计算保留期",
                          )
                        }
                      >
                        {r.restoredAt ? "已恢复" : "恢复"}
                      </Button>
                    </div>
                    <ul className="mt-2 space-y-1 text-xs text-muted">
                      {r.items.map((item, i) => (
                        <li key={i} className="break-all">
                          {item.kind === "worktree" ? "工作树" : "分支"} ·{" "}
                          {item.target} ·{" "}
                          {(
                            {
                              removed: "已清理",
                              restored: "已恢复",
                              skipped: "已保留",
                              pending: "待核对",
                            } as Record<string, string>
                          )[item.status] || item.status}
                          {item.error ? `：${item.error}` : ""}
                        </li>
                      ))}
                    </ul>
                  </div>
                ))
              ) : (
                <p className="text-sm text-muted">尚无清理记录。</p>
              )}
            </div>
          </details>
        </>
      )}
      {preview && (
        <CleanupPreview
          pending={pending}
          count={candidates(preview).length}
          onClose={() => update({ preview: null })}
          onApply={() =>
            perform(async () => {
              const result = await Service.cleanupGitWorkspace(
                path,
                preview.version,
              );
              const removed = result.items.filter(
                (i) => i.status === "removed",
              ).length;
              const skipped = result.items.filter(
                (i) => i.status !== "removed",
              ).length;
              update({
                notice: `已清理 ${removed} 项${skipped ? `，保留 ${skipped} 项，原因见清理记录` : ""}`,
              });
            }, "")
          }
        >
          <p className="text-sm leading-6 text-muted">
            仅移除以下工作树和本地分支。提交会保留恢复引用；有修改、忽略文件、未合并内容或正在使用的工作树会保留。
          </p>
          {candidates(preview).length ? (
            <ul className="grouped-list mt-4 max-h-[40dvh] overflow-auto">
              {candidates(preview).map((c) => (
                <li
                  key={`${c.kind}:${c.name}`}
                  className="break-all border-b border-line-subtle p-3 text-sm last:border-0"
                >
                  <span className="mr-2 text-muted">{c.kind}</span>
                  {c.name}
                </li>
              ))}
            </ul>
          ) : (
            <p className="py-5 text-sm">目前没有可清理内容。保留原因见列表。</p>
          )}
        </CleanupPreview>
      )}
    </div>
  );
}

function CleanupSettings({
  policy,
  branches,
  pending,
  onSave,
}: {
  policy: GitCleanupPolicy;
  branches: string[];
  pending: boolean;
  onSave: (policy: GitCleanupPolicy) => void;
}) {
  const [draft, update] = useReducer(
    (s: GitCleanupPolicy, p: Partial<GitCleanupPolicy>) => ({ ...s, ...p }),
    policy,
  );
  return (
    <div className="mt-3 space-y-3 text-sm">
      <label className="flex items-center gap-2">
        <input
          type="checkbox"
          className="size-4 accent-accent"
          checked={draft.enabled}
          disabled={!native || pending}
          onChange={(e) => update({ enabled: e.target.checked })}
        />
        为此仓库开启自动清理
      </label>
      <div className="flex flex-wrap items-end gap-3">
        <label className="min-w-0 space-y-1">
          <span className="block text-xs text-muted">合并到基准分支</span>
          <Select
            value={draft.baseBranch}
            disabled={!native || pending}
            onChange={(e) => update({ baseBranch: e.target.value })}
          >
            {!draft.baseBranch && <option value="">选择本地分支</option>}
            {branches.map((b) => (
              <option key={b} value={b}>
                {b}
              </option>
            ))}
          </Select>
        </label>
        <label className="space-y-1">
          <span className="block text-xs text-muted">保留天数</span>
          <input
            className="h-9 w-24 rounded-md border border-control px-3"
            type="number"
            min={1}
            max={365}
            value={draft.retentionDays}
            disabled={!native || pending}
            onChange={(e) => update({ retentionDays: Number(e.target.value) })}
          />
        </label>
        <Button
          disabled={
            !native ||
            pending ||
            !draft.baseBranch ||
            draft.retentionDays < 1 ||
            draft.retentionDays > 365
          }
          onClick={() => onSave(draft)}
        >
          保存设置
        </Button>
      </div>
      <p className="text-xs leading-5 text-muted">
        应用运行时每小时检查，开启后的下次检查执行。保留期从首次观察或最近变更起算；基准分支和主干、发布分支始终保留。设置对同一仓库的登记项目共享。
      </p>
      <p className="text-xs leading-5 text-muted">
        Codex
        管理的工作树需在对应聊天处理。无法确认文件或进程状态时不清理；远程分支不受影响。
      </p>
    </div>
  );
}

function CleanupPreview({
  children,
  count,
  pending,
  onClose,
  onApply,
}: {
  children: React.ReactNode;
  count: number;
  pending: boolean;
  onClose: () => void;
  onApply: () => void;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    ref.current?.showModal();
  }, []);
  return (
    <dialog
      ref={ref}
      aria-labelledby="git-cleanup-title"
      onCancel={(e) => {
        if (pending) e.preventDefault();
        else onClose();
      }}
      className="m-auto max-h-[calc(100dvh-32px)] w-[520px] max-w-[calc(100vw-32px)] overflow-auto rounded-xl bg-white p-4 text-ink shadow-xl backdrop:bg-black/20"
    >
      <div className="mb-3 flex items-center gap-3">
        <h2 id="git-cleanup-title" className="flex-1 text-lg font-semibold">
          清理预览
        </h2>
        <button
          className="icon-button"
          aria-label="关闭清理预览"
          disabled={pending}
          onClick={onClose}
        >
          <Icon name="close" />
        </button>
      </div>
      {children}
      <div className="mt-4 flex justify-end gap-2">
        <Button disabled={pending} onClick={onClose}>
          取消
        </Button>
        <Button primary disabled={pending || count === 0} onClick={onApply}>
          {pending ? "正在清理…" : `清理 ${count} 项`}
        </Button>
      </div>
    </dialog>
  );
}
