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
      <Empty icon="file" title="No Markdown documents found">
        Only .md and .markdown files in the project root are listed.
      </Empty>
    );
  return (
    <div className="grid h-full min-h-0 min-w-0 grid-cols-[144px_minmax(0,1fr)] gap-4">
      <nav
        aria-label="Project documents"
        className="min-h-0 min-w-0 overflow-y-auto"
      >
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
        aria-label={`${selected} Documents`}
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
        if (content === undefined) throw new Error("Document not found");
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
          setStorageError(
            "Could not read the session draft. Save or copy your edits before switching files.",
          );
        }
        setDraft(stored || { base: content, content });
        setLoadError("");
        if (stored && stored.base !== content)
          setSaveError(
            "The file changed externally. Your draft is preserved; copy your edits before reloading.",
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
      setStorageError(
        "Could not store the draft. Save or copy your edits before switching files.",
      );
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
        setStorageError(
          "File saved, but the session draft could not be cleared.",
        );
      }
      setNotice("Saved");
    } catch (error) {
      setSaveError(message(error));
    } finally {
      setBusy(false);
    }
  }

  function reload() {
    if (
      dirty &&
      !window.confirm("Discard unsaved changes and reload the file?")
    )
      return;
    try {
      sessionStorage.removeItem(key);
    } catch {
      setStorageError(
        "Could not clear the draft. Copy your edits and check browser storage.",
      );
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
          Could not read {name}: {loadError}
        </p>
        <Button onClick={reload}>Retry</Button>
      </div>
    );
  if (!draft) return <p role="status">Loading {name}…</p>;
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
          aria-label="Document view"
          className="flex gap-1 rounded-md bg-surface p-1"
        >
          {(
            [
              ["preview", "Preview"],
              ["edit", "Edit"],
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
            {dirty ? "Unsaved" : notice}
          </span>
          {mode === "preview" && <DocumentFind reader={reader} />}
          <button
            className="icon-button"
            aria-label="Reload document"
            title="Reload document"
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
              title={
                native
                  ? "Save (⌘S / Ctrl+S)"
                  : "File writes require the desktop app"
              }
            >
              {busy ? "Saving…" : "Save"}
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
          aria-label={`${name} Editor`}
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
          aria-label={`${name} Preview`}
          tabIndex={0}
          className="min-h-0 flex-1 overflow-auto rounded-md border border-line-subtle px-4 py-3"
        >
          {draft.content ? (
            <Suspense
              fallback={
                <p role="status" className="text-sm text-muted">
                  Generating preview…
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
            <p className="text-sm text-muted">Empty file</p>
          )}
        </div>
      )}
    </div>
  );
}
