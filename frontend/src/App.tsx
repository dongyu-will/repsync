import { t, useLanguage } from "./i18n";
import {
  lazy,
  Suspense,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";
import { isMyGo } from "mygo-runtime";
import { Service, type Project, type Inspection } from "./mygo";
import { Brand, Button, Empty, Icon } from "./components";
import ProjectContent, { type ProjectTab } from "./ProjectContent";
import { previewInspections } from "./preview";
import SharedWorkspace from "./SharedWorkspace";
import AddProjectDialog from "./AddProjectDialog";
import ManageProjectDialog from "./ManageProjectDialog";
import type { GitSection } from "./ProjectGit";
import Settings from "./Settings";
import { usePreferences, type Preferences } from "./preferences";
import {
  primaryProjectTabs,
  utilityProjectTabs,
  isUtilityProjectTab,
  projectTabLabel,
  restoreProjectTab,
  restorePrimaryProjectTab,
} from "./project-tabs";

const AIResources = lazy(() => import("./AIResources"));

const native = isMyGo();
type Page = "project" | "shared" | "ai-resources" | "settings";
const lastProjectKey = native
  ? "repsync:last-project"
  : "repsync:preview:last-project";
const previewProjects = previewInspections.map(({ project }) => project);

function startupProject(
  projects: Project[],
  preferences: Preferences,
): Project | null {
  try {
    const saved = preferences.rememberProject
      ? localStorage.getItem(lastProjectKey)
      : null;
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

function savedTab(path: string, preferences: Preferences): ProjectTab {
  if (!preferences.rememberTabs) return preferences.defaultTab;
  try {
    const value = JSON.parse(
      localStorage.getItem(`repsync:tab:${path}`) ||
        (!native &&
          localStorage.getItem(
            `repsync:tab:${path.replace(/^\/Projects(?=\/)/, "/示例项目")}`,
          )) ||
        JSON.stringify(preferences.defaultTab),
    );
    return restorePrimaryProjectTab(value, preferences.defaultTab);
  } catch {
    return preferences.defaultTab;
  }
}

export default function App() {
  useLanguage();
  const settings = usePreferences();
  const preferences = settings.values;
  const [page, setPage] = useState<Page>("project");
  const [project, setProject] = useState<Project | null>(() =>
    native ? null : startupProject(previewProjects, preferences),
  );
  const [projects, setProjects] = useState<Project[]>(
    native ? [] : previewProjects,
  );
  const [listing, setListing] = useState(native);
  const [listError, setListError] = useState("");
  const [listRefresh, setListRefresh] = useState(0);
  const [selectedTab, setTab] = useState<ProjectTab>(() =>
    project ? savedTab(project.path, preferences) : "Overview",
  );
  const tab = restoreProjectTab(selectedTab, "Overview");
  const tabs = primaryProjectTabs;
  const utilityPage = isUtilityProjectTab(tab);
  const [utilitiesExpanded, setUtilitiesExpanded] = useState(false);
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
              const next = startupProject(items, preferences);
              if (next) {
                setProject(next);
                setTab(savedTab(next.path, preferences));
                setLoading(true);
              }
            } else {
              const current = currentProjectRef.current;
              const next =
                items.find((item) => item.path === current?.path) ||
                items[0] ||
                null;
              if (next?.path !== current?.path) {
                setUtilitiesExpanded(false);
                currentProjectRef.current = next;
                setProject(next);
                setDetails(null);
                setError("");
                setLoading(!!next);
                setManaging(false);
                setTab(
                  next
                    ? savedTab(next.path, preferences)
                    : preferences.defaultTab,
                );
                try {
                  if (next && preferences.rememberProject)
                    localStorage.setItem(lastProjectKey, next.path);
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
  }, [listRefresh, preferences]);
  useEffect(() => {
    if (!native || !project || page !== "project") return;
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
    const timer = preferences.refreshSeconds
      ? window.setInterval(load, preferences.refreshSeconds * 1000)
      : undefined;
    window.addEventListener("focus", load);
    return () => {
      active = false;
      window.clearInterval(timer);
      window.removeEventListener("focus", load);
    };
  }, [project, page, refresh, preferences.refreshSeconds]);
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
    setUtilitiesExpanded(false);
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
    setTab(savedTab(next.path, preferences));
    setRefresh((value) => value + 1);
    try {
      if (preferences.rememberProject)
        localStorage.setItem(lastProjectKey, next.path);
    } catch {
      /* Project switching works without storage. */
    }
  }
  function chooseTab(next: ProjectTab) {
    setTab(next);
    setUtilitiesExpanded(isUtilityProjectTab(next));
    if (project && preferences.rememberTabs && !isUtilityProjectTab(next)) {
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
      ? "Project"
      : page === "shared"
        ? "Shared rules"
        : page === "ai-resources"
          ? "AI resources"
          : "Settings";

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
            aria-label={t("Search projects")}
            onClick={focusSearch}
          >
            <Icon name="search" size={16} />
          </button>
        </div>
        <nav
          className="flex min-h-0 flex-col px-2"
          aria-label={t("Project navigation")}
        >
          <div className="mb-2 flex shrink-0 items-center gap-1">
            <button
              type="button"
              id="project-group"
              className={`nav-item min-w-0 flex-1 ${!projectsExpanded && page === "project" ? "selected" : ""}`}
              aria-expanded={projectsExpanded}
              aria-controls="project-list"
              onClick={() => setProjectsExpanded((expanded) => !expanded)}
            >
              <Icon name="folder" />
              <span className="min-w-0 flex-1">
                <span className="block">{t("Projects")}</span>
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
                aria-label={t("Add project")}
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
            aria-labelledby="project-group"
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
                  {t("Loading projects…")}
                </p>
              ) : listError ? (
                <button
                  className="px-3 py-3 text-left text-xs text-removed"
                  onClick={() => {
                    setPage("project");
                    reloadProjects();
                  }}
                >
                  {t("Could not load projects. Click to retry.")}
                </button>
              ) : (
                !projects.length && (
                  <p className="px-3 py-3 text-xs leading-6 text-muted">
                    {t("No projects added")}
                  </p>
                )
              )}
            </div>
          </div>
        </nav>
        <nav
          className="shrink-0 px-2 pb-3 pt-3"
          aria-label={t("Global navigation")}
        >
          <button
            className={`nav-item ${page === "shared" ? "selected" : ""}`}
            aria-current={page === "shared" ? "page" : undefined}
            onClick={() => setPage("shared")}
          >
            <Icon name="layers" />
            {t("Shared rules")}
          </button>
          <button
            className={`nav-item ${page === "ai-resources" ? "selected" : ""}`}
            aria-current={page === "ai-resources" ? "page" : undefined}
            onClick={() => setPage("ai-resources")}
          >
            <Icon name="file" />
            {t("AI resources")}
          </button>
          <button
            className={`nav-item ${page === "settings" ? "selected" : ""}`}
            aria-current={page === "settings" ? "page" : undefined}
            onClick={() => setPage("settings")}
          >
            <Icon name="settings" />
            {t("Settings")}
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
                  title={t("View branches and workspaces")}
                  aria-label={t("View branches and workspaces")}
                  onClick={() => {
                    setGitSection("worktrees");
                    chooseTab("Worktrees");
                  }}
                >
                  <Icon name="branch" size={17} />
                  <span className="truncate">
                    {loading
                      ? t("Loading…")
                      : details
                        ? details.branch || t("Non-Git project")
                        : t("Not loaded")}
                  </span>
                </button>
                {native && (
                  <button
                    className="icon-button text-muted hover:text-ink"
                    onClick={() => setManaging(true)}
                    aria-label={t("Manage project")}
                    title={t("Manage project")}
                  >
                    <Icon name="settings" size={16} />
                  </button>
                )}
                {native && (
                  <button
                    className="icon-button text-muted hover:text-ink disabled:cursor-not-allowed disabled:opacity-45"
                    onClick={reloadProject}
                    disabled={loading}
                    aria-label={t("Refresh project")}
                    title={t("Refresh project")}
                  >
                    <Icon name="refresh" size={16} />
                  </button>
                )}
              </div>
              <div className="flex flex-wrap items-center justify-between gap-x-3 border-b border-line">
                <div
                  role="tablist"
                  aria-label={t("Project pages")}
                  className="flex flex-wrap gap-1"
                >
                  {tabs.map((name, index) => (
                    <button
                      key={name}
                      role="tab"
                      id={`tab-${name}`}
                      aria-selected={tab === name}
                      aria-controls="project-panel"
                      tabIndex={
                        tab === name || (utilityPage && index === 0) ? 0 : -1
                      }
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
                      className={`flex min-h-9 shrink-0 items-center border-b-2 px-3 text-sm ${tab === name ? "border-accent text-ink" : "border-transparent text-muted hover:text-ink"}`}
                    >
                      {t(projectTabLabel(name))}
                    </button>
                  ))}
                </div>
                <button
                  type="button"
                  aria-expanded={utilitiesExpanded}
                  aria-controls="project-utilities"
                  onClick={() => {
                    if (utilitiesExpanded && utilityPage) chooseTab("Overview");
                    else setUtilitiesExpanded(!utilitiesExpanded);
                  }}
                  className={`flex min-h-9 items-center gap-1 rounded-md px-2 text-sm hover:bg-nav-hover ${utilityPage ? "text-ink" : "text-muted"}`}
                >
                  {t("Utilities")}
                  <span
                    aria-hidden="true"
                    className={utilitiesExpanded ? "rotate-90" : ""}
                  >
                    <Icon name="chevron" size={14} />
                  </span>
                </button>
              </div>
              <nav
                id="project-utilities"
                aria-label={t("Utilities")}
                className={
                  utilitiesExpanded
                    ? "flex flex-wrap gap-1 border-b border-line py-1"
                    : "hidden"
                }
              >
                {utilitiesExpanded &&
                  utilityProjectTabs.map((name) => (
                    <button
                      type="button"
                      key={name}
                      id={`utility-${name.replaceAll(" ", "-")}`}
                      aria-current={tab === name ? "page" : undefined}
                      onClick={() => chooseTab(name)}
                      className={`min-h-8 rounded-md border px-3 text-sm ${tab === name ? "border-accent bg-selection text-ink" : "border-transparent text-muted hover:bg-nav-hover hover:text-ink"}`}
                    >
                      {t(projectTabLabel(name))}
                    </button>
                  ))}
              </nav>
            </header>
            <section
              id="project-panel"
              role={utilityPage ? "region" : "tabpanel"}
              aria-labelledby={
                utilityPage
                  ? `utility-${tab.replaceAll(" ", "-")}`
                  : `tab-${tab}`
              }
              className="workspace-gutter min-h-0 flex-1 overflow-auto py-4"
              tabIndex={0}
            >
              {loading ? (
                <p role="status" className="py-8 text-sm text-muted">
                  {t("Loading")} {project.name}…
                </p>
              ) : error ? (
                <div
                  role="alert"
                  className="rounded-md bg-red-50 p-4 text-sm leading-7 text-red-800"
                >
                  {t(error)}
                  <button className="ml-3 underline" onClick={reloadProject}>
                    {t("Retry")}
                  </button>
                </div>
              ) : details ? (
                <ProjectContent
                  details={details}
                  tab={tab}
                  onUpdated={() => setRefresh((value) => value + 1)}
                  gitSection={gitSection}
                  onOpenCI={() => chooseTab("CI/CD")}
                  onOpenStashes={() => {
                    setGitSection("stashes");
                    chooseTab("Worktrees");
                  }}
                />
              ) : (
                <Empty title={t("No project content yet")}>
                  {t("Refresh to try again.")}
                </Empty>
              )}
            </section>
          </>
        ) : (
          <>
            <header className="window-toolbar workspace-gutter flex shrink-0 items-center gap-3 border-b border-line">
              <h1 className="text-base font-semibold">{t(heading)}</h1>
            </header>
            <section
              className="workspace-gutter min-h-0 flex-1 overflow-auto py-4"
              tabIndex={0}
              aria-label={t(heading)}
            >
              {page === "project" ? (
                listing ? (
                  <p role="status" className="py-8 text-sm text-muted">
                    {t("Loading projects…")}
                  </p>
                ) : listError ? (
                  <div role="alert" className="space-y-3 text-sm text-red-800">
                    <p>{t(listError)}</p>
                    <Button onClick={reloadProjects}>{t("Retry")}</Button>
                  </div>
                ) : (
                  <Empty
                    title={
                      projects.length
                        ? t("Choose a project")
                        : t("Add your first project")
                    }
                  >
                    {projects.length ? (
                      <Button onClick={focusSearch}>
                        {t("Select project")}
                      </Button>
                    ) : native ? (
                      <>
                        {t(
                          "Add a local folder to browse project documents and manage shared rules.",
                        )}
                        <div className="mt-4">
                          <Button primary onClick={openAdd}>
                            <Icon name="plus" size={16} />
                            {t("Add project")}
                          </Button>
                        </div>
                      </>
                    ) : (
                      t("Use the desktop app to add local projects.")
                    )}
                  </Empty>
                )
              ) : page === "shared" ? (
                <SharedWorkspace projects={projects} />
              ) : page === "ai-resources" ? (
                <Suspense fallback={<p role="status">{t("Loading…")}</p>}>
                  <AIResources />
                </Suspense>
              ) : (
                <Settings
                  values={preferences}
                  error={settings.error}
                  notice={settings.notice}
                  projectCount={projects.length}
                  onChange={settings.change}
                  onReset={settings.reset}
                />
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
            {t("Find a project")}
          </h2>
          <button
            className="icon-button"
            aria-label={t("Close search")}
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
            placeholder={t("Project name or path…")}
            aria-label={t("Find a project")}
            className="h-9 w-full rounded-md border border-control px-3 text-sm"
          />
          <div className="mt-4 max-h-[40dvh] overflow-y-auto">
            {listing ? (
              <p role="status" className="py-3 text-sm text-muted">
                {t("Loading projects…")}
              </p>
            ) : listError ? (
              <p role="alert" className="py-3 text-sm text-removed">
                {t(listError)}
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
                {projects.length
                  ? t("No matching projects")
                  : t("No projects added")}
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
