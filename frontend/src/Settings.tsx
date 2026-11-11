import type { ReactNode } from "react";
import { isMyGo } from "mygo-runtime";
import { Button, Icon, Select } from "./components";
import { projectTabs, type ProjectTab } from "./project-tabs";
import type { Preferences } from "./preferences";
import appInfo from "../../mygo.json";

const repository = "https://github.com/dongyu-will/repsync";

export default function Settings({
  values,
  error,
  notice,
  projectCount,
  onChange,
  onReset,
}: {
  values: Preferences;
  error: string;
  notice: string;
  projectCount: number;
  onChange: (patch: Partial<Preferences>) => void;
  onReset: () => void;
}) {
  return (
    <div className="max-w-3xl space-y-7 pb-4">
      <div className="flex min-h-5 flex-wrap items-center justify-between gap-2 text-xs text-muted">
        <p>Changes are saved automatically on this device.</p>
        {notice && <p role="status">{notice}</p>}
      </div>
      {error && (
        <p
          role="alert"
          className="rounded-md bg-red-50 p-3 text-sm text-red-800"
        >
          {error}
        </p>
      )}

      <section aria-labelledby="startup-settings">
        <h2 id="startup-settings" className="mb-2 text-sm font-semibold">
          Startup
        </h2>
        <div className="grouped-list">
          <PreferenceRow
            id="remember-project"
            title="Reopen last project"
            description="Otherwise, open the first project in the sidebar."
          >
            <input
              id="remember-project"
              aria-describedby="remember-project-description"
              type="checkbox"
              className="size-4 accent-accent"
              checked={values.rememberProject}
              onChange={(event) =>
                onChange({ rememberProject: event.target.checked })
              }
            />
          </PreferenceRow>
          <PreferenceRow
            id="remember-tabs"
            title="Remember project tabs"
            description="Return to the last page you viewed in each project."
          >
            <input
              id="remember-tabs"
              aria-describedby="remember-tabs-description"
              type="checkbox"
              className="size-4 accent-accent"
              checked={values.rememberTabs}
              onChange={(event) =>
                onChange({ rememberTabs: event.target.checked })
              }
            />
          </PreferenceRow>
          <PreferenceRow
            id="default-tab"
            title="Default project page"
            description={
              values.rememberTabs
                ? "Used when a project has no saved tab."
                : "Used whenever you open a project."
            }
          >
            <Select
              id="default-tab"
              aria-describedby="default-tab-description"
              value={values.defaultTab}
              onChange={(event) =>
                onChange({ defaultTab: event.target.value as ProjectTab })
              }
            >
              {projectTabs.map((tab) => (
                <option key={tab}>{tab}</option>
              ))}
            </Select>
          </PreferenceRow>
        </div>
      </section>

      <section aria-labelledby="appearance-settings">
        <h2 id="appearance-settings" className="mb-2 text-sm font-semibold">
          Appearance
        </h2>
        <div className="grouped-list">
          <PreferenceRow
            id="text-size"
            title="Text size"
            description="Adjust text across the workspace."
          >
            <Select
              id="text-size"
              aria-describedby="text-size-description"
              value={values.textSize}
              onChange={(event) =>
                onChange({
                  textSize: event.target.value as Preferences["textSize"],
                })
              }
            >
              <option value="standard">Standard</option>
              <option value="larger">Larger</option>
            </Select>
          </PreferenceRow>
          <PreferenceRow
            id="motion"
            title="Motion"
            description="Reduce animation while keeping status indicators visible."
          >
            <Select
              id="motion"
              aria-describedby="motion-description"
              value={values.motion}
              onChange={(event) =>
                onChange({
                  motion: event.target.value as Preferences["motion"],
                })
              }
            >
              <option value="system">Follow system</option>
              <option value="reduced">Reduced</option>
            </Select>
          </PreferenceRow>
        </div>
      </section>

      <section aria-labelledby="refresh-settings">
        <h2 id="refresh-settings" className="mb-2 text-sm font-semibold">
          Project status
        </h2>
        <div className="grouped-list">
          <PreferenceRow
            id="overview-refresh"
            title="Overview refresh"
            description="Refresh the project overview automatically. Returning to the window also refreshes it."
          >
            <Select
              id="overview-refresh"
              aria-describedby="overview-refresh-description"
              value={values.refreshSeconds}
              onChange={(event) =>
                onChange({
                  refreshSeconds: Number(
                    event.target.value,
                  ) as Preferences["refreshSeconds"],
                })
              }
            >
              <option value={0}>On focus only</option>
              <option value={30}>Every 30 seconds</option>
              <option value={60}>Every minute</option>
              <option value={300}>Every 5 minutes</option>
            </Select>
          </PreferenceRow>
        </div>
      </section>

      <section aria-labelledby="about-settings">
        <h2 id="about-settings" className="mb-2 text-sm font-semibold">
          About repsync
        </h2>
        <dl className="grouped-list">
          {[
            ["Version", appInfo.version],
            [
              "Workspace",
              `${projectCount} ${projectCount === 1 ? "project" : "projects"}`,
            ],
            [
              "Preferences",
              isMyGo() ? "Saved in this desktop app" : "Saved in this browser",
            ],
            ["License", "MIT"],
          ].map(([label, value]) => (
            <div key={label} className="info-row">
              <dt className="text-muted">{label}</dt>
              <dd>{value}</dd>
            </div>
          ))}
        </dl>
        <div className="mt-3 flex flex-wrap gap-x-5 gap-y-2 text-sm">
          {[
            ["Documentation", `${repository}#readme`],
            ["Report an issue", `${repository}/issues/new`],
            ["Releases", `${repository}/releases`],
            ["Source code", repository],
          ].map(([label, href]) => (
            <a
              key={label}
              href={href}
              target="_blank"
              rel="noreferrer"
              className="inline-flex min-h-8 items-center gap-1 text-accent underline-offset-4 hover:underline"
            >
              {label}
              <Icon name="arrow" size={14} />
            </a>
          ))}
        </div>
      </section>

      <div className="flex flex-wrap items-center gap-3 border-t border-line pt-4">
        <Button
          onClick={() => {
            if (
              window.confirm(
                "Reset these preferences to defaults? Projects, document drafts, and history will be kept.",
              )
            )
              onReset();
          }}
        >
          Reset preferences
        </Button>
        <p className="text-xs text-muted">
          Keeps your projects, document drafts, and history.
        </p>
      </div>
    </div>
  );
}

function PreferenceRow({
  id,
  title,
  description,
  children,
}: {
  id: string;
  title: string;
  description: string;
  children: ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-x-6 gap-y-2 px-3 py-3">
      <div className="min-w-0 flex-1 basis-52">
        <label htmlFor={id} className="text-sm font-medium">
          {title}
        </label>
        <p
          id={`${id}-description`}
          className="mt-1 max-w-lg text-xs leading-5 text-muted"
        >
          {description}
        </p>
      </div>
      <div className="flex max-w-full shrink-0 items-center">{children}</div>
    </div>
  );
}
