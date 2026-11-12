import { t } from "./i18n";
import { lazy, Suspense, useState } from "react";
import type { Inspection } from "./mygo";
import ProjectCommands from "./ProjectCommands";
import ProjectChecks from "./ProjectChecks";
import type { GitSection } from "./ProjectGit";

const ProjectDocuments = lazy(() => import("./ProjectDocuments"));
const ProjectMap = lazy(() => import("./ProjectMap"));
const ProjectPipelines = lazy(() => import("./ProjectPipelines"));
const ProjectGit = lazy(() => import("./ProjectGit"));
const ProjectReviewPanel = lazy(() => import("./ProjectReview"));
const AIResources = lazy(() => import("./AIResources"));

import type { ProjectTab } from "./project-tabs";
export type { ProjectTab } from "./project-tabs";

export default function ProjectContent({
  details,
  tab,
  onUpdated,
  onOpenStashes,
  gitSection,
  onOpenCI,
}: {
  details: Inspection;
  tab: ProjectTab;
  onUpdated: () => void;
  onOpenStashes: () => void;
  gitSection: GitSection;
  onOpenCI: () => void;
}) {
  return (
    <Suspense fallback={<p role="status">{t("Loading…")}</p>}>
      <div
        className={
          tab === "Rules" ? "flex h-full min-h-0 flex-col" : "contents"
        }
      >
        {details.warnings.map((warning, i) => (
          <p
            key={i}
            className="mb-4 shrink-0 break-words rounded-md bg-amber-50 p-3 text-sm leading-6 text-amber-900"
          >
            {t(warning)}
          </p>
        ))}
        {tab === "Project map" ? (
          <ProjectMap key={details.project.path} path={details.project.path} />
        ) : tab === "Worktrees" ? (
          <ProjectGit
            key={details.project.path}
            path={details.project.path}
            initialSection={gitSection}
            onUpdated={onUpdated}
          />
        ) : tab === "CI/CD" ? (
          <ProjectPipelines
            key={details.project.path}
            path={details.project.path}
          />
        ) : tab === "Rules" ? (
          <ProjectStandards key={details.project.path} details={details} />
        ) : tab === "AI resources" ? (
          <AIResources key={details.project.path} path={details.project.path} />
        ) : tab === "Review" ? (
          <ProjectReviewPanel
            key={details.project.path}
            path={details.project.path}
            onGit={onOpenStashes}
            onCI={onOpenCI}
            stashCount={details.stashKnown ? details.stashCount : 0}
            workspaceStatus={
              !details.git
                ? t("Git status unavailable")
                : !details.statusKnown
                  ? t("Could not read workspace status")
                  : details.status
                    ? t("Uncommitted changes")
                    : t("Clean workspace")
            }
          />
        ) : (
          <ProjectCommands
            key={details.project.path}
            path={details.project.path}
          />
        )}
      </div>
    </Suspense>
  );
}

function ProjectStandards({ details }: { details: Inspection }) {
  const [section, setSection] = useState("current");
  const [visited, setVisited] = useState(["current"]);
  const guidelineOrder = [
    "agents",
    "claude",
    "gemini",
    "contributing",
    "rules",
    "styleguide",
    "development",
    "design",
    "code_of_conduct",
  ];
  const documentNames = [...details.documents].sort((a, b) => {
    const aRank = guidelineOrder.indexOf(
      a.replace(/\.(md|markdown)$/i, "").toLowerCase(),
    );
    const bRank = guidelineOrder.indexOf(
      b.replace(/\.(md|markdown)$/i, "").toLowerCase(),
    );
    return (
      (aRank < 0 ? guidelineOrder.length : aRank) -
        (bRank < 0 ? guidelineOrder.length : bRank) || a.localeCompare(b)
    );
  });
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4">
      <div
        role="group"
        aria-label={t("Rules sections")}
        className="flex w-fit max-w-full shrink-0 flex-wrap gap-1 rounded-lg bg-sidebar p-1"
      >
        {[
          ["current", "Guidelines and documents"],
          ["project", "Project checks"],
        ].map(([id, title]) => (
          <button
            key={id}
            type="button"
            aria-pressed={section === id}
            aria-controls={`standards-${id}`}
            onClick={() => {
              setSection(id);
              setVisited((current) =>
                current.includes(id) ? current : [...current, id],
              );
            }}
            className={`min-h-8 rounded-md px-3 text-sm ${section === id ? "bg-white font-medium text-ink" : "text-muted hover:text-ink"}`}
          >
            {t(title)}
          </button>
        ))}
      </div>
      <div
        id="standards-current"
        hidden={section !== "current"}
        className={
          section === "current" ? "flex min-h-0 flex-1 flex-col gap-3" : ""
        }
      >
        {visited.includes("current") && (
          <>
            <p className="shrink-0 text-xs text-muted">
              {t("Project-root files only.")}
            </p>
            <ProjectDocuments
              path={details.project.path}
              names={documentNames}
            />
          </>
        )}
      </div>
      <div id="standards-project" hidden={section !== "project"}>
        {visited.includes("project") && (
          <ProjectChecks
            key={details.project.path}
            path={details.project.path}
          />
        )}
      </div>
    </div>
  );
}
