import ActionDialog from "./ActionDialog";
import { useReducer, type FormEvent, type ReactNode } from "react";
import { isMyGo } from "mygo-runtime";
import {
  Service,
  type ProjectReview,
  type ReviewResource,
  type ReviewCoverage,
  type ReviewIssue,
  type ReviewRequirement,
} from "./mygo";
import { Button, Select } from "./components";
import { reviewPreview } from "./review-preview";
import ProjectDocumentReview from "./ProjectDocumentReview";

const native = isMyGo();
const sections = [
  ["changes", "改动"],
  ["checks", "检查"],
  ["delivery", "交付"],
  ["resources", "数据与环境"],
  ["handover", "交接"],
  ["settings", "适用项"],
] as const;
type Section = (typeof sections)[number][0];
const status: Record<string, string> = {
  pending: "待核验",
  passed: "命令通过",
  unknown: "未知",
  observed: "已读取本地状态",
  recorded: "已记录核验",
  success: "通过",
  failure: "未通过",
  failed: "未通过",
  stale: "已变化",
  current: "内容一致",
  not_applicable: "不适用",
  exists: "存在",
  missing: "缺失",
  set: "已定义",
  needs_confirmation: "需人工核验",
  tracked: "Git 已跟踪",
  ignored: "被 Git 忽略",
  untracked: "未纳入 Git",
  not_applicable_tracking: "",
};
const date = (value: string) =>
  new Date(value).toLocaleString("zh-CN", { hour12: false });
type State = {
  report: ProjectReview | null;
  section: Section;
  pending: boolean;
  error: string;
  notice: string;
  expanded: boolean;
  resourceTarget: string;
};

export default function ProjectReviewPanel({
  path,
  onGit,
  onCI,
  stashCount,
  workspaceStatus,
}: {
  path: string;
  onGit: () => void;
  onCI: () => void;
  stashCount: number;
  workspaceStatus: string;
}) {
  const [state, update] = useReducer(
    (s: State, p: Partial<State>) => ({ ...s, ...p }),
    {
      report: native ? null : reviewPreview(path),
      section: "changes",
      pending: false,
      error: "",
      notice: "",
      expanded: false,
      resourceTarget: "",
    },
  );
  const { report, pending } = state;
  async function perform(action: () => Promise<unknown>, notice: string) {
    if (!native) return false;
    update({ pending: true, error: "", notice: "" });
    try {
      await action();
      const report = await Service.reviewProject(path);
      update({ report, notice });
      return true;
    } catch (error) {
      update({ error: error instanceof Error ? error.message : String(error) });
      return false;
    } finally {
      update({ pending: false });
    }
  }
  async function refresh() {
    update({ pending: true, error: "", notice: "" });
    try {
      update({ report: await Service.reviewProject(path) });
    } catch (error) {
      update({ error: error instanceof Error ? error.message : String(error) });
    } finally {
      update({ pending: false });
    }
  }
  async function submit(
    event: FormEvent<HTMLFormElement>,
    action: (data: FormData) => Promise<unknown>,
    notice: string,
  ) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    if (await perform(() => action(data), notice)) form.reset();
  }
  const value = (data: FormData, name: string) => String(data.get(name) || "");
  const select = (section: string) => {
    update({ section: section as Section, expanded: true });
  };
  return (
    <section
      aria-label="项目待处理"
      className="mt-5 space-y-3 border-t border-line pt-5"
    >
      <div className="flex items-center justify-between gap-3">
        <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
          <h2 className="text-base font-semibold">
            待处理{report ? ` · ${report.issues.length}` : ""}
          </h2>
          <span className="text-xs text-muted">{workspaceStatus}</span>
        </div>
        <Button disabled={!native || pending} onClick={refresh}>
          {pending ? "正在检查…" : "检查当前状态"}
        </Button>
      </div>
      {state.error && (
        <p
          role="alert"
          className="break-words rounded-md bg-red-50 p-3 text-sm text-red-800"
        >
          {state.error}
        </p>
      )}
      {state.notice && (
        <p role="status" className="text-sm text-muted">
          {state.notice}
        </p>
      )}
      {!report ? (
        <>
          {stashCount > 0 && (
            <div
              role="status"
              className="flex flex-wrap items-center justify-between gap-2 rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-900"
            >
              <p>还有 {stashCount} 条 stash 待处理</p>
              <button
                className="rounded px-2 py-1 underline underline-offset-2"
                onClick={() => {
                  update({ expanded: false });
                  onGit();
                }}
              >
                查看 stash
              </button>
            </div>
          )}
        </>
      ) : (
        <>
          {report.issues.length ? (
            <ul className="grouped-list">
              {report.issues.slice(0, 3).map((issue) => (
                <li
                  key={issue.key || issue.kind + issue.message}
                  className="flex items-center justify-between gap-3 border-b border-line-subtle px-3 py-2 text-sm last:border-0"
                >
                  <span className="min-w-0 break-words">{issue.message}</span>
                  <button
                    className="shrink-0 rounded px-2 py-1 text-accent hover:bg-selection"
                    onClick={() =>
                      issue.kind.startsWith("stash")
                        ? onGit()
                        : select(issue.section)
                    }
                  >
                    查看
                  </button>
                </li>
              ))}
            </ul>
          ) : (
            <p role="status" className="text-sm">
              未发现已声明范围内的待处理项。
            </p>
          )}
          <ActionDialog
            title="项目检查与记录"
            trigger={
              <>
                {report.issues.length > 3
                  ? `查看全部 ${report.issues.length} 项与记录`
                  : "查看详情与记录"}
              </>
            }
            open={state.expanded}
            onOpenChange={(expanded) => update({ expanded })}
            busy={pending}
            error={state.error}
            notice={state.notice}
          >
            <div className="mt-3 space-y-4">
              <p className="text-xs text-muted">
                读取于 {date(report.checkedAt)} · 不代表提交、推送或发布已完成
              </p>
              <div
                role="group"
                aria-label="收尾检查分区"
                className="flex w-fit max-w-full flex-wrap gap-1 rounded-lg bg-sidebar p-1"
              >
                {sections.map(([id, label]) => (
                  <button
                    key={id}
                    aria-pressed={state.section === id}
                    onClick={() => select(id)}
                    className={`min-h-8 rounded-md px-3 text-sm ${state.section === id ? "bg-white font-medium" : "text-muted hover:text-ink"}`}
                  >
                    {label}
                  </button>
                ))}
              </div>
              {report.issues.some((i) => i.section === state.section) && (
                <section>
                  <h3 className="break-words text-sm font-medium">
                    核对与处理提醒 ·{" "}
                    {
                      report.issues.filter((i) => i.section === state.section)
                        .length
                    }
                  </h3>
                  <div className="mt-3">
                    <IssueList
                      issues={report.issues.filter(
                        (i) => i.section === state.section,
                      )}
                      pending={pending}
                      save={(issue, next, reason) =>
                        perform(
                          () =>
                            Service.setReviewIssueState(
                              path,
                              report.version,
                              issue.key || "",
                              next,
                              reason,
                            ),
                          "已更新提醒处理状态",
                        )
                      }
                    />
                  </div>
                </section>
              )}
              {report.acknowledged.some((i) => i.section === state.section) && (
                <section>
                  <h3 className="break-words text-sm font-medium">
                    已核对保留 / 稍后处理
                  </h3>
                  <IssueList
                    issues={report.acknowledged.filter(
                      (i) => i.section === state.section,
                    )}
                    pending={pending}
                    save={(issue, next, reason) =>
                      perform(
                        () =>
                          Service.setReviewIssueState(
                            path,
                            report.version,
                            issue.key || "",
                            next,
                            reason,
                          ),
                        "提醒已重新打开",
                      )
                    }
                  />
                </section>
              )}
              {state.section === "settings" && (
                <ReviewForm
                  key={report.configVersion}
                  pending={pending}
                  onSubmit={(event) =>
                    submit(
                      event,
                      (data) =>
                        Service.saveReviewProfile(path, report.configVersion, {
                          kind: value(data, "kind"),
                          areas: data.getAll("areas").map(String),
                          requirements: data.getAll("requirements").map(String),
                          stages: data.getAll("stages").map(String),
                        }),
                      "项目适用项已保存",
                    )
                  }
                >
                  <p className="text-xs leading-5 text-muted">
                    由 AI
                    按项目指南配置；这里可以调整。只提醒所选范围，保存后不执行检查。
                  </p>
                  <label className="block space-y-1">
                    <span className="text-xs text-muted">项目类型</span>
                    <Select
                      name="kind"
                      defaultValue={
                        report.profile.kind === "unconfigured"
                          ? "custom"
                          : report.profile.kind
                      }
                    >
                      <option value="web">Web 应用</option>
                      <option value="backend">后端服务</option>
                      <option value="desktop">桌面应用</option>
                      <option value="library">库 / 工具</option>
                      <option value="notes">文档 / 笔记</option>
                      <option value="custom">自定义</option>
                    </Select>
                  </label>
                  {Object.entries(areas).map(([area, name]) => (
                    <fieldset
                      key={area}
                      className="space-y-2 border-t border-line pt-3"
                    >
                      <legend className="text-sm font-medium">{name}</legend>
                      <CheckBox
                        name="areas"
                        value={area}
                        checked={report.profile.areas.includes(area)}
                      >
                        此项目包含{name}
                      </CheckBox>
                      <p className="text-xs text-muted">需要核对的检查项</p>
                      {report.coverageOptions
                        .filter((r) => r.area === area)
                        .map((r) => (
                          <CheckBox
                            key={r.id}
                            name="requirements"
                            value={r.id}
                            checked={report.profile.requirements.includes(r.id)}
                          >
                            {r.name}
                          </CheckBox>
                        ))}
                    </fieldset>
                  ))}
                  <fieldset className="space-y-2 border-t border-line pt-3">
                    <legend className="text-sm font-medium">交付阶段</legend>
                    {report.delivery
                      .filter((s) => !["working", "commit"].includes(s.stage))
                      .map((s) => (
                        <CheckBox
                          key={s.stage}
                          name="stages"
                          value={s.stage}
                          checked={report.profile.stages.includes(s.stage)}
                        >
                          {s.name}
                        </CheckBox>
                      ))}
                  </fieldset>
                  <Submit pending={pending}>保存适用项</Submit>
                </ReviewForm>
              )}
              {state.section === "changes" && (
                <>
                  <dl className="grouped-list">
                    <Info label="上游引用">
                      {report.snapshot.upstream || "未配置或无法读取"}
                    </Info>
                    <Info label="提交差异">
                      {report.snapshot.upstreamKnown
                        ? `领先 ${report.snapshot.ahead} / 落后 ${report.snapshot.behind}`
                        : "未核验"}
                    </Info>
                    <Info label="最近获取">
                      {report.snapshot.fetchedAt
                        ? date(report.snapshot.fetchedAt)
                        : "未知"}
                    </Info>
                  </dl>
                  <p className="text-xs leading-5 text-muted">
                    以上按本地 Git 引用计算，未联网确认远程现状。
                  </p>
                  <Button
                    onClick={() => {
                      update({ expanded: false });
                      onGit();
                    }}
                  >
                    查看工作树、分支和 stash
                  </Button>
                  {report.snapshot.files.length ? (
                    <ul className="grouped-list max-h-64 overflow-auto">
                      {report.snapshot.files.slice(0, 100).map((file) => (
                        <li
                          key={file.path}
                          className="flex items-baseline gap-3 border-b border-line-subtle px-3 py-2 text-sm last:border-0"
                        >
                          <span className="shrink-0 text-xs text-muted">
                            {file.conflict
                              ? "冲突"
                              : file.untracked
                                ? "未跟踪"
                                : `${file.index !== " " ? "已暂存" : ""}${file.index !== " " && file.worktree !== " " ? " / " : ""}${file.worktree !== " " ? "未暂存" : ""}`}
                          </span>
                          <span className="min-w-0 break-all">{file.path}</span>
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <p className="text-sm text-muted">
                      {report.snapshot.error || "没有已读取的文件改动。"}
                    </p>
                  )}
                  {report.snapshot.files.length > 100 && (
                    <p className="text-xs text-muted">
                      这里只列前 100 项，CLI 回执包含完整的已读取列表。
                    </p>
                  )}
                </>
              )}
              {state.section === "checks" && (
                <>
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <p className="text-xs leading-5 text-muted">
                      比较文件内容和命令配置；未覆盖被忽略的依赖或外部环境。提交变化后需确认检查是否依赖
                      Git 状态。
                    </p>
                    <Button
                      onClick={() => {
                        update({ expanded: false });
                        onCI();
                      }}
                    >
                      前往 CI/CD
                    </Button>
                  </div>
                  <div className="grouped-list">
                    {report.coverage.map((item) => (
                      <div
                        key={item.requirement.id}
                        className="space-y-1 border-b border-line-subtle p-3 last:border-0"
                      >
                        <p className="text-sm font-medium">
                          {item.requirement.name} ·{" "}
                          {status[item.status] || item.status}
                        </p>
                        <p className="break-words text-xs leading-5 text-muted">
                          {item.reason}
                        </p>
                        {(item.environment || item.target) && (
                          <p className="break-all text-xs text-muted">
                            环境：{item.environment || "未填"} · 目标：
                            {item.target || "未填"}
                          </p>
                        )}
                        {item.evidence && (
                          <p className="break-words text-xs text-muted">
                            {item.source === "pipeline"
                              ? "应用执行"
                              : "外部填报"}{" "}
                            · {item.evidence}
                          </p>
                        )}
                      </div>
                    ))}
                  </div>
                  {!report.coverage.length && (
                    <p className="text-sm text-muted">
                      尚未选择必需检查项，请在「适用项」中配置。
                    </p>
                  )}
                  {report.checks.length ? (
                    <div className="grouped-list">
                      {report.checks.map((check) => (
                        <div
                          key={check.id}
                          className="space-y-1 border-b border-line-subtle p-3 last:border-0"
                        >
                          <p className="break-words text-sm font-medium">
                            {check.command} ·{" "}
                            {status[check.status] || check.status}
                          </p>
                          <p className="text-xs text-muted">
                            {check.source === "pipeline"
                              ? "应用执行"
                              : "外部填报，未由应用执行"}{" "}
                            · {date(check.createdAt)} · {status[check.current]}
                          </p>
                          <p className="break-words text-xs leading-5 text-muted">
                            范围：{check.scope}；{check.reason}
                            {check.coverage?.length
                              ? `；${check.coverage.map((c) => coverageName(report.coverageOptions, c)).join("、")}`
                              : "；未声明核验类别"}
                          </p>
                          <p className="break-words text-xs text-muted">
                            {check.evidence}
                            {(check.environment || check.target) &&
                              ` · 环境：${check.environment || "未填"} / 目标：${check.target || "未填"}`}
                          </p>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <p className="text-sm text-muted">尚无检查记录。</p>
                  )}
                  <section>
                    <h3 className="break-words text-sm font-medium">
                      记录外部检查
                    </h3>
                    <ReviewForm
                      pending={pending}
                      onSubmit={(event) =>
                        submit(
                          event,
                          (data) =>
                            Service.recordReviewCheck(path, report.version, {
                              command: value(data, "command"),
                              scope: value(data, "scope"),
                              status: value(data, "status"),
                              evidence: value(data, "evidence"),
                              coverage: selectedCoverage(
                                data,
                                report.coverageOptions,
                              ),
                              environment: value(data, "environment"),
                              target: value(data, "target"),
                            }),
                          "已记录外部检查说明",
                        )
                      }
                    >
                      <Field name="command" label="检查命令" required />
                      <Field name="scope" label="实际覆盖范围与限制" required />
                      <fieldset className="space-y-2">
                        <legend className="mb-2 text-xs text-muted">
                          本次实际核验的类别（可多选）
                        </legend>
                        {report.coverageOptions.map((r) => (
                          <CheckBox key={r.id} name="coverage" value={r.id}>
                            {r.name}
                          </CheckBox>
                        ))}
                        <CheckBox name="coverage" value="database-static">
                          数据库脚本静态检查（不抵扣真实数据库核验）
                        </CheckBox>
                      </fieldset>
                      <EnvironmentFields />
                      <ResultSelect />
                      <Field
                        name="evidence"
                        label="结果说明或日志位置"
                        multiline
                        required
                      />
                      <Submit pending={pending}>记录检查</Submit>
                    </ReviewForm>
                  </section>
                </>
              )}
              {state.section === "delivery" && (
                <>
                  <div className="grouped-list">
                    {report.delivery.map((stage) => (
                      <div
                        key={stage.stage}
                        className="space-y-1 border-b border-line-subtle p-3 last:border-0"
                      >
                        <p className="text-sm font-medium">
                          {stage.name} · {status[stage.status] || stage.status}
                        </p>
                        <p className="break-words text-xs leading-5 text-muted">
                          {stage.detail}
                        </p>
                        {stage.recordedAt && (
                          <p className="text-xs text-muted">
                            记录于 {date(stage.recordedAt)}
                          </p>
                        )}
                      </div>
                    ))}
                  </div>
                  <section>
                    <h3 className="break-words text-sm font-medium">
                      记录阶段核验
                    </h3>
                    <ReviewForm
                      pending={pending}
                      onSubmit={(event) =>
                        submit(
                          event,
                          (data) =>
                            Service.recordReviewEvidence(path, report.version, {
                              stage: value(data, "stage"),
                              target: "",
                              status: value(data, "status"),
                              note: value(data, "note"),
                            }),
                          "已保存手工核验记录",
                        )
                      }
                    >
                      <label className="block space-y-1">
                        <span className="text-xs text-muted">阶段</span>
                        <Select name="stage">
                          {report.delivery
                            .filter(
                              (s) => !["working", "commit"].includes(s.stage),
                            )
                            .map((s) => (
                              <option key={s.stage} value={s.stage}>
                                {s.name}
                              </option>
                            ))}
                        </Select>
                      </label>
                      <ResultSelect allowNA />
                      <Field
                        name="note"
                        label="核验方式、提交或结果链接"
                        multiline
                        required
                      />
                      <p className="text-xs text-muted">
                        保存为手工核验记录，不会执行推送、部署或验收命令。
                      </p>
                      <Submit pending={pending}>保存核验</Submit>
                    </ReviewForm>
                  </section>
                </>
              )}
              {state.section === "resources" && (
                <>
                  {report.diagnosis.length > 0 && (
                    <ul className="grouped-list">
                      {report.diagnosis.map((d, i) => (
                        <li
                          key={i}
                          className="border-b border-line-subtle p-3 text-sm last:border-0"
                        >
                          {d.message}
                        </li>
                      ))}
                    </ul>
                  )}
                  {report.resources.length ? (
                    <div className="grouped-list">
                      {report.resources.map((resource) => (
                        <div
                          key={resource.state.definition.id}
                          className="space-y-2 border-b border-line-subtle p-3 last:border-0"
                        >
                          <div className="flex flex-wrap items-center justify-between gap-2">
                            <p className="text-sm font-medium">
                              {resource.state.definition.name} ·{" "}
                              {resource.status === "success"
                                ? "已记录核验"
                                : status[resource.status]}
                            </p>
                            <button
                              className="rounded px-2 py-1 text-xs text-accent hover:bg-selection"
                              onClick={() =>
                                update({
                                  resourceTarget: resource.state.definition.id,
                                })
                              }
                            >
                              记录核验
                            </button>
                          </div>
                          <p className="break-all text-xs text-muted">
                            {resource.state.definition.target || "手工确认"} ·{" "}
                            {status[resource.state.observed]}
                            {resource.state.tracking !== "not_applicable"
                              ? ` · ${status[resource.state.tracking] || resource.state.tracking}`
                              : ""}
                          </p>
                          {resource.state.error && (
                            <p className="break-words text-xs text-removed">
                              {resource.state.error}
                            </p>
                          )}
                          <p className="break-words text-xs leading-5 text-muted">
                            {resource.state.definition.guide ||
                              "存在不代表已备份或可恢复，请记录核验方法。"}
                          </p>
                          {resource.note && (
                            <p className="break-words text-xs text-muted">
                              填报：{resource.note}
                            </p>
                          )}
                          {resource.recordedAt && (
                            <p className="text-xs text-muted">
                              填报于 {date(resource.recordedAt)}
                            </p>
                          )}
                          {state.resourceTarget ===
                            resource.state.definition.id && (
                            <ReviewForm
                              pending={pending}
                              onSubmit={(event) =>
                                submit(
                                  event,
                                  (data) =>
                                    Service.recordReviewEvidence(
                                      path,
                                      report.version,
                                      {
                                        stage: "resource",
                                        target: resource.state.definition.id,
                                        status: value(data, "status"),
                                        note: value(data, "note"),
                                      },
                                    ),
                                  "已保存数据与环境核验说明",
                                )
                              }
                            >
                              <ResultSelect allowNA />
                              <Field
                                name="note"
                                label="备份、恢复演练或状态核验说明"
                                multiline
                                required
                              />
                              <Submit pending={pending}>保存核验</Submit>
                            </ReviewForm>
                          )}
                          <button
                            disabled={!native || pending}
                            className="rounded px-2 py-1 text-xs text-muted hover:bg-sidebar disabled:opacity-45"
                            onClick={() =>
                              perform(
                                () =>
                                  Service.saveReviewResources(
                                    path,
                                    report.configVersion,
                                    report.resources
                                      .filter(
                                        (r) =>
                                          r.state.definition.id !==
                                          resource.state.definition.id,
                                      )
                                      .map((r) => r.state.definition),
                                  ),
                                "已移除检查声明，未删除数据",
                              )
                            }
                          >
                            移除检查声明
                          </button>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <p className="text-sm text-muted">
                      尚未声明需要核验的数据或环境对象。
                    </p>
                  )}
                  <section>
                    <h3 className="break-words text-sm font-medium">
                      添加检查对象
                    </h3>
                    <ReviewForm
                      pending={pending}
                      onSubmit={(event) =>
                        submit(
                          event,
                          (data) => {
                            const resource: ReviewResource = {
                              id: crypto.randomUUID(),
                              name: value(data, "name"),
                              kind: value(data, "kind"),
                              target: value(data, "target"),
                              guide: value(data, "guide"),
                            };
                            return Service.saveReviewResources(
                              path,
                              report.configVersion,
                              [
                                ...report.resources.map(
                                  (r) => r.state.definition,
                                ),
                                resource,
                              ],
                            );
                          },
                          "检查对象已保存，不执行项目命令",
                        )
                      }
                    >
                      <Field name="name" label="名称" required />
                      <label className="block space-y-1">
                        <span className="text-xs text-muted">类型</span>
                        <Select name="kind">
                          <option value="file">项目内文件</option>
                          <option value="directory">项目内目录</option>
                          <option value="environment">
                            环境变量（仅检查是否定义）
                          </option>
                          <option value="manual">
                            手工核验数据库、迁移或备份
                          </option>
                        </Select>
                      </label>
                      <Field
                        name="target"
                        label="项目内相对路径或环境变量名（手工核验可留空）"
                      />
                      <Field
                        name="guide"
                        label="核验方法或项目指南"
                        multiline
                      />
                      <Submit pending={pending}>保存检查对象</Submit>
                    </ReviewForm>
                  </section>
                </>
              )}
              {state.section === "handover" && (
                <>
                  <ReviewForm
                    pending={pending}
                    onSubmit={(event) =>
                      submit(
                        event,
                        (data) =>
                          Service.startReviewTaskWithContext(
                            path,
                            report.version,
                            {
                              title: value(data, "title"),
                              owner: value(data, "owner"),
                              areas: data.getAll("areas").map(String),
                              paths: value(data, "paths")
                                .split("\n")
                                .map((v) => v.trim())
                                .filter(Boolean),
                              environment: value(data, "environment"),
                              target: value(data, "target"),
                            },
                          ),
                        "已记录任务开始状态",
                      )
                    }
                  >
                    <Field name="title" label="任务名称" required />
                    <section>
                      <h3 className="break-words text-sm font-medium">
                        声明负责人和修改范围
                      </h3>
                      <div className="mt-3 space-y-3">
                        <Field name="owner" label="负责人 / AI 会话别名" />
                        <fieldset className="space-y-2">
                          <legend className="mb-2 text-xs text-muted">
                            涉及范围
                          </legend>
                          {Object.entries(areas).map(([area, name]) => (
                            <CheckBox key={area} name="areas" value={area}>
                              {name}
                            </CheckBox>
                          ))}
                        </fieldset>
                        <Field
                          name="paths"
                          label="项目内文件或目录（每行一个相对路径）"
                          multiline
                        />
                        <EnvironmentFields />
                      </div>
                    </section>
                    <Submit pending={pending}>记录任务开始</Submit>
                  </ReviewForm>
                  <p className="text-xs leading-5 text-muted">
                    开始和结束均手动记录；期间新增内容不推定由某个 AI 创建。
                  </p>
                  {report.taskWarnings.map((warning) => (
                    <p
                      key={warning}
                      role="status"
                      className="break-words text-sm text-removed"
                    >
                      {warning}
                    </p>
                  ))}
                  {report.tasks.map((task) => (
                    <section
                      key={task.id}
                      className="border-t border-line pt-3"
                    >
                      <h3 className="break-words text-sm font-medium">
                        {task.title} · {task.endedAt ? "已交接" : "尚未交接"}
                      </h3>
                      <p className="mt-2 break-all text-xs text-muted">
                        {date(task.startedAt)} · {task.start.branch} ·{" "}
                        {task.start.head.slice(0, 8)}
                      </p>
                      {(task.owner ||
                        task.areas?.length ||
                        task.paths?.length ||
                        task.environment ||
                        task.target) && (
                        <p className="mt-2 break-words text-xs leading-5 text-muted">
                          {task.owner || "负责人未填"} ·{" "}
                          {task.areas?.map((a) => areas[a] || a).join("、") ||
                            "范围未填"}
                          {task.paths?.length
                            ? ` · ${task.paths.join("、")}`
                            : ""}
                          {(task.environment || task.target) &&
                            ` · ${task.environment} / ${task.target}`}
                        </p>
                      )}
                      {task.endedAt ? (
                        <div className="mt-3 space-y-2">
                          <p className="text-xs text-muted">
                            期间新增 {task.newStashes.length} 条 stash /{" "}
                            {task.newUntracked.length} 个未跟踪文件
                          </p>
                          <CopyHandover text={task.handover} />
                        </div>
                      ) : (
                        <ReviewForm
                          pending={pending}
                          onSubmit={(event) =>
                            submit(
                              event,
                              (data) =>
                                Service.finishReviewTask(
                                  path,
                                  task.id,
                                  report.version,
                                  {
                                    summary: value(data, "summary"),
                                    checks: value(data, "checks"),
                                    blockers: value(data, "blockers"),
                                  },
                                ),
                              "任务交接已保存",
                            )
                          }
                        >
                          <Field
                            name="summary"
                            label="完成内容"
                            multiline
                            required
                          />
                          <Field
                            name="checks"
                            label="检查说明与结果位置"
                            multiline
                          />
                          <Field
                            name="blockers"
                            label="遗留问题与下一步"
                            multiline
                          />
                          <Submit pending={pending}>保存交接</Submit>
                        </ReviewForm>
                      )}
                    </section>
                  ))}
                </>
              )}
            </div>
          </ActionDialog>
        </>
      )}
      <ProjectDocumentReview path={path} />
    </section>
  );
}

function Info({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="info-row">
      <dt className="text-muted">{label}</dt>
      <dd className="break-all">{children}</dd>
    </div>
  );
}
function ReviewForm({
  children,
  onSubmit,
  pending,
}: {
  children: ReactNode;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  pending: boolean;
}) {
  return (
    <form onSubmit={onSubmit} className="mt-3">
      <fieldset
        disabled={!native || pending}
        className="space-y-3 disabled:opacity-60"
      >
        {children}
      </fieldset>
    </form>
  );
}
function Field({
  name,
  label,
  multiline = false,
  required = false,
}: {
  name: string;
  label: string;
  multiline?: boolean;
  required?: boolean;
}) {
  return (
    <label className="block space-y-1">
      <span className="block text-xs text-muted">{label}</span>
      {multiline ? (
        <textarea
          name={name}
          required={required}
          maxLength={8192}
          rows={3}
          className="w-full rounded-md border border-control px-3 py-2 text-sm"
        />
      ) : (
        <input
          name={name}
          required={required}
          maxLength={2000}
          className="h-9 w-full rounded-md border border-control px-3 text-sm"
        />
      )}
    </label>
  );
}
function ResultSelect({ allowNA = false }: { allowNA?: boolean }) {
  return (
    <label className="block space-y-1">
      <span className="block text-xs text-muted">结果</span>
      <Select name="status">
        <option value="success">核验通过</option>
        <option value="failure">未通过</option>
        {allowNA && <option value="not_applicable">不适用</option>}
      </Select>
    </label>
  );
}
function Submit({
  pending,
  children,
}: {
  pending: boolean;
  children: ReactNode;
}) {
  return (
    <Button type="submit" disabled={!native || pending}>
      {pending ? "保存中…" : children}
    </Button>
  );
}
function CopyHandover({ text }: { text: string }) {
  const [notice, setNotice] = useReducer((_: string, n: string) => n, "");
  return (
    <>
      <Button
        onClick={async () => {
          try {
            await navigator.clipboard.writeText(text);
            setNotice("已复制交接");
          } catch {
            setNotice("无法写入剪贴板，请从下方选取文本复制。");
          }
        }}
      >
        复制交接
      </Button>
      {notice && (
        <p role="status" className="text-xs text-muted">
          {notice}
        </p>
      )}
      <pre tabIndex={0} className="source-panel max-h-80">
        {text}
      </pre>
    </>
  );
}

const areas: Record<string, string> = {
  frontend: "前端",
  backend: "后端",
  database: "数据库",
};
function coverageName(options: ReviewRequirement[], c: ReviewCoverage) {
  return (
    options.find((r) => r.area === c.area && r.method === c.method)?.name ||
    `${areas[c.area] || c.area} · ${c.method === "static" ? "静态检查" : c.method}`
  );
}
function selectedCoverage(
  data: FormData,
  options: ReviewRequirement[],
): ReviewCoverage[] {
  return data.getAll("coverage").flatMap((id) => {
    if (id === "database-static")
      return [{ area: "database", method: "static" }];
    const r = options.find((r) => r.id === id);
    return r ? [{ area: r.area, method: r.method }] : [];
  });
}
function CheckBox({
  name,
  value,
  checked = false,
  children,
}: {
  name: string;
  value: string;
  checked?: boolean;
  children: ReactNode;
}) {
  return (
    <label className="flex items-start gap-2 text-sm">
      <input
        className="mt-1 accent-accent"
        type="checkbox"
        name={name}
        value={value}
        defaultChecked={checked}
      />
      <span>{children}</span>
    </label>
  );
}
function EnvironmentFields() {
  return (
    <>
      <Field
        name="environment"
        label="环境别名（如 local / staging / production）"
      />
      <Field name="target" label="目标别名（服务、数据库或备份集名称）" />
      <p className="text-xs leading-5 text-muted">
        涉及数据库时必填。只填别名，勿填连接串、密码或令牌；内容一致不代表外部环境仍然一致。
      </p>
    </>
  );
}
function IssueList({
  issues,
  pending,
  save,
}: {
  issues: ReviewIssue[];
  pending: boolean;
  save: (issue: ReviewIssue, next: string, reason: string) => Promise<unknown>;
}) {
  if (!issues.length) return null;
  return (
    <div className="grouped-list">
      {issues.map((issue) => (
        <section
          key={issue.key || issue.kind}
          className="border-b border-line-subtle p-3 last:border-0"
        >
          <h3 className="break-words text-sm font-medium">{issue.message}</h3>
          {issue.acknowledgement ? (
            <div className="mt-2 space-y-2">
              <p className="break-words text-xs text-muted">
                {issue.acknowledgement.state === "retained"
                  ? "已核对保留"
                  : "稍后处理"}{" "}
                · {issue.acknowledgement.reason} ·{" "}
                {date(issue.acknowledgement.createdAt)}
              </p>
              <Button
                disabled={!native || pending}
                onClick={() => save(issue, "reopened", "")}
              >
                重新提醒
              </Button>
            </div>
          ) : (
            <ReviewForm
              pending={pending}
              onSubmit={(event) => {
                event.preventDefault();
                const form = event.currentTarget;
                const data = new FormData(form);
                void save(
                  issue,
                  String(data.get("state")),
                  String(data.get("reason")),
                ).then((ok) => {
                  if (ok) form.reset();
                });
              }}
            >
              <label className="block space-y-1">
                <span className="text-xs text-muted">处理状态</span>
                <Select name="state">
                  <option value="retained">已核对，保留</option>
                  <option value="later">稍后处理</option>
                </Select>
              </label>
              <Field name="reason" label="理由与后续安排" required multiline />
              <Submit pending={pending}>保存处理状态</Submit>
            </ReviewForm>
          )}
        </section>
      ))}
    </div>
  );
}
