import { getLocale, t } from "./i18n";
import { useState, type ReactNode } from "react";
import { isMyGo } from "mygo-runtime";
import { Button, Icon, Select } from "./components";
import {
  primaryProjectTabs,
  projectTabLabel,
  type ProjectTab,
} from "./project-tabs";
import type { Preferences } from "./preferences";
import appInfo from "../../mygo.json";
import { Desktop } from "./mygo";

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
  const [updateError, setUpdateError] = useState("");

  async function checkForUpdates() {
    setUpdateError("");
    try {
      await Desktop.checkForUpdates(getLocale());
    } catch (error) {
      setUpdateError(String(error));
    }
  }

  return (
    <div className="max-w-3xl space-y-7 pb-4">
      <div className="flex min-h-5 flex-wrap items-center justify-between gap-2 text-xs text-muted">
        <p>{t("Changes are saved automatically on this device.")}</p>
        {notice && <p role="status">{t(notice)}</p>}
      </div>
      {error && (
        <p
          role="alert"
          className="rounded-md bg-red-50 p-3 text-sm text-red-800"
        >
          {t(error)}
        </p>
      )}

      <section aria-labelledby="startup-settings">
        <h2 id="startup-settings" className="mb-2 text-sm font-semibold">
          {t("Startup")}
        </h2>
        <div className="grouped-list">
          <PreferenceRow
            id="remember-project"
            title={t("Reopen last project")}
            description={t("Otherwise, open the first project in the sidebar.")}
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
            title={t("Remember project tabs")}
            description={t(
              "Return to the last main page you viewed in each project. Utilities open only when selected.",
            )}
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
            title={t("Default project page")}
            description={
              values.rememberTabs
                ? t("Used when a project has no saved tab.")
                : t("Used whenever you open a project.")
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
              {primaryProjectTabs.map((tab) => (
                <option key={tab} value={tab}>
                  {t(projectTabLabel(tab))}
                </option>
              ))}
            </Select>
          </PreferenceRow>
        </div>
      </section>

      <section aria-labelledby="appearance-settings">
        <h2 id="appearance-settings" className="mb-2 text-sm font-semibold">
          {t("Appearance")}
        </h2>
        <div className="grouped-list">
          <PreferenceRow id="language" title={t("Language")}>
            <Select
              id="language"
              value={values.language}
              onChange={(event) =>
                onChange({
                  language: event.target.value as Preferences["language"],
                })
              }
            >
              <option value="system">{t("Follow system")}</option>
              <option value="zh-CN" lang="zh-CN">
                简体中文
              </option>
              <option value="en" lang="en">
                English
              </option>
            </Select>
          </PreferenceRow>
          <PreferenceRow
            id="text-size"
            title={t("Text size")}
            description={t("Adjust text across the workspace.")}
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
              <option value="standard">{t("Standard")}</option>
              <option value="larger">{t("Larger")}</option>
            </Select>
          </PreferenceRow>
          <PreferenceRow
            id="motion"
            title={t("Motion")}
            description={t(
              "Reduce animation while keeping status indicators visible.",
            )}
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
              <option value="system">{t("Follow system")}</option>
              <option value="reduced">{t("Reduced")}</option>
            </Select>
          </PreferenceRow>
        </div>
      </section>

      <section aria-labelledby="refresh-settings">
        <h2 id="refresh-settings" className="mb-2 text-sm font-semibold">
          {t("Project status")}
        </h2>
        <div className="grouped-list">
          <PreferenceRow
            id="overview-refresh"
            title={t("Overview refresh")}
            description={t(
              "Refresh the project overview automatically. Returning to the window also refreshes it.",
            )}
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
              <option value={0}>{t("On focus only")}</option>
              <option value={30}>{t("Every 30 seconds")}</option>
              <option value={60}>{t("Every minute")}</option>
              <option value={300}>{t("Every 5 minutes")}</option>
            </Select>
          </PreferenceRow>
        </div>
      </section>

      <section aria-labelledby="about-settings">
        <h2 id="about-settings" className="mb-2 text-sm font-semibold">
          {t("About repsync")}
        </h2>
        <dl className="grouped-list">
          {[
            ["Version", appInfo.version],
            [
              "Workspace",
              projectCount === 1
                ? t("1 project")
                : t("{0} projects", projectCount),
            ],
            [
              "Preferences",
              isMyGo() ? "Saved in this desktop app" : "Saved in this browser",
            ],
            ["License", "MIT"],
          ].map(([label, value]) => (
            <div key={label} className="info-row">
              <dt className="text-muted">{t(label)}</dt>
              <dd>{t(value)}</dd>
            </div>
          ))}
        </dl>
        <div className="mt-3 space-y-2">
          <Button disabled={!isMyGo()} onClick={() => void checkForUpdates()}>
            {t("Check for updates")}
          </Button>
          <p className="text-xs text-muted">
            {t(
              "The desktop app checks daily. Updates replace this app after it quits and keep your data.",
            )}
          </p>
          {updateError && (
            <p role="alert" className="break-words text-sm text-removed">
              {t(updateError)}
            </p>
          )}
        </div>
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
              {t(label)}
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
                t(
                  "Reset these preferences to defaults? Projects, document drafts, and history will be kept.",
                ),
              )
            )
              onReset();
          }}
        >
          {t("Reset preferences")}
        </Button>
        <p className="text-xs text-muted">
          {t("Keeps your projects, document drafts, and history.")}
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
  description?: string;
  children: ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-x-6 gap-y-2 px-3 py-3">
      <div className="min-w-0 flex-1 basis-52">
        <label htmlFor={id} className="text-sm font-medium">
          {t(title)}
        </label>
        {description && (
          <p
            id={`${id}-description`}
            className="mt-1 max-w-lg text-xs leading-5 text-muted"
          >
            {t(description)}
          </p>
        )}
      </div>
      <div className="flex max-w-full shrink-0 items-center">{children}</div>
    </div>
  );
}
