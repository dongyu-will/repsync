import ActionDialog from "./ActionDialog";
import { useEffect, useState } from "react";
import { isMyGo } from "mygo-runtime";
import { Button, Empty, Select } from "./components";
import {
  Service,
  type Project,
  type SharedSource,
  type SharedAdoption,
  type UpdateHistory,
  type UpdatePreview,
} from "./mygo";

import ProjectRules from "./ProjectRules";

const native = isMyGo();

export default function SharedRules({
  projects,
  project,
  showProjectRules = true,
  onOpenShared,
  onUpdated,
}: {
  projects: Project[];
  project?: Project;
  showProjectRules?: boolean;
  onOpenShared?: () => void;
  onUpdated?: () => void;
}) {
  const [source, setSource] = useState<SharedSource | null>(null);
  const [ready, setReady] = useState(!native);
  const [draft, setDraft] = useState<string | null>(null);
  const [projectPath, setProjectPath] = useState(projects[0]?.path || "");
  const path =
    project?.path ??
    (projects.some((project) => project.path === projectPath)
      ? projectPath
      : projects[0]?.path || "");
  const [preview, setPreview] = useState<UpdatePreview | null>(null);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [historyRevision, setHistoryRevision] = useState(0);

  useEffect(() => {
    if (!native) return;
    let active = true;
    Service.sharedSource()
      .then((value) => {
        if (active) setSource(value);
      })
      .catch((error) => {
        if (active) setError(String(error));
      })
      .finally(() => {
        if (active) setReady(true);
      });
    return () => {
      active = false;
    };
  }, []);

  async function connect(event: React.FormEvent) {
    event.preventDefault();
    setBusy("connect");
    setError("");
    setNotice("");
    try {
      const value = await Service.connectSharedSource(
        draft ?? source?.path ?? "",
      );
      setSource(value);
      setDraft(null);
      setPreview(null);
      setNotice("Shared rules connected. Project files have not changed.");
    } catch (error) {
      setError(String(error));
    } finally {
      setBusy("");
    }
  }

  async function previewUpdate() {
    setBusy("preview");
    setError("");
    setNotice("");
    setPreview(null);
    try {
      setPreview(await Service.previewUpdate(path));
    } catch (error) {
      setError(String(error));
    } finally {
      setBusy("");
    }
  }

  async function apply() {
    if (!preview) return;
    setBusy("apply");
    setError("");
    setNotice("");
    try {
      await Service.applyUpdate(preview.token);
      setPreview(null);
      setHistoryRevision((value) => value + 1);
      setNotice(`Updated ${preview.project.name}   AGENTS.md, Backup saved.`);
      onUpdated?.();
    } catch (error) {
      setError(String(error));
      setPreview(null);
      setHistoryRevision((value) => value + 1);
    } finally {
      setBusy("");
    }
  }

  async function restore(record: UpdateHistory) {
    setBusy(record.id);
    setError("");
    setNotice("");
    setPreview(null);
    try {
      await Service.restoreUpdate(record.id);
      setNotice(`Restored ${record.project.name} Previous AGENTS.md.`);
      onUpdated?.();
    } catch (error) {
      setError(String(error));
    } finally {
      setHistoryRevision((value) => value + 1);
      setBusy("");
    }
  }

  if (!native && project)
    return (
      <div className="space-y-5">
        {showProjectRules && (
          <ProjectRules path={project.path} onUpdated={onUpdated} />
        )}
        <section aria-label="Shared rules" className="space-y-3">
          <p className="text-sm text-muted">
            Connect a shared rules source to preview and apply project updates.
          </p>
          <Button disabled>Preview diff</Button>
        </section>
      </div>
    );
  if (!native)
    return (
      <div className="space-y-5">
        {showProjectRules && <ProjectRules />}
        <Empty icon="layers" title="No shared rules connected">
          Connect a rules file to preview changes for each project.
        </Empty>
      </div>
    );
  if (!ready) return <p role="status">Loading shared rules…</p>;

  return (
    <div className="space-y-5">
      {showProjectRules && (
        <ProjectRules path={project?.path} onUpdated={onUpdated} />
      )}
      {error && (
        <p
          role="alert"
          className="break-words rounded-md bg-red-50 p-3 text-sm text-red-800"
        >
          {error}
        </p>
      )}
      {notice && (
        <p role="status" className="rounded-md bg-selection p-3 text-sm">
          {notice}
        </p>
      )}
      {!project && (
        <section aria-label="Shared rules source" className="space-y-3">
          <div className="section-heading">
            <h2>Shared rules source</h2>
            <p>
              Choose a shared rules Markdown file. Preview each project before
              syncing to AGENTS.md shared block.
            </p>
          </div>
          <form onSubmit={connect} className="space-y-3">
            <label
              htmlFor="shared-source"
              className="block text-sm font-medium"
            >
              Shared rules file
            </label>
            <div className="flex gap-2">
              <input
                id="shared-source"
                className="min-w-0 flex-1 rounded-md border border-control px-3 text-sm"
                placeholder="/Users/you/Rules/COMMON.md"
                value={draft ?? source?.path ?? ""}
                onChange={(event) => {
                  setDraft(event.target.value);
                  setPreview(null);
                }}
                autoCapitalize="none"
                autoCorrect="off"
                spellCheck={false}
                required
                disabled={!!busy}
              />
              <Button
                type="submit"
                primary
                disabled={!!busy || !(draft ?? source?.path ?? "").trim()}
              >
                {busy === "connect" ? "Connecting…" : "Connect file"}
              </Button>
            </div>
          </form>
          {source?.path && (
            <>
              <div className="flex flex-wrap items-center gap-3 text-sm">
                <span className="text-muted">
                  Content version{" "}
                  <code title={source.version}>
                    {source.version.slice(0, 12)}
                  </code>
                </span>
              </div>
              <RuleContent source={source} />
            </>
          )}
        </section>
      )}
      <section
        aria-label={project ? "Shared rules" : "Project rule updates"}
        className={`space-y-4 ${project ? "" : "border-t border-line pt-5"}`}
      >
        <div className="section-heading">
          {!project && <h2>Project rule updates</h2>}
          <div className="space-y-3">
            <p className="mt-2 leading-5">
              Project content outside the shared block is preserved. Source and
              project files are rechecked before applying; backups stay on this
              device.
            </p>
          </div>
        </div>
        {!source?.path &&
          (project ? (
            <>
              <p className="text-sm text-muted">
                No shared rules source connected.
              </p>
              <Button onClick={onOpenShared}>Connect shared rules</Button>
            </>
          ) : (
            <p className="text-sm text-muted">
              Connect a shared rules file above to preview project updates.
            </p>
          ))}
        {projects.length ? (
          <>
            {source?.path && (
              <Adoptions
                key={`${source.path}:${source.version}:${historyRevision}`}
                projects={projects}
                project={project}
                onSelect={(selected) => {
                  setProjectPath(selected);
                  setPreview(null);
                }}
              />
            )}
            <div className="flex items-end gap-2">
              {project ? (
                <div className="min-w-0 flex-1 text-sm">
                  <p className="break-all text-muted">
                    {source?.path || "AGENTS.md"}
                  </p>
                  {source?.version && (
                    <p
                      className="mt-1 text-xs text-muted"
                      title={source.version}
                    >
                      Source version {source.version.slice(0, 12)}
                    </p>
                  )}
                </div>
              ) : (
                <label
                  className="min-w-0 flex-1 text-sm font-medium"
                  htmlFor="update-project"
                >
                  Target project
                  <Select
                    id="update-project"
                    className="mt-2 w-full"
                    value={path}
                    disabled={!!busy}
                    onChange={(event) => {
                      setProjectPath(event.target.value);
                      setPreview(null);
                      setError("");
                      setNotice("");
                    }}
                  >
                    {projects.map((project) => (
                      <option key={project.path} value={project.path}>
                        {project.name} · {project.path}
                      </option>
                    ))}
                  </Select>
                </label>
              )}
              <Button
                onClick={previewUpdate}
                disabled={!!busy || !source?.path}
              >
                {busy === "preview" ? "Generating preview…" : "Preview diff"}
              </Button>
            </div>
            {project && source?.path && <RuleContent source={source} />}
            {preview && (
              <section aria-label="Update preview" className="space-y-3">
                <div className="flex flex-wrap items-center gap-3">
                  <h3 className="min-w-0 flex-1 text-sm font-semibold">
                    {preview.project.name} / AGENTS.md
                  </h3>
                  <span className="text-xs text-muted" title={preview.version}>
                    Source version {preview.version.slice(0, 12)}
                  </span>
                  <Button
                    primary
                    onClick={apply}
                    disabled={!!busy || !preview.changed}
                  >
                    {busy === "apply" ? "Applying…" : "Apply update"}
                  </Button>
                </div>
                {preview.changed ? (
                  <pre
                    className="source-panel"
                    tabIndex={0}
                    role="region"
                    aria-label="AGENTS.md Update diff"
                  >
                    {preview.diff.split("\n").map((line, index) => (
                      <span
                        key={index}
                        className={`block min-h-5 ${line.startsWith("+") && !line.startsWith("+++") ? "bg-green-50 text-green-900" : line.startsWith("-") && !line.startsWith("---") ? "bg-red-50 text-red-900" : ""}`}
                      >
                        {line || " "}
                      </span>
                    ))}
                  </pre>
                ) : (
                  <p role="status" className="text-sm text-muted">
                    Shared rules are up to date.
                  </p>
                )}
                <ActionDialog
                  title="Preserved project content"
                  busy={!!busy}
                  error={error}
                  notice={notice}
                >
                  {preview.projectContent.trim() ? (
                    <pre
                      className="source-panel mt-3"
                      tabIndex={0}
                      aria-label="Project content outside the shared block"
                    >
                      {preview.projectContent}
                    </pre>
                  ) : (
                    <p className="mt-2 text-sm text-muted">
                      No project content outside the shared block.
                    </p>
                  )}
                </ActionDialog>
              </section>
            )}
            <ActionDialog
              title="Update history and restore"
              busy={!!busy}
              error={error}
              notice={notice}
            >
              <div className="mt-3">
                <History
                  key={`${path}:${historyRevision}`}
                  path={path}
                  busy={!!busy}
                  onRestore={restore}
                />
              </div>
            </ActionDialog>
          </>
        ) : (
          <Empty title="No projects added">
            Add a local project in the sidebar to preview rule updates.
          </Empty>
        )}
      </section>
    </div>
  );
}

const adoptionStates: Record<string, string> = {
  current: "Up to date",
  outdated: "Update available",
  modified: "Shared block modified",
  untracked: "Adopted version unknown",
  not_adopted: "Not adopted",
  no_source: "No source connected",
  error: "Read failed",
};

function Adoptions({
  projects,
  project,
  onSelect,
}: {
  projects: Project[];
  project?: Project;
  onSelect: (path: string) => void;
}) {
  const [items, setItems] = useState<SharedAdoption[] | null>(null);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    let active = true;
    Service.sharedAdoptions()
      .then((value) => {
        if (active) {
          setItems(value);
          setError("");
        }
      })
      .catch((error) => {
        if (active) {
          setError(String(error));
          setItems(null);
        }
      });
    return () => {
      active = false;
    };
  }, [projects, revision]);
  const selected = project
    ? items?.filter((item) => item.project.path === project.path)
    : items;
  return (
    <section>
      <h3 className="break-words text-sm font-medium">
        {project ? "Rule adoption" : "Adoption by project"}
      </h3>
      <div className="mt-3 space-y-3">
        <Button onClick={() => setRevision((value) => value + 1)}>
          Refresh adoption status
        </Button>
        {error ? (
          <p role="alert" className="break-words text-sm text-red-800">
            {error}
          </p>
        ) : !selected ? (
          <p role="status" className="text-sm text-muted">
            Loading adoption status…
          </p>
        ) : (
          <div className="grouped-list">
            {selected.map((item) => (
              <div
                key={item.project.path}
                className="flex flex-wrap items-center gap-3 px-3 py-3 text-sm"
              >
                <div className="min-w-0 flex-1">
                  {project ? (
                    <span>{item.project.name}</span>
                  ) : (
                    <button
                      className="text-left font-medium hover:underline"
                      onClick={() => onSelect(item.project.path)}
                    >
                      {item.project.name}
                    </button>
                  )}
                  <p
                    className="mt-1 break-all text-xs text-muted"
                    title={item.project.path}
                  >
                    {item.project.path}
                  </p>
                  <p className="mt-1 break-all text-xs text-muted">
                    Source {item.sourceVersion.slice(0, 12) || "Unknown"} ·
                    Adoption {item.adoptedVersion.slice(0, 12) || "Unknown"}
                    {item.hasProjectContent
                      ? " · Preserve project content"
                      : ""}
                  </p>
                  {item.error && (
                    <p
                      role="alert"
                      className="mt-1 break-words text-xs text-red-800"
                    >
                      {item.error}
                    </p>
                  )}
                </div>
                <span>{adoptionStates[item.status] || item.status}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </section>
  );
}

function RuleContent({ source }: { source: SharedSource }) {
  return (
    <ActionDialog
      title="Shared rules source"
      trigger="View shared rules source"
    >
      <pre
        className="source-panel mt-3"
        tabIndex={0}
        role="region"
        aria-label="Shared rules source"
      >
        {source.content}
      </pre>
    </ActionDialog>
  );
}

const historyStates: Record<string, string> = {
  applied: "Restorable",
  restored: "Restored",
  changed: "File changed since this update",
  "not-applied": "Not applied",
};

function History({
  path,
  busy,
  onRestore,
}: {
  path: string;
  busy: boolean;
  onRestore: (record: UpdateHistory) => void;
}) {
  const [records, setRecords] = useState<UpdateHistory[] | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    let active = true;
    Service.updateHistory(path)
      .then((value) => {
        if (active) setRecords(value);
      })
      .catch((error) => {
        if (active) setError(String(error));
      });
    return () => {
      active = false;
    };
  }, [path]);
  return (
    <section aria-label="Update history">
      {error ? (
        <p role="alert" className="break-words text-sm text-red-800">
          Could not load update history: {error}
        </p>
      ) : !records ? (
        <p role="status" className="text-sm text-muted">
          Loading history…
        </p>
      ) : !records.length ? (
        <p className="text-sm text-muted">
          No update history for this project yet.
        </p>
      ) : (
        <div className="grouped-list">
          {records.map((record) => (
            <div
              key={record.id}
              className="flex flex-wrap items-center gap-3 px-3 py-3 text-sm"
            >
              <div className="min-w-0 flex-1">
                <time dateTime={record.createdAt}>
                  {new Date(record.createdAt).toLocaleString()}
                </time>
                <p className="mt-1 break-all text-xs text-muted">
                  AGENTS.md · Source version {record.version.slice(0, 12)}
                </p>
              </div>
              <span className="text-muted">
                {historyStates[record.state] || record.state}
              </span>
              <Button
                disabled={busy || record.state !== "applied"}
                onClick={() => onRestore(record)}
              >
                Restore previous content
              </Button>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
