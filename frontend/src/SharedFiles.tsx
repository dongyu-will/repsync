import { formatMessage, getLocale, t } from "./i18n";
import { useEffect, useReducer, type FormEvent } from "react";
import { isMyGo } from "mygo-runtime";
import { Button, Empty, Icon, Select } from "./components";
import ActionDialog from "./ActionDialog";
import {
  Service,
  type Project,
  type ResourceOverview,
  type SharedResourceInfo,
  type UpdateHistory,
  type UpdatePreview,
} from "./mygo";
import { sharedFilesPreview, sharedFilePreview } from "./shared-files-preview";

const native = isMyGo();
const message = (error: unknown) =>
  error instanceof Error ? error.message : String(error);
const statuses: Record<string, string> = {
  current: "Up to date",
  outdated: "Update available",
  missing: "Pending write",
  modified: "Project modified",
  error: "Needs attention",
};
const filename = (path: string) => path.split("/").at(-1) || path;
const inputClass =
  "w-full min-w-0 rounded-md border border-control bg-white px-3 py-2 text-sm";

type State = {
  report: ResourceOverview | null;
  loadedPath: string;
  selected: string;
  busy: string;
  error: string;
  loadError: string;
  notice: string;
  revision: number;
  preview: UpdatePreview | null;
  historyPath: string;
};

export default function SharedFiles({
  projects,
  project,
  onUpdated,
  active = true,
}: {
  projects: Project[];
  project?: Project;
  onUpdated?: () => void;
  active?: boolean;
}) {
  const path = project?.path || "";
  const [state, update] = useReducer(
    (s: State, p: Partial<State>) => ({ ...s, ...p }),
    {
      report: null,
      loadedPath: "",
      selected: "",
      busy: "",
      error: "",
      loadError: "",
      notice: "",
      revision: 0,
      preview: null,
      historyPath: "",
    },
  );
  useEffect(() => {
    if (!active) return;
    let live = true;
    const request = native
      ? Service.sharedResources(path)
      : Promise.resolve(sharedFilesPreview(project ? [project] : projects));
    void request
      .then((report) => {
        if (live) update({ report, loadedPath: path, loadError: "" });
      })
      .catch((error) => {
        if (live) update({ loadError: message(error) });
      });
    return () => {
      live = false;
    };
  }, [active, path, projects, project, state.revision]);

  const refresh = () => update({ revision: state.revision + 1, preview: null });
  const registered = (id: string) =>
    update({
      selected: id,
      notice: "Shared source saved. Project files have not changed.",
      revision: state.revision + 1,
      preview: null,
    });
  async function perform(operation: () => Promise<unknown>, notice: string) {
    if (!native || state.busy) return;
    update({ busy: "write", error: "", notice: "", preview: null });
    try {
      await operation();
      update({ notice });
      onUpdated?.();
    } catch (error) {
      update({ error: message(error) });
    } finally {
      update({ busy: "", revision: state.revision + 1 });
    }
  }
  async function preview(project: Project, id: string) {
    if (!state.report || state.busy) return;
    update({ busy: "preview", error: "", notice: "", preview: null });
    try {
      const value = native
        ? await Service.previewResource(project.path, id)
        : sharedFilePreview(state.report, project, id);
      update({ preview: value });
    } catch (error) {
      update({ error: message(error) });
    } finally {
      update({ busy: "" });
    }
  }

  const report = state.loadedPath === path ? state.report : null;
  const selected =
    report?.resources.find((item) => item.id === state.selected) ||
    report?.resources[0];
  const selectResource = (selected: string) =>
    update({ selected, preview: null, error: "", notice: "" });
  return (
    <section aria-label={t("Shared file resources")} className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="min-w-0 text-sm text-muted">
          {t("Share test scripts and check configurations across projects.")}
        </p>
        <div className="flex flex-wrap gap-2">
          {!project && report && (
            <ResourceEditor
              key={report.version}
              version={report.version}
              onSaved={registered}
            />
          )}
          <button
            className="icon-button text-muted"
            aria-label={t("Reload shared files")}
            disabled={!!state.busy}
            onClick={refresh}
          >
            <Icon name="refresh" />
          </button>
        </div>
      </div>
      {(state.error || state.loadError) && (
        <p role="alert" className="break-words text-sm text-removed">
          {t(state.error || state.loadError)}
        </p>
      )}
      {state.notice && (
        <p role="status" className="text-sm text-muted">
          {t(state.notice)}
        </p>
      )}
      {state.busy && (
        <p role="status" className="text-sm text-muted">
          {state.busy === "preview" ? t("Generating diff…") : t("Saving…")}
        </p>
      )}
      {!selected &&
        report?.projects
          .filter((item) => item.error)
          .map((item) => (
            <p
              key={item.project.path}
              role="alert"
              className="break-words text-sm text-removed"
            >
              {item.project.name}: {t(item.error)}
            </p>
          ))}
      {!report ? (
        <p role="status" className="text-sm text-muted">
          {state.loadError
            ? t("Could not load shared resources. Try reloading.")
            : t("Loading shared resources…")}
        </p>
      ) : !selected ? (
        <Empty title={t("No shared files registered")} icon="layers">
          {t(
            "Register reusable test scripts or check configurations as shared sources.",
          )}
          {project && t("Register a source in global shared rules.")}
        </Empty>
      ) : (
        <div className="grid min-w-0 gap-5 min-[900px]:grid-cols-[176px_minmax(0,1fr)]">
          <label className="grid gap-1 text-xs text-muted min-[900px]:hidden">
            {t("Shared files")}
            <Select
              className="w-full"
              value={selected.id}
              onChange={(event) => selectResource(event.target.value)}
            >
              {report.resources.map((resource) => (
                <option key={resource.id} value={resource.id}>
                  {filename(resource.target) || resource.id} · {resource.id}
                </option>
              ))}
            </Select>
          </label>
          <div
            className="grouped-list hidden self-start min-[900px]:block"
            role="group"
            aria-label={t("Shared files")}
          >
            {report.resources.map((resource) => {
              const count = report.projects.filter((p) =>
                p.adoptions.some((a) => a.id === resource.id),
              ).length;
              return (
                <button
                  key={resource.id}
                  type="button"
                  aria-pressed={selected.id === resource.id}
                  onClick={() => selectResource(resource.id)}
                  className={`block w-full min-w-0 border-b border-line-subtle px-3 py-3 text-left last:border-0 ${selected.id === resource.id ? "bg-selection" : "hover:bg-sidebar"}`}
                >
                  <span className="block break-all text-sm font-medium">
                    {filename(resource.target) || resource.id}
                  </span>
                  <span className="mt-1 block text-xs text-muted">
                    {resource.error
                      ? t("Source needs attention")
                      : report.projects.some((item) => item.error)
                        ? t(
                            "{0}  known adoptions · Some statuses unknown",
                            count,
                          )
                        : project
                          ? count
                            ? t("Adopted by this project")
                            : t("Not adopted by this project")
                          : t("{0} projects using this", count)}
                  </span>
                </button>
              );
            })}
          </div>
          <div className="min-w-0 space-y-4">
            <div className="flex flex-wrap items-start justify-between gap-2">
              <div className="min-w-0">
                <h2 className="break-all text-base font-semibold">
                  {selected.id}
                </h2>
                <p className="mt-1 text-xs text-muted" title={selected.version}>
                  {t("Source version")}{" "}
                  {selected.version
                    ? selected.version.slice(0, 12)
                    : t("Unknown")}
                </p>
              </div>
              {!project && (
                <div className="flex flex-wrap gap-2">
                  <ResourceEditor
                    key={`${selected.id}:${report.version}`}
                    version={report.version}
                    resource={selected}
                    onSaved={registered}
                  />
                  <ActionDialog
                    title={t("Remove shared resource registration")}
                    trigger={t("Remove registration")}
                    busy={!!state.busy}
                    error={state.error}
                    notice={state.notice}
                  >
                    <p className="text-sm leading-6">
                      {t("Remove")} {selected.id}{" "}
                      {t(
                        "from shared resources? Adopted project files and history are preserved. Register the source again to resume updates.",
                      )}
                    </p>
                    <Button
                      disabled={!native || !!state.busy || !selected.path}
                      onClick={() =>
                        void perform(
                          () =>
                            Service.forgetResource(report.version, selected.id),
                          "Shared registration removed. Project files and history preserved.",
                        )
                      }
                    >
                      {t("Remove registration")}
                    </Button>
                  </ActionDialog>
                </div>
              )}
            </div>
            <dl className="space-y-2 text-xs leading-5">
              <div className="grid grid-cols-[64px_minmax(0,1fr)] gap-2">
                <dt className="text-muted">{t("Shared source")}</dt>
                <dd className="break-all">
                  {selected.path || t("Not registered")}
                </dd>
              </div>
              <div className="grid grid-cols-[64px_minmax(0,1fr)] gap-2">
                <dt className="text-muted">{t("Project location")}</dt>
                <dd className="break-all">{selected.target || t("Unknown")}</dd>
              </div>
            </dl>
            {selected.error && (
              <p role="alert" className="break-words text-sm text-removed">
                {t(selected.error)}
              </p>
            )}
            {report.projects.length ? (
              <div className="grouped-list" aria-label={t("Project adoption")}>
                {report.projects.map((entry) => {
                  const adopted = entry.adoptions.find(
                    (item) => item.id === selected.id,
                  );
                  const status = entry.error
                    ? "Read failed"
                    : adopted
                      ? statuses[adopted.status] || adopted.status
                      : "Not adopted";
                  return (
                    <section
                      key={entry.project.path}
                      className="space-y-2 border-b border-line-subtle p-3 last:border-0"
                    >
                      <div className="flex flex-wrap items-baseline justify-between gap-2">
                        <h3 className="min-w-0 break-all text-sm font-medium">
                          {entry.project.name}
                        </h3>
                        <span
                          className={`text-xs ${entry.error || adopted?.error || adopted?.status === "modified" ? "text-removed" : "text-muted"}`}
                        >
                          {t(status)}
                        </span>
                      </div>
                      <p className="break-all text-xs text-muted">
                        {entry.project.path}
                      </p>
                      {adopted?.adoptedVersion && (
                        <p
                          className="text-xs text-muted"
                          title={adopted.adoptedVersion}
                        >
                          {t("Adopted version")}{" "}
                          {adopted.adoptedVersion.slice(0, 12)}
                        </p>
                      )}
                      {(entry.error || adopted?.error) && (
                        <p className="break-words text-xs text-removed">
                          {t(entry.error || adopted?.error)}
                        </p>
                      )}
                      <div className="flex flex-wrap gap-2">
                        <Button
                          disabled={
                            !native ||
                            !!state.busy ||
                            !!entry.error ||
                            (!adopted && !!selected.error)
                          }
                          onClick={() =>
                            void perform(
                              () =>
                                Service.setResourceAdoption(
                                  entry.project.path,
                                  entry.configVersion,
                                  report.version,
                                  selected.id,
                                  !adopted,
                                ),
                              adopted
                                ? "Adoption removed. Project files and history preserved."
                                : "Adoption registered. Preview the diff before updating project files.",
                            )
                          }
                        >
                          {adopted
                            ? t("Remove adoption")
                            : t("Adopt this resource")}
                        </Button>
                        {adopted && (
                          <Button
                            disabled={
                              !!state.busy || !!entry.error || !!selected.error
                            }
                            onClick={() =>
                              void preview(entry.project, selected.id)
                            }
                          >
                            {t("Preview diff")}
                          </Button>
                        )}
                        <ActionDialog
                          title={t(
                            "{0} · Shared file history",
                            entry.project.name,
                          )}
                          trigger={t("Update history")}
                          onOpen={() =>
                            update({ historyPath: entry.project.path })
                          }
                          busy={!!state.busy}
                          error={state.error}
                          notice={state.notice}
                        >
                          {state.historyPath === entry.project.path && (
                            <ResourceHistory
                              path={entry.project.path}
                              revision={state.revision}
                              busy={!!state.busy}
                              onRestore={(record) =>
                                void perform(
                                  () => Service.restoreUpdate(record.id),
                                  formatMessage(
                                    "Restored {0}   {1}",
                                    record.project.name,
                                    record.target,
                                  ),
                                )
                              }
                            />
                          )}
                        </ActionDialog>
                      </div>
                    </section>
                  );
                })}
              </div>
            ) : (
              <p className="text-sm text-muted">
                {t("No registered projects.")}
              </p>
            )}
            {state.preview && (
              <ActionDialog
                title={`${state.preview.project.name} / ${state.preview.target}`}
                trigger={null}
                open
                onOpenChange={(open) => {
                  if (!open) update({ preview: null });
                }}
                busy={!!state.busy}
                error={state.error}
              >
                <section
                  aria-label={t("Shared file update preview")}
                  className="space-y-3"
                >
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <p
                      className="text-xs text-muted"
                      title={state.preview.version}
                    >
                      {t("Source version")} {state.preview.version.slice(0, 12)}
                    </p>
                    <Button
                      primary
                      disabled={
                        !native ||
                        !!state.busy ||
                        !state.preview.changed ||
                        !!state.preview.conflict
                      }
                      onClick={() => {
                        const plan = state.preview;
                        if (plan)
                          void perform(
                            () => Service.applyUpdate(plan.token),
                            formatMessage(
                              "Updated {0}   {1}, Backup saved",
                              plan.project.name,
                              plan.target,
                            ),
                          );
                      }}
                    >
                      {t("Apply update")}
                    </Button>
                  </div>
                  {state.preview.conflict && (
                    <p role="alert" className="text-sm text-removed">
                      {t(
                        "The project file has local edits and cannot be overwritten. Preserve project differences, or ask AI Connect an independent shared file.",
                      )}
                    </p>
                  )}
                  {!state.preview.changed ? (
                    <p role="status" className="text-sm text-muted">
                      {t("File matches the shared source.")}
                    </p>
                  ) : (
                    <pre
                      className="source-panel"
                      tabIndex={0}
                      aria-label={t("Shared file diff")}
                    >
                      {state.preview.diff.split("\n").map((line, i) => (
                        <span
                          key={i}
                          className={`block min-h-5 ${line.startsWith("+") && !line.startsWith("+++") ? "bg-green-50 text-green-900" : line.startsWith("-") && !line.startsWith("---") ? "bg-red-50 text-red-900" : ""}`}
                        >
                          {line || " "}
                        </span>
                      ))}
                    </pre>
                  )}
                </section>
              </ActionDialog>
            )}
          </div>
        </div>
      )}
    </section>
  );
}

function ResourceEditor({
  version,
  resource,
  onSaved,
}: {
  version: string;
  resource?: SharedResourceInfo;
  onSaved: (id: string) => void;
}) {
  const [state, update] = useReducer(
    (
      s: { open: boolean; version: string; busy: boolean; error: string },
      p: Partial<typeof s>,
    ) => ({ ...s, ...p }),
    { open: false, version, busy: false, error: "" },
  );
  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!native || state.busy) return;
    const data = new FormData(event.currentTarget);
    update({ busy: true, error: "" });
    try {
      await Service.saveResource(
        state.version,
        String(data.get("id")).trim(),
        String(data.get("path")).trim(),
        String(data.get("target")).trim(),
      );
      update({ open: false });
      onSaved(String(data.get("id")).trim());
    } catch (error) {
      update({ error: message(error) });
    } finally {
      update({ busy: false });
    }
  }
  return (
    <ActionDialog
      title={resource ? t("Edit shared source") : t("Register shared file")}
      trigger={resource ? t("Edit source") : t("Register shared file")}
      open={state.open}
      onOpenChange={(open) => update({ open })}
      onOpen={() => update({ error: "" })}
      busy={state.busy}
      error={state.error}
    >
      <form className="space-y-3" onSubmit={save}>
        <fieldset disabled={state.busy} className="space-y-3">
          <label className="block space-y-1 text-sm">
            {t("Resource ID")}
            <input
              className={inputClass}
              name="id"
              required
              maxLength={80}
              readOnly={!!resource}
              defaultValue={resource?.id}
            />
          </label>
          <label className="block space-y-1 text-sm">
            {t("Shared source file")}
            <input
              className={inputClass}
              name="path"
              required
              placeholder="/absolute/path/shared/http-health.sh"
              defaultValue={resource?.path}
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
            />
          </label>
          <label className="block space-y-1 text-sm">
            {t("Destination in project")}
            <input
              className={inputClass}
              name="target"
              required
              placeholder="tools/http-health.sh"
              defaultValue={resource?.target}
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
            />
          </label>
          <p className="text-xs leading-5 text-muted">
            {t(
              "The source is an independent UTF-8 text file. Registration does not write to projects. The destination parent folder is prepared by AI during setup.",
            )}
          </p>
          <Button type="submit" primary disabled={!native || state.busy}>
            {state.busy ? t("Saving…") : t("Save registration")}
          </Button>
        </fieldset>
      </form>
    </ActionDialog>
  );
}

function ResourceHistory({
  path,
  revision,
  busy,
  onRestore,
}: {
  path: string;
  revision: number;
  busy: boolean;
  onRestore: (record: UpdateHistory) => void;
}) {
  const [state, update] = useReducer(
    (
      s: { records: UpdateHistory[] | null; error: string },
      p: Partial<typeof s>,
    ) => ({ ...s, ...p }),
    { records: null, error: "" },
  );
  useEffect(() => {
    let active = true;
    const request = native
      ? Service.resourceHistory(path)
      : Promise.resolve([]);
    void request
      .then((records) => {
        if (active) update({ records, error: "" });
      })
      .catch((error) => {
        if (active) update({ error: message(error) });
      });
    return () => {
      active = false;
    };
  }, [path, revision]);
  if (state.error)
    return (
      <p role="alert" className="text-sm text-removed">
        {t(state.error)}
      </p>
    );
  if (!state.records)
    return <p role="status">{t("Loading update history…")}</p>;
  if (!state.records.length)
    return (
      <p className="text-sm text-muted">{t("No shared file updates yet.")}</p>
    );
  return (
    <div className="grouped-list">
      {state.records.map((record) => (
        <div
          key={record.id}
          className="space-y-2 border-b border-line-subtle p-3 last:border-0"
        >
          <p className="break-all text-sm font-medium">{record.target}</p>
          <p className="break-all text-xs text-muted">
            {new Date(record.createdAt).toLocaleString(getLocale())}{" "}
            {t("· Source")} {record.sourcePath} {t("· Version")}{" "}
            {record.version.slice(0, 12)}
          </p>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="text-xs text-muted">
              {t(
                (
                  {
                    applied: "Restorable",
                    restored: "Restored",
                    changed: "File changed since this update",
                    "not-applied": "Not applied",
                  } as Record<string, string>
                )[record.state] || record.state,
              )}
            </span>
            <Button
              disabled={!native || busy || record.state !== "applied"}
              onClick={() => onRestore(record)}
            >
              {t("Restore previous content")}
            </Button>
          </div>
        </div>
      ))}
    </div>
  );
}
