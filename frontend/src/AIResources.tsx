import { useEffect, useReducer } from "react";
import { isMyGo } from "mygo-runtime";
import { Button, Icon, Select } from "./components";
import {
  Service,
  type AIResource,
  type AIResourceReport,
  type AIUsageEvent,
  type AIUsageReport,
  type AIUsageSummary,
} from "./mygo";
import { previewAIResources, previewAIUsage } from "./ai-preview";

const native = isMyGo();
const kindNames: Record<string, string> = {
  skill: "Skill",
  mcp: "MCP",
  cli: "CLI",
};
const availability: Record<string, string> = {
  discovered: "已发现文件",
  available: "已定位命令",
  configured: "配置已启用",
  disabled: "配置已禁用",
  missing: "未找到",
  error: "读取失败",
  unknown: "待核验",
};
const applicability: Record<string, string> = {
  adopted: "项目已采用",
  recommended: "指南已提及",
  candidate: "按任务选择",
  unassigned: "未关联项目",
};
const statuses: Record<string, string> = {
  succeeded: "成功",
  failed: "失败",
  unknown: "结果未知",
  declined: "执行前已拒绝",
  cancelled: "已取消，执行情况未知",
};
const message = (error: unknown) =>
  error instanceof Error ? error.message : String(error);
const date = (value: string) => {
  const parsed = new Date(value);
  return Number.isNaN(parsed.valueOf())
    ? "时间未知"
    : parsed.toLocaleString("zh-CN", {
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
      });
};
type State = {
  report: AIResourceReport | null;
  usage: AIUsageReport | null;
  busy: string;
  error: string;
  view: string;
  kind: string;
  filter: string;
  query: string;
  page: number;
  revision: number;
};

export default function AIResources({ path = "" }: { path?: string }) {
  const [state, update] = useReducer(
    (state: State, patch: Partial<State>) => ({ ...state, ...patch }),
    {
      report: null,
      usage: null,
      busy: "inventory",
      error: "",
      view: "resources",
      kind: "all",
      filter: path ? "relevant" : "all",
      query: "",
      page: 0,
      revision: 0,
    },
  );
  useEffect(() => {
    let active = true;
    const request = native
      ? Service.aiResources(path)
      : Promise.resolve(previewAIResources(path));
    request
      .then((report) => {
        if (active) update({ report, busy: "" });
      })
      .catch((error: unknown) => {
        if (active) update({ busy: "", error: message(error) });
      });
    return () => {
      active = false;
    };
  }, [path, state.revision]);

  async function readUsage() {
    update({ busy: "usage", error: "" });
    try {
      const usage = native ? await Service.aiUsage(path) : previewAIUsage(path);
      update({ usage, busy: "" });
    } catch (error) {
      update({ busy: "", error: message(error) });
    }
  }
  const query = state.query.trim().toLowerCase();
  const resources = (state.report?.resources || []).filter(
    (resource) =>
      (state.kind === "all" || state.kind === resource.kind) &&
      (state.filter === "all" ||
        ["adopted", "recommended"].includes(resource.applicability)) &&
      `${resource.name} ${resource.description} ${resource.location} ${resource.reason}`
        .toLowerCase()
        .includes(query),
  );
  const summaries = (state.usage?.summaries || []).filter(
    (summary) =>
      (state.kind === "all" || state.kind === summary.kind) &&
      `${summary.name} ${summary.kind}`.toLowerCase().includes(query),
  );
  const total =
    state.view === "resources" ? resources.length : summaries.length;
  const currentPage = Math.min(
    state.page,
    Math.max(0, Math.ceil(total / 20) - 1),
  );
  const names = new Map<string, number>();
  for (const resource of state.report?.resources || []) {
    const key = `${resource.kind}:${resource.name}`;
    names.set(key, (names.get(key) || 0) + 1);
  }

  return (
    <div className="space-y-4 text-sm">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div
          role="group"
          aria-label="AI 资源分区"
          className="flex w-fit gap-1 rounded-lg bg-sidebar p-1"
        >
          {[
            ["resources", "资源"],
            ["usage", "使用记录"],
          ].map(([id, label]) => (
            <button
              key={id}
              type="button"
              aria-pressed={state.view === id}
              onClick={() => update({ view: id, page: 0 })}
              className={`min-h-8 rounded-md px-3 ${state.view === id ? "bg-white font-medium text-ink" : "text-muted hover:text-ink"}`}
            >
              {label}
            </button>
          ))}
        </div>
        {state.view === "resources" ? (
          <Button
            disabled={!!state.busy}
            onClick={() =>
              update({
                busy: "inventory",
                error: "",
                revision: state.revision + 1,
              })
            }
          >
            <Icon name="refresh" size={16} />
            {state.busy === "inventory" ? "正在盘点…" : "重新盘点"}
          </Button>
        ) : (
          <Button
            primary
            disabled={!!state.busy}
            onClick={() => void readUsage()}
          >
            {state.busy === "usage"
              ? "正在读取…"
              : state.usage
                ? "重新读取记录"
                : "读取使用记录"}
          </Button>
        )}
      </div>

      {state.error && (
        <p
          role="alert"
          className="break-words rounded-md bg-red-50 p-3 text-removed"
        >
          {state.error}
        </p>
      )}

      {state.view === "usage" && (
        <>
          <p className="leading-5 text-muted">
            统计最近 30 天可读取的证据。Skill 读取、工具调用和要求落实分别判断。
          </p>
          {!state.usage ? (
            <div className="grouped-list p-4 leading-6">
              <p className="font-medium">尚未读取使用记录</p>
              <p className="text-muted">
                按需读取 Codex 本地会话和 RepSync
                流水线回执。未接入的数据源保持未知。
              </p>
            </div>
          ) : (
            <section aria-label="采集覆盖范围" className="grouped-list">
              {state.usage.coverage.map((coverage) => (
                <details
                  key={coverage.source}
                  className="border-b border-line-subtle last:border-0"
                >
                  <summary className="cursor-pointer px-3 py-2.5 leading-5">
                    <span className="font-medium">{coverage.source}</span>
                    <span className="ml-3 text-muted">
                      {coverage.status === "unavailable"
                        ? "未接入"
                        : "部分覆盖"}{" "}
                      · {coverage.files} 个文件 · {coverage.sessions} 个关联会话
                    </span>
                  </summary>
                  <div className="space-y-1 px-3 pb-3 text-xs leading-5 text-muted">
                    <p>{coverage.detail}</p>
                    {coverage.partialFiles > 0 && (
                      <p>{coverage.partialFiles} 个文件存在读取缺口。</p>
                    )}
                    {coverage.versions.length > 0 && (
                      <p className="break-words">
                        记录版本：{coverage.versions.join("、")}
                      </p>
                    )}
                  </div>
                </details>
              ))}
              <p className="border-t border-line-subtle px-3 py-2 text-xs text-muted">
                {date(state.usage.since)} — {date(state.usage.until)}
              </p>
            </section>
          )}
        </>
      )}

      {state.view === "resources" && state.report && (
        <p className="text-xs leading-5 text-muted">
          {state.report.resources.length} 项资源 ·{" "}
          {date(state.report.checkedAt)} 盘点 · 当前会话可见性需另行核验
        </p>
      )}

      {(state.view === "resources" || state.usage) && (
        <div className="flex flex-wrap items-center gap-2">
          <label className="relative min-w-36 flex-1">
            <span className="sr-only">搜索 AI 资源</span>
            <span className="pointer-events-none absolute left-2.5 top-2.5 text-muted">
              <Icon name="search" size={16} />
            </span>
            <input
              value={state.query}
              onChange={(event) =>
                update({ query: event.target.value, page: 0 })
              }
              placeholder="搜索名称、来源或适配理由"
              className="h-9 w-full min-w-0 rounded-md border border-control bg-white pl-8 pr-3"
            />
          </label>
          <Select
            aria-label="资源类型"
            value={state.kind}
            onChange={(event) => update({ kind: event.target.value, page: 0 })}
          >
            <option value="all">全部类型</option>
            <option value="skill">Skill</option>
            <option value="mcp">MCP</option>
            <option value="cli">CLI</option>
          </Select>
          {state.view === "resources" && path && (
            <Select
              aria-label="项目适配筛选"
              value={state.filter}
              onChange={(event) =>
                update({ filter: event.target.value, page: 0 })
              }
            >
              <option value="relevant">推荐与已采用</option>
              <option value="all">全部资源</option>
            </Select>
          )}
        </div>
      )}

      {(state.view === "resources"
        ? state.report?.issues
        : state.usage?.issues
      )?.map((issue) => (
        <p key={issue} className="break-words text-xs leading-5 text-removed">
          {issue}
        </p>
      ))}

      {state.view === "resources" ? (
        !state.report ? (
          <p role="status" className="py-6 text-muted">
            {state.busy
              ? "正在读取资源来源与项目声明…"
              : "盘点未完成，请重试。"}
          </p>
        ) : resources.length ? (
          <div className="grouped-list">
            {resources
              .slice(currentPage * 20, currentPage * 20 + 20)
              .map((resource) => (
                <ResourceRow
                  key={resource.id}
                  resource={resource}
                  duplicates={
                    names.get(`${resource.kind}:${resource.name}`) || 0
                  }
                  usage={state.usage?.summaries.find(
                    (summary) => summary.resourceId === resource.id,
                  )}
                  usageRead={!!state.usage}
                />
              ))}
          </div>
        ) : (
          <div className="grouped-list space-y-2 p-4 leading-6">
            <p className="font-medium">没有符合条件的资源</p>
            <p className="text-muted">
              项目适配依据来自 AGENTS.md 与
              .repsync.json，技术栈候选可在全部资源中查看。
            </p>
            <Button
              onClick={() =>
                update({ query: "", kind: "all", filter: "all", page: 0 })
              }
            >
              查看全部资源
            </Button>
          </div>
        )
      ) : (
        state.usage &&
        (summaries.length ? (
          <div className="grouped-list">
            {summaries
              .slice(currentPage * 20, currentPage * 20 + 20)
              .map((summary) => (
                <UsageRow
                  key={summary.resourceId}
                  summary={summary}
                  events={state.usage!.events.filter(
                    (event) => event.resourceId === summary.resourceId,
                  )}
                />
              ))}
          </div>
        ) : (
          <div className="grouped-list p-4 leading-6">
            <p className="font-medium">本次记录未观察到匹配的使用证据</p>
            <p className="text-muted">
              请结合上方采集范围判断；不表示这些资源从未使用。
            </p>
          </div>
        ))
      )}

      {total > 20 && (
        <div className="flex items-center justify-between gap-3 text-xs text-muted">
          <span>
            第 {currentPage + 1} / {Math.ceil(total / 20)} 页 · {total} 项
          </span>
          <div className="flex gap-2">
            <Button
              disabled={currentPage === 0}
              onClick={() => update({ page: currentPage - 1 })}
            >
              上一页
            </Button>
            <Button
              disabled={(currentPage + 1) * 20 >= total}
              onClick={() => update({ page: currentPage + 1 })}
            >
              下一页
            </Button>
          </div>
        </div>
      )}
      {state.view === "resources" && (
        <details className="text-xs leading-5 text-muted">
          <summary className="cursor-pointer">如何维护项目适配</summary>
          <p className="mt-2">
            让 AI 读取项目指南，在 .repsync.json 的 aiResources
            中维护资源类型、名称、适配理由和任务条件。同名 Skill 用 source
            指定绝对来源路径。保存声明不会启用工具或执行命令。
          </p>
        </details>
      )}
    </div>
  );
}

function ResourceRow({
  resource,
  duplicates,
  usage,
  usageRead,
}: {
  resource: AIResource;
  duplicates: number;
  usage?: AIUsageSummary;
  usageRead: boolean;
}) {
  return (
    <details className="group border-b border-line-subtle last:border-0">
      <summary className="flex cursor-pointer list-none items-start gap-3 px-3 py-3 hover:bg-editor [&::-webkit-details-marker]:hidden">
        <span className="mt-0.5 text-muted group-open:rotate-90">
          <Icon name="chevron" size={16} />
        </span>
        <span className="min-w-0 flex-1">
          <span className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
            <span className="break-all font-medium">{resource.name}</span>
            <span className="text-xs text-muted">
              {kindNames[resource.kind] || resource.kind}
            </span>
            {duplicates > 1 && (
              <span className="text-xs text-muted">
                {duplicates} 个同名来源
              </span>
            )}
          </span>
          <span className="mt-1 block line-clamp-2 break-words text-xs leading-5 text-muted">
            {resource.reason || resource.description || "适配理由待补充"}
          </span>
        </span>
        <span className="max-w-[36%] shrink-0 text-right text-xs leading-5">
          <span className="block">
            {applicability[resource.applicability] || "待判断"}
          </span>
          <span
            className={
              resource.availability === "missing" ||
              resource.availability === "error"
                ? "text-removed"
                : "text-muted"
            }
          >
            {availability[resource.availability] || "待核验"}
          </span>
        </span>
      </summary>
      <dl className="space-y-2 border-t border-line-subtle bg-editor px-3 py-3 text-xs leading-5">
        {[
          [
            "适用条件",
            resource.tasks ||
              "遵循项目指南中的任务条件；未声明时由 AI 按任务判断",
          ],
          ["用途", resource.description],
          ["可用性", resource.detail],
          ["来源", resource.source],
          ["位置", resource.location || "未提供来源路径"],
          [
            "内容版本",
            resource.version ? resource.version.slice(0, 12) : "未采集版本",
          ],
          [
            "采用项目",
            resource.projects.length
              ? resource.projects.join("\n")
              : "没有明确采用记录",
          ],
          [
            "使用证据",
            usage
              ? resource.kind === "skill"
                ? `${usage.sessions} 个会话有读取证据；要求落实未核验`
                : `${usage.calls} 次调用；最近 ${date(usage.lastUsed)}`
              : usageRead
                ? "本次记录未观察到匹配证据"
                : "尚未读取使用记录",
          ],
        ]
          .filter(([, value]) => value)
          .map(([label, value]) => (
            <div
              key={label}
              className="grid grid-cols-[72px_minmax(0,1fr)] gap-3"
            >
              <dt className="text-muted">{label}</dt>
              <dd className="whitespace-pre-wrap break-words [overflow-wrap:anywhere]">
                {value}
              </dd>
            </div>
          ))}
      </dl>
    </details>
  );
}

function UsageRow({
  summary,
  events,
}: {
  summary: AIUsageSummary;
  events: AIUsageEvent[];
}) {
  return (
    <details className="group border-b border-line-subtle last:border-0">
      <summary className="flex cursor-pointer list-none items-start gap-3 px-3 py-3 hover:bg-editor [&::-webkit-details-marker]:hidden">
        <span className="mt-0.5 text-muted group-open:rotate-90">
          <Icon name="chevron" size={16} />
        </span>
        <span className="min-w-0 flex-1">
          <span className="break-all font-medium">{summary.name}</span>
          <span className="ml-2 text-xs text-muted">
            {kindNames[summary.kind]}
          </span>
          <span className="mt-1 block text-xs leading-5 text-muted">
            {summary.kind === "skill"
              ? "成功读取命令证据；正文完整性与要求落实未核验"
              : summary.appRuns
                ? "RepSync 入口回执，未归因于 AI"
                : `成功 ${summary.succeeded} · 失败 ${summary.failed} · 未知 ${summary.unknown} · 拒绝 ${summary.declined} · 取消 ${summary.cancelled}${summary.probes ? ` · 含探测 ${summary.probes}` : ""}`}
          </span>
        </span>
        <span className="shrink-0 text-right text-xs leading-5">
          <span className="block">
            {summary.kind === "skill"
              ? `${summary.sessions} 个会话`
              : summary.appRuns
                ? `${summary.appRuns} 次入口运行`
                : `${summary.calls} 次${summary.kind === "mcp" ? "请求" : "执行记录"}`}
          </span>
          <span className="text-muted">{date(summary.lastUsed)}</span>
        </span>
      </summary>
      <UsageEvidence events={events} />
    </details>
  );
}

function UsageEvidence({ events }: { events: AIUsageEvent[] }) {
  const [page, setPage] = useReducer((_state: number, next: number) => next, 0);
  const currentPage = Math.min(
    page,
    Math.max(0, Math.ceil(events.length / 10) - 1),
  );
  return (
    <div className="border-t border-line-subtle bg-editor px-3 py-3">
      <ol className="space-y-4">
        {events.slice(currentPage * 10, currentPage * 10 + 10).map((event) => (
          <li key={event.id} className="space-y-1 text-xs leading-5">
            <p className="flex flex-wrap gap-x-3">
              <span className="font-medium">
                {event.action === "read" ? "读取 Skill" : event.action}
              </span>
              <span>{statuses[event.status] || "未知"}</span>
              <span className="text-muted">
                {date(event.timestamp)} · {event.actor}
                {event.purpose === "probe" ? " · 环境探测" : ""}
              </span>
            </p>
            <p className="break-words [overflow-wrap:anywhere]">
              {event.detail}
            </p>
            <dl className="text-muted">
              {[
                ["项目", event.project || "归属未知"],
                ["会话", event.session || "未记录"],
                ["事件", event.id],
                [
                  "来源",
                  `${event.source}${event.line ? `:${event.line}` : ""}`,
                ],
              ].map(([label, value]) => (
                <div
                  key={label}
                  className="grid grid-cols-[40px_minmax(0,1fr)] gap-2"
                >
                  <dt>{label}</dt>
                  <dd className="break-words [overflow-wrap:anywhere]">
                    {value}
                  </dd>
                </div>
              ))}
            </dl>
          </li>
        ))}
      </ol>
      {events.length > 10 && (
        <div className="mt-3 flex flex-wrap items-center justify-between gap-2 text-xs text-muted">
          <span>
            {events.length} 条证据 · 第 {currentPage + 1} 页
          </span>
          <div className="flex gap-2">
            <Button
              disabled={currentPage === 0}
              onClick={() => setPage(currentPage - 1)}
            >
              前 10 条
            </Button>
            <Button
              disabled={(currentPage + 1) * 10 >= events.length}
              onClick={() => setPage(currentPage + 1)}
            >
              后 10 条
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
