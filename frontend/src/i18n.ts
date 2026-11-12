import { useLayoutEffect, useSyncExternalStore } from "react";
import { isMyGo } from "mygo-runtime";
import chinese from "./zh-CN.json";

export type Language = "system" | "zh-CN" | "en";
export type Locale = "zh-CN" | "en";
export const preferenceStorageKey = isMyGo()
  ? "repsync:preferences"
  : "repsync:preview:preferences";

export function readLanguage(value: unknown): Language {
  return value === "zh-CN" || value === "en" ? value : "system";
}

export function resolveLocale(
  preference: Language,
  systemLanguage = navigator.languages?.[0] || navigator.language,
): Locale {
  return preference === "system"
    ? /^zh(?:-|$)/i.test(systemLanguage)
      ? "zh-CN"
      : "en"
    : preference;
}

function initialLanguage(): Language {
  try {
    return readLanguage(
      JSON.parse(localStorage.getItem(preferenceStorageKey) || "null")
        ?.language,
    );
  } catch {
    return "system";
  }
}

let language = initialLanguage();
const listeners = new Set<() => void>();
export const getLocale = () => resolveLocale(language);
const snapshot = () => `${language}:${getLocale()}`;
const notify = () => listeners.forEach((listener) => listener());

export function setLanguage(next: Language) {
  if (language === next) return;
  language = next;
  notify();
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  window.addEventListener("languagechange", listener);
  return () => {
    listeners.delete(listener);
    window.removeEventListener("languagechange", listener);
  };
}

// The root subscribes once; its existing children retain their state while the
// whole interface re-renders. Translation never mutates rendered DOM or data.
export function useLanguage() {
  const current = useSyncExternalStore(subscribe, snapshot);
  useLayoutEffect(() => {
    document.documentElement.lang = getLocale();
  }, [current]);
}

const messages: Record<string, string> = chinese;
const interpolate = (text: string, values: readonly unknown[]) =>
  text.replace(/\{(\d+)\}/g, (placeholder, index: string) =>
    Number(index) < values.length ? String(values[Number(index)]) : placeholder,
  );

// Store operation notices in the source language so they can be translated
// again when the user changes language, without repeating the operation.
export const formatMessage = (source: string, ...values: readonly unknown[]) =>
  interpolate(source, values);

// Notices and errors may already include values when they arrive from a saved
// operation or the desktop service. Match only our named interface messages.
const patterns = Object.entries(messages)
  .filter(([source]) => source.includes("{0}") && /[a-zA-Z]{2}/.test(source))
  .map(([source, translated]) => {
    const indexes: number[] = [];
    const escaped = source.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    const pattern = escaped.replace(/\\\{(\d+)\\\}/g, (_, index: string) => {
      indexes.push(Number(index));
      return "([\\s\\S]*?)";
    });
    return { pattern: new RegExp(`^${pattern}$`), indexes, translated };
  });

export function translate(
  source: string,
  locale: Locale,
  ...values: readonly unknown[]
): string {
  if (locale === "en") return interpolate(source, values);
  if (source.startsWith("Error: ")) {
    const detail = source.slice(7);
    const translated = translate(detail, locale, ...values);
    return translated === detail ? source : translated;
  }
  const translated = Object.hasOwn(messages, source)
    ? messages[source]
    : undefined;
  if (translated !== undefined) return interpolate(translated, values);
  if (!values.length) {
    for (const { pattern, indexes, translated } of patterns) {
      const match = pattern.exec(source);
      if (match) {
        const captured: unknown[] = [];
        indexes.forEach((index, position) => {
          captured[index] = match[position + 1];
        });
        return interpolate(translated, captured);
      }
    }
  }
  return interpolate(source, values);
}

export function t(source: string | undefined, ...values: readonly unknown[]) {
  return translate(source ?? "", getLocale(), ...values);
}
