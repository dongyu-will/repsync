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
      setNotice("已接入公共规范，项目文件尚未修改。");
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
      setNotice(`已更新 ${preview.project.name} 的 AGENTS.md，备份已保存。`);
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
      setNotice(`已恢复 ${record.project.name} 更新前的 AGENTS.md。`);
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
        <section aria-label="公共规范" className="space-y-3">
          <h2 className="text-xl font-semibold">公共规范</h2>
          <p className="text-sm text-muted">
            接入公共规范源后，可预览并应用项目更新。
          </p>
          <Button disabled>预览差异</Button>
        </section>
      </div>
    );
  if (!native)
    return (
      <div className="space-y-5">
        {showProjectRules && <ProjectRules />}
        <Empty icon="layers" title="尚未接入公共规范">
          接入规则文件后，可预览各项目的规范差异。
        </Empty>
      </div>
    );
  if (!ready) return <p role="status">正在读取公共规范…</p>;

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
        <section aria-label="公共规则来源" className="space-y-3">
          <div className="section-heading">
            <h2>公共规则来源</h2>
            <p>
              选择一份公共规则 Markdown 文件，按项目预览后同步到 AGENTS.md
              的公共区块。
            </p>
          </div>
          <form onSubmit={connect} className="space-y-3">
            <label
              htmlFor="shared-source"
              className="block text-sm font-medium"
            >
              公共规则文件
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
                {busy === "connect" ? "正在接入…" : "接入文件"}
              </Button>
            </div>
          </form>
          {source?.path && (
            <>
              <div className="flex flex-wrap items-center gap-3 text-sm">
                <span className="text-muted">
                  内容版本{" "}
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
        aria-label={project ? "公共规范" : "项目规范更新"}
        className={`space-y-4 ${project ? "" : "border-t border-line pt-5"}`}
      >
        <div className="section-heading">
          <h2>{project ? "公共规范" : "项目规范更新"}</h2>
          <details className="mt-2 text-xs text-muted">
            <summary className="w-fit cursor-pointer">了解更新规则</summary>
            <p className="mt-2 leading-5">
              公共区块之外的项目内容保持原样。应用前重新核对源文件与项目文件，备份保存在本机。
            </p>
          </details>
        </div>
        {!source?.path &&
          (project ? (
            <>
              <p className="text-sm text-muted">尚未接入公共规范源。</p>
              <Button onClick={onOpenShared}>接入公共规范</Button>
            </>
          ) : (
            <p className="text-sm text-muted">
              接入上方的公共规则文件后，即可预览项目更新。
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
                      来源版本 {source.version.slice(0, 12)}
                    </p>
                  )}
                </div>
              ) : (
                <label
                  className="min-w-0 flex-1 text-sm font-medium"
                  htmlFor="update-project"
                >
                  目标项目
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
                {busy === "preview" ? "正在预览…" : "预览差异"}
              </Button>
            </div>
            {project && source?.path && <RuleContent source={source} />}
            {preview && (
              <section aria-label="更新预览" className="space-y-3">
                <div className="flex flex-wrap items-center gap-3">
                  <h3 className="min-w-0 flex-1 text-sm font-semibold">
                    {preview.project.name} / AGENTS.md
                  </h3>
                  <span className="text-xs text-muted" title={preview.version}>
                    来源版本 {preview.version.slice(0, 12)}
                  </span>
                  <Button
                    primary
                    onClick={apply}
                    disabled={!!busy || !preview.changed}
                  >
                    {busy === "apply" ? "正在应用…" : "应用更新"}
                  </Button>
                </div>
                {preview.changed ? (
                  <pre
                    className="source-panel"
                    tabIndex={0}
                    role="region"
                    aria-label="AGENTS.md 更新差异"
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
                    公共规则已是最新，无需更新。
                  </p>
                )}
                <details>
                  <summary className="cursor-pointer text-sm font-medium">
                    保留的项目内容
                  </summary>
                  {preview.projectContent.trim() ? (
                    <pre
                      className="source-panel mt-3"
                      tabIndex={0}
                      aria-label="公共区块外的项目内容"
                    >
                      {preview.projectContent}
                    </pre>
                  ) : (
                    <p className="mt-2 text-sm text-muted">
                      公共区块外没有项目内容。
                    </p>
                  )}
                </details>
              </section>
            )}
            <details>
              <summary className="cursor-pointer text-sm font-medium">
                更新记录与恢复
              </summary>
              <div className="mt-3">
                <History
                  key={`${path}:${historyRevision}`}
                  path={path}
                  busy={!!busy}
                  onRestore={restore}
                />
              </div>
            </details>
          </>
        ) : (
          <Empty title="尚未添加项目">
            先在侧栏添加本地项目，再预览规范更新。
          </Empty>
        )}
      </section>
    </div>
  );
}

const adoptionStates: Record<string, string> = {
  current: "已最新",
  outdated: "待更新",
  modified: "公共区块有改动",
  untracked: "采用版本未知",
  not_adopted: "尚未采用",
  no_source: "未接入来源",
  error: "读取失败",
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
    <details>
      <summary className="cursor-pointer text-sm font-medium">
        {project ? "规范采用情况" : "各项目采用情况"}
      </summary>
      <div className="mt-3 space-y-3">
        <Button onClick={() => setRevision((value) => value + 1)}>
          刷新采用状态
        </Button>
        {error ? (
          <p role="alert" className="break-words text-sm text-red-800">
            {error}
          </p>
        ) : !selected ? (
          <p role="status" className="text-sm text-muted">
            正在读取采用状态…
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
                    来源 {item.sourceVersion.slice(0, 12) || "未知"} · 采用{" "}
                    {item.adoptedVersion.slice(0, 12) || "未知"}
                    {item.hasProjectContent ? " · 保留项目内容" : ""}
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
    </details>
  );
}

function RuleContent({ source }: { source: SharedSource }) {
  return (
    <details>
      <summary className="cursor-pointer text-sm font-medium">
        查看公共规范原文
      </summary>
      <pre
        className="source-panel mt-3"
        tabIndex={0}
        role="region"
        aria-label="公共规范原文"
      >
        {source.content}
      </pre>
    </details>
  );
}

const historyStates: Record<string, string> = {
  applied: "可恢复",
  restored: "已恢复",
  changed: "文件已有后续改动",
  "not-applied": "未应用",
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
    <section aria-label="更新记录">
      {error ? (
        <p role="alert" className="break-words text-sm text-red-800">
          无法读取更新记录：{error}
        </p>
      ) : !records ? (
        <p role="status" className="text-sm text-muted">
          正在读取记录…
        </p>
      ) : !records.length ? (
        <p className="text-sm text-muted">该项目还没有更新记录。</p>
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
                  AGENTS.md · 来源版本 {record.version.slice(0, 12)}
                </p>
              </div>
              <span className="text-muted">
                {historyStates[record.state] || record.state}
              </span>
              <Button
                disabled={busy || record.state !== "applied"}
                onClick={() => onRestore(record)}
              >
                恢复更新前内容
              </Button>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
