import { useEffect, useReducer } from "react";
import { isMyGo } from "mygo-runtime";
import { t } from "./i18n";
import {
  Service,
  type ProjectMap as MapData,
  type ProjectMapModule,
  type ProjectMapReference,
} from "./mygo";
import { Button, Empty, Icon } from "./components";
import ActionDialog from "./ActionDialog";

const native = isMyGo();

export default function ProjectMap({ path }: { path: string }) {
  const [state, update] = useReducer(
    (
      current: { data: MapData | null; error: string; revision: number },
      patch: Partial<typeof current>,
    ) => ({ ...current, ...patch }),
    { data: null, error: "", revision: 0 },
  );
  useEffect(() => {
    let active = true;
    const request = native
      ? Service.projectMap(path)
      : Promise.resolve(previewMap(path));
    void request
      .then((data) => {
        if (active) update({ data, error: "" });
      })
      .catch((error: unknown) => {
        if (active)
          update({
            data: null,
            error: error instanceof Error ? error.message : String(error),
          });
      });
    return () => {
      active = false;
    };
  }, [path, state.revision]);

  const { data, error } = state;
  const refresh = () =>
    update({ data: null, error: "", revision: state.revision + 1 });
  return (
    <div className="max-w-4xl space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-xs text-muted">
          {data && data.modules.length > 0
            ? t("{0} feature modules", data.modules.length)
            : t("Features, implementations, and entry points")}
        </p>
        <div className="flex items-center gap-2">
          <MapInstructions path={path} />
          <button
            className="icon-button text-muted"
            aria-label={t("Refresh project map")}
            title={t("Refresh project map")}
            onClick={refresh}
          >
            <Icon name="refresh" size={16} />
          </button>
        </div>
      </div>
      {error ? (
        <div
          role="alert"
          className="space-y-3 break-words text-sm text-removed"
        >
          <p>{t(error)}</p>
          <Button onClick={refresh}>{t("Retry")}</Button>
        </div>
      ) : !data ? (
        <p role="status" className="py-8 text-sm text-muted">
          {t("Loading project map…")}
        </p>
      ) : data.modules.length === 0 ? (
        <Empty icon="layers" title={t("No project map yet")}>
          {t(
            "Ask AI to organize this project by feature, then return here and refresh.",
          )}
        </Empty>
      ) : (
        <div className="grouped-list">
          {data.modules.map((module) => (
            <ModuleRow key={module.id} module={module} />
          ))}
        </div>
      )}
    </div>
  );
}

function ModuleRow({ module }: { module: ProjectMapModule }) {
  const changed = module.implementations.some(
    ({ source, entry }) => !!source.issue || !!entry?.issue,
  );
  const missing = module.implementations.some(({ entry }) => !entry);
  const label = changed
    ? "References need updating"
    : module.overlap
      ? "Possible duplicate"
      : missing
        ? "Entry needs review"
        : "";
  return (
    <ActionDialog
      title={module.name}
      className="border-b border-line-subtle last:border-0"
      triggerClassName="flex min-h-14 w-full items-center gap-3 px-3 py-3 text-left hover:bg-sidebar"
      trigger={
        <>
          <span className="min-w-0 flex-1">
            <strong className="block break-words text-sm font-medium">
              {module.name}
            </strong>
            {module.summary && (
              <span className="mt-1 block break-words text-xs leading-5 text-muted">
                {module.summary}
              </span>
            )}
          </span>
          <span className="flex shrink-0 flex-col items-end gap-1 text-xs text-muted">
            {label && (
              <span
                className={changed || module.overlap ? "text-amber-800" : ""}
              >
                {t(label)}
              </span>
            )}
            <span>
              {module.implementations.length === 1
                ? t("1 implementation")
                : t("{0} implementations", module.implementations.length)}
            </span>
          </span>
        </>
      }
    >
      {module.summary && (
        <p className="text-sm leading-6 text-muted">{module.summary}</p>
      )}
      {module.overlap && (
        <p
          role="status"
          className="rounded-md bg-amber-50 p-3 text-sm leading-6 text-amber-900"
        >
          <strong className="font-medium">
            {t("Possible duplicate")}
            {": "}
          </strong>
          {module.overlap}
        </p>
      )}
      <ul className="grouped-list">
        {module.implementations.map((implementation) => (
          <li
            key={`${implementation.source.path}:${implementation.source.find}`}
            className="space-y-3 border-b border-line-subtle p-3 last:border-0"
          >
            <div className="flex flex-wrap items-baseline justify-between gap-2">
              <h3 className="break-words text-sm font-medium">
                {implementation.name}
              </h3>
              <span className="text-xs text-muted">
                {t(
                  implementation.source.issue
                    ? "Implementation needs review"
                    : implementation.entry && !implementation.entry.issue
                      ? "Entry located"
                      : "Entry needs review",
                )}
              </span>
            </div>
            <dl className="space-y-2 text-xs leading-5">
              <Reference
                label={t("Implementation")}
                reference={implementation.source}
              />
              <Reference
                label={t("Entry point")}
                reference={implementation.entry}
              />
            </dl>
          </li>
        ))}
      </ul>
      <p className="text-xs leading-5 text-muted">
        {t("Locating an entry does not verify the complete user flow.")}
      </p>
    </ActionDialog>
  );
}

function Reference({
  label,
  reference,
}: {
  label: string;
  reference?: ProjectMapReference | null;
}) {
  return (
    <div className="grid grid-cols-[80px_minmax(0,1fr)] gap-3">
      <dt className="text-muted">{label}</dt>
      <dd className="min-w-0">
        {reference ? (
          <>
            <span className="break-all">
              {reference.path}
              {reference.line ? `:${reference.line}` : ""}
            </span>
            {reference.issue && (
              <p className="mt-1 text-removed">{t(reference.issue)}</p>
            )}
          </>
        ) : (
          <span className="text-muted">{t("No entry recorded")}</span>
        )}
      </dd>
    </div>
  );
}

function MapInstructions({ path }: { path: string }) {
  const [notice, setNotice] = useReducer((_: string, next: string) => next, "");
  const prompt = [
    t(
      "Read this project's guides and organize its map by user-facing features. Save .repsync-map.json in the project root using the format below.",
    ),
    path,
    t(
      "Name modules by what users can do. Inspect actual implementations, navigation, routes, and mounts. Include implementations without a located entry; do not invent a route or infer reachability from an import alone.",
    ),
    t(
      "Keep alternative implementations of the same feature together. Use overlap only for a specific, evidence-based duplicate concern; multiple pages alone do not prove duplication. Keep intentional variations separate and explain their purpose.",
    ),
    t(
      "Each source and entry uses a project-relative path and a short, unique, single-line find string. Omit entry when it has not been located. The app resolves current line numbers. Cover up to 64 modules, 8 implementations per module, and 256 references; disclose omissions in the handoff.",
    ),
    JSON.stringify(
      {
        version: 1,
        modules: [
          {
            id: "feature-id",
            name: t("Feature name"),
            summary: t("What the user can do"),
            overlap: "",
            implementations: [
              {
                name: t("Page or module name"),
                source: {
                  path: "src/pages/Feature.tsx",
                  find: "export default function Feature",
                },
                entry: { path: "src/App.tsx", find: "<Feature />" },
              },
            ],
          },
        ],
      },
      null,
      2,
    ),
    t(
      "Read and preserve the existing map and local edits. Update only the map, then run repsync project-map with the project path to check its references. Do not change business code, routes, services, or databases while preparing the map.",
    ),
  ].join("\n\n");
  return (
    <ActionDialog
      title={t("Organize project map")}
      trigger={t("Ask AI to organize")}
    >
      <p className="text-sm leading-6 text-muted">
        {t(
          "Copy these instructions to your existing AI chat. Return here and refresh after it finishes.",
        )}
      </p>
      <Button
        onClick={async () => {
          try {
            await navigator.clipboard.writeText(prompt);
            setNotice("AI instructions copied");
          } catch {
            setNotice(
              "Could not access the clipboard. Select and copy the text below.",
            );
          }
        }}
      >
        {t("Copy AI instructions")}
      </Button>
      {notice && (
        <p role="status" className="text-xs text-muted">
          {t(notice)}
        </p>
      )}
      <pre
        tabIndex={0}
        className="source-panel max-h-80 whitespace-pre-wrap break-words [overflow-wrap:anywhere]"
      >
        {prompt}
      </pre>
    </ActionDialog>
  );
}

function previewMap(path: string): MapData {
  if (path !== "/Projects/atlas")
    return { file: ".repsync-map.json", exists: false, modules: [] };
  return {
    file: ".repsync-map.json",
    exists: true,
    modules: [
      {
        id: "projects",
        name: "项目接入",
        summary: "添加本地项目或克隆仓库",
        overlap: "",
        implementations: [
          {
            name: "添加项目",
            source: {
              path: "src/AddProject.tsx",
              find: "function AddProject",
              line: 18,
              issue: "",
            },
            entry: {
              path: "src/App.tsx",
              find: "<AddProject",
              line: 95,
              issue: "",
            },
          },
        ],
      },
      {
        id: "settings",
        name: "项目设置",
        summary: "调整项目的常用选项",
        overlap:
          "新旧设置页都提供项目名称和偏好设置，旧版页面未记录入口，需要核对是否仍有用途。",
        implementations: [
          {
            name: "设置页",
            source: {
              path: "src/pages/Settings.tsx",
              find: "function Settings",
              line: 12,
              issue: "",
            },
            entry: {
              path: "src/App.tsx",
              find: "<Settings",
              line: 128,
              issue: "",
            },
          },
          {
            name: "旧设置页",
            source: {
              path: "src/legacy/ProjectSettings.tsx",
              find: "function ProjectSettings",
              line: 9,
              issue: "",
            },
          },
        ],
      },
      {
        id: "documents",
        name: "项目文档",
        summary: "阅读和编辑项目说明",
        overlap: "",
        implementations: [
          {
            name: "文档页",
            source: {
              path: "src/pages/Documents.tsx",
              find: "function Documents",
              line: 26,
              issue: "",
            },
            entry: {
              path: "src/App.tsx",
              find: "<Documents",
              line: 0,
              issue: "Reference text no longer found",
            },
          },
        ],
      },
    ],
  };
}
