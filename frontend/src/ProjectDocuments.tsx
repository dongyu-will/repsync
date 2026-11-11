import { lazy, Suspense, useEffect, useRef, useState } from "react";
import { isMyGo } from "mygo-runtime";
import { Service } from "./mygo";
import { Button, Empty, Icon } from "./components";
import { previewDocuments } from "./preview";
import DocumentFind from "./DocumentFind";

const MarkdownPreview = lazy(() => import("./MarkdownPreview"));
const native = isMyGo();
const message = (error: unknown) =>
  error instanceof Error ? error.message : String(error);
type DocumentDraft = { base: string; content: string };
const storageKey = (path: string, name: string) =>
  `repsync:${native ? "document" : "preview-document"}:${JSON.stringify([native ? path : path.replace(/^\/Projects(?=\/)/, "/示例项目"), name])}`;

export default function ProjectDocuments({
  path,
  names,
}: {
  path: string;
  names: string[];
}) {
  const [choice, setChoice] = useState("");
  const selected = names.includes(choice) ? choice : names[0];
  if (!selected)
    return (
      <Empty icon="file" title="没有发现 Markdown 文档">
        当前仅查看项目根目录的 .md 和 .markdown 文件。
      </Empty>
    );
  return (
    <div className="grid h-full min-h-0 min-w-0 grid-cols-[144px_minmax(0,1fr)] gap-4">
      <nav aria-label="项目文档" className="min-h-0 min-w-0 overflow-y-auto">
        <ul className="space-y-1">
          {names.map((name) => (
            <li key={name}>
              <button
                aria-current={name === selected ? "true" : undefined}
                onClick={() => setChoice(name)}
                className={`flex min-h-9 w-full items-center gap-2 rounded-md px-2 py-2 text-left text-sm ${name === selected ? "bg-selection font-medium text-accent" : "hover:bg-sidebar"}`}
              >
                <Icon name="file" size={16} />
                <span className="min-w-0 break-all">{name}</span>
              </button>
            </li>
          ))}
        </ul>
      </nav>
      <section
        className="flex min-h-0 min-w-0 flex-col"
        aria-label={`${selected} 文档`}
      >
        <DocumentContent
          key={`${path}:${selected}`}
          path={path}
          name={selected}
          names={names}
          onDocument={setChoice}
        />
      </section>
    </div>
  );
}

function DocumentContent({
  path,
  name,
  names,
  onDocument,
}: {
  path: string;
  name: string;
  names: string[];
  onDocument: (name: string) => void;
}) {
  const reader = useRef<HTMLDivElement>(null);
  const [draft, setDraft] = useState<DocumentDraft | null>(null);
  const [mode, setMode] = useState<"preview" | "edit">("preview");
  const [loadError, setLoadError] = useState("");
  const [saveError, setSaveError] = useState("");
  const [storageError, setStorageError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [revision, setRevision] = useState(0);
  const dirty = !!draft && draft.base !== draft.content;
  const key = storageKey(path, name);

  useEffect(() => {
    let active = true;
    async function load() {
      try {
        const content = native
          ? await Service.readDocument(path, name)
          : previewDocuments[path]?.[name];
        if (content === undefined) throw new Error("文档不存在");
        if (!active) return;
        let stored: DocumentDraft | null = null;
        try {
          const value = JSON.parse(sessionStorage.getItem(key) || "null");
          if (
            value &&
            typeof value.base === "string" &&
            typeof value.content === "string"
          )
            stored = value;
        } catch {
          setStorageError("无法读取会话草稿，切换文件前请保存或复制修改。");
        }
        setDraft(stored || { base: content, content });
        setLoadError("");
        if (stored && stored.base !== content)
          setSaveError(
            "文件已在外部修改，草稿已保留；请复制需要的内容，再重新载入文件。",
          );
      } catch (error) {
        if (active) setLoadError(message(error));
      }
    }
    void load();
    return () => {
      active = false;
    };
  }, [path, name, key, revision]);

  useEffect(() => {
    if (!dirty) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);

  function change(content: string) {
    if (!draft) return;
    const next = { ...draft, content };
    setDraft(next);
    setNotice("");
    try {
      if (next.base === content) sessionStorage.removeItem(key);
      else sessionStorage.setItem(key, JSON.stringify(next));
      setStorageError("");
    } catch {
      setStorageError("无法保存会话草稿，切换文件前请保存或复制修改。");
    }
  }

  async function save() {
    if (!native || !draft || !dirty || busy) return;
    setBusy(true);
    setSaveError("");
    setNotice("");
    try {
      await Service.saveDocument(path, name, draft.base, draft.content);
      setDraft({ base: draft.content, content: draft.content });
      try {
        const stored = sessionStorage.getItem(key);
        if (stored === JSON.stringify(draft)) sessionStorage.removeItem(key);
        setStorageError("");
      } catch {
        setStorageError("文件已保存，但会话草稿未能清除。");
      }
      setNotice("已保存");
    } catch (error) {
      setSaveError(message(error));
    } finally {
      setBusy(false);
    }
  }

  function reload() {
    if (dirty && !window.confirm("放弃未保存的修改并重新载入文件？")) return;
    try {
      sessionStorage.removeItem(key);
    } catch {
      setStorageError("无法清除会话草稿，请先复制修改并检查浏览器存储。");
      return;
    }
    setDraft(null);
    setSaveError("");
    setLoadError("");
    setNotice("");
    setMode("preview");
    setRevision((value) => value + 1);
  }

  if (loadError)
    return (
      <div role="alert" className="text-sm">
        <p className="mb-3 break-words text-removed">
          无法读取 {name}：{loadError}
        </p>
        <Button onClick={reload}>重试</Button>
      </div>
    );
  if (!draft) return <p role="status">正在读取 {name}…</p>;
  return (
    <div
      className="flex min-h-0 flex-1 flex-col gap-3"
      onKeyDown={(event) => {
        if (
          (event.metaKey || event.ctrlKey) &&
          event.key.toLowerCase() === "s"
        ) {
          event.preventDefault();
          void save();
        }
      }}
    >
      <div className="flex shrink-0 flex-wrap items-center justify-between gap-2">
        <div
          role="group"
          aria-label="文档显示方式"
          className="flex gap-1 rounded-md bg-surface p-1"
        >
          {(
            [
              ["preview", "预览"],
              ["edit", "编辑"],
            ] as const
          ).map(([value, label]) => (
            <button
              key={value}
              aria-pressed={mode === value}
              onClick={() => setMode(value)}
              className={`min-h-7 rounded px-3 text-sm ${mode === value ? "bg-white font-medium text-ink" : "text-muted hover:text-ink"}`}
            >
              {label}
            </button>
          ))}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <span role="status" className="text-xs text-muted">
            {dirty ? "未保存" : notice}
          </span>
          {mode === "preview" && <DocumentFind reader={reader} />}
          <button
            className="icon-button"
            aria-label="重新载入文档"
            title="重新载入文档"
            disabled={busy}
            onClick={reload}
          >
            <Icon name="refresh" size={16} />
          </button>
          {(mode === "edit" || dirty) && (
            <Button
              primary
              disabled={!native || !dirty || busy}
              onClick={() => void save()}
              title={native ? "保存（⌘S / Ctrl+S）" : "当前环境无法写入文件"}
            >
              {busy ? "保存中…" : "保存"}
            </Button>
          )}
        </div>
      </div>
      {(saveError || storageError) && (
        <p role="alert" className="shrink-0 break-words text-sm text-removed">
          {saveError || storageError}
        </p>
      )}
      {mode === "edit" ? (
        <textarea
          aria-label={`${name} 编辑内容`}
          spellCheck={false}
          autoCapitalize="none"
          autoCorrect="off"
          disabled={busy}
          value={draft.content}
          onChange={(event) => change(event.target.value)}
          className="source-panel min-h-0 w-full flex-1 resize-none"
        />
      ) : (
        <div
          ref={reader}
          role="region"
          aria-label={`${name} 预览`}
          tabIndex={0}
          className="min-h-0 flex-1 overflow-auto rounded-md border border-line-subtle px-4 py-3"
        >
          {draft.content ? (
            <Suspense
              fallback={
                <p role="status" className="text-sm text-muted">
                  正在生成预览…
                </p>
              }
            >
              <article className="markdown-document max-w-[75ch]">
                <MarkdownPreview
                  content={draft.content}
                  names={names}
                  onDocument={onDocument}
                />
              </article>
            </Suspense>
          ) : (
            <p className="text-sm text-muted">文件为空</p>
          )}
        </div>
      )}
    </div>
  );
}
