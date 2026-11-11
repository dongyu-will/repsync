import { useLayoutEffect, useReducer } from "react";
import { isMyGo } from "mygo-runtime";
import { projectTabs, type ProjectTab } from "./project-tabs";

export type Preferences = {
  rememberProject: boolean;
  rememberTabs: boolean;
  defaultTab: ProjectTab;
  refreshSeconds: 0 | 30 | 60 | 300;
  textSize: "standard" | "larger";
  motion: "system" | "reduced";
};

export const defaultPreferences: Preferences = {
  rememberProject: true,
  rememberTabs: true,
  defaultTab: "Overview",
  refreshSeconds: 60,
  textSize: "standard",
  motion: "system",
};

const storageKey = isMyGo()
  ? "repsync:preferences"
  : "repsync:preview:preferences";

type State = { values: Preferences; error: string; notice: string };

function load(): State {
  try {
    const raw = localStorage.getItem(storageKey);
    if (!raw) return { values: defaultPreferences, error: "", notice: "" };
    const saved = JSON.parse(raw);
    if (!saved || typeof saved !== "object" || Array.isArray(saved)) {
      throw new Error("Invalid preferences");
    }
    return {
      values: {
        rememberProject:
          typeof saved.rememberProject === "boolean"
            ? saved.rememberProject
            : defaultPreferences.rememberProject,
        rememberTabs:
          typeof saved.rememberTabs === "boolean"
            ? saved.rememberTabs
            : defaultPreferences.rememberTabs,
        defaultTab: projectTabs.includes(saved.defaultTab)
          ? saved.defaultTab
          : defaultPreferences.defaultTab,
        refreshSeconds: [0, 30, 60, 300].includes(saved.refreshSeconds)
          ? saved.refreshSeconds
          : defaultPreferences.refreshSeconds,
        textSize: saved.textSize === "larger" ? "larger" : "standard",
        motion: saved.motion === "reduced" ? "reduced" : "system",
      },
      error: "",
      notice: "",
    };
  } catch {
    return {
      values: defaultPreferences,
      error:
        "Could not read saved preferences. Defaults are in use. Reset preferences to try saving again.",
      notice: "",
    };
  }
}

export function usePreferences() {
  const [state, update] = useReducer(
    (_: State, next: State) => next,
    undefined,
    load,
  );

  useLayoutEffect(() => {
    document.documentElement.dataset.textSize = state.values.textSize;
    document.documentElement.dataset.motion = state.values.motion;
  }, [state.values.textSize, state.values.motion]);

  function save(values: Preferences, notice = "Preferences saved") {
    try {
      localStorage.setItem(storageKey, JSON.stringify(values));
      update({ values, error: "", notice });
    } catch {
      update({
        values,
        error:
          "Could not save preferences. Changes apply to this window only. Check browser storage and try again.",
        notice: "",
      });
    }
  }

  return {
    ...state,
    change: (patch: Partial<Preferences>) =>
      save({ ...state.values, ...patch }),
    reset: () => save(defaultPreferences, "Default preferences restored"),
  };
}
