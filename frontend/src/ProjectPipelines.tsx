import { useEffect, useState, type ReactNode } from "react";
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

const native = isMyGo();
const message = (error: unknown) =>
  error instanceof Error ? error.message : String(error);
const labels: Record<string, string> = {
  running: "运行中",
  in_progress: "运行中",
  queued: "排队中",
  waiting: "等待中",
  pending: "等待中",
  requested: "等待中",
  success: "通过",
  failure: "失败",
  cancelled: "已停止",
  timed_out: "超时",
  interrupted: "已中断",
  skipped: "已跳过",
  neutral: "无结论",
  action_required: "需要处理",
  stale: "已过期",
  startup_failure: "启动失败",
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
      {labels[status] || status || "结果未知"}
    </span>
  );
}
function timestamp(value: string) {
  return new Date(value).toLocaleString("zh-CN", {
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
      aria-label="流水线运行位置"
    >
      {[
        ["local", "本地"],
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
  const [selected, setSelected] = useState(
    native ? "" : preview.runs[0]?.id || "",
  );
  const [output, setOutput] = useState<PipelineOutput | null>(null);
  const [error, setError] = useState("");
  const [logError, setLogError] = useState("");
  const [configInvalid, setConfigInvalid] = useState(false);
  const [pending, setPending] = useState(false);
  const [revision, setRevision] = useState(0);
  const running = runs.find((run) => run.status === "running");

  useEffect(() => {
    if (!native) return;
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      try {
        const next = await Service.pipelineRuns(path);
        if (!active) return;
        setRuns(next);
        setSelected((value) => value || next[0]?.id || "");
        if (next.some((run) => run.status === "running"))
          timer = setTimeout(poll, 1000);
      } catch (error) {
        if (active) setError(message(error));
      }
    }
    Service.pipelines(path)
      .then((value) => {
        if (active) {
          setConfig(value);
          setConfigInvalid(false);
          setError("");
        }
      })
      .catch(async (error) => {
        if (!active) return;
        setConfigInvalid(true);
        setError(message(error));
        try {
          const editable = await Service.projectConfiguration(path);
          if (active)
            setConfig({
              pipelines: editable.pipelines,
              version: "",
              source: ".repsync.json",
            });
        } catch {
          if (active) setConfig(null);
        }
      });
    void poll();
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [path, revision]);

  useEffect(() => {
    if (!selected) return;
    let active = true;
    const load = native
      ? Service.pipelineLog(path, selected)
      : Promise.resolve({ run: runs[0], log: preview.log });
    load
      .then((value) => {
        if (active) {
          setOutput(value);
          setLogError("");
        }
      })
      .catch((error) => {
        if (active) setLogError(message(error));
      });
    return () => {
      active = false;
    };
  }, [path, selected, runs, preview.log]);

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
              aria-label="刷新本地流水线"
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
      {!config ? (
        !error && (
          <p role="status" className="text-sm text-muted">
            正在读取流水线…
          </p>
        )
      ) : config.pipelines.length ? (
        <>
          <div className="grouped-list">
            {config.pipelines.map((pipeline) => (
              <div
                key={pipeline.id}
                className="border-b border-line px-3 py-2 last:border-b-0"
              >
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-xs text-muted">
                    {pipeline.kind.toUpperCase()}
                  </span>
                  <h3 className="min-w-0 flex-1 break-words text-sm font-medium">
                    {pipeline.name}
                  </h3>
                  <ProjectConfigEditor
                    path={path}
                    section="pipelines"
                    id={pipeline.id}
                    onSaved={() => setRevision((value) => value + 1)}
                  />
                  <Button
                    disabled={!native || pending || !!running || configInvalid}
                    aria-label={`运行${pipeline.name}`}
                    onClick={() => void run(pipeline)}
                  >
                    {running?.pipelineId === pipeline.id ? "运行中…" : "运行"}
                  </Button>
                </div>
                <details className="mt-1 text-xs text-muted">
                  <summary className="w-fit cursor-pointer">
                    {(pipeline.steps || []).length} 个步骤 · 超时{" "}
                    {Math.ceil(pipeline.timeoutSeconds / 60)} 分钟 · 查看命令
                  </summary>
                  <pre className="source-panel mt-2" tabIndex={0}>
                    {(pipeline.steps || [])
                      .map(
                        (args, i) =>
                          `${i + 1}. ${args.map((arg) => (/\s/.test(arg) ? JSON.stringify(arg) : arg)).join(" ")}`,
                      )
                      .join("\n")}
                    {`\n工作目录：${pipeline.cwd || "项目根目录"}`}
                    {Object.keys(pipeline.env || {}).length
                      ? `\n环境变量：${Object.keys(pipeline.env).join("、")}`
                      : ""}
                  </pre>
                </details>
              </div>
            ))}
          </div>
        </>
      ) : (
        <Empty icon="file" title="还没有本地流水线">
          点击「添加流水线」，填入项目已有的检查或发布命令。
        </Empty>
      )}
      <section aria-label="本地运行记录">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
          <h3 className="text-sm font-semibold">最近运行</h3>
          {running && (
            <Button disabled={pending} onClick={() => void stop()}>
              停止运行
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
                  if (selected === run.id) return;
                  setSelected(run.id);
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
                    {run.commit ? run.commit.slice(0, 7) : "无 Git 提交"}
                    {run.worktree === "dirty"
                      ? "（含本地改动）"
                      : run.commit && run.worktree !== "clean"
                        ? "（工作区状态未知）"
                        : ""}{" "}
                    ·{" "}
                    {run.status === "running"
                      ? `步骤 ${run.step || 1}`
                      : run.exitCode >= 0
                        ? `退出码 ${run.exitCode}`
                        : "无退出码"}
                  </span>
                </span>
                <Status status={run.status} />
              </button>
            ))}
          </div>
        ) : (
          <p className="text-sm text-muted">
            尚未从 repsync 运行过此项目的流水线。
          </p>
        )}
      </section>
      {selected && (
        <section aria-label="流水线日志">
          <h3 className="mb-3 text-sm font-semibold">运行日志</h3>
          {logError ? (
            <p role="alert" className="text-sm text-removed">
              {logError}
            </p>
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
                aria-label="运行日志内容"
              >
                {output.log || "暂无输出"}
              </pre>
            </>
          ) : (
            <p role="status" className="text-sm text-muted">
              正在读取日志…
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
            刷新
          </Button>
        )}
      </div>
      <p className="text-sm text-muted">
        {result?.data?.repository || "尚未连接远程仓库。"}
      </p>
      {!result ? (
        <p role="status" className="text-sm text-muted">
          正在读取 GitHub Actions…
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
                  aria-label={`查看 ${run.workflowName || "Workflow"} 的日志`}
                >
                  日志
                  <Icon name="arrow" size={14} />
                </a>
              )}
            </div>
          ))}
        </div>
      ) : (
        <Empty icon="branch" title="没有远程运行记录">
          此仓库尚无可读取的 GitHub Actions 记录。
        </Empty>
      )}
      <p className="text-xs leading-5 text-muted">
        远程状态与本地结果分别记录。日志和远程操作在 GitHub 中完成。
      </p>
    </div>
  );
}
