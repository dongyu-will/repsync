import { useState } from "react";
import type { Inspection } from "./mygo";
import ProjectDocuments from "./ProjectDocuments";
import SharedRules from "./SharedRules";
import ProjectPipelines from "./ProjectPipelines";
import ProjectCommands from "./ProjectCommands";
import ProjectRules from "./ProjectRules";
import ProjectGit, { type GitSection } from "./ProjectGit";
import ProjectReviewPanel from "./ProjectReview";
import AIResources from "./AIResources";

export type ProjectTab =
  "概览" | "工作树" | "CI/CD" | "规范" | "AI 资源" | "文档";

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
      className={tab === "文档" ? "flex h-full min-h-0 flex-col" : "contents"}
    >
      {details.warnings.map((warning, i) => (
        <p
          key={i}
          className="mb-4 shrink-0 break-words rounded-md bg-amber-50 p-3 text-sm leading-6 text-amber-900"
        >
          {warning}
        </p>
      ))}
      {tab === "工作树" ? (
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
      ) : tab === "规范" ? (
        <ProjectStandards
          key={details.project.path}
          details={details}
          onOpenShared={onOpenShared}
          onUpdated={onUpdated}
        />
      ) : tab === "AI 资源" ? (
        <AIResources key={details.project.path} path={details.project.path} />
      ) : tab === "文档" ? (
        <ProjectDocuments
          key={details.project.path}
          path={details.project.path}
          names={details.documents}
        />
      ) : (
        <>
          <ProjectReviewPanel
            key={details.project.path}
            path={details.project.path}
            onGit={onOpenStashes}
            onCI={onOpenCI}
            stashCount={details.stashKnown ? details.stashCount : 0}
            workspaceStatus={
              !details.git
                ? "Git 状态不可用"
                : !details.statusKnown
                  ? "工作区状态读取失败"
                  : details.status
                    ? "有未提交改动"
                    : "工作区干净"
            }
          />
          <section aria-label="本地服务">
            <ProjectCommands
              key={details.project.path}
              path={details.project.path}
              scripts={details.scripts}
            />
          </section>
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
        aria-label="规范分区"
        className="flex w-fit gap-1 rounded-lg bg-sidebar p-1"
      >
        {[
          ["project", "项目规则"],
          ["shared", "公共规范"],
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
    </div>
  );
}
