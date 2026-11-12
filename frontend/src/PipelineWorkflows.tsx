import { useEffect, useReducer } from "react";
import { isMyGo } from "mygo-runtime";
import { t } from "./i18n";
import { Service, type WorkflowOverview } from "./mygo";

const native = isMyGo();
const preview: WorkflowOverview = {
  workflows: [
    {
      content:
        "name: CI\non: [push, pull_request]\njobs:\n  quality:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n      - run: npm run check\n",
      path: ".github/workflows/ci.yml",
      name: "CI",
      triggers: "push:\n  branches: [main]\npull_request:",
      jobs: [
        {
          id: "quality",
          name: "Quality",
          needs: [],
          if: "",
          uses: "",
          runsOn: "ubuntu-latest",
          strategy: "",
          steps: [
            {
              name: "Checkout",
              uses: "actions/checkout@v4",
              run: "",
              if: "",
              workingDirectory: "",
            },
            {
              name: "Project checks",
              run: "npm run check",
              uses: "",
              if: "",
              workingDirectory: "",
            },
          ],
        },
      ],
    },
  ],
  warnings: [],
  version: "preview",
};

export default function PipelineWorkflows({
  path,
  revision,
  filter = "",
}: {
  path: string;
  revision: number;
  filter?: string;
}) {
  const [state, update] = useReducer(
    (
      state: { data: WorkflowOverview | null; error: string },
      patch: Partial<{ data: WorkflowOverview | null; error: string }>,
    ) => ({ ...state, ...patch }),
    { data: native ? null : preview, error: "" },
  );
  useEffect(() => {
    if (!native) return;
    let active = true;
    update({ data: null, error: "" });
    Service.pipelineWorkflows(path)
      .then((data) => {
        if (active) update({ data });
      })
      .catch((error) => {
        if (active) update({ error: String(error) });
      });
    return () => {
      active = false;
    };
  }, [path, revision]);
  const visible =
    state.data?.workflows.filter((workflow) =>
      `GitHub Actions ${workflow.name} ${workflow.path}`
        .toLocaleLowerCase()
        .includes(filter.trim().toLocaleLowerCase()),
    ) || [];
  return (
    <section className="space-y-3" aria-label={t("Repository workflows")}>
      <h3 className="text-sm font-semibold">{t("Repository workflows")}</h3>
      <p className="text-xs leading-5 text-muted">
        {t(
          "Definitions come from this worktree. GitHub run results are listed separately; expressions and matrices remain unevaluated.",
        )}
      </p>
      {state.error ? (
        <p role="alert" className="break-words text-sm text-removed">
          {state.error}
        </p>
      ) : !state.data ? (
        <p role="status" className="text-sm text-muted">
          {t("Reading workflow definitions…")}
        </p>
      ) : (
        <>
          {state.data.warnings.map((warning) => (
            <p
              key={warning}
              role="alert"
              className="break-words text-sm text-removed"
            >
              {warning}
            </p>
          ))}
          {state.data.workflows.length && !visible.length ? (
            <p role="status" className="text-sm text-muted">
              {t("No matching repository workflows.")}
            </p>
          ) : state.data.workflows.length ? (
            <div className="grouped-list">
              {visible.map((workflow) => (
                <details
                  key={workflow.path}
                  className="border-b border-line px-3 py-3 last:border-b-0"
                >
                  <summary className="cursor-pointer break-words text-sm font-medium">
                    <span className="mr-2 text-xs font-normal text-muted">
                      {t("GitHub Actions")}
                    </span>
                    {workflow.name}
                    <span className="ml-3 break-all text-xs font-normal text-muted">
                      {workflow.path}
                    </span>
                  </summary>
                  <div className="mt-3 space-y-3">
                    {workflow.triggers && (
                      <div>
                        <h4 className="mb-2 text-xs font-medium">
                          {t("Triggers")}
                        </h4>
                        <pre
                          className="source-panel whitespace-pre-wrap break-words"
                          tabIndex={0}
                        >
                          {workflow.triggers}
                        </pre>
                      </div>
                    )}
                    <details>
                      <summary className="cursor-pointer text-xs">
                        {t("Workflow source")}
                      </summary>
                      <pre
                        className="source-panel mt-2 max-h-[50vh] overflow-auto whitespace-pre-wrap break-words"
                        tabIndex={0}
                      >
                        {workflow.content}
                      </pre>
                    </details>
                    <ol className="space-y-4">
                      {workflow.jobs.map((job) => (
                        <li key={job.id} className="space-y-2">
                          <h4 className="break-words text-sm font-medium">
                            {job.name || job.id}
                          </h4>
                          <p className="break-words text-xs text-muted">
                            {job.id}
                            {job.runsOn ? ` · ${job.runsOn}` : ""}
                          </p>
                          {job.needs.length > 0 && (
                            <p className="break-words text-xs text-muted">
                              {t("Depends on: {0}", job.needs.join(", "))}
                            </p>
                          )}
                          {job.if && (
                            <p className="break-all text-xs text-muted">
                              {t("Condition: {0}", job.if)}
                            </p>
                          )}
                          {job.strategy && (
                            <pre
                              className="source-panel whitespace-pre-wrap break-words"
                              tabIndex={0}
                            >
                              {job.strategy}
                            </pre>
                          )}
                          {job.uses && (
                            <pre
                              className="source-panel whitespace-pre-wrap break-words"
                              tabIndex={0}
                            >
                              {job.uses}
                            </pre>
                          )}
                          <ol className="space-y-2">
                            {job.steps.map((step, index) => (
                              <li key={index} className="space-y-1">
                                <p className="break-words text-xs font-medium">
                                  {step.name || t("Step {0}", index + 1)}
                                </p>
                                <pre
                                  className="source-panel whitespace-pre-wrap break-words"
                                  tabIndex={0}
                                >
                                  {step.run || step.uses}
                                </pre>
                                {step.if && (
                                  <p className="break-all text-xs text-muted">
                                    {t("Condition: {0}", step.if)}
                                  </p>
                                )}
                                {step.workingDirectory && (
                                  <p className="break-all text-xs text-muted">
                                    {t("Working directory:")}{" "}
                                    {step.workingDirectory}
                                  </p>
                                )}
                              </li>
                            ))}
                          </ol>
                        </li>
                      ))}
                    </ol>
                  </div>
                </details>
              ))}
            </div>
          ) : (
            <p className="text-sm text-muted">
              {t("No workflow definitions found in .github/workflows.")}
            </p>
          )}
        </>
      )}
    </section>
  );
}
