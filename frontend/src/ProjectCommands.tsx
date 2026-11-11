import { useEffect, useRef, useState } from "react";
import { isMyGo } from "mygo-runtime";
import {
  Service,
  type LocalServicesOverview,
  type LocalServiceState,
  type LocalServiceOutput,
  type LocalPortOwner,
} from "./mygo";
import { Button, Empty, Icon } from "./components";
import { servicePreview, servicePreviewLog } from "./service-preview";

import ProjectConfigEditor from "./ProjectConfigEditor";

const native = isMyGo();
const errorMessage = (error: unknown) =>
  error instanceof Error ? error.message : String(error);
const isRunning = (state: LocalServiceState) =>
  ["starting", "running", "stopping"].includes(state.status);
const statusLabels: Record<string, string> = {
  starting: "启动中",
  running: "运行中",
  stopping: "停止中",
  stopped: "已停止",
  failure: "启动或运行失败",
  exited: "已退出",
  interrupted: "已中断",
};
const portLabels: Record<string, string> = {
  listening: "端口可连接",
  waiting: "等待端口",
  occupied: "端口被占用",
  available: "端口空闲",
  unknown: "端口状态未知",
  none: "未配置端口",
};

export default function ProjectCommands({
  path,
  scripts,
}: {
  path: string;
  scripts: Record<string, string>;
}) {
  const [overview, setOverview] = useState<LocalServicesOverview | null>(
    native ? null : servicePreview(path),
  );
  const [loadError, setLoadError] = useState("");
  const [actionError, setActionError] = useState("");
  const [pending, setPending] = useState("");
  const [revision, setRevision] = useState(0);
  const [selected, setSelected] = useState("");
  const [output, setOutput] = useState<LocalServiceOutput | null>(null);
  const [logError, setLogError] = useState("");
  const [ownerID, setOwnerID] = useState("");
  const ownerRequest = useRef(0);
  const [owners, setOwners] = useState<{
    data?: LocalPortOwner[];
    error?: string;
  } | null>(null);

  useEffect(() => {
    if (!native) return;
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    async function load() {
      try {
        const result = await Service.localServices(path);
        // Keep the edit action available when a saved directory no longer exists.
        if (result.error) {
          const editable = await Service.projectConfiguration(path);
          for (const definition of editable.services) {
            if (
              !result.services.some(
                (state) => state.definition.id === definition.id,
              )
            )
              result.services.push({
                definition,
                runId: "",
                status: "stopped",
                pid: 0,
                portState: "unknown",
                startedAt: "",
                endedAt: "",
                exitCode: -1,
                error: "",
                configChanged: false,
              });
          }
        }
        if (active) {
          setOverview(result);
          setLoadError("");
        }
      } catch (error) {
        if (active) setLoadError(errorMessage(error));
      }
      if (active) timer = setTimeout(load, 2000);
    }
    void load();
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [path, revision]);

  useEffect(() => {
    const state = overview?.services.find(
      (service) => service.definition.id === selected,
    );
    if (!state?.runId) return;
    let active = true;
    const request = native
      ? Service.localServiceLog(path, selected, state.runId)
      : Promise.resolve({ state, log: servicePreviewLog });
    request
      .then((result) => {
        if (active) {
          setOutput(result);
          setLogError("");
        }
      })
      .catch((error) => {
        if (active) setLogError(errorMessage(error));
      });
    return () => {
      active = false;
    };
  }, [path, selected, overview]);

  async function action(
    state: LocalServiceState,
    kind: "start" | "stop" | "restart",
  ) {
    if (!overview) return;
    setPending(state.definition.id);
    setActionError("");
    try {
      if (kind !== "start")
        await Service.stopLocalService(path, state.definition.id, state.runId);
      if (kind !== "stop") {
        const started = await Service.startLocalService(
          path,
          state.definition.id,
          overview.version,
        );
        setOverview((current) =>
          current
            ? {
                ...current,
                services: current.services.map((service) =>
                  service.definition.id === started.definition.id
                    ? started
                    : service,
                ),
              }
            : current,
        );
        setSelected(state.definition.id);
        setOutput(null);
      }
    } catch (error) {
      setActionError(errorMessage(error));
    } finally {
      setPending("");
      setRevision((value) => value + 1);
    }
  }

  async function inspectPort(state: LocalServiceState) {
    const request = ++ownerRequest.current;
    setOwnerID(state.definition.id);
    setOwners(null);
    try {
      const data = native
        ? await Service.inspectLocalServicePort(path, state.definition.id)
        : [{ pid: 54321, name: "python" }];
      if (request === ownerRequest.current) setOwners({ data });
    } catch (error) {
      if (request === ownerRequest.current)
        setOwners({ error: errorMessage(error) });
    }
  }

  const selectedState = overview?.services.find(
    (service) => service.definition.id === selected,
  );
  return (
    <div className="min-w-0 space-y-4">
      <div className="section-heading flex items-center justify-between gap-3">
        <h2>本地服务</h2>
        <div className="flex items-center gap-2">
          <ProjectConfigEditor
            path={path}
            section="services"
            onSaved={() => setRevision((value) => value + 1)}
          />
          {native && (
            <button
              className="icon-button"
              aria-label="刷新本地服务"
              onClick={() => setRevision((value) => value + 1)}
            >
              <Icon name="refresh" size={16} />
            </button>
          )}
        </div>
      </div>
      {(loadError || actionError || overview?.error) && (
        <p role="alert" className="break-words text-sm text-removed">
          {actionError || loadError || overview?.error}
        </p>
      )}
      {!overview ? (
        !loadError && (
          <p role="status" className="text-sm text-muted">
            正在读取本地服务…
          </p>
        )
      ) : overview.services.length ? (
        <>
          <div className="grouped-list">
            {overview.services.map((state) => {
              const d = state.definition;
              const active = isRunning(state);
              const canStart =
                !active &&
                !overview.error &&
                (d.autoPort || ["available", "none"].includes(state.portState));
              return (
                <div key={d.id} className="px-3 py-3">
                  <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-2">
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-baseline gap-2">
                        <h3 className="break-words text-sm font-medium">
                          {d.name}
                        </h3>
                        <span
                          role="status"
                          className={`text-xs ${state.status === "failure" ? "text-removed" : state.status === "running" ? "text-green-700" : "text-muted"}`}
                        >
                          {statusLabels[state.status] || state.status}
                        </span>
                      </div>
                      <p className="mt-1 break-all text-xs text-muted">
                        {d.port ? `端口 ${d.port} · ` : ""}
                        {portLabels[state.portState] || state.portState}
                        {!active && state.exitCode >= 0
                          ? ` · 上次退出码 ${state.exitCode}`
                          : ""}
                      </p>
                    </div>
                    <div className="flex flex-wrap items-center gap-2">
                      <ProjectConfigEditor
                        path={path}
                        section="services"
                        id={d.id}
                        onSaved={() => setRevision((value) => value + 1)}
                      />
                      {active ? (
                        <>
                          <Button
                            aria-label={`停止${d.name}`}
                            disabled={
                              !native ||
                              !!pending ||
                              state.status === "stopping"
                            }
                            onClick={() => void action(state, "stop")}
                          >
                            停止
                          </Button>
                          <Button
                            aria-label={`重启${d.name}`}
                            disabled={
                              !native ||
                              !!pending ||
                              state.status === "stopping" ||
                              state.configChanged ||
                              !!overview.error
                            }
                            onClick={() => void action(state, "restart")}
                          >
                            重启
                          </Button>
                        </>
                      ) : (
                        <Button
                          aria-label={`启动${d.name}`}
                          disabled={!native || !!pending || !canStart}
                          onClick={() => void action(state, "start")}
                        >
                          {pending === d.id ? "处理中…" : "启动"}
                        </Button>
                      )}
                      <Button
                        aria-label={`查看${d.name}日志`}
                        disabled={!state.runId}
                        onClick={() => {
                          setSelected((current) =>
                            current === d.id ? "" : d.id,
                          );
                          setOutput(null);
                          setLogError("");
                        }}
                      >
                        日志
                      </Button>
                    </div>
                  </div>
                  <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
                    {d.url &&
                      native &&
                      active &&
                      state.portState === "listening" && (
                        <a
                          className="inline-flex min-h-7 items-center gap-1 text-accent"
                          href={d.url}
                          target="_blank"
                          rel="noreferrer"
                        >
                          打开页面
                          <Icon name="arrow" size={13} />
                        </a>
                      )}
                    {state.portState === "occupied" && (
                      <button
                        className="min-h-7 text-accent"
                        onClick={() => void inspectPort(state)}
                      >
                        查看端口占用
                      </button>
                    )}
                    <details className="min-w-0 flex-1 text-muted">
                      <summary className="w-fit cursor-pointer">
                        启动命令
                      </summary>
                      <pre tabIndex={0} className="source-panel mt-2">
                        {(d.command || [])
                          .map((arg) =>
                            /\s/.test(arg) ? JSON.stringify(arg) : arg,
                          )
                          .join(" ")}
                        {`\n工作目录：${d.cwd || "项目根目录"}`}
                        {Object.keys(d.env || {}).length
                          ? `\n环境变量：${Object.keys(d.env).join("、")}`
                          : ""}
                        {d.url ? `\n${d.url}` : ""}
                        {state.pid ? `\n进程 PID：${state.pid}` : ""}
                      </pre>
                    </details>
                  </div>
                  {state.configChanged && (
                    <p className="mt-2 text-xs text-muted">
                      配置已变化，停止后刷新并核对新命令再启动。
                    </p>
                  )}
                  {state.error && (
                    <p className="mt-2 break-words text-xs text-removed">
                      {state.error}
                    </p>
                  )}
                  {ownerID === d.id && (
                    <div className="mt-2 text-xs text-muted">
                      {!owners ? (
                        <span role="status">正在读取端口占用…</span>
                      ) : owners.error ? (
                        <span role="alert">{owners.error}</span>
                      ) : owners.data?.length ? (
                        owners.data.map((owner) => (
                          <p key={owner.pid}>
                            {owner.name} · PID {owner.pid}
                          </p>
                        ))
                      ) : (
                        "未找到监听进程；端口状态可能已变化，请刷新。"
                      )}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </>
      ) : (
        !overview.error && (
          <Empty icon="file" title="还没有本地服务">
            点击「添加服务」，填入项目的启动命令。
          </Empty>
        )
      )}
      {selectedState?.runId && (
        <section aria-label="服务日志">
          <div className="mb-2 flex flex-wrap items-baseline justify-between gap-2">
            <h3 className="text-sm font-semibold">
              {selectedState.definition.name} · 日志
            </h3>
            <span className="text-xs text-muted">最近 256 KiB</span>
          </div>
          {logError ? (
            <p role="alert" className="text-sm text-removed">
              {logError}
            </p>
          ) : output?.state.runId === selectedState.runId ? (
            <pre
              tabIndex={0}
              className="source-panel max-h-64"
              aria-label="服务日志内容"
            >
              {output.log || "暂无输出"}
            </pre>
          ) : (
            <p role="status" className="text-sm text-muted">
              正在读取日志…
            </p>
          )}
        </section>
      )}
      {!!Object.keys(scripts).length && (
        <details className="text-sm">
          <summary className="w-fit cursor-pointer text-muted">
            package.json 脚本 · {Object.keys(scripts).length}
          </summary>
          <div className="grouped-list mt-3">
            {Object.entries(scripts).map(([name, command]) => (
              <div key={name} className="info-row">
                <strong className="font-medium">{name}</strong>
                <code className="text-muted">{command}</code>
              </div>
            ))}
          </div>
        </details>
      )}
    </div>
  );
}
