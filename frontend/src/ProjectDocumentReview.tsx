import { formatMessage, t } from "./i18n";
import ActionDialog from "./ActionDialog";
import { useEffect, useReducer, useRef, type FormEvent } from "react";
import { isMyGo } from "mygo-runtime";
import { Button, Select } from "./components";
import {
  Service,
  type DocumentFinding,
  type DocumentReview,
  type DocumentReviewEdit,
  type DocumentReviewPreview,
} from "./mygo";

const native = isMyGo();
const sample: DocumentReview = {
  basis: "preview",
  files: 8,
  graph: "CodeGraph Queryable symbols checked",
  warnings: [],
  findings: [
    {
      path: "README.md",
      startLine: 26,
      endLine: 26,
      version: "preview",
      original: "Before starting locally, use Docker Start MySQL.\n",
      terms: ["mysql"],
      evidence: [
        {
          source: "Git diff",
          path: "src/db.ts",
          line: 12,
          detail: "Remove openMysql",
        },
        {
          source: "CodeGraph",
          path: "",
          line: 0,
          detail: "Index not found openMysql",
        },
        {
          source: "Confirmed project memories",
          path: "Deployment record #42 · Database migration verification",
          line: 0,
          detail:
            "Production · The production database has migrated to PostgreSQL.",
          memoryId: "example-db",
        },
      ],
      suggestion:
        "Verify PostgreSQL  configuration, then rewrite the local startup steps in this paragraph.",
    },
  ],
};

type State = {
  scope: string;
  keywords: string;
  report: DocumentReview | null;
  pending: boolean;
  error: string;
  notice: string;
  finding: DocumentFinding | null;
  replacement: string;
  preview: (DocumentReviewPreview & { edit: DocumentReviewEdit }) | null;
};

const message = (error: unknown) =>
  error instanceof Error ? error.message : String(error);
const editableText = (original: string) =>
  original.replaceAll("\r\n", "\n").replace(/\n$/, "");

export default function ProjectDocumentReview({ path }: { path: string }) {
  const [state, update] = useReducer(
    (s: State, p: Partial<State>) => ({ ...s, ...p }),
    {
      scope: "worktree",
      keywords: "",
      report: !native && path === "/Projects/atlas" ? sample : null,
      pending: false,
      error: "",
      notice: "",
      finding: null,
      replacement: "",
      preview: null,
    },
  );
  const { report, pending, finding, replacement, preview } = state;
  const previewRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (preview) previewRef.current?.focus();
  }, [preview]);
  const clear = { error: "", notice: "", preview: null, finding: null };
  async function scan(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!native || pending) return;
    update({ ...clear, pending: true, report: null });
    try {
      update({
        report: await Service.scanDocumentReview(
          path,
          state.scope,
          state.keywords,
        ),
      });
    } catch (error) {
      update({ error: message(error) });
    } finally {
      update({ pending: false });
    }
  }

  async function showPreview(next: string) {
    if (!finding || !report || pending) return;
    const edit: DocumentReviewEdit = {
      scope: state.scope,
      basis: report.basis,
      path: finding.path,
      startLine: finding.startLine,
      endLine: finding.endLine,
      version: finding.version,
      replacement: next,
    };
    update({
      replacement: next,
      pending: true,
      error: "",
      notice: "",
      preview: null,
    });
    try {
      const result = native
        ? await Service.previewDocumentReview(path, edit)
        : { before: finding.original, after: next, token: "preview" };
      update({ preview: { ...result, edit } });
    } catch (error) {
      update({ error: message(error) });
    } finally {
      update({ pending: false });
    }
  }

  async function apply() {
    if (!native || !preview || !report || pending) return;
    update({ pending: true, error: "", notice: "" });
    try {
      await Service.applyDocumentReview(path, preview.edit, preview.token);
      update({
        ...clear,
        report: {
          ...report,
          findings: report.findings.filter(
            (item) => item.path !== preview.edit.path,
          ),
        },
        notice: formatMessage(
          "Updated {0}.Rescan to continue reviewing this document.",
          preview.edit.path,
        ),
      });
    } catch (error) {
      update({ error: message(error) });
    } finally {
      update({ pending: false });
    }
  }

  return (
    <ActionDialog
      title={
        <>
          {t("Document review")}
          {report?.findings.length ? ` · ${report.findings.length}` : ""}
        </>
      }
    >
      <div className="space-y-3 border-t border-line-subtle p-3">
        <form className="flex flex-wrap items-end gap-2" onSubmit={scan}>
          <label className="grid gap-1 text-xs text-muted">
            {t("Change scope")}
            <Select
              value={state.scope}
              disabled={!native || pending}
              onChange={(event) =>
                update({ ...clear, scope: event.target.value, report: null })
              }
            >
              <option value="worktree">{t("Uncommitted changes")}</option>
              <option value="commit">{t("Latest commit")}</option>
            </Select>
          </label>
          <label className="grid min-w-32 flex-1 gap-1 text-xs text-muted">
            {t("Additional keywords")}
            <input
              className="min-h-9 w-full rounded-md border border-line bg-white px-2 text-sm text-ink"
              placeholder={t("Separate old names with commas")}
              value={state.keywords}
              maxLength={256}
              disabled={!native || pending}
              onChange={(event) =>
                update({ ...clear, keywords: event.target.value, report: null })
              }
            />
          </label>
          <Button type="submit" disabled={!native || pending}>
            {pending && !report ? t("Scanning…") : t("Scan documents")}
          </Button>
        </form>
        {state.notice && (
          <p role="status" className="break-words text-sm">
            {t(state.notice)}
          </p>
        )}
        {state.error && (
          <p
            role="alert"
            className="break-words rounded-md bg-red-50 p-2 text-sm text-red-800"
          >
            {t(state.error)}
          </p>
        )}
        {report && (
          <>
            <p role="status" className="text-xs leading-5 text-muted">
              {t("Loaded")} {report.files} {t("documents ·")} {t(report.graph)}
            </p>
            {report.warnings.length > 0 && (
              <ul className="space-y-1 text-xs text-muted">
                {report.warnings.map((warning) => (
                  <li key={warning}>{t(warning)}</li>
                ))}
              </ul>
            )}
            {report.findings.length === 0 && !state.notice && (
              <p className="text-sm text-muted">
                {t("No matching paragraphs in this scope.")}
              </p>
            )}
            <ul className="grouped-list">
              {report.findings.map((item) => (
                <li
                  key={item.path + ":" + item.startLine}
                  className="border-b border-line-subtle last:border-0"
                >
                  <button
                    type="button"
                    aria-expanded={finding === item}
                    disabled={pending}
                    onClick={() =>
                      update({
                        ...clear,
                        finding: finding === item ? null : item,
                        replacement: editableText(item.original),
                      })
                    }
                    className="flex w-full flex-wrap items-center justify-between gap-x-3 gap-y-1 rounded px-3 py-2 text-left text-sm hover:bg-sidebar"
                  >
                    <span className="min-w-0 break-all font-medium">
                      {item.path}:{item.startLine}
                      {item.endLine !== item.startLine && `–${item.endLine}`}
                    </span>
                    <span className="min-w-0 break-all text-xs text-muted">
                      {item.terms.join(", ")}
                    </span>
                  </button>
                  {finding === item && (
                    <div className="space-y-3 px-3 pb-3">
                      <div className="space-y-1 text-xs leading-5">
                        <p className="font-medium">{t("Evidence")}</p>
                        <ul className="space-y-1 text-muted">
                          {item.evidence.map((e, index) => (
                            <li
                              key={index}
                              className="break-all"
                              title={
                                e.memoryId
                                  ? t("Memory ID {0}", e.memoryId)
                                  : undefined
                              }
                            >
                              {t(e.source)} ·{" "}
                              {e.path &&
                                `${e.path}${e.line > 0 ? `:${e.line}` : ""} · `}
                              {e.detail}
                            </li>
                          ))}
                        </ul>
                      </div>
                      <p className="text-sm leading-6">{t(item.suggestion)}</p>
                      <p className="text-xs text-muted">
                        {t(
                          "Verify production status and retention needs before rewriting.",
                        )}
                      </p>
                      {preview ? (
                        <div
                          ref={previewRef}
                          tabIndex={-1}
                          className="space-y-2"
                          aria-label={t("Change preview")}
                        >
                          <p className="text-xs font-medium">{t("Before")}</p>
                          <pre className="max-h-44 overflow-auto whitespace-pre-wrap break-all rounded-md bg-red-50 p-2 font-mono text-xs text-red-900">
                            {preview.before}
                          </pre>
                          <p className="text-xs font-medium">{t("After")}</p>
                          {preview.after ? (
                            <pre className="max-h-44 overflow-auto whitespace-pre-wrap break-all rounded-md bg-green-50 p-2 font-mono text-xs text-green-900">
                              {preview.after}
                            </pre>
                          ) : (
                            <p className="text-xs text-muted">
                              {t("Remove paragraph")}
                            </p>
                          )}
                          <div className="flex flex-wrap justify-end gap-2">
                            <Button
                              disabled={pending}
                              onClick={() =>
                                update({ preview: null, error: "" })
                              }
                            >
                              {t("Back to editor")}
                            </Button>
                            <Button
                              primary
                              disabled={!native || pending}
                              onClick={apply}
                            >
                              {t("Apply changes")}
                            </Button>
                          </div>
                        </div>
                      ) : (
                        <>
                          <label className="grid gap-1 text-xs text-muted">
                            {t("Revised paragraph")}
                            <textarea
                              value={replacement}
                              rows={Math.min(
                                8,
                                Math.max(3, replacement.split("\n").length + 1),
                              )}
                              disabled={pending}
                              className="w-full resize-y rounded-md border border-line bg-white p-2 font-mono text-sm text-ink"
                              onChange={(event) =>
                                update({
                                  replacement: event.target.value,
                                  error: "",
                                })
                              }
                            />
                          </label>
                          <div className="flex flex-wrap justify-end gap-2">
                            <Button
                              disabled={pending}
                              onClick={() => showPreview("")}
                            >
                              {t("Preview deletion")}
                            </Button>
                            <Button
                              primary
                              disabled={
                                pending ||
                                replacement === editableText(item.original)
                              }
                              onClick={() => showPreview(replacement)}
                            >
                              {t("Preview changes")}
                            </Button>
                          </div>
                        </>
                      )}
                    </div>
                  )}
                </li>
              ))}
            </ul>
          </>
        )}
      </div>
    </ActionDialog>
  );
}
