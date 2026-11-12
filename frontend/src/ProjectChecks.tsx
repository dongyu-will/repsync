import { useEffect, useReducer } from "react";
import { isMyGo } from "mygo-runtime";
import { getLocale, t } from "./i18n";
import { Button, Empty } from "./components";
import ActionDialog from "./ActionDialog";
import AISetup from "./AISetup";
import PipelineFlow from "./PipelineFlow";
import {
  Service,
  type PipelineOutput,
  type PipelineRun,
  type ProjectCheckOverview,
} from "./mygo";

const native = isMyGo();
const message = (error: unknown) =>
  error instanceof Error ? error.message : String(error);
type State = {
  overview: ProjectCheckOverview | null;
  runs: PipelineRun[];
  loadError: string;
  runsError: string;
  error: string;
  pending: boolean;
  search: string;
  selected: string;
  output: PipelineOutput | null;
  logError: string;
  revision: number;
};
const initial: State = {
  overview: null,
  runs: [],
  loadError: "",
  runsError: "",
  error: "",
  pending: false,
  search: "",
  selected: "",
  output: null,
  logError: "",
  revision: 0,
};

function preview(): ProjectCheckOverview {
  return {
    configurations: [
      { tool: "ESLint", path: "eslint.config.js" },
      { tool: "TypeScript", path: "tsconfig.json" },
    ],
    guidance: ["AGENTS.md", "README.md"],
    checks: [
      {
        source: "package.json",
        pipeline: {
          id: "check-preview",
          name: "check",
          kind: "ci",
          steps: [["npm", "run", "check"]],
          cwd: "",
          env: {},
          timeoutSeconds: 1800,
        },
      },
    ],
    warnings: [],
    version: "preview",
  };
}

export default function ProjectChecks({ path }: { path: string }) {
  const [state, update] = useReducer(
    (state: State, patch: Partial<State>) => ({ ...state, ...patch }),
    { ...initial, overview: native ? null : preview() },
  );
  useEffect(() => {
    if (!native) return;
    let active = true;
    Service.projectChecks(path)
      .then((overview) => {
        if (active) update({ overview, loadError: "" });
      })
      .catch((error) => {
        if (active) update({ loadError: message(error) });
      });
    return () => {
      active = false;
    };
  }, [path, state.revision]);
  useEffect(() => {
    if (!native) return;
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      let delay = 5000;
      try {
        const runs = await Service.pipelineRuns(path);
        if (active) update({ runs, runsError: "" });
        if (runs.some((run) => run.status === "running")) delay = 1000;
      } catch (error) {
        if (active) update({ runsError: message(error) });
      } finally {
        if (active) timer = setTimeout(poll, delay);
      }
    }
    void poll();
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [path, state.revision]);
  useEffect(() => {
    if (!native || !state.selected) return;
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      try {
        const output = await Service.pipelineLog(path, state.selected);
        if (active) update({ output, logError: "" });
        if (active && output.run.status === "running")
          timer = setTimeout(poll, 1000);
      } catch (error) {
        if (active) update({ logError: message(error) });
      }
    }
    void poll();
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [path, state.selected]);

  async function run(id: string) {
    if (!state.overview) return;
    update({ pending: true, error: "" });
    try {
      const run = await Service.startProjectCheck(
        path,
        id,
        state.overview.version,
      );
      update({
        runs: [run, ...state.runs].slice(0, 20),
        selected: run.id,
        output: null,
        logError: "",
        revision: state.revision + 1,
      });
    } catch (error) {
      update({ error: message(error) });
    } finally {
      update({ pending: false });
    }
  }
  async function stop(id: string) {
    update({ pending: true, error: "" });
    try {
      await Service.stopPipeline(path, id);
      update({ revision: state.revision + 1 });
    } catch (error) {
      update({ error: message(error) });
    } finally {
      update({ pending: false });
    }
  }
  const overview = state.overview;
  const running = state.runs.find((run) => run.status === "running");
  const search = state.search.trim().toLocaleLowerCase();
  const checks =
    overview?.checks.filter((check) =>
      `${check.pipeline.name} ${check.source} ${check.pipeline.cwd}`
        .toLocaleLowerCase()
        .includes(search),
    ) || [];
  return (
    <section className="space-y-4" aria-label={t("Project checks")}>
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="min-w-0 flex-1 text-base font-semibold">
          {t("Project checks")}
        </h2>
        <AISetup path={path} task="checks" />
        <Button
          disabled={!native || state.pending}
          onClick={() => update({ revision: state.revision + 1 })}
        >
          {t("Refresh")}
        </Button>
        {running && (
          <Button
            disabled={state.pending}
            onClick={() => void stop(running.id)}
          >
            {t("Stop")}
          </Button>
        )}
      </div>
      <p className="text-xs leading-5 text-muted">
        {t(
          "Checks use this project's existing commands. Configuration discovery does not execute them or verify written guidelines.",
        )}
      </p>
      {[state.loadError, state.runsError, state.error]
        .filter(Boolean)
        .map((error, index) => (
          <p
            key={index}
            role="alert"
            className="break-words text-sm text-removed"
          >
            {error}
          </p>
        ))}
      {!overview ? (
        !state.loadError && (
          <p role="status">{t("Discovering project checks…")}</p>
        )
      ) : (
        <>
          {overview.warnings.length > 0 && (
            <div role="status" className="space-y-1 text-sm text-muted">
              {overview.warnings.map((warning) => (
                <p key={warning} className="break-words">
                  {t(warning)}
                </p>
              ))}
            </div>
          )}
          <details className="text-sm">
            <summary className="cursor-pointer py-1">
              {t("Configuration sources · {0}", overview.configurations.length)}
            </summary>
            {overview.configurations.length ? (
              <ul className="grouped-list mt-2">
                {overview.configurations.map((config) => (
                  <li
                    key={`${config.path}-${config.tool}`}
                    className="flex flex-wrap gap-x-3 gap-y-1 border-b border-line px-3 py-2 last:border-b-0"
                  >
                    <span className="w-28 shrink-0 font-medium">
                      {config.tool}
                    </span>
                    <span className="min-w-0 flex-1 break-all text-muted">
                      {config.path}
                    </span>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="py-2 text-muted">
                {t("No recognized check configuration found.")}
              </p>
            )}
          </details>
          <details className="text-sm">
            <summary className="cursor-pointer py-1">
              {t("Written guidelines · {0}", overview.guidance.length)}
            </summary>
            <p className="mt-2 text-xs leading-5 text-muted">
              {t(
                "These documents describe project requirements. Automatic coverage must be confirmed against the project's tools.",
              )}
            </p>
            <ul className="mt-2 space-y-1">
              {overview.guidance.map((name) => (
                <li key={name} className="break-all text-muted">
                  {name}
                </li>
              ))}
            </ul>
          </details>
          {overview.checks.length ? (
            <>
              <label className="block text-sm">
                <span className="sr-only">{t("Filter checks")}</span>
                <input
                  type="search"
                  className="h-9 w-full rounded-md border border-control bg-white px-3 text-sm"
                  placeholder={t("Filter checks")}
                  value={state.search}
                  onChange={(event) => update({ search: event.target.value })}
                />
              </label>
              <div className="grouped-list">
                {checks.map(({ pipeline, source }) => {
                  const latest = state.runs.find(
                    (run) => run.pipelineId === pipeline.id,
                  );
                  return (
                    <div
                      key={pipeline.id}
                      className="space-y-2 border-b border-line px-3 py-3 last:border-b-0"
                    >
                      <div className="flex flex-wrap items-center gap-2">
                        <div className="min-w-0 flex-1">
                          <h3 className="break-words text-sm font-medium">
                            {pipeline.name}
                          </h3>
                          <p className="mt-1 break-all text-xs text-muted">
                            {source}
                            {pipeline.cwd ? ` · ${pipeline.cwd}` : ""}
                          </p>
                        </div>
                        <Button
                          disabled={
                            !native ||
                            state.pending ||
                            !!running ||
                            !!state.loadError ||
                            !!state.runsError
                          }
                          onClick={() => void run(pipeline.id)}
                        >
                          {t("Run")}
                        </Button>
                      </div>
                      {latest && (
                        <p className="text-xs text-muted">
                          {t(
                            "Last run: {0}",
                            new Date(latest.startedAt).toLocaleString(
                              getLocale(),
                            ),
                          )}
                        </p>
                      )}
                      <PipelineFlow
                        pipeline={pipeline}
                        run={latest}
                        version={state.loadError ? "" : overview.version}
                        observed={!state.runsError}
                        onOpenLog={(selected) =>
                          update({ selected, output: null, logError: "" })
                        }
                      />
                    </div>
                  );
                })}
              </div>
              {!checks.length && (
                <p role="status" className="text-sm text-muted">
                  {t("No matching checks.")}
                </p>
              )}
            </>
          ) : (
            <Empty icon="file" title={t("No runnable check commands found")}>
              <p>
                {t(
                  "Keep check commands in the project's task runner or package scripts. Ask AI to connect an existing entry when discovery is incomplete.",
                )}
              </p>
            </Empty>
          )}
        </>
      )}
      <ActionDialog
        title={t("Check logs")}
        trigger={null}
        open={!!state.selected}
        onOpenChange={(open) => {
          if (!open) update({ selected: "", output: null, logError: "" });
        }}
      >
        {state.logError ? (
          <p role="alert" className="break-words text-sm text-removed">
            {state.logError}
          </p>
        ) : (
          <pre
            className="source-panel max-h-[60vh] overflow-auto whitespace-pre-wrap break-words"
            tabIndex={0}
          >
            {state.output?.log || t("Loading logs…")}
          </pre>
        )}
        {state.output?.run.status === "running" && (
          <Button
            disabled={state.pending}
            onClick={() => void stop(state.output!.run.id)}
          >
            {t("Stop")}
          </Button>
        )}
      </ActionDialog>
    </section>
  );
}
