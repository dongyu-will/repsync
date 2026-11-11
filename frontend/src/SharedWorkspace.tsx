import { useReducer } from "react";
import type { Project } from "./mygo";
import SharedRules from "./SharedRules";
import SharedFiles from "./SharedFiles";

export default function SharedWorkspace({ projects }: { projects: Project[] }) {
  const [section, select] = useReducer(
    (_state: string, next: string) => next,
    "rules",
  );
  return (
    <div className="space-y-5">
      <div
        role="group"
        aria-label="公共资源分区"
        className="flex w-fit gap-1 rounded-lg bg-sidebar p-1"
      >
        {[
          ["rules", "公共规则"],
          ["files", "共享文件"],
        ].map(([id, label]) => (
          <button
            key={id}
            type="button"
            aria-pressed={section === id}
            aria-controls={`shared-${id}`}
            onClick={() => select(id)}
            className={`min-h-8 rounded-md px-3 text-sm ${section === id ? "bg-white font-medium text-ink" : "text-muted hover:text-ink"}`}
          >
            {label}
          </button>
        ))}
      </div>
      <div id="shared-rules" hidden={section !== "rules"}>
        <SharedRules projects={projects} />
      </div>
      <div id="shared-files" hidden={section !== "files"}>
        <SharedFiles projects={projects} active={section === "files"} />
      </div>
    </div>
  );
}
