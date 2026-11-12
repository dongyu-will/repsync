import { useCallback, useEffect, useId, useReducer, useRef } from "react";
import { t } from "./i18n";
import ActionDialog from "./ActionDialog";
import { Button } from "./components";

export type SetupTask =
  | "services"
  | "pipelines"
  | "checks"
  | "resources"
  | "shared-files"
  | "shared-source"
  | "review"
  | "cleanup";

const goals: Record<SetupTask, string> = {
  services:
    "Connect or adjust the complete local project runtime in .repsync.json using its existing start commands: frontend, backend, database, cache, and other required components. Declare shared dependencies with observeOnly and a local port, without a command. Use envFrom for actual endpoint dependencies so managed components wait for their prerequisites. Preserve cwd, environment references, ports, and unrelated configuration. Read repsync doctor PATH and status PATH to check setup and existing instances.",
  pipelines:
    "Read repsync pipelines PATH and workflows PATH first. Reuse detected project-owned check, build, and delivery entries and repository workflows. Repair discovery gaps through existing task declarations; use .repsync.json only for necessary explicit overrides. Preserve environment, validation, release gates, and unrelated configuration. Do not invent a standard pipeline or inferred internal steps. Reading or configuring pipelines does not authorize running a release or publishing.",
  checks:
    "Read repsync checks PATH, project AGENTS.md, developer guides, existing tool configurations, and task-runner commands. Reuse project-owned ESLint, type checks, lint, and test entry points; keep one authoritative source per rule. Connect missing existing commands through documented project scripts or CI pipelines without replacing project gates. Explain written requirements whose automatic coverage is unknown. Do not create repsync policies or invent universal code rules. Configuration does not authorize executing checks or changing source rules.",
  resources:
    "Read repsync ai-resources [PATH]. For a registered project also read codex-resources PATH. Assess the requested Skill or MCP against project rules and existing resources, using exact source identities. Maintain aiResources declarations in .repsync.json. For configuration changes use codex-resource-preview PATH JSON_FILE and codex-resource-apply PATH VERSION JSON_FILE, preserving the returned version protection. Use an empty quoted PATH for global MCP defaults. Ask only for missing source details; refer to credentials by environment variable name, never include secret values.",
  "shared-files":
    "Read repsync resources. For a registered project also read resource-status PATH. Reuse existing registered shared files and maintain this project's resources declarations in .repsync.json. Use resource-add only when a new shared source is requested. Keep project exceptions and existing files; use resource-preview and sync-apply for protected updates, and resource-history for recovery. Do not register or write to unrelated projects.",
  "shared-source":
    "Read repsync sync-source and sync-status. Connect the requested existing shared rules file through sync-source FILE. Inspect the source and affected projects before proposing updates. Preserve project-specific content and use sync-preview, sync-apply, and sync-history for protected updates and recovery. If the intended source cannot be determined from the chat and existing configuration, ask for that source only.",
  review:
    "Read repsync review PATH and the project's current guides. Maintain reviewProfile and reviewResources in .repsync.json, or use review-profile and review-resources with the current configVersion. Declare only actual project requirements, data, environments, and verification methods. Keep existing records, never expose credential values, and do not treat a declaration as completed verification.",
  cleanup:
    "Read repsync git-workspace PATH. Configure automatic cleanup through git-policy PATH POLICY_VERSION JSON_FILE using the current policyVersion. Preserve the existing base branch and retention unless the user requests a change. Enabling automatic cleanup requires an explicit request in this chat. Changing policy does not authorize immediate cleanup; keep worktree, stash, active-process, and recovery protections.",
};

export default function AISetup({
  path = "",
  task,
  context,
}: {
  path?: string;
  task: SetupTask;
  context?: unknown;
}) {
  const textId = useId();
  const text = useRef<HTMLPreElement>(null);
  const [notice, update] = useReducer(
    (
      previous: { message: string; repeat: boolean; last: string },
      message: string,
    ) => ({
      message,
      repeat: message
        ? message === previous.last && !previous.repeat
        : previous.repeat,
      last: message || previous.last,
    }),
    { message: "", repeat: false, last: "" },
  );
  const prompt = [
    t(
      "Configure this using the goals already stated in our chat. Read the applicable AGENTS.md, README, and current configuration first. Work out technical details from existing files; ask only for missing decisions that affect the result.",
    ),
    JSON.stringify({ scope: path || "global", context }, null, 2),
    t(goals[task]),
    t(
      "Use the existing repsync CLI documented in its README or app bundle. Reread files before writing, preserve unrelated edits and file permissions, and keep one authoritative configuration source. Do not start services, deploy, clean up files, or modify business data just to configure this.",
    ),
    t(
      "Finish the authorized configuration and report the actual changes, checks, and remaining unknowns. Return to repsync and refresh to inspect the result. Copying these instructions does not execute the task.",
    ),
  ].join("\n\n");
  const copy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(prompt);
      update("AI instructions copied");
    } catch {
      update("Could not access the clipboard. Select and copy the text below.");
    }
  }, [prompt]);
  useEffect(() => {
    const element = text.current;
    const handle = () => void copy();
    element?.addEventListener("command", handle);
    return () => element?.removeEventListener("command", handle);
  }, [copy]);
  useEffect(() => {
    if (notice.message !== "AI instructions copied") return;
    const timer = window.setTimeout(() => update(""), 2500);
    return () => window.clearTimeout(timer);
  }, [notice]);
  return (
    <ActionDialog title={t("Ask AI to configure")} onOpen={() => update("")}>
      <p className="text-sm leading-6 text-muted">
        {t(
          "Copy these instructions to your existing AI chat. Return here and refresh after it finishes.",
        )}
      </p>
      <Button
        type="button"
        {...{ command: "--copy", commandfor: textId }}
        onClick={() => {
          if (!("CommandEvent" in window)) void copy();
        }}
      >
        {t("Copy AI instructions")}
      </Button>
      <p role="status" className="text-xs text-muted">
        {t(notice.message)}
        {notice.repeat ? "\u00a0" : ""}
      </p>
      <pre
        ref={text}
        id={textId}
        tabIndex={0}
        className="source-panel max-h-80 whitespace-pre-wrap break-words"
      >
        {prompt}
      </pre>
    </ActionDialog>
  );
}
