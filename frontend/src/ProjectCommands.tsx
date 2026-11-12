import { t } from "./i18n";
import ActionDialog from "./ActionDialog";
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
  starting: "Starting",
  running: "Running",
  stopping: "Stopping",
  stopped: "Stopped",
  failure: "Failed to start or run",
  exited: "Exited",
  interrupted: "Interrupted",
};
const portLabels: Record<string, string> = {
  listening: "Port reachable",
  waiting: "Waiting for port",
  occupied: "Port in use",
  available: "Port available",
  unknown: "Port status unknown",
  none: "No port configured",
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
        <h2>{t("Local services")}</h2>
        <div className="flex items-center gap-2">
          <ProjectConfigEditor
            path={path}
            section="services"
            onSaved={() => setRevision((value) => value + 1)}
          />
          {native && (
            <button
              className="icon-button"
              aria-label={t("Refresh services")}
              onClick={() => setRevision((value) => value + 1)}
            >
              <Icon name="refresh" size={16} />
            </button>
          )}
        </div>
      </div>
      {(loadError || actionError || overview?.error) && (
        <p role="alert" className="break-words text-sm text-removed">
          {t(actionError || loadError || overview?.error)}
        </p>
      )}
      {!overview ? (
        !loadError && (
          <p role="status" className="text-sm text-muted">
            {t("Loading local services…")}
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
                          {t(statusLabels[state.status] || state.status)}
                        </span>
                      </div>
                      <p className="mt-1 break-all text-xs text-muted">
                        {d.port ? t("Port {0} · ", d.port) : ""}
                        {t(portLabels[state.portState] || state.portState)}
                        {!active && state.exitCode >= 0
                          ? t(" · Last exit code {0}", state.exitCode)
                          : ""}
                      </p>
                    </div>
                    <div className="flex flex-wrap items-center gap-2">
                      {active ? (
                        <>
                          <Button
                            aria-label={t("Stop {0}", d.name)}
                            disabled={
                              !native ||
                              !!pending ||
                              state.status === "stopping"
                            }
                            onClick={() => void action(state, "stop")}
                          >
                            {t("Stop")}
                          </Button>
                          <Button
                            aria-label={t("Restart {0}", d.name)}
                            disabled={
                              !native ||
                              !!pending ||
                              state.status === "stopping" ||
                              state.configChanged ||
                              !!overview.error
                            }
                            onClick={() => void action(state, "restart")}
                          >
                            {t("Restart")}
                          </Button>
                        </>
                      ) : (
                        <Button
                          aria-label={t("Start {0}", d.name)}
                          disabled={!native || !!pending || !canStart}
                          onClick={() => void action(state, "start")}
                        >
                          {pending === d.id ? t("Working…") : t("Start")}
                        </Button>
                      )}
                      <Button
                        aria-label={t("View {0} logs", d.name)}
                        disabled={!state.runId}
                        onClick={() => {
                          setSelected((current) =>
                            current === d.id ? "" : d.id,
                          );
                          setOutput(null);
                          setLogError("");
                        }}
                      >
                        {t("Logs")}
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
                          {t("Open page")}
                          <Icon name="arrow" size={13} />
                        </a>
                      )}
                    {state.portState === "occupied" && (
                      <button
                        className="min-h-7 text-accent"
                        onClick={() => void inspectPort(state)}
                      >
                        {t("View port usage")}
                      </button>
                    )}
                    <ActionDialog
                      title={t("Start command")}
                      className="min-w-0"
                    >
                      <pre tabIndex={0} className="source-panel mt-2">
                        {(d.command || [])
                          .map((arg) =>
                            /\s/.test(arg) ? JSON.stringify(arg) : arg,
                          )
                          .join(" ")}
                        {t(
                          "\nWorking directory: {0}",
                          d.cwd || t("Project root"),
                        )}
                        {Object.keys(d.env || {}).length
                          ? t(
                              "\nEnvironment variables: {0}",
                              Object.keys(d.env).join(", "),
                            )
                          : ""}
                        {d.url ? `\n${d.url}` : ""}
                        {state.pid ? t("\nProcess PID: {0}", state.pid) : ""}
                      </pre>
                    </ActionDialog>
                  </div>
                  {state.configChanged && (
                    <p className="mt-2 text-xs text-muted">
                      {t(
                        "Configuration changed. Stop, refresh, and review the command before restarting.",
                      )}
                    </p>
                  )}
                  {state.error && (
                    <p className="mt-2 break-words text-xs text-removed">
                      {t(state.error)}
                    </p>
                  )}
                  {ownerID === d.id && (
                    <div className="mt-2 text-xs text-muted">
                      {!owners ? (
                        <span role="status">{t("Loading port usage…")}</span>
                      ) : owners.error ? (
                        <span role="alert">{t(owners.error)}</span>
                      ) : owners.data?.length ? (
                        owners.data.map((owner) => (
                          <p key={owner.pid}>
                            {owner.name} · PID {owner.pid}
                          </p>
                        ))
                      ) : (
                        t(
                          "No listening process found. The port may have changed; refresh to check.",
                        )
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
          <Empty icon="file" title={t("No local services yet")}>
            {t(
              "Ask AI to connect existing start commands, or add them in Configure services.",
            )}
          </Empty>
        )
      )}
      {selectedState?.runId && (
        <section aria-label={t("Service logs")}>
          <div className="mb-2 flex flex-wrap items-baseline justify-between gap-2">
            <h3 className="text-sm font-semibold">
              {selectedState.definition.name} {t("· Logs")}
            </h3>
            <span className="text-xs text-muted">{t("Recent 256 KiB")}</span>
          </div>
          {logError ? (
            <p role="alert" className="text-sm text-removed">
              {t(logError)}
            </p>
          ) : output?.state.runId === selectedState.runId ? (
            <pre
              tabIndex={0}
              className="source-panel max-h-64"
              aria-label={t("Service log content")}
            >
              {output.log || t("No output yet")}
            </pre>
          ) : (
            <p role="status" className="text-sm text-muted">
              {t("Loading logs…")}
            </p>
          )}
        </section>
      )}
      {!!Object.keys(scripts).length && (
        <ActionDialog
          title={
            <>
              {t("package.json Script ·")} {Object.keys(scripts).length}
            </>
          }
          className="min-w-0"
        >
          <div className="grouped-list mt-3">
            {Object.entries(scripts).map(([name, command]) => (
              <div key={name} className="info-row">
                <strong className="font-medium">{name}</strong>
                <code className="text-muted">{command}</code>
              </div>
            ))}
          </div>
        </ActionDialog>
      )}
    </div>
  );
}
