import { useCallback, useEffect, useRef, useState } from "react";
import { isMyGo } from "mygo-runtime";
import { Service, type Project, type Inspection } from "./mygo";
import { Brand, Button, Empty, Icon } from "./components";
import ProjectContent, { type ProjectTab } from "./ProjectContent";
import { previewInspections } from "./preview";
import SharedRules from "./SharedRules";

const native = isMyGo();
type Page = "project" | "all" | "shared" | "updates" | "settings";
const tabs: ProjectTab[] = ["概览", "命令", "文档"];
function savedTab(path: string): ProjectTab {
  try {
    const value = JSON.parse(
      localStorage.getItem(`repsync:tab:${path}`) || '"概览"',
    );
    if (value === "规范") return "文档";
    return tabs.includes(value) ? value : "概览";
  } catch {
    return "概览";
  }
}

export default function App() {
  const [page, setPage] = useState<Page>("all");
  const [project, setProject] = useState<Project | null>(null);
  const [projects, setProjects] = useState<Project[]>(
    native ? [] : previewInspections.map(({ project }) => project),
  );
  const [listing, setListing] = useState(native);
  const [listError, setListError] = useState("");
  const [listRefresh, setListRefresh] = useState(0);
  const [tab, setTab] = useState<ProjectTab>("概览");
  const [query, setQuery] = useState("");
  const [searching, setSearching] = useState(false);
  const [projectsExpanded, setProjectsExpanded] = useState(true);
  const [details, setDetails] = useState<Inspection | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [refresh, setRefresh] = useState(0);
  const [adding, setAdding] = useState(false);
  const [path, setPath] = useState("");
  const [addError, setAddError] = useState("");
  const [saving, setSaving] = useState(false);
  const searchDialogRef = useRef<HTMLDialogElement>(null);
  const dialogRef = useRef<HTMLDialogElement>(null);
  const focusSearch = useCallback(() => {
    if (dialogRef.current?.open) return;
    setSearching(true);
    if (searchDialogRef.current?.open) {
      searchDialogRef.current.querySelector("input")?.focus();
    }
  }, []);

  useEffect(() => {
    if (!native) return;
    let active = true;
    Service.list()
      .then((items) => {
        if (active) {
          setProjects(items);
          setListing(false);
        }
      })
      .catch((e) => {
        if (active) {
          setListError(String(e));
          setListing(false);
        }
      });
    return () => {
      active = false;
    };
  }, [listRefresh]);
  useEffect(() => {
    if (!native || !project) return;
    let active = true;
    Service.inspect(project.path)
      .then((result) => {
        if (active) {
          setDetails(result);
          setLoading(false);
        }
      })
      .catch((e) => {
        if (active) {
          setError(String(e));
          setLoading(false);
        }
      });
    return () => {
      active = false;
    };
  }, [project, refresh]);
  useEffect(() => {
    const key = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key === "k") {
        event.preventDefault();
        focusSearch();
      }
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, [focusSearch]);
  useEffect(() => {
    if (adding) dialogRef.current?.showModal();
    else dialogRef.current?.close();
  }, [adding]);
  useEffect(() => {
    if (searching) {
      searchDialogRef.current?.showModal();
      searchDialogRef.current?.querySelector("input")?.focus();
    } else searchDialogRef.current?.close();
  }, [searching]);

  function closeSearch() {
    setSearching(false);
    setQuery("");
  }

  function choose(next: Project) {
    closeSearch();
    setProjectsExpanded(true);
    setProject(next);
    setPage("project");
    setDetails(
      native
        ? null
        : previewInspections.find(
            ({ project }) => project.path === next.path,
          ) || null,
    );
    setError("");
    setLoading(native);
    setTab(savedTab(next.path));
    setRefresh((value) => value + 1);
  }
  function chooseTab(next: ProjectTab) {
    setTab(next);
    if (project) {
      try {
        localStorage.setItem(
          `repsync:tab:${project.path}`,
          JSON.stringify(next),
        );
      } catch {
        /* Navigation works without storage. */
      }
    }
  }
  function openAdd() {
    setAddError("");
    setAdding(true);
  }
  function reloadProject() {
    if (!native) return;
    setDetails(null);
    setLoading(true);
    setError("");
    setRefresh((value) => value + 1);
  }
  function reloadProjects() {
    setListing(true);
    setListError("");
    setListRefresh((value) => value + 1);
  }
  async function addProject(event: React.FormEvent) {
    event.preventDefault();
    setSaving(true);
    setAddError("");
    try {
      const added = await Service.add(path.trim());
      setProjects((previous) =>
        previous.some((p) => p.path === added.path)
          ? previous
          : [...previous, added],
      );
      setAdding(false);
      setPath("");
      choose(added);
    } catch (e) {
      setAddError(String(e));
    } finally {
      setSaving(false);
    }
  }
  const filtered = projects.filter((p) =>
    `${p.name} ${p.path}`.toLowerCase().includes(query.toLowerCase()),
  );
  const heading =
    page === "all"
      ? "所有项目"
      : page === "shared"
        ? "公共规范"
        : page === "updates"
          ? "更新"
          : "设置";

  return (
    <div className="flex h-dvh min-h-0 bg-white text-ink">
      <aside className="sidebar flex shrink-0 flex-col border-r border-line bg-sidebar">
        <div className="flex h-14 shrink-0 items-center gap-2 px-3">
          <Brand />
          <span className="text-xl font-bold tracking-tight">repsync</span>
          <button
            className="icon-button ml-auto"
            aria-label="搜索项目"
            onClick={focusSearch}
          >
            <Icon name="search" size={16} />
          </button>
        </div>
        <nav className="flex min-h-0 flex-col px-2" aria-label="项目导航">
          <div className="mb-2 flex shrink-0 items-center gap-1">
            <button
              type="button"
              id="project-group-toggle"
              className={`nav-item min-w-0 flex-1 ${!projectsExpanded && page === "project" ? "selected" : ""}`}
              aria-expanded={projectsExpanded}
              aria-controls="project-list"
              onClick={() => setProjectsExpanded((expanded) => !expanded)}
            >
              <Icon name="folder" />
              <span className="min-w-0 flex-1">
                <span className="block">项目</span>
                {!projectsExpanded && project && (
                  <span
                    className="block truncate text-xs font-normal text-muted"
                    title={project.path}
                  >
                    {project.name}
                  </span>
                )}
              </span>
              <span
                className={`transition-transform duration-150 motion-reduce:transition-none ${projectsExpanded ? "rotate-90" : ""}`}
              >
                <Icon name="chevron" size={16} />
              </span>
            </button>
            {native && (
              <button
                className="icon-button"
                aria-label="添加项目"
                onClick={openAdd}
              >
                <Icon name="plus" size={18} />
              </button>
            )}
          </div>
          <div
            id="project-list"
            hidden={!projectsExpanded}
            className={projectsExpanded ? "project-subnav" : "hidden"}
            role="group"
            aria-labelledby="project-group-toggle"
          >
            <div className="min-h-0 flex-1 overflow-y-auto">
              {projects.map((p) => (
                <button
                  key={p.path}
                  className={`nav-item ${page === "project" && project?.path === p.path ? "selected" : ""}`}
                  aria-current={
                    page === "project" && project?.path === p.path
                      ? "page"
                      : undefined
                  }
                  onClick={() => choose(p)}
                  title={p.path}
                >
                  <span className="truncate">{p.name}</span>
                </button>
              ))}
              {listing ? (
                <p role="status" className="px-3 py-3 text-xs text-muted">
                  正在读取项目…
                </p>
              ) : listError ? (
                <button
                  className="px-3 py-3 text-left text-xs text-removed"
                  onClick={() => {
                    setPage("all");
                    reloadProjects();
                  }}
                >
                  项目读取失败，点击重试
                </button>
              ) : (
                !projects.length && (
                  <p className="px-3 py-3 text-xs leading-6 text-muted">
                    {native ? "尚未添加项目" : "请在桌面应用中添加项目"}
                  </p>
                )
              )}
            </div>
          </div>
        </nav>
        <nav className="shrink-0 px-2 pb-3 pt-3" aria-label="全局导航">
          <button
            className={`nav-item ${page === "shared" ? "selected" : ""}`}
            aria-current={page === "shared" ? "page" : undefined}
            onClick={() => setPage("shared")}
          >
            <Icon name="layers" />
            公共规范
          </button>
          <button
            className={`nav-item ${page === "updates" ? "selected" : ""}`}
            aria-current={page === "updates" ? "page" : undefined}
            onClick={() => setPage("updates")}
          >
            <Icon name="download" />
            更新
          </button>
          <button
            className={`nav-item ${page === "settings" ? "selected" : ""}`}
            aria-current={page === "settings" ? "page" : undefined}
            onClick={() => setPage("settings")}
          >
            <Icon name="settings" />
            设置
          </button>
        </nav>
      </aside>
      <main className="flex min-w-0 flex-1 flex-col">
        {page === "project" && project ? (
          <>
            <header className="workspace-gutter shrink-0 pt-2">
              <div className="flex min-h-12 items-center gap-3">
                <span className="text-muted">
                  <Icon name="project" size={24} />
                </span>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <h1 className="truncate text-2xl font-semibold leading-7 tracking-tight">
                      {project.name}
                    </h1>
                    {!native && <span className="preview-badge">示例</span>}
                  </div>
                  <p
                    className="truncate text-sm leading-5 text-muted"
                    title={project.path}
                  >
                    {project.path}
                  </p>
                </div>
                <div
                  className="flex min-h-7 max-w-[30%] items-center gap-2 rounded-md bg-surface px-2 text-sm"
                  title="当前分支（只读）"
                >
                  <Icon name="branch" size={17} />
                  <span className="truncate">
                    {loading
                      ? "读取中…"
                      : details
                        ? details.branch || "非 Git 项目"
                        : "未读取"}
                  </span>
                </div>
                {native && (
                  <Button onClick={reloadProject} disabled={loading}>
                    <Icon name="refresh" size={16} />
                    刷新
                  </Button>
                )}
              </div>
              <div
                role="tablist"
                aria-label="项目页面"
                className="mt-1 flex gap-4 border-b border-line"
              >
                {tabs.map((name, index) => (
                  <button
                    key={name}
                    role="tab"
                    id={`tab-${name}`}
                    aria-selected={tab === name}
                    aria-controls="project-panel"
                    tabIndex={tab === name ? 0 : -1}
                    onKeyDown={(e) => {
                      if (
                        ["ArrowLeft", "ArrowRight", "Home", "End"].includes(
                          e.key,
                        )
                      ) {
                        e.preventDefault();
                        const next =
                          e.key === "Home"
                            ? 0
                            : e.key === "End"
                              ? tabs.length - 1
                              : (index +
                                  (e.key === "ArrowRight"
                                    ? 1
                                    : tabs.length - 1)) %
                                tabs.length;
                        chooseTab(tabs[next]);
                        document.getElementById(`tab-${tabs[next]}`)?.focus();
                      }
                    }}
                    onClick={() => chooseTab(name)}
                    className={`flex min-h-9 items-center border-b-2 px-3 text-sm ${tab === name ? "border-accent text-ink" : "border-transparent text-muted hover:text-ink"}`}
                  >
                    {name}
                  </button>
                ))}
              </div>
            </header>
            <section
              id="project-panel"
              role="tabpanel"
              aria-labelledby={`tab-${tab}`}
              className="workspace-gutter min-h-0 flex-1 overflow-auto py-5"
              tabIndex={0}
            >
              {loading ? (
                <p role="status" className="py-8 text-sm text-muted">
                  正在读取 {project.name}…
                </p>
              ) : error ? (
                <div
                  role="alert"
                  className="rounded-md bg-red-50 p-4 text-sm leading-7 text-red-800"
                >
                  {error}
                  <button className="ml-3 underline" onClick={reloadProject}>
                    重试
                  </button>
                </div>
              ) : details ? (
                <ProjectContent details={details} tab={tab} />
              ) : (
                <Empty title="暂时没有项目内容">请刷新重试。</Empty>
              )}
            </section>
          </>
        ) : (
          <>
            <header className="workspace-gutter flex min-h-14 shrink-0 items-center gap-3 border-b border-line">
              <h1 className="text-2xl font-semibold tracking-tight">
                {heading}
              </h1>
              {!native && <span className="preview-badge">示例预览</span>}
              {page === "all" && native && (
                <Button primary className="ml-auto" onClick={openAdd}>
                  <Icon name="plus" size={17} />
                  添加项目
                </Button>
              )}
            </header>
            <section
              className="workspace-gutter min-h-0 flex-1 overflow-auto py-5"
              tabIndex={0}
              aria-label={heading}
            >
              {page === "all" ? (
                listing ? (
                  <p role="status" className="py-8 text-sm text-muted">
                    正在读取项目…
                  </p>
                ) : listError ? (
                  <div
                    role="alert"
                    className="rounded-md bg-red-50 p-4 text-sm leading-7 text-red-800"
                  >
                    {listError}
                    <button className="ml-3 underline" onClick={reloadProjects}>
                      重试
                    </button>
                  </div>
                ) : projects.length ? (
                  <>
                    <p className="mb-4 text-sm leading-5 text-muted">
                      {native
                        ? "已登记的本地目录"
                        : "以下为示例数据，仅供界面预览，不读取或修改本地文件。"}
                    </p>
                    <div className="grouped-list">
                      {projects.map((p) => (
                        <button
                          className="project-row"
                          key={p.path}
                          onClick={() => choose(p)}
                          title={p.path}
                        >
                          <span className="text-muted">
                            <Icon name="folder" />
                          </span>
                          <span className="min-w-0 flex-1">
                            <strong className="block truncate text-sm font-medium">
                              {p.name}
                            </strong>
                            <span className="mt-1 block truncate text-xs text-muted">
                              {p.path}
                            </span>
                          </span>
                          {!native && (
                            <span className="preview-badge">示例</span>
                          )}
                          <span className="shrink-0 text-muted">
                            <Icon name="arrow" size={18} />
                          </span>
                        </button>
                      ))}
                    </div>
                  </>
                ) : (
                  <Empty
                    icon="inbox"
                    title={
                      native ? "添加你的第一个项目" : "请在桌面应用中管理项目"
                    }
                  >
                    {native ? (
                      <>
                        选择本地目录，查看项目状态、命令与文档。
                        <div className="mt-5">
                          <Button primary onClick={openAdd}>
                            <Icon name="plus" size={17} />
                            添加项目
                          </Button>
                        </div>
                      </>
                    ) : (
                      "浏览器无法读取本地目录。打开 repsync 桌面应用后即可添加项目。"
                    )}
                  </Empty>
                )
              ) : page === "shared" || page === "updates" ? (
                <SharedRules
                  key={page}
                  mode={page}
                  projects={projects}
                  onNavigate={setPage}
                />
              ) : (
                <>
                  <div className="section-heading">
                    <h2>本地工作区</h2>
                  </div>
                  <dl className="grouped-list">
                    {[
                      ["运行模式", native ? "桌面应用" : "浏览器示例预览"],
                      [
                        native ? "项目登记" : "示例数据",
                        native
                          ? "保存于系统用户配置目录 / repsync / projects.json"
                          : "仅在浏览器内展示，不登记本地项目",
                      ],
                      ["当前版本", "0.1.0"],
                    ].map(([label, value]) => (
                      <div key={label} className="info-row">
                        <dt className="text-muted">{label}</dt>
                        <dd>{value}</dd>
                      </div>
                    ))}
                  </dl>
                </>
              )}
            </section>
          </>
        )}
      </main>
      <dialog
        ref={searchDialogRef}
        aria-labelledby="search-project-title"
        onCancel={closeSearch}
        onClose={(event) => {
          if (!event.currentTarget.open) closeSearch();
        }}
        className="m-auto max-h-[calc(100dvh-32px)] w-[440px] max-w-[calc(100vw-32px)] overflow-y-auto rounded-xl bg-white p-0 text-ink shadow-xl backdrop:bg-black/20"
      >
        <div className="flex items-center gap-3 px-4 pt-4">
          <h2
            id="search-project-title"
            className="flex-1 text-lg font-semibold"
          >
            查找项目
          </h2>
          <button
            className="icon-button"
            aria-label="关闭搜索"
            onClick={closeSearch}
          >
            <Icon name="close" size={18} />
          </button>
        </div>
        <div className="p-4">
          <input
            autoFocus
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.nativeEvent.isComposing) return;
              if (e.key === "Escape") {
                e.preventDefault();
                closeSearch();
              } else if (e.key === "Enter" && filtered[0]) {
                e.preventDefault();
                choose(filtered[0]);
              }
            }}
            placeholder="项目名称或目录…"
            aria-label="查找项目"
            className="h-9 w-full rounded-md border border-control px-3 text-sm"
          />
          <div className="mt-4 max-h-[40dvh] overflow-y-auto">
            {listing ? (
              <p role="status" className="py-3 text-sm text-muted">
                正在读取项目…
              </p>
            ) : listError ? (
              <p role="alert" className="py-3 text-sm text-removed">
                {listError}
              </p>
            ) : filtered.length ? (
              <div className="grouped-list">
                {filtered.map((p) => (
                  <button
                    key={p.path}
                    className="project-row"
                    onClick={() => choose(p)}
                    title={p.path}
                  >
                    <Icon name="folder" />
                    <span className="min-w-0 flex-1 text-left">
                      <strong className="block truncate text-sm font-medium">
                        {p.name}
                      </strong>
                      <span className="mt-1 block truncate text-xs text-muted">
                        {p.path}
                      </span>
                    </span>
                    {!native && <span className="preview-badge">示例</span>}
                  </button>
                ))}
              </div>
            ) : (
              <p role="status" className="py-3 text-sm text-muted">
                {projects.length ? "没有匹配的项目" : "尚未添加项目"}
              </p>
            )}
          </div>
        </div>
      </dialog>
      <dialog
        aria-labelledby="add-project-title"
        ref={dialogRef}
        onCancel={() => setAdding(false)}
        onClose={() => setAdding(false)}
        className="m-auto max-h-[calc(100dvh-32px)] w-[400px] max-w-[calc(100vw-32px)] overflow-y-auto rounded-xl bg-white p-0 text-ink shadow-xl backdrop:bg-black/20"
      >
        <form onSubmit={addProject} className="p-5">
          <h2 id="add-project-title" className="text-xl font-semibold">
            添加本地项目
          </h2>
          <p className="mt-2 text-sm leading-6 text-muted">
            登记目录后读取项目状态，项目文件保持原样。
          </p>
          <label
            className="mb-2 mt-5 block text-sm font-medium"
            htmlFor="project-path"
          >
            项目绝对路径
          </label>
          <input
            id="project-path"
            autoFocus
            autoCapitalize="none"
            autoCorrect="off"
            spellCheck={false}
            required
            value={path}
            onChange={(e) => setPath(e.target.value)}
            placeholder="/Users/you/Projects/my-project"
            className="h-9 w-full rounded-md border border-control px-3 text-sm leading-5"
          />
          {addError && (
            <p role="alert" className="mt-3 text-sm leading-6 text-red-800">
              {addError}
            </p>
          )}
          <div className="mt-5 flex justify-end gap-2">
            <Button
              type="button"
              disabled={saving}
              onClick={() => setAdding(false)}
            >
              取消
            </Button>
            <Button primary type="submit" disabled={saving || !path.trim()}>
              {saving ? "正在登记…" : "添加项目"}
            </Button>
          </div>
        </form>
      </dialog>
    </div>
  );
}
