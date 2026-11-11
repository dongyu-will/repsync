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
  current: "已是最新",
  outdated: "可更新",
  missing: "待写入",
  modified: "项目有改动",
  error: "需处理",
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
      notice: "已保存公共来源，项目文件尚未更新",
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
    <section aria-label="共享文件资源" className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="min-w-0 text-sm text-muted">
          测试脚本与检查配置共用公共来源，项目按需采用。
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
            aria-label="重新读取共享文件"
            disabled={!!state.busy}
            onClick={refresh}
          >
            <Icon name="refresh" />
          </button>
        </div>
      </div>
      {(state.error || state.loadError) && (
        <p role="alert" className="break-words text-sm text-removed">
          {state.error || state.loadError}
        </p>
      )}
      {state.notice && (
        <p role="status" className="text-sm text-muted">
          {state.notice}
        </p>
      )}
      {state.busy && (
        <p role="status" className="text-sm text-muted">
          {state.busy === "preview" ? "正在生成差异…" : "正在保存…"}
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
              {item.project.name}：{item.error}
            </p>
          ))}
      {!report ? (
        <p role="status" className="text-sm text-muted">
          {state.loadError
            ? "共享资源读取失败，请重新读取。"
            : "正在读取共享资源…"}
        </p>
      ) : !selected ? (
        <Empty title="尚未登记共享文件" icon="layers">
          将可复用的测试脚本或检查配置登记为公共来源。
          {project && "可在全局公共规范中登记。"}
        </Empty>
      ) : (
        <div className="grid min-w-0 gap-5 min-[900px]:grid-cols-[176px_minmax(0,1fr)]">
          <label className="grid gap-1 text-xs text-muted min-[900px]:hidden">
            共享文件
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
            aria-label="共享文件列表"
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
                      ? "来源需处理"
                      : report.projects.some((item) => item.error)
                        ? `${count} 个已知采用 · 部分状态未知`
                        : project
                          ? count
                            ? "本项目已采用"
                            : "本项目未采用"
                          : `${count} 个项目采用`}
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
                  来源版本{" "}
                  {selected.version ? selected.version.slice(0, 12) : "未知"}
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
                    title="移除共享资源登记"
                    trigger="移除登记"
                    busy={!!state.busy}
                    error={state.error}
                    notice={state.notice}
                  >
                    <p className="text-sm leading-6">
                      移除 {selected.id}{" "}
                      的公共登记。已采用的项目文件和更新记录会保留，项目需重新登记来源后才能继续更新。
                    </p>
                    <Button
                      disabled={!native || !!state.busy || !selected.path}
                      onClick={() =>
                        void perform(
                          () =>
                            Service.forgetResource(report.version, selected.id),
                          "已移除公共登记；项目文件和历史保留",
                        )
                      }
                    >
                      移除登记
                    </Button>
                  </ActionDialog>
                </div>
              )}
            </div>
            <dl className="space-y-2 text-xs leading-5">
              <div className="grid grid-cols-[64px_minmax(0,1fr)] gap-2">
                <dt className="text-muted">公共来源</dt>
                <dd className="break-all">{selected.path || "未登记"}</dd>
              </div>
              <div className="grid grid-cols-[64px_minmax(0,1fr)] gap-2">
                <dt className="text-muted">项目位置</dt>
                <dd className="break-all">{selected.target || "未知"}</dd>
              </div>
            </dl>
            {selected.error && (
              <p role="alert" className="break-words text-sm text-removed">
                {selected.error}
              </p>
            )}
            {report.projects.length ? (
              <div className="grouped-list" aria-label="项目采用情况">
                {report.projects.map((entry) => {
                  const adopted = entry.adoptions.find(
                    (item) => item.id === selected.id,
                  );
                  const status = entry.error
                    ? "读取失败"
                    : adopted
                      ? statuses[adopted.status] || adopted.status
                      : "未采用";
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
                          {status}
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
                          采用版本 {adopted.adoptedVersion.slice(0, 12)}
                        </p>
                      )}
                      {(entry.error || adopted?.error) && (
                        <p className="break-words text-xs text-removed">
                          {entry.error || adopted?.error}
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
                                ? "已取消采用；项目文件和历史保留"
                                : "已登记采用，预览差异后再写入项目文件",
                            )
                          }
                        >
                          {adopted ? "取消采用" : "采用此资源"}
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
                            预览差异
                          </Button>
                        )}
                        <ActionDialog
                          title={`${entry.project.name} · 共享文件记录`}
                          trigger="更新记录"
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
                                  `已恢复 ${record.project.name} 的 ${record.target}`,
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
              <p className="text-sm text-muted">尚无已登记项目。</p>
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
                <section aria-label="共享文件更新预览" className="space-y-3">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <p
                      className="text-xs text-muted"
                      title={state.preview.version}
                    >
                      来源版本 {state.preview.version.slice(0, 12)}
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
                            `已更新 ${plan.project.name} 的 ${plan.target}，备份已保存`,
                          );
                      }}
                    >
                      应用更新
                    </Button>
                  </div>
                  {state.preview.conflict && (
                    <p role="alert" className="text-sm text-removed">
                      项目文件有本地改动，无法覆盖。请保留项目差异，或由 AI
                      接入独立的公共文件。
                    </p>
                  )}
                  {!state.preview.changed ? (
                    <p role="status" className="text-sm text-muted">
                      文件已与公共来源一致。
                    </p>
                  ) : (
                    <pre
                      className="source-panel"
                      tabIndex={0}
                      aria-label="共享文件更新差异"
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
      title={resource ? "编辑共享来源" : "登记共享文件"}
      trigger={resource ? "编辑来源" : "登记共享文件"}
      open={state.open}
      onOpenChange={(open) => update({ open })}
      onOpen={() => update({ error: "" })}
      busy={state.busy}
      error={state.error}
    >
      <form className="space-y-3" onSubmit={save}>
        <fieldset disabled={state.busy} className="space-y-3">
          <label className="block space-y-1 text-sm">
            资源标识
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
            公共来源文件
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
            项目内目标位置
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
            来源为独立的 UTF-8 文本文件。登记不写入项目；目标父目录由 AI
            接入时准备。
          </p>
          <Button type="submit" primary disabled={!native || state.busy}>
            {state.busy ? "正在保存…" : "保存登记"}
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
        {state.error}
      </p>
    );
  if (!state.records) return <p role="status">正在读取更新记录…</p>;
  if (!state.records.length)
    return <p className="text-sm text-muted">尚无共享文件更新记录。</p>;
  return (
    <div className="grouped-list">
      {state.records.map((record) => (
        <div
          key={record.id}
          className="space-y-2 border-b border-line-subtle p-3 last:border-0"
        >
          <p className="break-all text-sm font-medium">{record.target}</p>
          <p className="break-all text-xs text-muted">
            {new Date(record.createdAt).toLocaleString()} · 来源{" "}
            {record.sourcePath} · 版本 {record.version.slice(0, 12)}
          </p>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="text-xs text-muted">
              {(
                {
                  applied: "可恢复",
                  restored: "已恢复",
                  changed: "文件已有后续改动",
                  "not-applied": "未应用",
                } as Record<string, string>
              )[record.state] || record.state}
            </span>
            <Button
              disabled={!native || busy || record.state !== "applied"}
              onClick={() => onRestore(record)}
            >
              恢复更新前内容
            </Button>
          </div>
        </div>
      ))}
    </div>
  );
}
