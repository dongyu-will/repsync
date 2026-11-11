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
  ["changes", "Changes"],
  ["checks", "Check"],
  ["delivery", "Delivery"],
  ["resources", "Data and environment"],
  ["handover", "Handoff"],
  ["settings", "Applicability"],
] as const;
type Section = (typeof sections)[number][0];
const status: Record<string, string> = {
  pending: "Unverified",
  passed: "Command passed",
  unknown: "Unknown",
  observed: "Local state loaded",
  recorded: "Verification recorded",
  success: "Passed",
  failure: "Failed",
  failed: "Failed",
  stale: "Changed",
  current: "Content matches",
  not_applicable: "Not applicable",
  exists: "Exists",
  missing: "Missing",
  set: "Defined",
  needs_confirmation: "Manual verification required",
  tracked: "Git Tracked",
  ignored: "Ignored by Git",
  untracked: "Not included Git",
  not_applicable_tracking: "",
};
const date = (value: string) =>
  new Date(value).toLocaleString("en-US", { hour12: false });
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
      aria-label="Pending project items"
      className="mt-5 space-y-3 border-t border-line pt-5"
    >
      <div className="flex items-center justify-between gap-3">
        <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
          <h2 className="text-base font-semibold">
            Pending{report ? ` · ${report.issues.length}` : ""}
          </h2>
          <span className="text-xs text-muted">{workspaceStatus}</span>
        </div>
        <Button disabled={!native || pending} onClick={refresh}>
          {pending ? "Checking…" : "Check current state"}
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
              <p>More {stashCount} records stash Pending</p>
              <button
                className="rounded px-2 py-1 underline underline-offset-2"
                onClick={() => {
                  update({ expanded: false });
                  onGit();
                }}
              >
                View stash
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
                    View
                  </button>
                </li>
              ))}
            </ul>
          ) : (
            <p role="status" className="text-sm">
              No pending items in the declared scope.
            </p>
          )}
          <ActionDialog
            title="Project checks and records"
            trigger={
              <>
                {report.issues.length > 3
                  ? `View all ${report.issues.length}  items and records`
                  : "View details and history"}
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
                Loaded at {date(report.checkedAt)} · Does not confirm commit,
                push, or release completion
              </p>
              <div
                role="group"
                aria-label="Review sections"
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
                    Review and resolve reminders ·{" "}
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
                          "Reminder status updated",
                        )
                      }
                    />
                  </div>
                </section>
              )}
              {report.acknowledged.some((i) => i.section === state.section) && (
                <section>
                  <h3 className="break-words text-sm font-medium">
                    Reviewed; kept / Defer
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
                        "Reminder reopened",
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
                      "Project applicability saved",
                    )
                  }
                >
                  <p className="text-xs leading-5 text-muted">
                    Configure according to the project guide. Only selected
                    scopes generate reminders. Saving does not run checks.
                  </p>
                  <label className="block space-y-1">
                    <span className="text-xs text-muted">Project type</span>
                    <Select
                      name="kind"
                      defaultValue={
                        report.profile.kind === "unconfigured"
                          ? "custom"
                          : report.profile.kind
                      }
                    >
                      <option value="web">Web app</option>
                      <option value="backend">Backend service</option>
                      <option value="desktop">Desktop app</option>
                      <option value="library">Library / tool</option>
                      <option value="notes">Documents / Notes</option>
                      <option value="custom">Custom</option>
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
                        This project includes {name}
                      </CheckBox>
                      <p className="text-xs text-muted">Checks to verify</p>
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
                    <legend className="text-sm font-medium">
                      Delivery stages
                    </legend>
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
                  <Submit pending={pending}>Save applicability</Submit>
                </ReviewForm>
              )}
              {state.section === "changes" && (
                <>
                  <dl className="grouped-list">
                    <Info label="Upstream ref">
                      {report.snapshot.upstream ||
                        "Not configured or unreadable"}
                    </Info>
                    <Info label="Commit differences">
                      {report.snapshot.upstreamKnown
                        ? `Ahead ${report.snapshot.ahead} / Behind ${report.snapshot.behind}`
                        : "Unverified"}
                    </Info>
                    <Info label="Last fetch">
                      {report.snapshot.fetchedAt
                        ? date(report.snapshot.fetchedAt)
                        : "Unknown"}
                    </Info>
                  </dl>
                  <p className="text-xs leading-5 text-muted">
                    Calculated from local Git refs; remote state has not been
                    checked online.
                  </p>
                  <Button
                    onClick={() => {
                      update({ expanded: false });
                      onGit();
                    }}
                  >
                    View worktrees, branches, and stash
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
                              ? "Conflicts"
                              : file.untracked
                                ? "Untracked"
                                : `${file.index !== " " ? "Staged" : ""}${file.index !== " " && file.worktree !== " " ? " / " : ""}${file.worktree !== " " ? "Unstaged" : ""}`}
                          </span>
                          <span className="min-w-0 break-all">{file.path}</span>
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <p className="text-sm text-muted">
                      {report.snapshot.error || "No file changes loaded."}
                    </p>
                  )}
                  {report.snapshot.files.length > 100 && (
                    <p className="text-xs text-muted">
                      Showing the first 100 items, CLI The receipt includes the
                      full loaded list.
                    </p>
                  )}
                </>
              )}
              {state.section === "checks" && (
                <>
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <p className="text-xs leading-5 text-muted">
                      Compares file contents and command configuration; ignored
                      dependencies and external environments are excluded. After
                      a commit changes, check whether verification depends on
                      Git status.
                    </p>
                    <Button
                      onClick={() => {
                        update({ expanded: false });
                        onCI();
                      }}
                    >
                      Open CI/CD
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
                            Environment: {item.environment || "Not provided"} ·
                            Target:
                            {item.target || "Not provided"}
                          </p>
                        )}
                        {item.evidence && (
                          <p className="break-words text-xs text-muted">
                            {item.source === "pipeline"
                              ? "App execution"
                              : "Externally reported"}{" "}
                            · {item.evidence}
                          </p>
                        )}
                      </div>
                    ))}
                  </div>
                  {!report.coverage.length && (
                    <p className="text-sm text-muted">
                      Select required checks under Applicability.
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
                              ? "App execution"
                              : "Externally reported; not executed by the app"}{" "}
                            · {date(check.createdAt)} · {status[check.current]}
                          </p>
                          <p className="break-words text-xs leading-5 text-muted">
                            Scope: {check.scope}; {check.reason}
                            {check.coverage?.length
                              ? `; ${check.coverage.map((c) => coverageName(report.coverageOptions, c)).join(", ")}`
                              : "; No verification categories declared"}
                          </p>
                          <p className="break-words text-xs text-muted">
                            {check.evidence}
                            {(check.environment || check.target) &&
                              ` · Environment: ${check.environment || "Not provided"} / Target: ${check.target || "Not provided"}`}
                          </p>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <p className="text-sm text-muted">No check records yet.</p>
                  )}
                  <section>
                    <h3 className="break-words text-sm font-medium">
                      Record external check
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
                          "External check recorded",
                        )
                      }
                    >
                      <Field name="command" label="Check command" required />
                      <Field
                        name="scope"
                        label="Actual coverage and limitations"
                        required
                      />
                      <fieldset className="space-y-2">
                        <legend className="mb-2 text-xs text-muted">
                          Categories actually verified (select all that apply)
                        </legend>
                        {report.coverageOptions.map((r) => (
                          <CheckBox key={r.id} name="coverage" value={r.id}>
                            {r.name}
                          </CheckBox>
                        ))}
                        <CheckBox name="coverage" value="database-static">
                          Database script static checks (do not replace real
                          database verification)
                        </CheckBox>
                      </fieldset>
                      <EnvironmentFields />
                      <ResultSelect />
                      <Field
                        name="evidence"
                        label="Result notes or log location"
                        multiline
                        required
                      />
                      <Submit pending={pending}>Record check</Submit>
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
                            Recorded at {date(stage.recordedAt)}
                          </p>
                        )}
                      </div>
                    ))}
                  </div>
                  <section>
                    <h3 className="break-words text-sm font-medium">
                      Record stage verification
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
                          "Manual verification saved",
                        )
                      }
                    >
                      <label className="block space-y-1">
                        <span className="text-xs text-muted">Stage</span>
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
                        label="Verification method, commit, or result URL"
                        multiline
                        required
                      />
                      <p className="text-xs text-muted">
                        Save a manual verification record. This does not run
                        push, deployment, or acceptance commands.
                      </p>
                      <Submit pending={pending}>Save verification</Submit>
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
                                ? "Verification recorded"
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
                              Record verification
                            </button>
                          </div>
                          <p className="break-all text-xs text-muted">
                            {resource.state.definition.target ||
                              "Manual confirmation"}{" "}
                            · {status[resource.state.observed]}
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
                              "Existence does not prove a valid backup or recovery. Record how you verified it."}
                          </p>
                          {resource.note && (
                            <p className="break-words text-xs text-muted">
                              Reported: {resource.note}
                            </p>
                          )}
                          {resource.recordedAt && (
                            <p className="text-xs text-muted">
                              Reported at {date(resource.recordedAt)}
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
                                  "Data and environment verification saved",
                                )
                              }
                            >
                              <ResultSelect allowNA />
                              <Field
                                name="note"
                                label="Backup, recovery drill, or status verification notes"
                                multiline
                                required
                              />
                              <Submit pending={pending}>
                                Save verification
                              </Submit>
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
                                "Verification declaration removed. Data preserved.",
                              )
                            }
                          >
                            Remove declaration
                          </button>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <p className="text-sm text-muted">
                      No data or environment resources declared for
                      verification.
                    </p>
                  )}
                  <section>
                    <h3 className="break-words text-sm font-medium">
                      Add verification resource
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
                          "Verification resource saved. No project commands run.",
                        )
                      }
                    >
                      <Field name="name" label="Name" required />
                      <label className="block space-y-1">
                        <span className="text-xs text-muted">Type</span>
                        <Select name="kind">
                          <option value="file">Project file</option>
                          <option value="directory">Project folder</option>
                          <option value="environment">
                            Environment variable (checks definition only)
                          </option>
                          <option value="manual">
                            Manually verify databases, migrations, or backups
                          </option>
                        </Select>
                      </label>
                      <Field
                        name="target"
                        label="Project-relative path or environment variable name (optional for manual verification)"
                      />
                      <Field
                        name="guide"
                        label="Verification method or project guide"
                        multiline
                      />
                      <Submit pending={pending}>Save resource</Submit>
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
                        "Task start state recorded",
                      )
                    }
                  >
                    <Field name="title" label="Task name" required />
                    <section>
                      <h3 className="break-words text-sm font-medium">
                        Declare owner and change scope
                      </h3>
                      <div className="mt-3 space-y-3">
                        <Field name="owner" label="Owner / AI Session alias" />
                        <fieldset className="space-y-2">
                          <legend className="mb-2 text-xs text-muted">
                            Affected scope
                          </legend>
                          {Object.entries(areas).map(([area, name]) => (
                            <CheckBox key={area} name="areas" value={area}>
                              {name}
                            </CheckBox>
                          ))}
                        </fieldset>
                        <Field
                          name="paths"
                          label="Project files or folders (one relative path per line)"
                          multiline
                        />
                        <EnvironmentFields />
                      </div>
                    </section>
                    <Submit pending={pending}>Record task start</Submit>
                  </ReviewForm>
                  <p className="text-xs leading-5 text-muted">
                    Start and end are recorded manually. New content is not
                    attributed to any AI .
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
                        {task.title} ·{" "}
                        {task.endedAt ? "Handed off" : "Not handed off"}
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
                          {task.owner || "No owner declared"} ·{" "}
                          {task.areas?.map((a) => areas[a] || a).join(", ") ||
                            "No scope declared"}
                          {task.paths?.length
                            ? ` · ${task.paths.join(", ")}`
                            : ""}
                          {(task.environment || task.target) &&
                            ` · ${task.environment} / ${task.target}`}
                        </p>
                      )}
                      {task.endedAt ? (
                        <div className="mt-3 space-y-2">
                          <p className="text-xs text-muted">
                            Added during task {task.newStashes.length} records
                            stash / {task.newUntracked.length} untracked files
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
                              "Task handoff saved",
                            )
                          }
                        >
                          <Field
                            name="summary"
                            label="Completed work"
                            multiline
                            required
                          />
                          <Field
                            name="checks"
                            label="Check notes and result location"
                            multiline
                          />
                          <Field
                            name="blockers"
                            label="Open issues and next steps"
                            multiline
                          />
                          <Submit pending={pending}>Save handoff</Submit>
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
      <span className="block text-xs text-muted">Result</span>
      <Select name="status">
        <option value="success">Verified</option>
        <option value="failure">Failed</option>
        {allowNA && <option value="not_applicable">Not applicable</option>}
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
      {pending ? "Saving…" : children}
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
            setNotice("Handoff copied");
          } catch {
            setNotice(
              "Could not access the clipboard. Select and copy the text below.",
            );
          }
        }}
      >
        Copy handoff
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
  frontend: "Frontend",
  backend: "Backend",
  database: "Database",
};
function coverageName(options: ReviewRequirement[], c: ReviewCoverage) {
  return (
    options.find((r) => r.area === c.area && r.method === c.method)?.name ||
    `${areas[c.area] || c.area} · ${c.method === "static" ? "Static checks" : c.method}`
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
        label="Environment alias (e.g. local / staging / production)"
      />
      <Field
        name="target"
        label="Target alias (service, database, or backup set)"
      />
      <p className="text-xs leading-5 text-muted">
        Required for database checks. Use aliases only, never connection
        strings, passwords, or tokens. Matching content does not verify external
        state.
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
                  ? "Reviewed; kept"
                  : "Defer"}{" "}
                · {issue.acknowledgement.reason} ·{" "}
                {date(issue.acknowledgement.createdAt)}
              </p>
              <Button
                disabled={!native || pending}
                onClick={() => save(issue, "reopened", "")}
              >
                Reopen reminder
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
                <span className="text-xs text-muted">Resolution status</span>
                <Select name="state">
                  <option value="retained">Reviewed; keep</option>
                  <option value="later">Defer</option>
                </Select>
              </label>
              <Field
                name="reason"
                label="Reason and follow-up"
                required
                multiline
              />
              <Submit pending={pending}>Save status</Submit>
            </ReviewForm>
          )}
        </section>
      ))}
    </div>
  );
}
