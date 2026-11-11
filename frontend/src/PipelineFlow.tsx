import { useEffect, useReducer, useRef } from "react";
import {
  Check,
  Circle,
  CircleHelp,
  LoaderCircle,
  Minus,
  Pause,
  X,
} from "lucide-react";
import type { Pipeline, PipelineRun } from "./mygo";
import { Button } from "./components";
import ActionDialog from "./ActionDialog";

const states = {
  pending: { label: "Not run", icon: Circle },
  success: { label: "Completed", icon: Check },
  running: { label: "Running", icon: LoaderCircle },
  failure: { label: "Failed", icon: X },
  cancelled: { label: "Stopped", icon: Pause },
  timed_out: { label: "Timed out", icon: Pause },
  interrupted: { label: "Interrupted", icon: CircleHelp },
  skipped: { label: "Not executed", icon: Minus },
  unknown: { label: "Needs review", icon: CircleHelp },
};
type StepState = keyof typeof states;

export default function PipelineFlow({
  pipeline,
  run,
  version,
  observed = true,
  onOpenLog,
}: {
  pipeline: Pipeline;
  run?: PipelineRun;
  version: string;
  observed?: boolean;
  onOpenLog: (runID: string) => void;
}) {
  const [selected, select] = useReducer(
    (_current: number | null, next: number | null) => next,
    null,
  );
  const steps = pipeline.steps || [];
  const flow = useRef<HTMLOListElement>(null);
  const known =
    observed &&
    !!run &&
    run.pipelineId === pipeline.id &&
    !!version &&
    run.configVersion === version &&
    [
      "success",
      "running",
      "failure",
      "cancelled",
      "timed_out",
      "interrupted",
    ].includes(run.status) &&
    Number.isInteger(run.step) &&
    run.step >= 0 &&
    run.step <= steps.length &&
    (run.status !== "success" || run.step === steps.length);
  const currentStep = known ? run.step : 0;
  useEffect(() => {
    const list = flow.current;
    const node = list?.children[currentStep - 1];
    if (!list || !node) return;
    const bounds = list.getBoundingClientRect();
    const target = node.getBoundingClientRect();
    if (target.left < bounds.left) list.scrollLeft += target.left - bounds.left;
    else if (target.right > bounds.right)
      list.scrollLeft += target.right - bounds.right;
  }, [currentStep, run?.id]);

  function stepState(index: number): StepState {
    if (!observed) return "unknown";
    if (!run) return "pending";
    if (!known) return "unknown";
    if (index + 1 < run.step || run.status === "success") return "success";
    if (index + 1 === run.step) return run.status as StepState;
    return run.status === "running" ? "pending" : "skipped";
  }

  return (
    <>
      <ol
        ref={flow}
        className="pipeline-flow"
        aria-label={`${pipeline.name}Execution steps`}
      >
        {steps.map((args, index) => {
          const state = stepState(index);
          const { label, icon: Symbol } = states[state];
          const command = args.join(" ");
          return (
            <li key={index} className="pipeline-stage" data-state={state}>
              {index > 0 && (
                <span className="pipeline-connector" aria-hidden="true" />
              )}
              <button
                type="button"
                className="pipeline-node"
                aria-label={`Step ${index + 1}: ${command}, ${label}`}
                aria-haspopup="dialog"
                aria-current={currentStep === index + 1 ? "step" : undefined}
                onClick={() => select(index)}
              >
                <span className="pipeline-step-icon" aria-hidden="true">
                  <Symbol size={16} strokeWidth={1.75} />
                </span>
                <span className="min-w-0">
                  <span
                    className="block truncate font-mono text-xs"
                    title={command}
                  >
                    {command}
                  </span>
                  <span className="mt-1 block text-xs text-muted">{label}</span>
                </span>
              </button>
            </li>
          );
        })}
      </ol>
      {observed && run && !known && (
        <p className="mt-2 text-xs text-muted">
          Step evidence is unavailable for this run. Check the logs; nodes show
          the current configuration.
        </p>
      )}
      {known && run.step === 0 && (
        <p className="mt-2 text-xs text-muted">
          {run.status === "running"
            ? "Preparing to run. No steps have started."
            : "The run ended before any steps executed. Check the logs."}
        </p>
      )}
      <ActionDialog
        title={selected === null ? "Execution steps" : `Step ${selected + 1}`}
        trigger={null}
        open={selected !== null}
        onOpenChange={(open) => {
          if (!open) select(null);
        }}
      >
        {selected !== null && (
          <>
            <p className="text-sm">{states[stepState(selected)].label}</p>
            <pre className="source-panel" tabIndex={0}>
              {(steps[selected] || [])
                .map((arg) => (/\s/.test(arg) ? JSON.stringify(arg) : arg))
                .join(" ")}
            </pre>
            <p className="text-xs text-muted">
              Working directory: {pipeline.cwd || "Project root"}
            </p>
            <p className="text-xs text-muted">
              Timeout: {Math.ceil(pipeline.timeoutSeconds / 60)} minutes
              {Object.keys(pipeline.env || {}).length
                ? ` · Environment variables: ${Object.keys(pipeline.env).join(", ")}`
                : ""}
            </p>
            {run && (
              <Button
                onClick={() => {
                  select(null);
                  onOpenLog(run.id);
                }}
              >
                View logs for this run
              </Button>
            )}
          </>
        )}
      </ActionDialog>
    </>
  );
}
