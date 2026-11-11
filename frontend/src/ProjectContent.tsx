import { useState } from "react";
import type { Inspection } from "./mygo";
import ProjectDocuments from "./ProjectDocuments";
import SharedRules from "./SharedRules";
import SharedFiles from "./SharedFiles";
import ProjectPipelines from "./ProjectPipelines";
import ProjectCommands from "./ProjectCommands";
import ProjectRules from "./ProjectRules";
import ProjectGit, { type GitSection } from "./ProjectGit";
import ProjectReviewPanel from "./ProjectReview";
import AIResources from "./AIResources";

import type { ProjectTab } from "./project-tabs";
export type { ProjectTab } from "./project-tabs";

export default function ProjectContent({
  details,
  tab,
  onOpenShared,
  onUpdated,
  onOpenStashes,
  gitSection,
  onOpenCI,
}: {
  details: Inspection;
  tab: ProjectTab;
  onOpenShared: () => void;
  onUpdated: () => void;
  onOpenStashes: () => void;
  gitSection: GitSection;
  onOpenCI: () => void;
}) {
  return (
    <div
      className={
        tab === "Documents" ? "flex h-full min-h-0 flex-col" : "contents"
      }
    >
      {details.warnings.map((warning, i) => (
        <p
          key={i}
          className="mb-4 shrink-0 break-words rounded-md bg-amber-50 p-3 text-sm leading-6 text-amber-900"
        >
          {warning}
        </p>
      ))}
      {tab === "Worktrees" ? (
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
        <ProjectStandards
          key={details.project.path}
          details={details}
          onOpenShared={onOpenShared}
          onUpdated={onUpdated}
        />
      ) : tab === "AI resources" ? (
        <AIResources key={details.project.path} path={details.project.path} />
      ) : tab === "Documents" ? (
        <ProjectDocuments
          key={details.project.path}
          path={details.project.path}
          names={details.documents}
        />
      ) : (
        <>
          <section aria-label="Local services">
            <ProjectCommands
              key={details.project.path}
              path={details.project.path}
              scripts={details.scripts}
            />
          </section>
          <ProjectReviewPanel
            key={details.project.path}
            path={details.project.path}
            onGit={onOpenStashes}
            onCI={onOpenCI}
            stashCount={details.stashKnown ? details.stashCount : 0}
            workspaceStatus={
              !details.git
                ? "Git status unavailable"
                : !details.statusKnown
                  ? "Could not read workspace status"
                  : details.status
                    ? "Uncommitted changes"
                    : "Clean workspace"
            }
          />
        </>
      )}
    </div>
  );
}

function ProjectStandards({
  details,
  onOpenShared,
  onUpdated,
}: {
  details: Inspection;
  onOpenShared: () => void;
  onUpdated: () => void;
}) {
  const [section, setSection] = useState("project");
  return (
    <div className="space-y-5">
      <div
        role="group"
        aria-label="Rules sections"
        className="flex w-fit gap-1 rounded-lg bg-sidebar p-1"
      >
        {[
          ["project", "Project rules"],
          ["shared", "Shared rules"],
          ["files", "Shared files"],
        ].map(([id, title]) => (
          <button
            key={id}
            type="button"
            aria-pressed={section === id}
            aria-controls={`standards-${id}`}
            onClick={() => setSection(id)}
            className={`min-h-8 rounded-md px-3 text-sm ${section === id ? "bg-white font-medium text-ink" : "text-muted hover:text-ink"}`}
          >
            {title}
          </button>
        ))}
      </div>
      <div id="standards-project" hidden={section !== "project"}>
        <ProjectRules path={details.project.path} onUpdated={onUpdated} />
      </div>
      <div id="standards-shared" hidden={section !== "shared"}>
        <SharedRules
          projects={[details.project]}
          project={details.project}
          showProjectRules={false}
          onOpenShared={onOpenShared}
          onUpdated={onUpdated}
        />
      </div>
      <div id="standards-files" hidden={section !== "files"}>
        <SharedFiles
          projects={[details.project]}
          project={details.project}
          onUpdated={onUpdated}
          active={section === "files"}
        />
      </div>
    </div>
  );
}
