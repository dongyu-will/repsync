import { getLocale, t } from "./i18n";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { isMyGo } from "mygo-runtime";
import {
  Service,
  type ActionsOverview,
  type Pipeline,
  type PipelineConfig,
  type PipelineOutput,
  type PipelineRun,
} from "./mygo";
import { Button, Empty, Icon } from "./components";
import { pipelinePreview } from "./pipeline-preview";

import AISetup from "./AISetup";
import PipelineFlow from "./PipelineFlow";
import PipelineWorkflows from "./PipelineWorkflows";

const native = isMyGo();
const message = (error: unknown) =>
  error instanceof Error ? error.message : String(error);
const labels: Record<string, string> = {
  running: "Running",
  in_progress: "Running",
  queued: "Queued",
  waiting: "Waiting",
  pending: "Waiting",
  requested: "Waiting",
  success: "Succeeded",
  failure: "Failed",
  cancelled: "Stopped",
  timed_out: "Timed out",
  interrupted: "Interrupted",
  skipped: "Skipped",
  neutral: "No conclusion",
  action_required: "Needs attention",
  stale: "Stale",
  startup_failure: "Failed to start",
  unknown: "Needs review",
};
function Status({ status }: { status: string }) {
  const color =
    status === "success"
      ? "text-green-700"
      : ["failure", "timed_out", "startup_failure"].includes(status)
        ? "text-removed"
        : "text-muted";
  return (
    <span className={`shrink-0 text-xs font-medium ${color}`}>
      {t(labels[status] || status || t("Result unknown"))}
    </span>
  );
}
function timestamp(value: string) {
  return new Date(value).toLocaleString(getLocale(), {
    month: "numeric",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}

export default function ProjectPipelines({ path }: { path: string }) {
  const [revision, setRevision] = useState(0);
  const [filter, setFilter] = useState("");
  return (
    <div className="min-w-0 space-y-4">
      <div className="flex flex-wrap items-center justify-end gap-2">
        <label className="min-w-0 flex-1">
          <span className="sr-only">{t("Filter pipelines")}</span>
          <input
            type="search"
            className="h-9 w-full rounded-md border border-control bg-white px-3 text-sm"
            placeholder={t("Filter pipelines")}
            value={filter}
            onChange={(event) => setFilter(event.target.value)}
          />
        </label>
        <AISetup path={path} task="pipelines" />
        {native && (
          <button
            className="icon-button"
            aria-label={t("Refresh pipelines")}
            onClick={() => setRevision((value) => value + 1)}
          >
            <Icon name="refresh" size={16} />
          </button>
        )}
      </div>

      <LocalPipelines
        key={path}
        path={path}
        filter={filter}
        refreshRevision={revision}
        workflows={
          <PipelineWorkflows
            key={path}
            path={path}
            revision={revision}
            filter={filter}
          />
        }
      />
      <RemotePipelines
        key={`${path}:${revision}`}
        path={path}
        revision={revision}
      />
    </div>
  );
}

function LocalPipelines({
  path,
  filter,
  refreshRevision,
  workflows,
}: {
  path: string;
  filter: string;
  refreshRevision: number;
  workflows: ReactNode;
}) {
  const preview = pipelinePreview(path);
  const [config, setConfig] = useState<PipelineConfig | null>(
    native ? null : preview.config,
  );
  const [runs, setRuns] = useState<PipelineRun[]>(native ? [] : preview.runs);
  const [selected, setSelected] = useState("");
  const [output, setOutput] = useState<PipelineOutput | null>(null);
  const [error, setError] = useState("");
  const [configError, setConfigError] = useState("");
  const [runsError, setRunsError] = useState("");
  const [runsKnown, setRunsKnown] = useState(!native);
  const [logError, setLogError] = useState("");
  const [configInvalid, setConfigInvalid] = useState(false);
  const [pending, setPending] = useState(false);
  const [revision, setRevision] = useState(0);
  const [logRevision, setLogRevision] = useState(0);
  const running = runs.find((run) => run.status === "running");
  const logSection = useRef<HTMLElement>(null);
  const revealLog = useRef(false);

  useEffect(() => {
    if (!native) return;
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      let delay = 5000;
      try {
        const [configuration, records] = await Promise.allSettled([
          Service.pipelines(path),
          Service.pipelineRuns(path),
        ]);
        if (!active) return;
        if (configuration.status === "fulfilled") {
          setConfig(configuration.value);
          setConfigInvalid(false);
          setConfigError("");
        } else {
          setConfigInvalid(true);
          setConfigError(message(configuration.reason));
        }
        if (records.status === "fulfilled") {
          setRuns(records.value);
          setRunsKnown(true);
          setRunsError("");
          if (
            configuration.status === "fulfilled" &&
            records.value.some((run) => run.status === "running")
          )
            delay = 1000;
        } else {
          setRunsKnown(false);
          setRunsError(message(records.reason));
        }
      } finally {
        if (active) timer = setTimeout(poll, delay);
      }
    }
    void poll();
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [path, revision, refreshRevision]);

  useEffect(() => {
    if (!selected) return;
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    async function load() {
      try {
        const data = pipelinePreview(path);
        const value = native
          ? await Service.pipelineLog(path, selected)
          : {
              run: data.runs.find((run) => run.id === selected),
              log: data.logs[selected] || "",
            };
        if (!active) return;
        if (!value.run) throw new Error("Run not found. Select another run.");
        setOutput({ run: value.run, log: value.log });
        setLogError("");
        if (native && value.run.status === "running") {
          timer = setTimeout(load, 1000);
        }
      } catch (error) {
        if (active) setLogError(message(error));
      }
    }
    void load();
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [path, selected, logRevision]);

  useEffect(() => {
    if (
      !revealLog.current ||
      !selected ||
      (!logError && output?.run.id !== selected)
    )
      return;
    const frame = requestAnimationFrame(() => {
      revealLog.current = false;
      logSection.current?.scrollIntoView({ block: "start" });
      logSection.current?.focus({ preventScroll: true });
    });
    return () => cancelAnimationFrame(frame);
  }, [selected, output, logError]);

  async function run(pipeline: Pipeline) {
    if (!config) return;
    setPending(true);
    setError("");
    try {
      const result = await Service.startPipeline(
        path,
        pipeline.id,
        config.version,
      );
      setRuns((current) => [result, ...current].slice(0, 20));
      setSelected(result.id);
      setOutput(null);
      setRevision((value) => value + 1);
    } catch (error) {
      setError(message(error));
    } finally {
      setPending(false);
    }
  }
  async function stop() {
    if (!running) return;
    setPending(true);
    setError("");
    try {
      await Service.stopPipeline(path, running.id);
      setRevision((value) => value + 1);
    } catch (error) {
      setError(message(error));
    } finally {
      setPending(false);
    }
  }
  return (
    <div className="min-w-0 space-y-4">
      {error && (
        <p role="alert" className="break-words text-sm text-removed">
          {t(error)}
        </p>
      )}
      {configError && (
        <p role="alert" className="break-words text-sm text-removed">
          {t("Could not load configuration:")} {t(configError)}
          {t(". Ask AI to repair the configuration, then refresh.")}
        </p>
      )}
      {runsError && (
        <p role="alert" className="break-words text-sm text-removed">
          {t("Could not load run status:")} {t(runsError)}
          {t(
            ".Showing the last loaded records. Current steps need verification; this page retries automatically.",
          )}
        </p>
      )}
      {config?.warnings?.map((warning) => (
        <p
          key={warning}
          role="alert"
          className="break-words text-sm text-removed"
        >
          {t(warning)}
        </p>
      ))}
      {!config ? (
        !configError && (
          <p role="status" className="text-sm text-muted">
            {t("Loading pipelines…")}
          </p>
        )
      ) : config.pipelines.length ? (
        <>
          <div className="grouped-list">
            {config.pipelines
              .filter((pipeline) =>
                `${t("Local")} ${pipeline.kind} ${pipeline.name} ${pipeline.source || config.source} ${pipeline.cwd}`
                  .toLocaleLowerCase()
                  .includes(filter.trim().toLocaleLowerCase()),
              )
              .map((pipeline) => {
                const latest = runs.find(
                  (run) => run.pipelineId === pipeline.id,
                );
                return (
                  <div
                    key={pipeline.id}
                    className="border-b border-line px-3 py-3 last:border-b-0"
                  >
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-xs text-muted">
                        {t("Local")} · {pipeline.kind.toUpperCase()}
                      </span>
                      <h3 className="min-w-0 flex-1 break-words text-sm font-medium">
                        {pipeline.name}
                      </h3>
                      {latest && (
                        <Status
                          status={
                            runsKnown &&
                            !configInvalid &&
                            latest.configVersion === config.version
                              ? latest.status
                              : "unknown"
                          }
                        />
                      )}
                      <Button
                        disabled={
                          !native || pending || !!running || configInvalid
                        }
                        aria-label={t("Run {0}", pipeline.name)}
                        onClick={() => void run(pipeline)}
                      >
                        {running?.pipelineId === pipeline.id
                          ? runsKnown
                            ? t("Running…")
                            : t("Run needs review")
                          : t("Run")}
                      </Button>
                    </div>
                    <p className="mt-1 break-all text-xs text-muted">
                      {pipeline.source || config.source}
                      {pipeline.cwd ? ` · ${pipeline.cwd}` : ""}
                    </p>
                    <PipelineFlow
                      key={pipeline.id + config.version}
                      pipeline={pipeline}
                      run={latest}
                      version={configInvalid ? "" : config.version}
                      observed={runsKnown}
                      onOpenLog={(id) => {
                        revealLog.current = true;
                        setSelected(id);
                        setOutput(null);
                        setLogError("");
                        setLogRevision((value) => value + 1);
                      }}
                    />
                  </div>
                );
              })}
          </div>
          {!config.pipelines.some((pipeline) =>
            `${t("Local")} ${pipeline.kind} ${pipeline.name} ${pipeline.source || config.source} ${pipeline.cwd}`
              .toLocaleLowerCase()
              .includes(filter.trim().toLocaleLowerCase()),
          ) && (
            <p role="status" className="text-sm text-muted">
              {t("No matching local pipelines.")}
            </p>
          )}
        </>
      ) : (
        <Empty icon="file" title={t("No local pipelines yet")}>
          {t(
            "No existing CI/CD entry was discovered. Ask AI to check the project guides and connect an existing command.",
          )}
        </Empty>
      )}
      {workflows}
      <section aria-label={t("Local run history")}>
        <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
          <h3 className="text-sm font-semibold">{t("Local recent runs")}</h3>
          {running && (
            <Button disabled={!native || pending} onClick={() => void stop()}>
              {t("Stop run")}
            </Button>
          )}
        </div>
        {runs.length ? (
          <div className="grouped-list">
            {runs.map((run) => (
              <button
                key={run.id}
                aria-pressed={selected === run.id}
                onClick={() => {
                  revealLog.current = false;
                  setSelected(selected === run.id ? "" : run.id);
                  setOutput(null);
                  setLogError("");
                }}
                className={`flex w-full flex-wrap items-center gap-x-3 gap-y-1 border-b border-line px-3 py-3 text-left last:border-b-0 ${selected === run.id ? "bg-selection" : "hover:bg-sidebar"}`}
              >
                <span className="min-w-0 flex-1">
                  <strong className="block break-words text-sm font-medium">
                    {run.name}
                  </strong>
                  <span className="mt-1 block text-xs text-muted">
                    {timestamp(run.startedAt)} ·{" "}
                    {run.commit ? run.commit.slice(0, 7) : t("No Git commit")}
                    {run.worktree === "dirty"
                      ? t("(includes local edits)")
                      : run.commit && run.worktree !== "clean"
                        ? t("(workspace status unknown)")
                        : ""}{" "}
                    ·{" "}
                    {run.status === "running"
                      ? run.step > 0
                        ? t("Step {0}", run.step)
                        : t("Preparing")
                      : run.exitCode >= 0
                        ? t("Exit code {0}", run.exitCode)
                        : t("No exit code")}
                  </span>
                </span>
                <Status
                  status={
                    !runsKnown && run.status === "running"
                      ? "unknown"
                      : run.status
                  }
                />
                <span
                  className={`text-muted ${selected === run.id ? "rotate-90" : ""}`}
                >
                  <Icon name="chevron" size={16} />
                </span>
              </button>
            ))}
          </div>
        ) : (
          <p className="text-sm text-muted">
            {runsKnown
              ? t("Not run from repsync  to run this project's pipelines yet.")
              : runsError
                ? t(
                    "Run history is unavailable. Existing runs cannot be confirmed.",
                  )
                : t("Loading run history…")}
          </p>
        )}
      </section>
      {selected && (
        <section ref={logSection} tabIndex={-1} aria-label={t("Pipeline logs")}>
          <h3 className="mb-3 text-sm font-semibold">
            {t("Run logs ·")}{" "}
            {runs.find((run) => run.id === selected)?.name || selected}
          </h3>
          {logError ? (
            <div className="space-y-2">
              <p role="alert" className="text-sm text-removed">
                {t(logError)}
              </p>
              <Button
                onClick={() => {
                  revealLog.current = true;
                  setLogError("");
                  setLogRevision((value) => value + 1);
                }}
              >
                {t("Reload logs")}
              </Button>
            </div>
          ) : output && output.run.id === selected ? (
            <>
              {output.run.error && (
                <p className="mb-2 break-words text-sm text-removed">
                  {t(output.run.error)}
                </p>
              )}
              <pre
                className="source-panel max-h-64"
                tabIndex={0}
                aria-label={t("Run log content")}
              >
                {output.log || t("No output yet")}
              </pre>
            </>
          ) : (
            <p role="status" className="text-sm text-muted">
              {t("Loading logs…")}
            </p>
          )}
        </section>
      )}
    </div>
  );
}

function RemotePipelines({
  path,
  revision,
}: {
  path: string;
  revision: number;
}) {
  const [result, setResult] = useState<{
    data?: ActionsOverview;
    error?: string;
  } | null>(native ? null : { data: pipelinePreview(path).actions });
  useEffect(() => {
    if (!native) return;
    let active = true;
    Service.gitHubActions(path)
      .then((data) => {
        if (active) setResult({ data });
      })
      .catch((error) => {
        if (active) setResult({ error: message(error) });
      });
    return () => {
      active = false;
    };
  }, [path, revision]);
  return (
    <div className="space-y-4">
      <h3 className="text-sm font-semibold">
        {t("GitHub Actions recent runs")}
      </h3>
      {result?.data?.repository && (
        <p className="text-sm text-muted">{result.data.repository}</p>
      )}
      {!result ? (
        <p role="status" className="text-sm text-muted">
          {t("Loading GitHub Actions…")}
        </p>
      ) : result.error ? (
        <p role="alert" className="break-words text-sm text-removed">
          {t(result.error)}
        </p>
      ) : result.data?.runs.length ? (
        <div className="grouped-list">
          {result.data.runs.map((run) => (
            <div
              key={run.databaseId}
              className="flex flex-wrap items-center gap-3 border-b border-line px-3 py-3 last:border-b-0"
            >
              <div className="min-w-0 flex-1">
                <h3 className="break-words text-sm font-medium">
                  <span className="mr-2 text-xs font-normal text-muted">
                    {t("GitHub Actions")}
                  </span>
                  {run.workflowName || t("Workflow")} · {run.displayTitle}
                </h3>
                <p className="mt-1 break-all text-xs text-muted">
                  {run.headBranch} · {run.headSha.slice(0, 7)} ·{" "}
                  {timestamp(run.createdAt)}
                </p>
              </div>
              <Status
                status={
                  run.status === "completed" ? run.conclusion : run.status
                }
              />
              {run.url && (
                <a
                  href={run.url}
                  target="_blank"
                  rel="noreferrer"
                  className="inline-flex min-h-9 items-center gap-1 text-sm text-accent"
                  aria-label={t(
                    "View {0}  logs",
                    run.workflowName || t("Workflow"),
                  )}
                >
                  {t("Logs")}
                  <Icon name="arrow" size={14} />
                </a>
              )}
            </div>
          ))}
        </div>
      ) : (
        <Empty icon="branch" title={t("No remote runs")}>
          {t("No readable runs for this repository on GitHub Actions records.")}
        </Empty>
      )}
      <p className="text-xs leading-5 text-muted">
        {t(
          "Remote status and local results are tracked separately. Open logs and manage remote runs on GitHub .",
        )}
      </p>
    </div>
  );
}
