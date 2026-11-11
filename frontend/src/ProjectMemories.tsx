import ActionDialog from "./ActionDialog";
import { useEffect, useReducer, type FormEvent } from "react";
import { isMyGo } from "mygo-runtime";
import { Button, Select } from "./components";
import { Service, type MemorySessions, type ProjectMemory } from "./mygo";

const native = isMyGo();
const memoryTools = [
  ["codex", "Codex"],
  ["claude", "Claude Code"],
] as const;
const memoryToolName = (value: string) =>
  memoryTools.find(([id]) => id === value)?.[1] || "Codex";
const tabs = [
  ["confirmed", "已确认"],
  ["pending", "待确认"],
  ["archived", "已归档"],
] as const;
type Status = (typeof tabs)[number][0];

const demo: ProjectMemory = {
  id: "example-db",
  statement: "生产数据库已完成 PostgreSQL 切换。",
  source: "部署记录 #42 · 数据库迁移验收",
  environment: "生产",
  keywords: ["MySQL"],
  origin: "user",
  status: "confirmed",
  version: "preview",
  createdAt: "2026-11-10T05:30:00Z",
  updatedAt: "2026-11-10T05:30:00Z",
  confirmedAt: "2026-11-10T05:30:00Z",
  archivedFrom: "",
};

type State = {
  items: ProjectMemory[];
  filter: Status;
  busy: boolean;
  error: string;
  notice: string;
  undo: ProjectMemory | null;
  revision: number;
};

const field = "grid gap-1 text-xs text-muted";
const control =
  "w-full min-h-9 rounded-md border border-line bg-white px-2 py-1.5 text-sm text-ink";
const date = (value: string) =>
  new Date(value).toLocaleString("zh-CN", { hour12: false });
const message = (error: unknown) =>
  error instanceof Error ? error.message : String(error);

export default function ProjectMemories({ path }: { path: string }) {
  const [state, update] = useReducer(
    (s: State, patch: Partial<State>) => ({ ...s, ...patch }),
    {
      items: !native && path === "/Projects/atlas" ? [demo] : [],
      filter: "confirmed",
      busy: native,
      error: "",
      notice: "",
      undo: null,
      revision: 0,
    },
  );
  const { items, filter, busy, error, notice, undo } = state;
  useEffect(() => {
    if (!native) return;
    let active = true;
    Service.projectMemories(path)
      .then((items) => {
        if (active) update({ items, busy: false });
      })
      .catch((error: unknown) => {
        if (active) update({ busy: false, error: message(error) });
      });
    return () => {
      active = false;
    };
  }, [path, state.revision]);

  async function record(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!native || busy) return;
    const form = event.currentTarget;
    const data = new FormData(form);
    const get = (name: string) => String(data.get(name) || "");
    update({ busy: true, error: "", notice: "", undo: null });
    try {
      const item = await Service.recordProjectMemory(path, {
        statement: get("statement"),
        source: get("source"),
        environment: get("environment"),
        keywords: get("keywords")
          .split(/[,，\n]/)
          .map((keyword) => keyword.trim())
          .filter(Boolean),
      });
      update({
        items: [item, ...items.filter((existing) => existing.id !== item.id)],
        filter: item.status as Status,
        notice:
          item.status === "archived"
            ? "相同记录已归档，可撤销归档。"
            : "已保存项目记忆",
      });
      form.reset();
      const details = form.closest("details");
      if (details) details.open = false;
    } catch (error) {
      update({ error: message(error) });
    } finally {
      update({ busy: false });
    }
  }

  async function change(
    item: ProjectMemory,
    action: "confirm" | "archive" | "restore",
  ) {
    if (!native || busy) return;
    update({ busy: true, error: "", notice: "" });
    try {
      const result = await Service.setProjectMemoryState(
        path,
        item.id,
        item.version,
        action,
      );
      update({
        items: [result, ...items.filter((existing) => existing.id !== item.id)],
        notice: {
          confirm: "已确认记忆",
          archive: "已归档记忆",
          restore: "已撤销归档",
        }[action],
        undo: action === "archive" ? result : null,
        ...(action === "restore" ? { filter: result.status as Status } : {}),
      });
    } catch (error) {
      update({ error: message(error) });
    } finally {
      update({ busy: false });
    }
  }

  const visible = items.filter((item) => item.status === filter);
  return (
    <section aria-label="项目记忆" className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div
          role="group"
          aria-label="记忆状态"
          className="flex flex-wrap gap-1 rounded-lg bg-sidebar p-1"
        >
          {tabs.map(([id, title]) => (
            <button
              key={id}
              type="button"
              aria-pressed={filter === id}
              className={`min-h-8 rounded-md px-3 ${filter === id ? "bg-white font-medium" : "text-muted hover:text-ink"}`}
              onClick={() => update({ filter: id })}
            >
              {title} {items.filter((item) => item.status === id).length}
            </button>
          ))}
        </div>
        <Button
          disabled={!native || busy}
          onClick={() =>
            update({
              revision: state.revision + 1,
              busy: true,
              error: "",
              notice: "",
              undo: null,
            })
          }
        >
          重新读取
        </Button>
      </div>
      {error && (
        <p role="alert" className="break-words text-removed">
          {error}
        </p>
      )}
      {notice && (
        <div className="flex flex-wrap items-center gap-2">
          <p role="status" className="text-muted">
            {notice}
          </p>
          {undo && (
            <Button
              disabled={busy}
              onClick={() => void change(undo, "restore")}
            >
              撤销归档
            </Button>
          )}
        </div>
      )}
      <ProjectMemoryExtract
        path={path}
        onAdded={() =>
          update({
            revision: state.revision + 1,
            filter: "pending",
            busy: true,
          })
        }
      />
      <ActionDialog
        title="添加记忆"
        busy={busy}
        error={state.error}
        notice={state.notice}
      >
        <form
          className="space-y-3 border-t border-line-subtle p-3"
          onSubmit={record}
        >
          <label className={field}>
            记忆内容
            <textarea
              name="statement"
              className={control}
              required
              maxLength={2048}
              rows={3}
              disabled={!native || busy}
            />
          </label>
          <label className={field}>
            来源
            <input
              name="source"
              className={control}
              required
              maxLength={1024}
              placeholder="链接、文件位置或会话引用"
              disabled={!native || busy}
            />
          </label>
          <div className="grid gap-3 md:grid-cols-2">
            <label className={field}>
              适用环境（可选）
              <input
                name="environment"
                className={control}
                maxLength={256}
                disabled={!native || busy}
              />
            </label>
            <label className={field}>
              文档核对关键词（可选）
              <input
                name="keywords"
                className={control}
                maxLength={1000}
                placeholder="旧名称，用逗号分隔"
                disabled={!native || busy}
              />
            </label>
          </div>
          <div className="flex justify-end">
            <Button type="submit" primary disabled={!native || busy}>
              保存为已确认
            </Button>
          </div>
        </form>
      </ActionDialog>
      {visible.length ? (
        <ul className="grouped-list">
          {visible.map((item) => (
            <li
              key={item.id}
              className="space-y-2 border-b border-line-subtle p-3 last:border-0"
            >
              <p className="whitespace-pre-wrap break-words font-medium">
                {item.statement}
              </p>
              <dl className="space-y-1 break-words text-xs text-muted">
                <div className="flex flex-wrap gap-x-2">
                  <dt>来源</dt>
                  <dd className="min-w-0 break-all">{item.source}</dd>
                </div>
                {item.environment && (
                  <div className="flex flex-wrap gap-x-2">
                    <dt>适用环境</dt>
                    <dd>{item.environment}</dd>
                  </div>
                )}
                {item.keywords.length > 0 && (
                  <div className="flex flex-wrap gap-x-2">
                    <dt>文档关键词</dt>
                    <dd>{item.keywords.join("、")}</dd>
                  </div>
                )}
              </dl>
              {item.session && (
                <section className="space-y-2 text-xs">
                  <h3 className="break-words text-sm font-medium">
                    原会话与用户原话
                  </h3>
                  <p className="break-all text-muted">
                    {item.session.file}:{item.session.line}
                  </p>
                  {item.session.extractor && (
                    <p className="text-muted">
                      由 {memoryToolName(item.session.extractor)} 提炼
                    </p>
                  )}
                  <blockquote className="whitespace-pre-wrap break-words border-l-2 border-line pl-2">
                    {item.session.quote}
                  </blockquote>
                </section>
              )}
              <div className="flex flex-wrap items-center justify-between gap-2">
                <p className="text-xs text-muted">
                  {item.origin === "ai" ? "AI 推断" : "手动记录"} ·{" "}
                  {date(item.updatedAt)}
                </p>
                <div className="flex flex-wrap gap-2">
                  {item.status === "pending" && (
                    <Button
                      primary
                      disabled={!native || busy}
                      onClick={() => void change(item, "confirm")}
                    >
                      确认记忆
                    </Button>
                  )}
                  {item.status === "archived" ? (
                    <Button
                      disabled={!native || busy}
                      onClick={() => void change(item, "restore")}
                    >
                      撤销归档
                    </Button>
                  ) : (
                    <Button
                      disabled={!native || busy}
                      onClick={() => void change(item, "archive")}
                    >
                      归档
                    </Button>
                  )}
                </div>
              </div>
            </li>
          ))}
        </ul>
      ) : (
        <p role="status" className="py-3 text-muted">
          {busy
            ? "正在读取项目记忆…"
            : {
                confirmed: "暂无已确认记忆",
                pending: "暂无待确认记忆",
                archived: "暂无归档记忆",
              }[filter]}
        </p>
      )}
    </section>
  );
}

function ProjectMemoryExtract({
  path,
  onAdded,
}: {
  path: string;
  onAdded: () => void;
}) {
  type ExtractState = {
    report: MemorySessions | null;
    provider: string;
    extractor: string;
    selected: string;
    busy: "list" | "extract" | "";
    error: string;
    notice: string;
  };
  const [state, update] = useReducer(
    (previous: ExtractState, next: Partial<ExtractState>) => ({
      ...previous,
      ...next,
    }),
    {
      report: null,
      provider: "codex",
      extractor: "codex",
      selected: "",
      busy: "",
      error: "",
      notice: "",
    },
  );
  async function load(provider = state.provider) {
    if (state.busy) return;
    update({ busy: "list", error: "", notice: "" });
    try {
      const report: MemorySessions = native
        ? await Service.listMemorySessions(path, provider)
        : { sessions: [], partial: false };
      update({
        provider,
        report,
        selected:
          provider === state.provider &&
          report.sessions.some((session) => session.file === state.selected)
            ? state.selected
            : "",
      });
    } catch (error) {
      update({ error: message(error) });
    } finally {
      update({ busy: "" });
    }
  }
  async function extract(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const session = state.report?.sessions.find(
      (session) => session.file === state.selected,
    );
    if (!native || !session || state.busy) return;
    update({ busy: "extract", error: "", notice: "" });
    try {
      const result = await Service.extractProjectMemories(
        path,
        state.provider,
        session.file,
        session.id,
        state.extractor,
      );
      const details = [
        result.added.length
          ? `新增 ${result.added.length} 条待确认记忆`
          : "没有新增待确认记忆",
        result.messages === 0 ? "会话中没有可提炼的已结束轮次" : "",
        result.existing ? `${result.existing} 条已存在` : "",
        result.rejected ? `${result.rejected} 条未通过原话核对或字段校验` : "",
        result.partial ? "只处理了读取范围内的消息" : "",
      ].filter(Boolean);
      update({ notice: details.join(" · ") });
      if (result.added.length > 0) onAdded();
    } catch (error) {
      update({ error: message(error) });
    } finally {
      update({ busy: "" });
    }
  }

  return (
    <ActionDialog
      title="从 AI 会话提炼"
      onOpen={() => {
        if (!state.report && !state.error) void load();
      }}
      busy={!!state.busy}
    >
      <form
        className="space-y-3 border-t border-line-subtle p-3"
        onSubmit={extract}
      >
        <p className="text-xs leading-5 text-muted">
          将所选会话中已结束轮次的用户文字发送给
          {memoryToolName(state.extractor)}
          的默认模型，提炼结果进入待确认。
        </p>
        <div className="grid grid-cols-2 gap-3">
          <label className={field}>
            会话来源
            <Select
              className="w-full"
              value={state.provider}
              disabled={!!state.busy}
              onChange={(event) => {
                const provider = event.target.value;
                update({
                  provider,
                  report: null,
                  selected: "",
                  error: "",
                  notice: "",
                });
                void load(provider);
              }}
            >
              {memoryTools.map(([id, label]) => (
                <option key={id} value={id}>
                  {label}
                </option>
              ))}
            </Select>
          </label>
          <label className={field}>
            提炼工具
            <Select
              className="w-full"
              value={state.extractor}
              disabled={!!state.busy}
              onChange={(event) =>
                update({ extractor: event.target.value, error: "", notice: "" })
              }
            >
              {memoryTools.map(([id, label]) => (
                <option key={id} value={id}>
                  {label}
                </option>
              ))}
            </Select>
          </label>
        </div>
        <label className={field}>
          最近修改的项目会话
          <Select
            className="w-full"
            disabled={!native || !!state.busy}
            value={state.selected}
            onChange={(event) =>
              update({ selected: event.target.value, error: "", notice: "" })
            }
          >
            <option value="">
              {state.busy === "list" ? "正在读取会话…" : "选择一个会话"}
            </option>
            {state.report?.sessions.map((session) => (
              <option key={session.file} value={session.file}>
                {session.title || session.id} ·{" "}
                {date(session.started || session.modified)}
              </option>
            ))}
          </Select>
        </label>
        {state.report && (
          <p role="status" className="text-xs text-muted">
            {state.report.sessions.length === 0 &&
              `最近 30 天未找到当前项目的 ${memoryToolName(state.provider)} 会话。`}
            {state.report.partial && " 部分会话超出扫描范围或无法读取。"}
          </p>
        )}
        {state.error && (
          <p role="alert" className="break-words text-removed">
            {state.error}
          </p>
        )}
        {state.notice && (
          <p role="status" className="text-muted">
            {state.notice}
          </p>
        )}
        <div className="flex flex-wrap justify-end gap-2">
          <Button
            type="button"
            disabled={!native || !!state.busy}
            onClick={() => void load()}
          >
            刷新会话
          </Button>
          <Button
            type="submit"
            primary
            disabled={!native || !!state.busy || !state.selected}
          >
            {state.busy === "extract" ? "正在提炼…" : "提炼并加入待确认"}
          </Button>
        </div>
      </form>
    </ActionDialog>
  );
}
