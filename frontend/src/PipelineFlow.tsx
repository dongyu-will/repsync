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
  pending: { label: "未运行", icon: Circle },
  success: { label: "已完成", icon: Check },
  running: { label: "运行中", icon: LoaderCircle },
  failure: { label: "未通过", icon: X },
  cancelled: { label: "已停止", icon: Pause },
  timed_out: { label: "超时", icon: Pause },
  interrupted: { label: "已中断", icon: CircleHelp },
  skipped: { label: "未执行", icon: Minus },
  unknown: { label: "待核对", icon: CircleHelp },
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
        aria-label={`${pipeline.name}执行步骤`}
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
                aria-label={`步骤 ${index + 1}：${command}，${label}`}
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
          这次运行缺少可匹配的步骤依据，请在日志中核对；节点显示当前配置。
        </p>
      )}
      {known && run.step === 0 && (
        <p className="mt-2 text-xs text-muted">
          {run.status === "running"
            ? "正在准备运行，尚未开始执行步骤。"
            : "运行在执行步骤前结束，请查看日志。"}
        </p>
      )}
      <ActionDialog
        title={selected === null ? "执行步骤" : `步骤 ${selected + 1}`}
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
              工作目录：{pipeline.cwd || "项目根目录"}
            </p>
            <p className="text-xs text-muted">
              超时：{Math.ceil(pipeline.timeoutSeconds / 60)} 分钟
              {Object.keys(pipeline.env || {}).length
                ? ` · 环境变量：${Object.keys(pipeline.env).join("、")}`
                : ""}
            </p>
            {run && (
              <Button
                onClick={() => {
                  select(null);
                  onOpenLog(run.id);
                }}
              >
                查看这次运行日志
              </Button>
            )}
          </>
        )}
      </ActionDialog>
    </>
  );
}
