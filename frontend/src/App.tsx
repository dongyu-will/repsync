import { useCallback, useEffect, useRef, useState } from "react";
import { isMyGo } from "mygo-runtime";
import { Service, type Project, type Inspection } from "./mygo";
import { Brand, Button, Empty, Icon } from "./components";
import ProjectContent, { type ProjectTab } from "./ProjectContent";
import { previewInspections } from "./preview";
import SharedRules from "./SharedRules";
import AddProjectDialog from "./AddProjectDialog";
import ManageProjectDialog from "./ManageProjectDialog";
import type { GitSection } from "./ProjectGit";
import AIResources from "./AIResources";

const native = isMyGo();
type Page = "project" | "shared" | "ai-resources" | "settings";
const tabs: ProjectTab[] = [
  "概览",
  "工作树",
  "CI/CD",
  "规范",
  "AI 资源",
  "文档",
];
const lastProjectKey = native
  ? "repsync:last-project"
  : "repsync:preview:last-project";
const previewProjects = previewInspections.map(({ project }) => project);

function startupProject(projects: Project[]): Project | null {
  try {
    const saved = localStorage.getItem(lastProjectKey);
    const path = native
      ? saved
      : saved?.replace(/^\/示例项目(?=\/)/, "/Projects");
    return (
      projects.find((project) => project.path === path) || projects[0] || null
    );
  } catch {
    return projects[0] || null;
  }
}

function savedTab(path: string): ProjectTab {
  try {
    const value = JSON.parse(
      localStorage.getItem(`repsync:tab:${path}`) ||
        (!native &&
          localStorage.getItem(
            `repsync:tab:${path.replace(/^\/Projects(?=\/)/, "/示例项目")}`,
          )) ||
        '"概览"',
    );
    return tabs.includes(value) ? value : "概览";
  } catch {
    return "概览";
  }
}

export default function App() {
  const [page, setPage] = useState<Page>("project");
  const [project, setProject] = useState<Project | null>(() =>
    native ? null : startupProject(previewProjects),
  );
  const [projects, setProjects] = useState<Project[]>(
    native ? [] : previewProjects,
  );
  const [listing, setListing] = useState(native);
  const [listError, setListError] = useState("");
  const [listRefresh, setListRefresh] = useState(0);
  const [selectedTab, setTab] = useState<ProjectTab>(() =>
    project ? savedTab(project.path) : "概览",
  );
  const tab = tabs.includes(selectedTab) ? selectedTab : "概览";
  const [query, setQuery] = useState("");
  const [searching, setSearching] = useState(false);
  const [projectsExpanded, setProjectsExpanded] = useState(true);
  const [details, setDetails] = useState<Inspection | null>(() =>
    native
      ? null
      : previewInspections.find(
          (item) => item.project.path === project?.path,
        ) || null,
  );
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [refresh, setRefresh] = useState(0);
  const [adding, setAdding] = useState(false);
  const [managing, setManaging] = useState(false);
  const [gitSection, setGitSection] = useState<GitSection>("worktrees");
  const restoredProjectRef = useRef(false);
  const currentProjectRef = useRef(project);
  useEffect(() => {
    currentProjectRef.current = project;
  }, [project]);
  const searchDialogRef = useRef<HTMLDialogElement>(null);
  const focusSearch = useCallback(() => {
    if (adding || managing) return;
    setSearching(true);
    if (searchDialogRef.current?.open) {
      searchDialogRef.current.querySelector("input")?.focus();
    }
  }, [adding, managing]);

  useEffect(() => {
    if (!native) return;
    let active = true;
    const load = () =>
      Service.list()
        .then((items) => {
          if (active) {
            setProjects((previous) =>
              JSON.stringify(previous) === JSON.stringify(items)
                ? previous
                : items,
            );
            setListError("");
            setListing(false);
            if (!restoredProjectRef.current) {
              restoredProjectRef.current = true;
              const next = startupProject(items);
              if (next) {
                setProject(next);
                setTab(savedTab(next.path));
                setLoading(true);
              }
            } else {
              const current = currentProjectRef.current;
              const next =
                items.find((item) => item.path === current?.path) ||
                items[0] ||
                null;
              if (next?.path !== current?.path) {
                currentProjectRef.current = next;
                setProject(next);
                setDetails(null);
                setError("");
                setLoading(!!next);
                setManaging(false);
                setTab(next ? savedTab(next.path) : "概览");
                try {
                  if (next) localStorage.setItem(lastProjectKey, next.path);
                  else localStorage.removeItem(lastProjectKey);
                } catch {
                  /* Navigation works without storage. */
                }
              }
            }
          }
        })
        .catch((e) => {
          if (active) {
            setListError(String(e));
            setListing(false);
          }
        });
    void load();
    const interval = window.setInterval(load, 3000);
    window.addEventListener("focus", load);
    return () => {
      active = false;
      window.clearInterval(interval);
      window.removeEventListener("focus", load);
    };
  }, [listRefresh]);
  useEffect(() => {
    if (!native || !project) return;
    let active = true;
    let request = 0;
    const load = () => {
      const current = ++request;
      void Service.inspect(project.path)
        .then((result) => {
          if (active && current === request) {
            setDetails(result);
            setError("");
            setLoading(false);
          }
        })
        .catch((e) => {
          if (active && current === request) {
            setError(String(e));
            setLoading(false);
          }
        });
    };
    load();
    const timer = window.setInterval(load, 60000);
    window.addEventListener("focus", load);
    return () => {
      active = false;
      window.clearInterval(timer);
      window.removeEventListener("focus", load);
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
    try {
      localStorage.setItem(lastProjectKey, next.path);
    } catch {
      /* Project switching works without storage. */
    }
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
  function addedProject(added: Project) {
    setProjects((previous) =>
      previous.some((p) => p.path === added.path)
        ? previous
        : [...previous, added],
    );
    setAdding(false);
    choose(added);
  }
  function changedProject(next: Project | null) {
    if (!project) return;
    const previous = project.path;
    const items = next
      ? projects.map((item) => (item.path === previous ? next : item))
      : projects.filter((item) => item.path !== previous);
    setProjects(items);
    setManaging(false);
    try {
      if (next && next.path !== previous)
        localStorage.setItem(`repsync:tab:${next.path}`, JSON.stringify(tab));
      localStorage.removeItem(`repsync:tab:${previous}`);
      localStorage.removeItem(lastProjectKey);
    } catch {
      /* Registration works without storage. */
    }
    const selected = next || items[0];
    if (selected) choose(selected);
    else {
      setProject(null);
      setDetails(null);
      setError("");
      setLoading(false);
    }
    reloadProjects();
  }
  const filtered = projects.filter((p) =>
    `${p.name} ${p.path}`.toLowerCase().includes(query.toLowerCase()),
  );
  const heading =
    page === "project"
      ? "项目"
      : page === "shared"
        ? "公共规范"
        : page === "ai-resources"
          ? "AI 资源"
          : "设置";

  return (
    <div className="flex h-dvh min-h-0 bg-white text-ink">
      <aside className="sidebar flex shrink-0 flex-col border-r border-line bg-sidebar">
        <div className="sidebar-titlebar window-toolbar flex shrink-0 items-center gap-2">
          <div className="flex min-w-0 items-center gap-2 rounded-md">
            <Brand />
            <span className={native ? "sr-only" : "text-base font-semibold"}>
              repsync
            </span>
          </div>
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
                    setPage("project");
                    reloadProjects();
                  }}
                >
                  项目读取失败，点击重试
                </button>
              ) : (
                !projects.length && (
                  <p className="px-3 py-3 text-xs leading-6 text-muted">
                    尚未添加项目
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
            className={`nav-item ${page === "ai-resources" ? "selected" : ""}`}
            aria-current={page === "ai-resources" ? "page" : undefined}
            onClick={() => setPage("ai-resources")}
          >
            <Icon name="file" />
            AI 资源
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
            <header className="workspace-gutter shrink-0">
              <div className="window-toolbar flex items-center gap-3">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <h1 className="truncate text-base font-semibold">
                      {project.name}
                    </h1>
                  </div>
                  <p
                    className="project-path truncate text-xs text-muted"
                    title={project.path}
                  >
                    {project.path}
                  </p>
                </div>
                <button
                  className="flex min-h-7 max-w-[30%] items-center gap-2 rounded-md bg-surface px-2 text-sm hover:bg-nav-hover"
                  title="查看工作树和分支"
                  aria-label="查看工作树和分支"
                  onClick={() => {
                    setGitSection("worktrees");
                    chooseTab("工作树");
                  }}
                >
                  <Icon name="branch" size={17} />
                  <span className="truncate">
                    {loading
                      ? "读取中…"
                      : details
                        ? details.branch || "非 Git 项目"
                        : "未读取"}
                  </span>
                </button>
                {native && (
                  <button
                    className="icon-button text-muted hover:text-ink"
                    onClick={() => setManaging(true)}
                    aria-label="管理项目"
                    title="管理项目"
                  >
                    <Icon name="settings" size={16} />
                  </button>
                )}
                {native && (
                  <button
                    className="icon-button text-muted hover:text-ink disabled:cursor-not-allowed disabled:opacity-45"
                    onClick={reloadProject}
                    disabled={loading}
                    aria-label="刷新项目"
                    title="刷新项目"
                  >
                    <Icon name="refresh" size={16} />
                  </button>
                )}
              </div>
              <div
                role="tablist"
                aria-label="项目页面"
                className="flex gap-4 border-b border-line"
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
              className="workspace-gutter min-h-0 flex-1 overflow-auto py-4"
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
                <ProjectContent
                  details={details}
                  tab={tab}
                  onOpenShared={() => setPage("shared")}
                  onUpdated={() => setRefresh((value) => value + 1)}
                  gitSection={gitSection}
                  onOpenCI={() => chooseTab("CI/CD")}
                  onOpenStashes={() => {
                    setGitSection("stashes");
                    chooseTab("工作树");
                  }}
                />
              ) : (
                <Empty title="暂时没有项目内容">请刷新重试。</Empty>
              )}
            </section>
          </>
        ) : (
          <>
            <header className="window-toolbar workspace-gutter flex shrink-0 items-center gap-3 border-b border-line">
              <h1 className="text-base font-semibold">{heading}</h1>
            </header>
            <section
              className="workspace-gutter min-h-0 flex-1 overflow-auto py-4"
              tabIndex={0}
              aria-label={heading}
            >
              {page === "project" ? (
                listing ? (
                  <p role="status" className="py-8 text-sm text-muted">
                    正在读取项目…
                  </p>
                ) : listError ? (
                  <div role="alert" className="space-y-3 text-sm text-red-800">
                    <p>{listError}</p>
                    <Button onClick={reloadProjects}>重试</Button>
                  </div>
                ) : (
                  <Empty
                    title={
                      projects.length ? "选择一个项目" : "添加你的第一个项目"
                    }
                  >
                    {projects.length ? (
                      <Button onClick={focusSearch}>选择项目</Button>
                    ) : native ? (
                      <>
                        添加本地目录，开始查看项目文档和管理公共规范。
                        <div className="mt-4">
                          <Button primary onClick={openAdd}>
                            <Icon name="plus" size={16} />
                            添加项目
                          </Button>
                        </div>
                      </>
                    ) : (
                      "请在桌面应用中添加本地项目。"
                    )}
                  </Empty>
                )
              ) : page === "shared" ? (
                <SharedRules projects={projects} />
              ) : page === "ai-resources" ? (
                <AIResources />
              ) : (
                <>
                  <dl className="grouped-list" aria-label="本地工作区">
                    {[
                      ...(native ? [["运行模式", "桌面应用"]] : []),
                      [
                        "项目登记",
                        native
                          ? "保存于系统用户配置目录 / repsync / projects.json"
                          : `${projects.length} 个项目`,
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
      {adding && (
        <AddProjectDialog
          onClose={() => setAdding(false)}
          onAdded={addedProject}
        />
      )}
      {managing && project && (
        <ManageProjectDialog
          key={project.path}
          project={project}
          onClose={() => setManaging(false)}
          onChanged={changedProject}
        />
      )}
    </div>
  );
}
