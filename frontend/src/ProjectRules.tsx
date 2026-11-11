import ActionDialog from "./ActionDialog";
import { useEffect, useReducer } from "react";
import { isMyGo } from "mygo-runtime";
import { Button, Icon, Select } from "./components";
import {
  Service,
  type PolicyRule,
  type PolicyDefaults,
  type ProjectPolicies,
  type PolicyReport,
  type UpdatePreview,
} from "./mygo";

const native = isMyGo();
const titles: Record<string, string> = {
  "no-unit-tests": "Disallow new unit tests",
  "react-no-use-state": "Use useReducer instead of useState",
};
const levels = [
  ["off", "Off"],
  ["advice", "AI guidance"],
  ["warn", "Warn"],
  ["error", "Enforce"],
];
const field =
  "min-h-9 min-w-0 rounded-md border border-control bg-white px-2 py-1.5 text-sm";
const errorText = (error: unknown) =>
  error instanceof Error ? error.message : String(error);
const lines = (value: string) => value.split("\n");
const defaults: PolicyDefaults = {
  version: "preview",
  rules: [
    {
      cwd: "",
      id: "no-unit-tests",
      level: "off",
      scope: "all",
      include: [],
      exclude: [],
      excludeMarkers: [],
    },
    {
      cwd: "",
      id: "react-no-use-state",
      level: "off",
      scope: "react",
      include: [],
      exclude: [],
      excludeMarkers: [],
    },
  ],
};
type State = {
  defaults: PolicyDefaults | null;
  project: ProjectPolicies | null;
  drafts: PolicyRule[];
  busy: string;
  error: string;
  notice: string;
  report: PolicyReport | null;
  preview: UpdatePreview | null;
  filter: string;
  revision: number;
};
const initial: State = {
  defaults: null,
  project: null,
  drafts: [],
  busy: "",
  error: "",
  notice: "",
  report: null,
  preview: null,
  filter: "new",
  revision: 0,
};

export default function ProjectRules({
  path,
  onUpdated,
}: {
  path?: string;
  onUpdated?: () => void;
}) {
  const [state, update] = useReducer(
    (state: State, patch: Partial<State>) => ({ ...state, ...patch }),
    initial,
  );
  useEffect(() => {
    let active = true;
    const load = native
      ? Promise.all([
          Service.policyDefaults(),
          path ? Service.projectPolicies(path) : Promise.resolve(null),
        ])
      : Promise.resolve([
          defaults,
          path
            ? {
                version: "preview",
                projectVersion: "preview",
                defaultsVersion: "preview",
                overrides: [],
                effective: defaults.rules.map((rule) => ({
                  rule,
                  origin: "default",
                  applies: true,
                })),
              }
            : null,
        ] as const);
    load
      .then(([defaults, project]) => {
        if (active)
          update({
            defaults,
            project,
            drafts: structuredClone(
              project ? project.overrides : defaults.rules,
            ),
            error: "",
          });
      })
      .catch((error) => {
        if (active) update({ error: errorText(error) });
      });
    return () => {
      active = false;
    };
  }, [path, state.revision]);
  function change(id: string, patch: Partial<PolicyRule>) {
    const current =
      state.drafts.find((rule) => rule.id === id) ||
      state.project?.effective.find((entry) => entry.rule.id === id)?.rule ||
      state.defaults?.rules.find((rule) => rule.id === id) ||
      defaults.rules.find((rule) => rule.id === id)!;
    const next = { ...current, ...patch };
    update({
      drafts: [...state.drafts.filter((rule) => rule.id !== id), next],
      report: null,
      preview: null,
      notice: "",
    });
  }
  async function save() {
    if (!native || !state.defaults) return;
    update({ busy: "save", error: "", notice: "", preview: null });
    try {
      const rules = state.drafts.map((rule) => ({
        ...rule,
        include: rule.include.map((value) => value.trim()).filter(Boolean),
        exclude: rule.exclude.map((value) => value.trim()).filter(Boolean),
        excludeMarkers: rule.excludeMarkers
          .map((value) => value.trim())
          .filter(Boolean),
      }));
      if (path && state.project)
        await Service.saveProjectPolicies(
          path,
          state.project.projectVersion,
          rules,
        );
      else await Service.savePolicyDefaults(state.defaults.version, rules);
      update({
        revision: state.revision + 1,
        report: null,
        notice: "Rules saved. Run checks again for updated results.",
      });
    } catch (error) {
      update({ error: errorText(error) });
    } finally {
      update({ busy: "" });
    }
  }
  async function check() {
    if (!path || !native) return;
    update({ busy: "check", error: "", report: null });
    try {
      update({ report: await Service.checkPolicies(path), filter: "new" });
    } catch (error) {
      update({ error: errorText(error) });
    } finally {
      update({ busy: "" });
    }
  }
  async function preview() {
    if (!path || !native) return;
    update({ busy: "preview", error: "", preview: null });
    try {
      update({ preview: await Service.previewPolicyGuidance(path) });
    } catch (error) {
      update({ error: errorText(error) });
    } finally {
      update({ busy: "" });
    }
  }
  async function apply() {
    if (!state.preview) return;
    update({ busy: "apply", error: "" });
    try {
      await Service.applyUpdate(state.preview.token);
      update({
        preview: null,
        notice: "Updated AGENTS.md, Previous content backed up.",
      });
      onUpdated?.();
    } catch (error) {
      update({ preview: null, error: errorText(error) });
    } finally {
      update({ busy: "" });
    }
  }
  const dirty =
    state.defaults &&
    JSON.stringify(state.drafts) !==
      JSON.stringify(path ? state.project?.overrides : state.defaults.rules);
  return (
    <section
      aria-label={path ? "Project rules" : "Default project rules"}
      className="space-y-4"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        {path ? (
          <h2 className="sr-only">Project rules</h2>
        ) : (
          <h2 className="text-base font-semibold">Default project rules</h2>
        )}
        <div className="flex items-center gap-2">
          <button
            className="icon-button"
            aria-label="Reload rules"
            title="Reload rules"
            disabled={!!state.busy}
            onClick={() => {
              if (dirty && !window.confirm("Discard unsaved rules and reload?"))
                return;
              update({
                revision: state.revision + 1,
                defaults: null,
                project: null,
                error: "",
                report: null,
                preview: null,
                notice: "",
              });
            }}
          >
            <Icon name="refresh" size={16} />
          </button>
          {path && (
            <Button
              disabled={!native || !!state.busy || !!dirty || !state.project}
              onClick={() => void check()}
            >
              {state.busy === "check" ? "Checking…" : "Check rules"}
            </Button>
          )}
          <Button
            primary
            disabled={!native || !!state.busy || !state.defaults || !dirty}
            onClick={() => void save()}
          >
            {state.busy === "save" ? "Saving…" : "Save rules"}
          </Button>
        </div>
      </div>
      {!path && (
        <p className="text-sm text-muted">
          as project defaults. Each project can define exceptions.
        </p>
      )}
      {state.error && (
        <p role="alert" className="break-words text-sm text-removed">
          {state.error}
        </p>
      )}
      {state.notice && (
        <p role="status" className="text-sm text-muted">
          {state.notice}
        </p>
      )}
      {!state.defaults ? (
        !state.error && (
          <p role="status" className="text-sm text-muted">
            Loading rules…
          </p>
        )
      ) : (
        <div className="grouped-list">
          {defaults.rules.map((builtin) => {
            const override = state.drafts.find(
              (rule) => rule.id === builtin.id,
            );
            const effective = state.project?.effective.find(
              (entry) => entry.rule.id === builtin.id,
            );
            const inherited =
              state.defaults?.rules.find((rule) => rule.id === builtin.id) ||
              builtin;
            const rule = override || inherited;
            const level = path && !override ? "inherit" : rule.level;
            return (
              <div key={rule.id} className="space-y-3 p-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="min-w-0">
                    <h3 className="text-sm font-medium">{titles[rule.id]}</h3>
                    {path && !effective?.applies && rule.scope === "react" && (
                      <p className="mt-1 text-xs text-muted">
                        Not detected in the working directory: React
                        Dependencies
                      </p>
                    )}
                  </div>
                  <label className="text-sm">
                    <span className="sr-only">
                      {titles[rule.id]}Enforcement
                    </span>
                    <Select
                      aria-label={`${titles[rule.id]} enforcement`}
                      disabled={!!state.busy}
                      value={level}
                      onChange={(event) => {
                        if (event.target.value === "inherit")
                          update({
                            drafts: state.drafts.filter(
                              (item) => item.id !== rule.id,
                            ),
                            report: null,
                            preview: null,
                            notice: "",
                          });
                        else change(rule.id, { level: event.target.value });
                      }}
                    >
                      {path && (
                        <option value="inherit">
                          Inherit ·{" "}
                          {
                            levels.find(
                              ([value]) => value === inherited.level,
                            )?.[1]
                          }
                        </option>
                      )}
                      {levels.map(([value, label]) => (
                        <option key={value} value={value}>
                          {label}
                        </option>
                      ))}
                    </Select>
                  </label>
                </div>
                <ActionDialog
                  title="Scope and exceptions"
                  busy={!!state.busy}
                  error={state.error}
                  notice={state.notice}
                >
                  <fieldset
                    disabled={!!state.busy || (path ? !override : false)}
                    className="mt-3 space-y-3"
                  >
                    <label className="flex flex-wrap items-center justify-between gap-2 text-sm">
                      Applicable projects
                      <Select
                        value={rule.scope}
                        onChange={(event) =>
                          change(rule.id, { scope: event.target.value })
                        }
                      >
                        <option value="all">All projects</option>
                        <option value="react">React Project</option>
                      </Select>
                    </label>
                    {path && !override && (
                      <p className="text-xs text-muted">
                        Choose project enforcement to edit exceptions.
                      </p>
                    )}
                    {rule.id === "react-no-use-state" && (
                      <label className="block space-y-1 text-sm">
                        ESLint Working directory
                        <input
                          aria-label="ESLint Working directory"
                          className={`${field} w-full`}
                          value={rule.cwd}
                          placeholder="Leave blank for the project root, e.g. frontend"
                          onChange={(event) =>
                            change(rule.id, { cwd: event.target.value })
                          }
                        />
                      </label>
                    )}
                    <label className="block space-y-1 text-sm">
                      {rule.id === "no-unit-tests"
                        ? "Unit test paths (one per line)"
                        : "Source paths (leave blank to check all JS / TS  source files)"}
                      <textarea
                        aria-label={`${titles[rule.id]} detected paths`}
                        rows={2}
                        className={`${field} w-full resize-y font-mono`}
                        value={(rule.include || []).join("\n")}
                        placeholder={
                          rule.id === "no-unit-tests"
                            ? "e.g. **/*.unit.test.ts"
                            : "e.g. frontend/src/**"
                        }
                        onChange={(event) =>
                          change(rule.id, {
                            include: lines(event.target.value),
                          })
                        }
                      />
                    </label>
                    <label className="block space-y-1 text-sm">
                      Path exceptions
                      <textarea
                        aria-label={`${titles[rule.id]} path exceptions`}
                        rows={2}
                        className={`${field} w-full resize-y font-mono`}
                        value={(rule.exclude || []).join("\n")}
                        placeholder="e.g. **/integration/**"
                        onChange={(event) =>
                          change(rule.id, {
                            exclude: lines(event.target.value),
                          })
                        }
                      />
                    </label>
                    <label className="block space-y-1 text-sm">
                      File exception marker (whole line match)
                      <textarea
                        aria-label={`${titles[rule.id]}Exception marker`}
                        rows={2}
                        className={`${field} w-full resize-y font-mono`}
                        value={(rule.excludeMarkers || []).join("\n")}
                        placeholder="e.g. //go:build integration"
                        onChange={(event) =>
                          change(rule.id, {
                            excludeMarkers: lines(event.target.value),
                          })
                        }
                      />
                    </label>
                  </fieldset>
                </ActionDialog>
                {rule.level !== "off" &&
                  rule.id === "no-unit-tests" &&
                  !(rule.include || []).length && (
                    <p className="text-xs text-removed">
                      Declare unit test paths and integration test exceptions
                      before checking code.
                    </p>
                  )}
              </div>
            );
          })}
        </div>
      )}
      {path && (
        <>
          {dirty && (
            <p role="status" className="text-xs text-muted">
              Save rule changes before checking or updating AI rules.
            </p>
          )}
          {state.report && (
            <section aria-label="Rule check results" className="space-y-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <h3 className="text-sm font-semibold">
                  {!state.report.rulesChecked.length
                    ? "Code checks disabled"
                    : !state.report.complete
                      ? "Checks incomplete"
                      : state.report.passed
                        ? "Checks passed"
                        : "Checks failed"}
                </h3>
                <span className="text-xs text-muted">
                  {new Date(state.report.checkedAt).toLocaleTimeString("en-US")}
                </span>
              </div>
              {state.report.errors.map((error, index) => (
                <p
                  key={index}
                  role="alert"
                  className="break-words text-sm text-removed"
                >
                  {error}
                </p>
              ))}
              {state.report.skipped.length > 0 && (
                <ActionDialog
                  title={
                    <>
                      {state.report.skipped.length} files by ESLint Skipped by
                      configuration
                    </>
                  }
                  busy={!!state.busy}
                  error={state.error}
                  notice={state.notice}
                >
                  <pre className="source-panel mt-2">
                    {state.report.skipped.join("\n")}
                  </pre>
                </ActionDialog>
              )}
              {state.report.baseline === "unavailable" && (
                <p className="text-xs text-muted">
                  Missing Git HEAD baseline. Existing and new violations cannot
                  yet be distinguished.
                </p>
              )}
              <div
                role="group"
                aria-label="Violation scope"
                className="flex flex-wrap gap-2"
              >
                {[
                  ["new", "New"],
                  ["historical", "Existing"],
                  ["unknown", "Unclassified"],
                ].map(([kind, label]) => (
                  <button
                    key={kind}
                    aria-pressed={state.filter === kind}
                    className={`min-h-8 rounded-md px-3 text-sm ${state.filter === kind ? "bg-selection text-accent" : "bg-surface text-muted"}`}
                    onClick={() => update({ filter: kind })}
                  >
                    {label} ·{" "}
                    {
                      state.report!.findings.filter(
                        (finding) => finding.kind === kind,
                      ).length
                    }
                  </button>
                ))}
              </div>
              <div className="grouped-list">
                {state.report.findings
                  .filter((finding) => finding.kind === state.filter)
                  .map((finding, index) => (
                    <div key={index} className="space-y-1 px-3 py-2">
                      <p className="break-all font-mono text-xs">
                        {finding.path}:{finding.line}:{finding.column}
                      </p>
                      <p
                        className={`text-sm ${finding.level === "error" ? "text-removed" : "text-muted"}`}
                      >
                        {finding.message}
                      </p>
                    </div>
                  ))}
              </div>
              {!state.report.findings.some(
                (finding) => finding.kind === state.filter,
              ) && (
                <p className="text-sm text-muted">
                  No violations in this scope.
                </p>
              )}
            </section>
          )}
          <ActionDialog
            title="AI Rule guidance"
            busy={!!state.busy}
            error={state.error}
            notice={state.notice}
          >
            <Button
              disabled={!native || !!state.busy || !!dirty || !state.project}
              onClick={() => void preview()}
            >
              {state.busy === "preview"
                ? "Generating preview…"
                : "Preview AGENTS.md Update"}
            </Button>
            {state.preview && (
              <div className="space-y-3">
                <pre
                  className="source-panel max-h-80"
                  tabIndex={0}
                  aria-label="Project rule diff"
                >
                  {state.preview.diff || "Rule guidance is up to date."}
                </pre>
                <Button
                  primary
                  disabled={!!state.busy || !state.preview.changed}
                  onClick={() => void apply()}
                >
                  {state.busy === "apply" ? "Applying…" : "Apply rule update"}
                </Button>
              </div>
            )}
          </ActionDialog>
        </>
      )}
    </section>
  );
}
