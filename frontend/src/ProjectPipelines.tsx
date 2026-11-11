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

import ProjectConfigEditor from "./ProjectConfigEditor";
import PipelineFlow from "./PipelineFlow";

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
  success: "Passed",
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
      {labels[status] || status || "Result unknown"}
    </span>
  );
}
function timestamp(value: string) {
  return new Date(value).toLocaleString("en-US", {
    month: "numeric",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}

export default function ProjectPipelines({ path }: { path: string }) {
  const [source, setSource] = useState("local");
  const location = (
    <div
      className="flex gap-1 rounded-lg bg-sidebar p-1"
      role="group"
      aria-label="Pipeline location"
    >
      {[
        ["local", "Local"],
        ["github", "GitHub Actions"],
      ].map(([id, name]) => (
        <button
          key={id}
          onClick={() => setSource(id)}
          aria-pressed={source === id}
          className={`min-h-8 rounded-md px-3 text-sm ${source === id ? "bg-white font-medium text-ink" : "text-muted hover:text-ink"}`}
        >
          {name}
        </button>
      ))}
    </div>
  );
  return source === "local" ? (
    <LocalPipelines path={path} location={location} />
  ) : (
    <RemotePipelines path={path} location={location} />
  );
}

function LocalPipelines({
  path,
  location,
}: {
  path: string;
  location: ReactNode;
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
  }, [path, revision]);

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
      <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
        {location}
        <div className="flex items-center gap-2">
          <ProjectConfigEditor
            path={path}
            section="pipelines"
            onSaved={() => setRevision((value) => value + 1)}
          />
          {native && (
            <button
              className="icon-button"
              aria-label="Refresh pipelines"
              onClick={() => setRevision((value) => value + 1)}
            >
              <Icon name="refresh" size={16} />
            </button>
          )}
        </div>
      </div>
      {error && (
        <p role="alert" className="break-words text-sm text-removed">
          {error}
        </p>
      )}
      {configError && (
        <p role="alert" className="break-words text-sm text-removed">
          Could not load configuration: {configError}
          .Fix this in Configure pipelines. This page retries automatically.
        </p>
      )}
      {runsError && (
        <p role="alert" className="break-words text-sm text-removed">
          Could not load run status: {runsError}
          .Showing the last loaded records. Current steps need verification;
          this page retries automatically.
        </p>
      )}
      {!config ? (
        !configError && (
          <p role="status" className="text-sm text-muted">
            Loading pipelines…
          </p>
        )
      ) : config.pipelines.length ? (
        <>
          <div className="grouped-list">
            {config.pipelines.map((pipeline) => {
              const latest = runs.find((run) => run.pipelineId === pipeline.id);
              return (
                <div
                  key={pipeline.id}
                  className="border-b border-line px-3 py-3 last:border-b-0"
                >
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-xs text-muted">
                      {pipeline.kind.toUpperCase()}
                    </span>
                    <h3 className="min-w-0 flex-1 break-words text-sm font-medium">
                      {pipeline.name}
                    </h3>
                    {latest && (
                      <Status status={runsKnown ? latest.status : "unknown"} />
                    )}
                    <Button
                      disabled={
                        !native || pending || !!running || configInvalid
                      }
                      aria-label={`Run ${pipeline.name}`}
                      onClick={() => void run(pipeline)}
                    >
                      {running?.pipelineId === pipeline.id
                        ? runsKnown
                          ? "Running…"
                          : "Run needs review"
                        : "Run"}
                    </Button>
                  </div>
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
        </>
      ) : (
        <Empty icon="file" title="No local pipelines yet">
          Ask AI to connect existing check or release commands, or add them in
          Configure pipelines.
        </Empty>
      )}
      <section aria-label="Local run history">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
          <h3 className="text-sm font-semibold">Recent runs</h3>
          {running && (
            <Button disabled={!native || pending} onClick={() => void stop()}>
              Stop run
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
                    {run.commit ? run.commit.slice(0, 7) : "No Git commit"}
                    {run.worktree === "dirty"
                      ? "(includes local edits)"
                      : run.commit && run.worktree !== "clean"
                        ? "(workspace status unknown)"
                        : ""}{" "}
                    ·{" "}
                    {run.status === "running"
                      ? run.step > 0
                        ? `Step ${run.step}`
                        : "Preparing"
                      : run.exitCode >= 0
                        ? `Exit code ${run.exitCode}`
                        : "No exit code"}
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
              ? "Not run from repsync  to run this project's pipelines yet."
              : runsError
                ? "Run history is unavailable. Existing runs cannot be confirmed."
                : "Loading run history…"}
          </p>
        )}
      </section>
      {selected && (
        <section ref={logSection} tabIndex={-1} aria-label="Pipeline logs">
          <h3 className="mb-3 text-sm font-semibold">
            Run logs ·{" "}
            {runs.find((run) => run.id === selected)?.name || selected}
          </h3>
          {logError ? (
            <div className="space-y-2">
              <p role="alert" className="text-sm text-removed">
                {logError}
              </p>
              <Button
                onClick={() => {
                  revealLog.current = true;
                  setLogError("");
                  setLogRevision((value) => value + 1);
                }}
              >
                Reload logs
              </Button>
            </div>
          ) : output && output.run.id === selected ? (
            <>
              {output.run.error && (
                <p className="mb-2 break-words text-sm text-removed">
                  {output.run.error}
                </p>
              )}
              <pre
                className="source-panel max-h-64"
                tabIndex={0}
                aria-label="Run log content"
              >
                {output.log || "No output yet"}
              </pre>
            </>
          ) : (
            <p role="status" className="text-sm text-muted">
              Loading logs…
            </p>
          )}
        </section>
      )}
    </div>
  );
}

function RemotePipelines({
  path,
  location,
}: {
  path: string;
  location: ReactNode;
}) {
  const [result, setResult] = useState<{
    data?: ActionsOverview;
    error?: string;
  } | null>(native ? null : { data: pipelinePreview(path).actions });
  const [revision, setRevision] = useState(0);
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
      <div className="flex flex-wrap items-center justify-between gap-2">
        {location}
        {native && (
          <Button
            onClick={() => {
              setResult(null);
              setRevision((value) => value + 1);
            }}
            disabled={!result}
          >
            Refresh
          </Button>
        )}
      </div>
      <p className="text-sm text-muted">
        {result?.data?.repository || "No remote repository connected."}
      </p>
      {!result ? (
        <p role="status" className="text-sm text-muted">
          Loading GitHub Actions…
        </p>
      ) : result.error ? (
        <p role="alert" className="break-words text-sm text-removed">
          {result.error}
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
                  {run.workflowName || "Workflow"} · {run.displayTitle}
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
                  aria-label={`View ${run.workflowName || "Workflow"}  logs`}
                >
                  Logs
                  <Icon name="arrow" size={14} />
                </a>
              )}
            </div>
          ))}
        </div>
      ) : (
        <Empty icon="branch" title="No remote runs">
          No readable runs for this repository on GitHub Actions records.
        </Empty>
      )}
      <p className="text-xs leading-5 text-muted">
        Remote status and local results are tracked separately. Open logs and
        manage remote runs on GitHub .
      </p>
    </div>
  );
}
