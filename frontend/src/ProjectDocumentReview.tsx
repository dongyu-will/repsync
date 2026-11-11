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
  graph: "CodeGraph 已核对可查符号",
  warnings: [],
  findings: [
    {
      path: "README.md",
      startLine: 26,
      endLine: 26,
      version: "preview",
      original: "本地启动前，先用 Docker 启动 MySQL。\n",
      terms: ["mysql"],
      evidence: [
        {
          source: "Git diff",
          path: "src/db.ts",
          line: 12,
          detail: "移除 openMysql",
        },
        {
          source: "CodeGraph",
          path: "",
          line: 0,
          detail: "索引未找到 openMysql",
        },
        {
          source: "已确认项目记忆",
          path: "部署记录 #42 · 数据库迁移验收",
          line: 0,
          detail: "生产 · 生产数据库已完成 PostgreSQL 切换。",
          memoryId: "example-db",
        },
      ],
      suggestion: "核实 PostgreSQL 的实际配置后，改写本段的本地启动步骤。",
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
        notice: `已更新 ${preview.edit.path}。重新扫描可继续核对该文档。`,
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
          文档核对
          {report?.findings.length ? ` · ${report.findings.length}` : ""}
        </>
      }
    >
      <div className="space-y-3 border-t border-line-subtle p-3">
        <form className="flex flex-wrap items-end gap-2" onSubmit={scan}>
          <label className="grid gap-1 text-xs text-muted">
            变更范围
            <Select
              value={state.scope}
              disabled={!native || pending}
              onChange={(event) =>
                update({ ...clear, scope: event.target.value, report: null })
              }
            >
              <option value="worktree">未提交改动</option>
              <option value="commit">最近一次提交</option>
            </Select>
          </label>
          <label className="grid min-w-32 flex-1 gap-1 text-xs text-muted">
            补充关键词
            <input
              className="min-h-9 w-full rounded-md border border-line bg-white px-2 text-sm text-ink"
              placeholder="用逗号分隔旧名称"
              value={state.keywords}
              maxLength={256}
              disabled={!native || pending}
              onChange={(event) =>
                update({ ...clear, keywords: event.target.value, report: null })
              }
            />
          </label>
          <Button type="submit" disabled={!native || pending}>
            {pending && !report ? "正在扫描…" : "扫描文档"}
          </Button>
        </form>
        {state.notice && (
          <p role="status" className="break-words text-sm">
            {state.notice}
          </p>
        )}
        {state.error && (
          <p
            role="alert"
            className="break-words rounded-md bg-red-50 p-2 text-sm text-red-800"
          >
            {state.error}
          </p>
        )}
        {report && (
          <>
            <p role="status" className="text-xs leading-5 text-muted">
              已读取 {report.files} 份文档 · {report.graph}
            </p>
            {report.warnings.length > 0 && (
              <ul className="space-y-1 text-xs text-muted">
                {report.warnings.map((warning) => (
                  <li key={warning}>{warning}</li>
                ))}
              </ul>
            )}
            {report.findings.length === 0 && !state.notice && (
              <p className="text-sm text-muted">本次范围内没有匹配的段落。</p>
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
                      {item.terms.join("、")}
                    </span>
                  </button>
                  {finding === item && (
                    <div className="space-y-3 px-3 pb-3">
                      <div className="space-y-1 text-xs leading-5">
                        <p className="font-medium">依据</p>
                        <ul className="space-y-1 text-muted">
                          {item.evidence.map((e, index) => (
                            <li
                              key={index}
                              className="break-all"
                              title={
                                e.memoryId
                                  ? `记忆编号 ${e.memoryId}`
                                  : undefined
                              }
                            >
                              {e.source} ·{" "}
                              {e.path &&
                                `${e.path}${e.line > 0 ? `:${e.line}` : ""} · `}
                              {e.detail}
                            </li>
                          ))}
                        </ul>
                      </div>
                      <p className="text-sm leading-6">{item.suggestion}</p>
                      <p className="text-xs text-muted">
                        核对线上状态及保留用途后再改写。
                      </p>
                      {preview ? (
                        <div
                          ref={previewRef}
                          tabIndex={-1}
                          className="space-y-2"
                          aria-label="修改预览"
                        >
                          <p className="text-xs font-medium">修改前</p>
                          <pre className="max-h-44 overflow-auto whitespace-pre-wrap break-all rounded-md bg-red-50 p-2 font-mono text-xs text-red-900">
                            {preview.before}
                          </pre>
                          <p className="text-xs font-medium">修改后</p>
                          {preview.after ? (
                            <pre className="max-h-44 overflow-auto whitespace-pre-wrap break-all rounded-md bg-green-50 p-2 font-mono text-xs text-green-900">
                              {preview.after}
                            </pre>
                          ) : (
                            <p className="text-xs text-muted">移除此段</p>
                          )}
                          <div className="flex flex-wrap justify-end gap-2">
                            <Button
                              disabled={pending}
                              onClick={() =>
                                update({ preview: null, error: "" })
                              }
                            >
                              返回编辑
                            </Button>
                            <Button
                              primary
                              disabled={!native || pending}
                              onClick={apply}
                            >
                              应用修改
                            </Button>
                          </div>
                        </div>
                      ) : (
                        <>
                          <label className="grid gap-1 text-xs text-muted">
                            修改后的段落
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
                              预览删除
                            </Button>
                            <Button
                              primary
                              disabled={
                                pending ||
                                replacement === editableText(item.original)
                              }
                              onClick={() => showPreview(replacement)}
                            >
                              预览修改
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
