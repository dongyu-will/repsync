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
  "no-unit-tests": "禁止新增单元测试",
  "react-no-use-state": "React 使用 useReducer，禁用 useState",
};
const levels = [
  ["off", "关闭"],
  ["advice", "仅提示 AI"],
  ["warn", "检查警告"],
  ["error", "检查失败"],
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
        notice: "规则已保存，重新检查后查看结果。",
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
      update({ preview: null, notice: "已更新 AGENTS.md，更新前内容已备份。" });
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
      aria-label={path ? "项目规则" : "公共项目规则"}
      className="space-y-4"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-xl font-semibold">
          {path ? "项目规则" : "公共项目规则"}
        </h2>
        <div className="flex items-center gap-2">
          <button
            className="icon-button"
            aria-label="重新载入规则"
            title="重新载入规则"
            disabled={!!state.busy}
            onClick={() => {
              if (dirty && !window.confirm("放弃未保存的规则并重新载入？"))
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
              {state.busy === "check" ? "检查中…" : "检查规则"}
            </Button>
          )}
          <Button
            primary
            disabled={!native || !!state.busy || !state.defaults || !dirty}
            onClick={() => void save()}
          >
            {state.busy === "save" ? "保存中…" : "保存规则"}
          </Button>
        </div>
      </div>
      {!path && (
        <p className="text-sm text-muted">
          作为项目默认值，项目可单独设置例外。
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
            正在读取规则…
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
                    {path && (
                      <p className="mt-1 text-xs text-muted">
                        {override
                          ? "项目覆盖"
                          : `公共默认 · ${levels.find(([value]) => value === inherited.level)?.[1]}`}
                        {!effective?.applies && rule.scope === "react"
                          ? " · 检查工作目录中未识别到 React 依赖"
                          : ""}
                      </p>
                    )}
                  </div>
                  <label className="text-sm">
                    <span className="sr-only">{titles[rule.id]}执行强度</span>
                    <Select
                      aria-label={`${titles[rule.id]}执行强度`}
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
                      {path && <option value="inherit">继承公共默认</option>}
                      {levels.map(([value, label]) => (
                        <option key={value} value={value}>
                          {label}
                        </option>
                      ))}
                    </Select>
                  </label>
                </div>
                <details>
                  <summary className="w-fit cursor-pointer text-xs text-muted">
                    适用范围与例外
                  </summary>
                  <fieldset
                    disabled={!!state.busy || (path ? !override : false)}
                    className="mt-3 space-y-3"
                  >
                    <label className="flex flex-wrap items-center justify-between gap-2 text-sm">
                      适用项目
                      <Select
                        value={rule.scope}
                        onChange={(event) =>
                          change(rule.id, { scope: event.target.value })
                        }
                      >
                        <option value="all">全部项目</option>
                        <option value="react">React 项目</option>
                      </Select>
                    </label>
                    {path && !override && (
                      <p className="text-xs text-muted">
                        选择项目执行强度后，可修改项目例外。
                      </p>
                    )}
                    {rule.id === "react-no-use-state" && (
                      <label className="block space-y-1 text-sm">
                        ESLint 工作目录
                        <input
                          aria-label="ESLint 工作目录"
                          className={`${field} w-full`}
                          value={rule.cwd}
                          placeholder="留空使用项目根目录，例如 frontend"
                          onChange={(event) =>
                            change(rule.id, { cwd: event.target.value })
                          }
                        />
                      </label>
                    )}
                    <label className="block space-y-1 text-sm">
                      {rule.id === "no-unit-tests"
                        ? "单元测试路径（每行一个）"
                        : "源码路径（留空检查全部 JS / TS 源码）"}
                      <textarea
                        aria-label={`${titles[rule.id]}识别路径`}
                        rows={2}
                        className={`${field} w-full resize-y font-mono`}
                        value={(rule.include || []).join("\n")}
                        placeholder={
                          rule.id === "no-unit-tests"
                            ? "例如 **/*.unit.test.ts"
                            : "例如 frontend/src/**"
                        }
                        onChange={(event) =>
                          change(rule.id, {
                            include: lines(event.target.value),
                          })
                        }
                      />
                    </label>
                    <label className="block space-y-1 text-sm">
                      路径例外
                      <textarea
                        aria-label={`${titles[rule.id]}路径例外`}
                        rows={2}
                        className={`${field} w-full resize-y font-mono`}
                        value={(rule.exclude || []).join("\n")}
                        placeholder="例如 **/integration/**"
                        onChange={(event) =>
                          change(rule.id, {
                            exclude: lines(event.target.value),
                          })
                        }
                      />
                    </label>
                    <label className="block space-y-1 text-sm">
                      文件例外标记（整行匹配）
                      <textarea
                        aria-label={`${titles[rule.id]}例外标记`}
                        rows={2}
                        className={`${field} w-full resize-y font-mono`}
                        value={(rule.excludeMarkers || []).join("\n")}
                        placeholder="例如 //go:build integration"
                        onChange={(event) =>
                          change(rule.id, {
                            excludeMarkers: lines(event.target.value),
                          })
                        }
                      />
                    </label>
                  </fieldset>
                </details>
                {rule.level !== "off" &&
                  rule.id === "no-unit-tests" &&
                  !(rule.include || []).length && (
                    <p className="text-xs text-removed">
                      代码检查前需要明确单元测试路径及集成测试例外。
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
              有未保存的规则，保存后再检查或更新 AI 规范。
            </p>
          )}
          {state.report && (
            <section aria-label="规则检查结果" className="space-y-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <h3 className="text-sm font-semibold">
                  {!state.report.rulesChecked.length
                    ? "未启用代码检查"
                    : !state.report.complete
                      ? "检查未完成"
                      : state.report.passed
                        ? "检查通过"
                        : "检查未通过"}
                </h3>
                <span className="text-xs text-muted">
                  {new Date(state.report.checkedAt).toLocaleTimeString("zh-CN")}
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
                <details className="text-xs text-muted">
                  <summary className="cursor-pointer">
                    {state.report.skipped.length} 个文件按 ESLint 配置跳过
                  </summary>
                  <pre className="source-panel mt-2">
                    {state.report.skipped.join("\n")}
                  </pre>
                </details>
              )}
              {state.report.baseline === "unavailable" && (
                <p className="text-xs text-muted">
                  缺少 Git HEAD 基线，违规暂时无法区分历史与新增。
                </p>
              )}
              <div
                role="group"
                aria-label="违规范围"
                className="flex flex-wrap gap-2"
              >
                {[
                  ["new", "新增"],
                  ["historical", "历史"],
                  ["unknown", "待判定"],
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
              ) && <p className="text-sm text-muted">此范围没有违规。</p>}
            </section>
          )}
          <details className="space-y-3">
            <summary className="w-fit cursor-pointer text-sm font-medium">
              AI 规范说明
            </summary>
            <Button
              disabled={!native || !!state.busy || !!dirty || !state.project}
              onClick={() => void preview()}
            >
              {state.busy === "preview" ? "预览中…" : "预览 AGENTS.md 更新"}
            </Button>
            {state.preview && (
              <div className="space-y-3">
                <pre
                  className="source-panel max-h-80"
                  tabIndex={0}
                  aria-label="项目规则差异"
                >
                  {state.preview.diff || "规则说明已是最新。"}
                </pre>
                <Button
                  primary
                  disabled={!!state.busy || !state.preview.changed}
                  onClick={() => void apply()}
                >
                  {state.busy === "apply" ? "应用中…" : "应用规范更新"}
                </Button>
              </div>
            )}
          </details>
        </>
      )}
    </section>
  );
}
