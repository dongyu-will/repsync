import { useEffect, useId, useReducer } from "react";
import { isMyGo } from "mygo-runtime";
import { Button, Icon, Select } from "./components";
import {
  Service,
  type AIResource,
  type AIResourceReport,
  type AIUsageEvent,
  type AIUsageReport,
  type AIUsageSummary,
  type CodexResourcePlacement,
} from "./mygo";
import { previewAIResources, previewAIUsage } from "./ai-preview";
import ResourceSettings from "./ProjectCodexResources";
import ProjectMemories from "./ProjectMemories";

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
  filtersOpen: boolean;
  query: string;
  page: number;
  revision: number;
  placements: CodexResourcePlacement[];
  placementIssue: string;
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
      filtersOpen: false,
      query: "",
      page: 0,
      revision: 0,
      placements: [],
      placementIssue: "",
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
    if (native && path) {
      void Service.projectCodexResources(path)
        .then(({ items, issue }) => {
          if (active) update({ placements: items, placementIssue: issue });
        })
        .catch((error: unknown) => {
          if (active) update({ placementIssue: message(error) });
        });
    }
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
  const filterPanelId = useId();
  const filtersChanged =
    state.kind !== "all" ||
    (state.view === "resources" && !!path && state.filter !== "relevant");
  const query = state.query.trim().toLowerCase();
  function refreshResources() {
    update({ busy: "inventory", error: "", revision: state.revision + 1 });
  }
  const inventory = (state.report?.resources || []).map((resource) => ({
    resource,
    placement: state.placements.find(
      (item) =>
        item.name === resource.name &&
        item.kind === resource.kind &&
        (item.kind === "mcp" ||
          resource.location === item.source + "/SKILL.md"),
    ),
  }));
  for (const placement of state.placements) {
    if (inventory.some((item) => item.placement === placement)) continue;
    inventory.push({
      placement,
      resource: {
        id: "project-setting:" + placement.kind + ":" + placement.name,
        kind: placement.kind,
        name: placement.name,
        source: "项目配置",
        location: placement.target,
        availability:
          placement.state === "applied"
            ? "configured"
            : placement.state === "disabled"
              ? "disabled"
              : "unknown",
        applicability: "adopted",
        detail: "",
        description: "",
        version: "",
        reason: "",
        tasks: "",
        projects: [path],
      },
    });
  }
  const resources = inventory.filter(
    ({ resource, placement }) =>
      (state.kind === "all" || state.kind === resource.kind) &&
      (state.filter === "all" ||
        !!placement ||
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
    state.view === "resources"
      ? resources.length
      : state.view === "usage"
        ? summaries.length
        : 0;
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
            ...(path ? [["memory", "项目记忆"]] : []),
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
        ) : state.view === "usage" ? (
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
        ) : null}
      </div>

      {state.view === "resources" && path && (
        <details className="border-b border-line-subtle pb-2">
          <summary className="w-fit cursor-pointer py-1 text-sm text-accent">
            添加项目 MCP
          </summary>
          <div className="mt-3 max-w-xl pb-2">
            <ResourceSettings path={path} onChanged={refreshResources} />
          </div>
        </details>
      )}
      {state.view === "resources" && state.placementIssue && (
        <p role="status" className="break-words text-xs text-removed">
          {state.placementIssue}
        </p>
      )}
      {state.view === "memory" && <ProjectMemories path={path} />}
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
          {!state.usage ? (
            <p className="py-3 leading-5 text-muted">
              按需读取最近 30 天的 Codex 会话和 RepSync 流水线回执。
            </p>
          ) : (
            <details className="text-xs leading-5 text-muted">
              <summary className="cursor-pointer">近 30 天 · 采集范围</summary>
              <div className="mt-2 space-y-3 border-b border-line-subtle pb-3">
                <p>
                  {date(state.usage.since)} — {date(state.usage.until)}
                </p>
                <p>未接入的数据源保持未知。Skill 读取和要求落实分别判断。</p>
                {state.usage.coverage.map((coverage) => (
                  <div key={coverage.source} className="space-y-1">
                    <p className="font-medium text-ink">
                      {coverage.source} ·{" "}
                      {coverage.status === "unavailable"
                        ? "未接入"
                        : "部分覆盖"}
                      {" · "}
                      {coverage.files} 个文件 · {coverage.sessions} 个关联会话
                    </p>
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
                ))}
              </div>
            </details>
          )}
        </>
      )}

      {(state.view === "resources" ||
        (state.view === "usage" && state.usage)) && (
        <div className="space-y-2">
          <div className="flex items-center gap-2">
            <label className="relative min-w-0 flex-1">
              <span className="sr-only">搜索 AI 资源</span>
              <span className="pointer-events-none absolute left-2.5 top-2.5 text-muted">
                <Icon name="search" size={16} />
              </span>
              <input
                value={state.query}
                onChange={(event) =>
                  update({ query: event.target.value, page: 0 })
                }
                placeholder="搜索资源"
                className="h-9 w-full min-w-0 rounded-md border border-control bg-white pl-8 pr-3"
              />
            </label>
            <Button
              type="button"
              aria-expanded={state.filtersOpen}
              aria-controls={filterPanelId}
              onClick={() => update({ filtersOpen: !state.filtersOpen })}
            >
              <Icon name="settings" size={16} />
              {filtersChanged ? "筛选 · 已调整" : "筛选"}
            </Button>
          </div>
          <div id={filterPanelId} hidden={!state.filtersOpen}>
            <div className="flex flex-wrap items-center gap-2 rounded-md bg-sidebar p-2">
              <Select
                aria-label="资源类型"
                value={state.kind}
                onChange={(event) =>
                  update({ kind: event.target.value, page: 0 })
                }
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
          </div>
        </div>
      )}

      {(state.view === "resources"
        ? state.report?.issues
        : state.view === "usage"
          ? state.usage?.issues
          : undefined
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
              .map(({ resource, placement }) => (
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
                  path={path}
                  placement={placement}
                  onChanged={refreshResources}
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
        state.view === "usage" &&
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
            <p className="text-muted">采集范围列出了可读取的来源和缺口。</p>
          </div>
        ))
      )}

      {total > 20 && (
        <div className="flex items-center justify-between gap-3 text-xs text-muted">
          <span>
            第 {currentPage + 1} / {Math.ceil(total / 20)} 页
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
      {state.view === "resources" && state.report && (
        <details className="text-xs leading-5 text-muted">
          <summary className="cursor-pointer">
            {resources.length} 项资源 · 盘点详情
          </summary>
          <div className="mt-2 max-w-2xl space-y-2">
            <p>
              最近盘点：{date(state.report.checkedAt)}
              。当前会话可见性需另行核验。
            </p>
            <p>让 AI 根据项目指南在 .repsync.json 中维护资源适配与任务条件。</p>
          </div>
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
  path,
  placement,
  onChanged,
}: {
  resource: AIResource;
  duplicates: number;
  usage?: AIUsageSummary;
  usageRead: boolean;
  path: string;
  placement?: CodexResourcePlacement;
  onChanged: () => void;
}) {
  const availabilityLabel = availability[resource.availability] || "待核验";
  const availabilityDetail = resource.detail.startsWith(availabilityLabel)
    ? resource.detail
    : [availabilityLabel, resource.detail].filter(Boolean).join(" · ");
  const fitLabel = applicability[resource.applicability] || "待判断";
  const hasIssue = ["missing", "error", "disabled", "unknown"].includes(
    resource.availability,
  );
  const needsAttention = ["missing", "error"].includes(resource.availability);
  const rowStatus =
    placement?.state === "disabled"
      ? "项目已停用"
      : placement?.state === "applied"
        ? "项目已启用"
        : hasIssue
          ? availabilityLabel
          : resource.applicability === "unassigned"
            ? ""
            : fitLabel;

  return (
    <details className="group border-b border-line-subtle last:border-0">
      <summary className="flex min-h-11 cursor-pointer list-none items-center gap-3 px-3 py-2 hover:bg-editor [&::-webkit-details-marker]:hidden">
        <span className="text-muted group-open:rotate-90">
          <Icon name="chevron" size={16} />
        </span>
        <span className="flex min-w-0 flex-1 flex-wrap items-baseline gap-x-2">
          <span className="font-medium [overflow-wrap:anywhere]">
            {resource.name}
          </span>
          <span className="text-xs text-muted">
            {kindNames[resource.kind] || resource.kind}
            {duplicates > 1 && " · 同名"}
          </span>
        </span>
        {rowStatus && (
          <span
            className={`shrink-0 text-xs ${needsAttention ? "text-removed" : "text-muted"}`}
          >
            {rowStatus}
          </span>
        )}
      </summary>
      {(placement ||
        (resource.kind === "mcp" && resource.source === "Codex CLI 当前配置") ||
        (path &&
          resource.kind === "skill" &&
          resource.availability === "discovered")) && (
        <div className="border-t border-line-subtle bg-editor px-3 py-3">
          <ResourceSettings
            path={path}
            resource={resource}
            placement={placement}
            onChanged={onChanged}
          />
        </div>
      )}
      <dl className="space-y-2 border-t border-line-subtle bg-editor px-3 py-3 text-xs leading-5">
        {[
          ["项目适配", [fitLabel, resource.reason].filter(Boolean).join(" · ")],
          [
            "适用条件",
            resource.tasks ||
              "遵循项目指南中的任务条件；未声明时由 AI 按任务判断",
          ],
          [
            "用途",
            resource.description === resource.reason
              ? ""
              : resource.description,
          ],
          ["可用性", availabilityDetail],
          [
            "来源",
            [resource.source, duplicates > 1 ? `${duplicates} 个同名来源` : ""]
              .filter(Boolean)
              .join(" · "),
          ],
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
  const count =
    summary.kind === "skill"
      ? `${summary.sessions} 个会话`
      : summary.appRuns
        ? `${summary.appRuns} 次入口运行`
        : `${summary.calls} 次${summary.kind === "mcp" ? "请求" : "执行记录"}`;
  const breakdown =
    summary.kind === "skill"
      ? "成功读取命令证据。正文完整性与要求落实未核验。"
      : summary.appRuns
        ? "RepSync 入口回执，未归因于 AI。"
        : `成功 ${summary.succeeded} · 失败 ${summary.failed} · 未知 ${summary.unknown} · 拒绝 ${summary.declined} · 取消 ${summary.cancelled}${summary.probes ? ` · 含探测 ${summary.probes}` : ""}`;

  return (
    <details className="group border-b border-line-subtle last:border-0">
      <summary className="flex min-h-11 cursor-pointer list-none items-center gap-3 px-3 py-2 hover:bg-editor [&::-webkit-details-marker]:hidden">
        <span className="text-muted group-open:rotate-90">
          <Icon name="chevron" size={16} />
        </span>
        <span className="flex min-w-0 flex-1 flex-wrap items-baseline gap-x-2">
          <span className="font-medium [overflow-wrap:anywhere]">
            {summary.name}
          </span>
          <span className="text-xs text-muted">{kindNames[summary.kind]}</span>
        </span>
        <span className="shrink-0 text-right text-xs text-muted">{count}</span>
      </summary>
      <p className="border-t border-line-subtle bg-editor px-3 pt-3 text-xs leading-5 text-muted">
        {breakdown} 最近记录 {date(summary.lastUsed)}
      </p>
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
    <div className="bg-editor px-3 py-3">
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
