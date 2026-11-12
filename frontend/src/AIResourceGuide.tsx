import { lazy, Suspense, useId, useReducer, useRef, useEffect } from "react";
import { t, getLocale } from "./i18n";
import type { AIResource, AIResourceAssessment } from "./mygo";
import ActionDialog from "./ActionDialog";
import { Button, Select } from "./components";

const MarkdownPreview = lazy(() => import("./MarkdownPreview"));

export function AssessmentVerdict({
  assessment,
}: {
  assessment: AIResourceAssessment;
}) {
  const labels: Record<string, string> = {
    recommended: "Recommended to adopt",
    conditional: "Adopt with conditions",
    not_recommended: "Not recommended to adopt",
    unknown: "More information needed",
  };
  return t(
    assessment.stale
      ? "Assessment needs refreshing"
      : labels[assessment.verdict] || "More information needed",
  );
}

export function ResourceUpdateStatus({ resource }: { resource: AIResource }) {
  const labels: Record<string, string> = {
    available: "Update available",
    current: "Current at last check",
    failed: "Update check failed",
    unknown: "Update status unknown",
  };
  return t(
    resource.note?.stale
      ? "Report needs refreshing"
      : labels[resource.note?.updateStatus || ""] || "Updates not checked",
  );
}

export default function AIResourceGuide({
  resource,
  path,
}: {
  resource: AIResource;
  path: string;
}) {
  const note = resource.note;
  const candidate = resource.availability === "candidate";
  return (
    <section className="space-y-3 border-t border-line-subtle pt-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="font-medium">{t("Assessment, updates and manual")}</h3>
        <AIMaintenance path={path} resource={resource} />
      </div>
      <AssessmentDetails assessment={resource.assessment} />
      {!candidate && (
        <>
          <p
            className={`text-xs ${note?.stale || note?.updateStatus === "failed" ? "text-removed" : "text-muted"}`}
          >
            <ResourceUpdateStatus resource={resource} />
          </p>
          {note ? (
            <>
              {note.stale && (
                <p role="status" className="text-xs leading-5 text-removed">
                  {t(
                    "The local resource changed. This report describes an earlier snapshot; ask AI to refresh it.",
                  )}
                </p>
              )}
              <p className="whitespace-pre-wrap break-words leading-6">
                {note.summary}
              </p>
              <dl className="space-y-1 text-xs leading-5">
                {[
                  [
                    "Installed version",
                    note.installedVersion || t("Version unknown"),
                  ],
                  [
                    "Upstream version",
                    note.latestVersion || t("Version unknown"),
                  ],
                  [
                    "AI checked",
                    new Date(note.checkedAt).toLocaleString(getLocale()),
                  ],
                ].map(([label, value]) => (
                  <div
                    key={label}
                    className="grid grid-cols-[96px_minmax(0,1fr)] gap-3"
                  >
                    <dt className="text-muted">{t(label)}</dt>
                    <dd className="break-words [overflow-wrap:anywhere]">
                      {value}
                    </dd>
                  </div>
                ))}
              </dl>
              <p className="text-xs leading-5 text-muted">
                {t(
                  "AI-authored findings with sources; installation and successful use require separate verification.",
                )}
              </p>
              {note.changes && (
                <GuideSection
                  title={t("What changed")}
                  content={note.changes}
                />
              )}
              {note.manual ? (
                <GuideSection title={t("User manual")} content={note.manual} />
              ) : (
                <p className="text-xs text-muted">
                  {t(
                    "No manual yet. Ask AI to write one from the installed resource.",
                  )}
                </p>
              )}
              <ReportSources sources={note.sources} />
            </>
          ) : (
            <p className="text-xs leading-5 text-muted">
              {t(
                "Ask AI to check the original source, explain new capabilities, and write a practical manual. Rescanning only refreshes the local inventory.",
              )}
            </p>
          )}
        </>
      )}
    </section>
  );
}

function AssessmentDetails({
  assessment,
}: {
  assessment?: AIResourceAssessment;
}) {
  const kinds: Record<string, string> = {
    overlap: "Capability overlap",
    conflict: "Requirement conflict",
    prerequisite: "Missing prerequisite",
    unknown: "Needs confirmation",
  };
  return (
    <div className="space-y-3">
      <h4 className="font-medium">{t("Before adopting")}</h4>
      {assessment ? (
        <>
          <p
            className={`font-medium ${assessment.stale || assessment.verdict === "not_recommended" ? "text-removed" : "text-ink"}`}
          >
            <AssessmentVerdict assessment={assessment} />
          </p>
          {assessment.stale && (
            <p className="text-xs leading-5 text-removed">
              {t(
                "This assessment targets a different scope or an earlier resource and project context. Ask AI to reassess before adopting.",
              )}
            </p>
          )}
          <p className="whitespace-pre-wrap break-words leading-6">
            {assessment.summary}
          </p>
          {!!assessment.findings.length && (
            <p className="flex flex-wrap gap-x-3 text-xs leading-5 text-muted">
              {Object.entries(kinds).map(([kind, label]) => {
                const count = assessment.findings.filter(
                  (finding) => finding.kind === kind,
                ).length;
                return count ? (
                  <span key={kind}>
                    {t(label)} {count}
                  </span>
                ) : null;
              })}
            </p>
          )}
          <p className="break-words text-xs leading-5 text-muted [overflow-wrap:anywhere]">
            {t("Assessed scope:")} {assessment.scope || t("Global")}
            {" · "}
            {new Date(assessment.checkedAt).toLocaleString(getLocale())}
          </p>
          {assessment.candidate && (
            <p className="break-words text-xs leading-5 text-muted [overflow-wrap:anywhere]">
              {t("Candidate source version:")}{" "}
              {assessment.candidate.sourceVersion || t("Version unknown")}
            </p>
          )}
          <GuideSection
            title={t("How it fits your work")}
            content={assessment.fit}
          />
          {assessment.findings.length ? (
            <ul role="list" className="divide-y divide-line-subtle">
              {assessment.findings.map((finding, index) => (
                <li key={index} className="space-y-2 py-3">
                  <h5 className="break-words font-medium">
                    {t(kinds[finding.kind])}
                    {finding.resource && " · " + finding.resource}
                  </h5>
                  <p className="whitespace-pre-wrap break-words leading-6">
                    {finding.evidence}
                  </p>
                  <p className="whitespace-pre-wrap break-words leading-6">
                    <span className="text-muted">
                      {t("Suggested handling:")}{" "}
                    </span>
                    {finding.recommendation}
                  </p>
                  {!!finding.sources?.length && (
                    <ReportSources sources={finding.sources} />
                  )}
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-xs leading-5 text-muted">
              {t(
                "No specific issues recorded within this assessment's coverage. This is not a guarantee of compatibility.",
              )}
            </p>
          )}
          <GuideSection
            title={t("Setup cost and rollback")}
            content={assessment.adoptionCost}
          />
          <GuideSection
            title={t("Assessment coverage and gaps")}
            content={assessment.coverage}
          />
          <ReportSources sources={assessment.sources} />
          <p className="text-xs leading-5 text-muted">
            {t(
              "AI recommendation with cited evidence. Assessment does not install a Skill or prove successful use.",
            )}
          </p>
        </>
      ) : (
        <p className="text-xs leading-5 text-muted">
          {t(
            "Not assessed yet. Ask AI to compare this resource with your tasks, project rules and existing resources before adopting it.",
          )}
        </p>
      )}
    </div>
  );
}

function ReportSources({ sources }: { sources: string[] }) {
  if (!sources.length) return null;
  return (
    <div className="space-y-1 text-xs leading-5">
      <h5 className="font-medium">{t("Report sources")}</h5>
      <ul role="list" className="space-y-1">
        {sources.map((source, index) => (
          <li key={index} className="break-words [overflow-wrap:anywhere]">
            {/^https?:\/\//i.test(source) ? (
              <a
                href={source}
                target="_blank"
                rel="noreferrer"
                className="text-accent underline underline-offset-2"
              >
                {source}
              </a>
            ) : (
              source
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}

function GuideSection({ title, content }: { title: string; content: string }) {
  return (
    <div className="space-y-2">
      <h4 className="font-medium">{title}</h4>
      <div className="markdown-document min-w-0 text-sm [overflow-wrap:anywhere]">
        <Suspense fallback={<p role="status">{t("Loading manual…")}</p>}>
          <MarkdownPreview content={content} names={[]} onDocument={() => {}} />
        </Suspense>
      </div>
    </div>
  );
}

export function AIMaintenance({
  path,
  resource,
}: {
  path: string;
  resource?: AIResource;
}) {
  const [task, setTask] = useReducer(
    (_: string, next: string) => next,
    resource?.availability === "candidate" ? "assess" : "check",
  );
  const [notice, setNotice] = useReducer(
    (current: { message: string; repeat: boolean }, message: string) => ({
      message,
      repeat: current.message === message && !current.repeat,
    }),
    { message: "", repeat: false },
  );
  const textId = useId();
  const taskId = useId();
  const text = useRef<HTMLPreElement>(null);
  const prompt = maintenancePrompt(path, task, resource);
  async function copy() {
    try {
      await navigator.clipboard.writeText(prompt);
      setNotice("AI instructions copied");
    } catch {
      setNotice(
        "Could not access the clipboard. Select and copy the text below.",
      );
    }
  }
  useEffect(() => {
    const element = text.current;
    const handle = () => {
      void copy();
    };
    // Native Invoker Commands and the click fallback share one clipboard path.
    element?.addEventListener("command", handle);
    return () => element?.removeEventListener("command", handle);
  });
  return (
    <ActionDialog
      title={t("AI resource maintenance")}
      trigger={t("Ask AI to maintain")}
      onOpen={() => setNotice("")}
    >
      <p className="text-sm leading-6 text-muted">
        {t(
          "Copy these instructions to your existing AI chat. Return here and rescan after it finishes.",
        )}
      </p>
      <label htmlFor={taskId} className="block space-y-1 text-xs text-muted">
        <span>{t("Maintenance task")}</span>
        <Select
          id={taskId}
          className="w-full"
          value={task}
          onChange={(event) => {
            setTask(event.target.value);
            setNotice("");
          }}
        >
          <option value="assess">
            {t("Assess suitability and conflicts")}
          </option>
          {resource?.availability !== "candidate" && (
            <>
              <option value="check">
                {t("Check updates and write manual")}
              </option>
              <option value="update">{t("Update and refresh manual")}</option>
              <option value="manual">{t("Write manual only")}</option>
            </>
          )}
        </Select>
      </label>
      <Button
        type="button"
        {...{ command: "--copy", commandfor: textId }}
        onClick={() => {
          if (!("CommandEvent" in window)) void copy();
        }}
      >
        {t("Copy AI instructions")}
      </Button>
      <p role="status" className="text-xs text-muted">
        {notice.message
          ? t(notice.message) + (notice.repeat ? "\u00a0" : "")
          : ""}
      </p>
      <pre
        ref={text}
        id={textId}
        tabIndex={0}
        className="source-panel max-h-80 whitespace-pre-wrap break-words [overflow-wrap:anywhere]"
      >
        {prompt}
      </pre>
    </ActionDialog>
  );
}

function maintenancePrompt(path: string, task: string, resource?: AIResource) {
  if (task === "assess") return assessmentPrompt(path, resource);
  const goal =
    task === "update"
      ? t(
          "Update the selected resources using their existing installation channels, then refresh the findings and manual. Preserve local edits; report failures and actual verification separately.",
        )
      : task === "manual"
        ? t(
            "Write a practical manual for the installed resource. Preserve prior update findings and checkedAt only if their resourceRevision still matches; otherwise use unknown and explain that no upstream check was performed.",
          )
        : t(
            "Check upstream updates and explain what is new and useful for the affected projects. Write a manual for the installed version. Do not install updates during this check.",
          );
  return [
    goal,
    t(
      "Use repsync ai-resources with the scope below (omit PATH for global). Reread the inventory before recording; select by id and source, never by name alone.",
    ),
    JSON.stringify(
      {
        scope: path || "global",
        ...(resource
          ? {
              id: resource.id,
              name: resource.name,
              location: resource.location,
            }
          : {}),
      },
      null,
      2,
    ),
    !resource
      ? t(
          "Process the inventoried Skills and MCPs, starting with adopted resources. Record each result separately and disclose resources not checked.",
        )
      : "",
    t(
      "Read the installed Skill and its referenced documentation. Find its original repository or owning plugin; update plugins through their owner. For MCPs distinguish local package updates from remote tool changes. Do not infer upstream versions from local fingerprints; use unknown when evidence is missing. Do not include secrets.",
    ),
    t(
      "Write summary, changes and manual in Chinese. The manual should cover purpose, when to use it, prerequisites, ready-to-copy prompts for AI, expected results and limitations. Describe the installed version; clearly separate features available only after updating. Cite original sources and explain project relevance without inventing compatibility.",
    ),
    t(
      "Save each report as JSON with the fields below, then call repsync ai-resource-record [PATH] FILE using the same scope. Use resource.revision and resource.note.version from the freshly read inventory; expectedVersion is empty only if there is no note. checkedAt is the actual RFC3339 check time. updateStatus is current, available, unknown or failed. Known status requires installedVersion, latestVersion and sources. Keep each report under 64 KiB.",
    ),
    JSON.stringify(
      {
        resourceId: resource?.id || "<resource.id>",
        resourceRevision: "<resource.revision>",
        expectedVersion: "<resource.note.version or empty>",
        installedVersion: "",
        latestVersion: "",
        updateStatus: "unknown",
        checkedAt: "<actual RFC3339 time>",
        summary: "",
        changes: "",
        manual: "",
        sources: [],
      },
      null,
      2,
    ),
    t(
      "After updating, rescan and regenerate the manual against the new revision. Saving a report does not install or verify a resource. Report what was checked, changed, verified and left unknown.",
    ),
  ]
    .filter(Boolean)
    .join("\n\n");
}

function assessmentPrompt(path: string, resource?: AIResource) {
  return [
    t(
      "Assess this resource before installing or adopting it. For a new Skill use the candidate link or repository provided in this chat; otherwise use the selected resource below. If no source was provided, ask for it; do not invent one. Do not install anything during assessment.",
    ),
    JSON.stringify(
      {
        scope: path || "global",
        resource: resource
          ? {
              id: resource.id,
              name: resource.name,
              location: resource.location,
              candidate: resource.assessment?.candidate,
            }
          : undefined,
      },
      null,
      2,
    ),
    t(
      "Read repsync ai-resources [PATH] for this scope. Read the project's AGENTS.md, current guides and referenced rules, technology declarations and AI resource choices; compare the relevant installed Skills, MCPs and CLIs. For global assessments describe the common workflows and the actual projects checked. Use goals and preferences already stated in the chat; ask only for missing information that could change the recommendation.",
    ),
    t(
      "Read the selected resource or candidate Skill and referenced documentation from its original source without installing it. Pin the checked source version, commit or content digest. Explain fit, unique benefits, overlap, conflicting requirements, missing prerequisites, setup changes, ownership and rollback. Separate overlap, conflict, prerequisite and unknown findings; cite both sides of each conflict and suggest concrete handling. Unavailable sources mean unknown, not compatible.",
    ),
    t(
      "Write the findings in Chinese. Choose recommended, conditional, not_recommended or unknown, then lead with one sentence. Include checked tasks, files, resources and uncovered areas in coverage. A global recommendation does not establish suitability for every project. Do not claim universal compatibility or treat matching names as evidence of a conflict.",
    ),
    t(
      "Reread the inventory and ensure the assessed project guides and resources have not changed before recording. Save JSON in the format below and call repsync ai-resource-assess [PATH] FILE. contextVersion comes from data.contextVersion; expectedVersion comes from the matching assessment.version, or is empty for a new report. Use actual RFC3339 checkedAt; keep the report under 64 KiB.",
    ),
    t(
      "For an uninstalled candidate include candidate with its original source and checked sourceVersion; resourceId and resourceRevision may be empty and repsync derives them. For an existing resource omit candidate and copy its id and revision. To refresh an existing candidate preserve its source identity. Only after a separately requested installation, link candidate.installedResourceId to the exact inventoried Skill and supply its current revision; compare actual source content first. Names alone do not establish installation.",
    ),
    JSON.stringify(
      {
        resourceId: "<existing resource.id, or empty for a new candidate>",
        resourceRevision:
          "<existing resource.revision, or empty for an uninstalled candidate>",
        contextVersion: "<inventory.contextVersion>",
        expectedVersion: "<assessment.version or empty>",
        candidate: {
          name: "<Skill name>",
          source: "<original URL or absolute source path>",
          sourceVersion: "<checked version, commit or digest; empty if unread>",
          description: "<purpose>",
        },
        verdict: "unknown",
        summary: "",
        fit: "",
        adoptionCost: "",
        coverage: "",
        findings: [
          {
            kind: "conflict",
            resource: "<other resource or project rule>",
            evidence: "<specific conflicting requirements>",
            recommendation: "<concrete handling>",
            sources: [
              "<candidate source and location>",
              "<other requirement and location>",
            ],
          },
        ],
        sources: [],
        checkedAt: "<actual RFC3339 time>",
      },
      null,
      2,
    ),
    t(
      "Use findings: [] when no specific issues were found within the stated coverage. Every conflict needs sources for both requirements. An unconditional recommendation cannot contain unresolved conflicts, prerequisites or unknown findings. Missing candidate source versions require unknown. After a future installation, rescan and reassess the changed context; do not present saving this report as installing or enabling anything.",
    ),
  ].join("\n\n");
}
