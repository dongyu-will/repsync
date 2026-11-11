import ActionDialog from "./ActionDialog";
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
  discovered: "File discovered",
  available: "Command located",
  configured: "Enabled in configuration",
  disabled: "Disabled in configuration",
  missing: "Not found",
  error: "Read failed",
  unknown: "Unverified",
};
const applicability: Record<string, string> = {
  adopted: "Adopted by project",
  recommended: "Mentioned in guide",
  candidate: "Choose per task",
  unassigned: "No linked project",
};
const statuses: Record<string, string> = {
  succeeded: "Succeeded",
  failed: "Failed",
  unknown: "Result unknown",
  declined: "Rejected before execution",
  cancelled: "Cancelled; execution unknown",
};
const message = (error: unknown) =>
  error instanceof Error ? error.message : String(error);
const date = (value: string) => {
  const parsed = new Date(value);
  return Number.isNaN(parsed.valueOf())
    ? "Time unknown"
    : parsed.toLocaleString("en-US", {
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
        source: "Project configuration",
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
          aria-label="AI resource sections"
          className="flex w-fit gap-1 rounded-lg bg-sidebar p-1"
        >
          {[
            ["resources", "Resources"],
            ...(path ? [["memory", "Project memory"]] : []),
            ["usage", "Usage records"],
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
            {state.busy === "inventory" ? "Scanning…" : "Rescan"}
          </Button>
        ) : state.view === "usage" ? (
          <Button
            primary
            disabled={!!state.busy}
            onClick={() => void readUsage()}
          >
            {state.busy === "usage"
              ? "Loading…"
              : state.usage
                ? "Reload records"
                : "Load usage records"}
          </Button>
        ) : null}
      </div>

      {state.view === "resources" && path && (
        <ActionDialog title="Add project MCP">
          <div className="mt-3 max-w-xl pb-2">
            <ResourceSettings path={path} onChanged={refreshResources} />
          </div>
        </ActionDialog>
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
              Load recent records on demand: 30 days of Codex sessions and
              RepSync pipeline receipts.
            </p>
          ) : (
            <ActionDialog title="Last  30  days · Coverage">
              <div className="mt-2 space-y-3 border-b border-line-subtle pb-3">
                <p>
                  {date(state.usage.since)} — {date(state.usage.until)}
                </p>
                <p>
                  Unconnected data sources remain unknown. Skill reads and
                  compliance are evaluated separately.
                </p>
                {state.usage.coverage.map((coverage) => (
                  <div key={coverage.source} className="space-y-1">
                    <p className="font-medium text-ink">
                      {coverage.source} ·{" "}
                      {coverage.status === "unavailable"
                        ? "Not connected"
                        : "Partial coverage"}
                      {" · "}
                      {coverage.files} files · {coverage.sessions} linked
                      sessions
                    </p>
                    <p>{coverage.detail}</p>
                    {coverage.partialFiles > 0 && (
                      <p>
                        {coverage.partialFiles} files could not be fully read.
                      </p>
                    )}
                    {coverage.versions.length > 0 && (
                      <p className="break-words">
                        Recorded version: {coverage.versions.join(", ")}
                      </p>
                    )}
                  </div>
                ))}
              </div>
            </ActionDialog>
          )}
        </>
      )}

      {(state.view === "resources" ||
        (state.view === "usage" && state.usage)) && (
        <div className="space-y-2">
          <div className="flex items-center gap-2">
            <label className="relative min-w-0 flex-1">
              <span className="sr-only">Search AI resources</span>
              <span className="pointer-events-none absolute left-2.5 top-2.5 text-muted">
                <Icon name="search" size={16} />
              </span>
              <input
                value={state.query}
                onChange={(event) =>
                  update({ query: event.target.value, page: 0 })
                }
                placeholder="Search resources"
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
              {filtersChanged ? "Filter · Customized" : "Filter"}
            </Button>
          </div>
          <div id={filterPanelId} hidden={!state.filtersOpen}>
            <div className="flex flex-wrap items-center gap-2 rounded-md bg-sidebar p-2">
              <Select
                aria-label="Resource type"
                value={state.kind}
                onChange={(event) =>
                  update({ kind: event.target.value, page: 0 })
                }
              >
                <option value="all">All types</option>
                <option value="skill">Skill</option>
                <option value="mcp">MCP</option>
                <option value="cli">CLI</option>
              </Select>
              {state.view === "resources" && path && (
                <Select
                  aria-label="Filter by project fit"
                  value={state.filter}
                  onChange={(event) =>
                    update({ filter: event.target.value, page: 0 })
                  }
                >
                  <option value="relevant">Recommended and adopted</option>
                  <option value="all">All resources</option>
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
              ? "Loading resource sources and project declarations…"
              : "Inventory incomplete. Try again."}
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
            <p className="font-medium">No matching resources</p>
            <p className="text-muted">
              Project fit is based on AGENTS.md and .repsync.json, View
              stack-based suggestions under All resources.
            </p>
            <Button
              onClick={() =>
                update({ query: "", kind: "all", filter: "all", page: 0 })
              }
            >
              View all resources
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
            <p className="font-medium">
              No matching usage evidence observed in these records
            </p>
            <p className="text-muted">
              Coverage lists readable sources and known gaps.
            </p>
          </div>
        ))
      )}

      {total > 20 && (
        <div className="flex items-center justify-between gap-3 text-xs text-muted">
          <span>
            Page {currentPage + 1} / {Math.ceil(total / 20)} page
          </span>
          <div className="flex gap-2">
            <Button
              disabled={currentPage === 0}
              onClick={() => update({ page: currentPage - 1 })}
            >
              Previous page
            </Button>
            <Button
              disabled={(currentPage + 1) * 20 >= total}
              onClick={() => update({ page: currentPage + 1 })}
            >
              Next page
            </Button>
          </div>
        </div>
      )}
      {state.view === "resources" && state.report && (
        <section className="text-xs leading-5 text-muted">
          <h3 className="break-words text-sm font-medium">
            {resources.length} resources · Inventory details
          </h3>
          <div className="mt-2 max-w-2xl space-y-2">
            <p>
              Last inventory: {date(state.report.checkedAt)}. Availability in
              the current session requires separate verification.
            </p>
            <p>
              Ask AI to maintain resource selection and task conditions in
              .repsync.json, following the project guide.
            </p>
          </div>
        </section>
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
  const availabilityLabel = availability[resource.availability] || "Unverified";
  const availabilityDetail = resource.detail.startsWith(availabilityLabel)
    ? resource.detail
    : [availabilityLabel, resource.detail].filter(Boolean).join(" · ");
  const fitLabel = applicability[resource.applicability] || "Needs evaluation";
  const hasIssue = ["missing", "error", "disabled", "unknown"].includes(
    resource.availability,
  );
  const needsAttention = ["missing", "error"].includes(resource.availability);
  const rowStatus =
    placement?.state === "disabled"
      ? "Disabled for project"
      : placement?.state === "applied"
        ? "Enabled for project"
        : hasIssue
          ? availabilityLabel
          : resource.applicability === "unassigned"
            ? ""
            : fitLabel;

  return (
    <ActionDialog
      title={resource.name}
      className="border-b border-line-subtle last:border-0"
      triggerClassName="flex min-h-11 w-full items-center gap-3 px-3 py-2 text-left hover:bg-editor"
      trigger={
        <>
          <span className="flex min-w-0 flex-1 flex-wrap items-baseline gap-x-2">
            <span className="font-medium [overflow-wrap:anywhere]">
              {resource.name}
            </span>
            <span className="text-xs text-muted">
              {kindNames[resource.kind] || resource.kind}
              {duplicates > 1 && " · Same name"}
            </span>
          </span>
          {rowStatus && (
            <span
              className={`shrink-0 text-xs ${needsAttention ? "text-removed" : "text-muted"}`}
            >
              {rowStatus}
            </span>
          )}
        </>
      }
    >
      {(placement ||
        (resource.kind === "mcp" &&
          resource.source === "Codex CLI Current configuration") ||
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
          [
            "Project fit",
            [fitLabel, resource.reason].filter(Boolean).join(" · "),
          ],
          [
            "When to use",
            resource.tasks ||
              "Follow task conditions in the project guide. If unspecified, ask AI Evaluate per task",
          ],
          [
            "Purpose",
            resource.description === resource.reason
              ? ""
              : resource.description,
          ],
          ["Availability", availabilityDetail],
          [
            "Source",
            [
              resource.source,
              duplicates > 1 ? `${duplicates}  sources with this name` : "",
            ]
              .filter(Boolean)
              .join(" · "),
          ],
          ["Location", resource.location || "No source path provided"],
          [
            "Content version",
            resource.version
              ? resource.version.slice(0, 12)
              : "Version not collected",
          ],
          [
            "Adopting projects",
            resource.projects.length
              ? resource.projects.join("\n")
              : "No explicit adoption record",
          ],
          [
            "Usage evidence",
            usage
              ? resource.kind === "skill"
                ? `${usage.sessions}  sessions with read evidence; compliance not verified`
                : `${usage.calls}  calls; latest ${date(usage.lastUsed)}`
              : usageRead
                ? "No matching evidence observed in these records"
                : "Usage records not loaded",
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
    </ActionDialog>
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
      ? `${summary.sessions}  sessions`
      : summary.appRuns
        ? `${summary.appRuns}  entry runs`
        : `${summary.calls}  times${summary.kind === "mcp" ? "Requests" : "Execution records"}`;
  const breakdown =
    summary.kind === "skill"
      ? "Read-command evidence found. Content completeness and compliance were not verified."
      : summary.appRuns
        ? "RepSync entry receipts; not attributed to AI."
        : `Succeeded ${summary.succeeded} · Failed ${summary.failed} · Unknown ${summary.unknown} · Rejected ${summary.declined} · Cancel ${summary.cancelled}${summary.probes ? ` · includes probes ${summary.probes}` : ""}`;

  return (
    <ActionDialog
      title={summary.name}
      className="border-b border-line-subtle last:border-0"
      triggerClassName="flex min-h-11 w-full items-center gap-3 px-3 py-2 text-left hover:bg-editor"
      trigger={
        <>
          <span className="flex min-w-0 flex-1 flex-wrap items-baseline gap-x-2">
            <span className="font-medium [overflow-wrap:anywhere]">
              {summary.name}
            </span>
            <span className="text-xs text-muted">
              {kindNames[summary.kind]}
            </span>
          </span>
          <span className="shrink-0 text-right text-xs text-muted">
            {count}
          </span>
        </>
      }
    >
      <p className="border-t border-line-subtle bg-editor px-3 pt-3 text-xs leading-5 text-muted">
        {breakdown} Recent records {date(summary.lastUsed)}
      </p>
      <UsageEvidence events={events} />
    </ActionDialog>
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
                {event.action === "read" ? "Read Skill" : event.action}
              </span>
              <span>{statuses[event.status] || "Unknown"}</span>
              <span className="text-muted">
                {date(event.timestamp)} · {event.actor}
                {event.purpose === "probe" ? " · Environment probe" : ""}
              </span>
            </p>
            <p className="break-words [overflow-wrap:anywhere]">
              {event.detail}
            </p>
            <dl className="text-muted">
              {[
                ["Project", event.project || "Attribution unknown"],
                ["Session", event.session || "Not recorded"],
                ["Events", event.id],
                [
                  "Source",
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
            {events.length} evidence records · Page {currentPage + 1} page
          </span>
          <div className="flex gap-2">
            <Button
              disabled={currentPage === 0}
              onClick={() => setPage(currentPage - 1)}
            >
              Before 10 records
            </Button>
            <Button
              disabled={(currentPage + 1) * 10 >= events.length}
              onClick={() => setPage(currentPage + 1)}
            >
              After 10 records
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
