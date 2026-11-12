import { t } from "./i18n";
import { useEffect, useRef, useState, type RefObject } from "react";
import { Icon } from "./components";

const matchLimit = 2000;
const highlightsName = "document-find-matches";
const activeName = "document-find-active";

function findRanges(reader: HTMLElement, term: string) {
  const pattern = new RegExp(
    term.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"),
    "giu",
  );
  const walker = document.createTreeWalker(reader, NodeFilter.SHOW_TEXT);
  const ranges: Range[] = [];
  let block: Element | null = null;
  let content = "";
  let pieces: { node: Text; start: number }[] = [];

  // Keep text split by inline Markdown formatting in the same searchable block.
  function collect() {
    let first = 0;
    let last = 0;
    for (const match of content.matchAll(pattern)) {
      if (ranges.length === matchLimit) return false;
      const start = match.index;
      const end = start + match[0].length;
      while (pieces[first + 1]?.start <= start) first++;
      last = Math.max(first, last);
      while (pieces[last + 1]?.start < end) last++;
      const range = document.createRange();
      range.setStart(pieces[first].node, start - pieces[first].start);
      range.setEnd(pieces[last].node, end - pieces[last].start);
      ranges.push(range);
    }
    return true;
  }

  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    if (!(node instanceof Text) || !node.data) continue;
    const parent =
      node.parentElement?.closest(
        "p, li, h1, h2, h3, h4, h5, h6, pre, td, th, blockquote",
      ) ?? reader;
    if (parent !== block) {
      if (!collect()) return { ranges, truncated: true };
      block = parent;
      content = "";
      pieces = [];
    }
    pieces.push({ node, start: content.length });
    content += node.data;
  }
  return { ranges, truncated: !collect() };
}

export default function DocumentFind({
  reader,
}: {
  reader: RefObject<HTMLDivElement | null>;
}) {
  const [query, setQuery] = useState<string | null>(null);
  const [index, setIndex] = useState(0);
  const [result, setResult] = useState<{
    term: string;
    ranges: Range[];
    truncated: boolean;
  } | null>(null);
  const input = useRef<HTMLInputElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const open = query !== null;
  const term = query?.trim() || "";
  const count = term && result?.term === term ? result.ranges.length : 0;
  const current = Math.min(index, Math.max(0, count - 1));

  useEffect(() => {
    function shortcut(event: KeyboardEvent) {
      if (
        event.defaultPrevented ||
        event.isComposing ||
        event.altKey ||
        event.shiftKey ||
        !(event.metaKey || event.ctrlKey) ||
        event.key.toLowerCase() !== "f" ||
        !reader.current?.getClientRects().length ||
        document.querySelector("dialog[open]")
      ) {
        return;
      }
      event.preventDefault();
      setQuery((value) => value ?? "");
      input.current?.focus();
      input.current?.select();
    }
    window.addEventListener("keydown", shortcut);
    return () => window.removeEventListener("keydown", shortcut);
  }, [reader]);

  useEffect(() => {
    if (open) input.current?.focus();
  }, [open]);

  useEffect(() => {
    const container = reader.current;
    if (!term || !container) return;
    let frame = 0;
    function schedule() {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        if (container) setResult({ term, ...findRanges(container, term) });
      });
    }
    schedule();
    const observer = new MutationObserver(schedule);
    observer.observe(container, {
      childList: true,
      subtree: true,
      characterData: true,
    });
    return () => {
      observer.disconnect();
      cancelAnimationFrame(frame);
    };
  }, [reader, term]);

  useEffect(() => {
    if (
      !count ||
      !result ||
      !CSS.highlights ||
      typeof Highlight === "undefined"
    )
      return;
    CSS.highlights.set(highlightsName, new Highlight(...result.ranges));
    return () => {
      CSS.highlights.delete(highlightsName);
    };
  }, [count, result]);

  useEffect(() => {
    const container = reader.current;
    const range = count ? result?.ranges[current] : undefined;
    if (!range || !container) return;

    const rect = range.getBoundingClientRect();
    const viewport = container.getBoundingClientRect();
    if (rect.top < viewport.top || rect.bottom > viewport.bottom) {
      container.scrollBy({
        top: rect.top - viewport.top - container.clientHeight / 3,
      });
    }

    if (CSS.highlights && typeof Highlight !== "undefined") {
      const active = new Highlight(range);
      active.priority = 1;
      CSS.highlights.set(activeName, active);
      return () => {
        CSS.highlights.delete(activeName);
      };
    }

    const selection = document.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
    return () => {
      if (selection?.rangeCount && selection.getRangeAt(0) === range)
        selection.removeAllRanges();
    };
  }, [count, current, reader, result]);

  function move(step: number) {
    if (count) setIndex((current + step + count) % count);
  }

  function close() {
    setQuery(null);
    setResult(null);
    setIndex(0);
    requestAnimationFrame(() => trigger.current?.focus());
  }

  if (!open) {
    return (
      <button
        ref={trigger}
        type="button"
        aria-label={t("Find in document")}
        aria-keyshortcuts="Meta+F Control+F"
        title={t("Find in document (⌘F / Ctrl+F)")}
        onClick={() => setQuery("")}
        className="icon-button text-muted"
      >
        <Icon name="search" size={16} />
      </button>
    );
  }

  return (
    <div
      role="search"
      aria-label={t("Document body")}
      className="flex max-w-full items-center gap-0.5 rounded-md border border-control has-[input:focus]:border-accent"
      onKeyDown={(event) => {
        if (event.nativeEvent.isComposing) return;
        if (event.key === "Escape") {
          event.preventDefault();
          close();
        }
      }}
    >
      <input
        ref={input}
        aria-label={t("Find in document")}
        placeholder={t("Find in document")}
        value={query}
        onChange={(event) => {
          setQuery(event.target.value);
          setIndex(0);
        }}
        onKeyDown={(event) => {
          if (event.key !== "Enter" || event.nativeEvent.isComposing) return;
          event.preventDefault();
          move(event.shiftKey ? -1 : 1);
        }}
        className="h-8 w-28 min-w-0 rounded-md px-2 text-sm focus-visible:outline-none!"
      />
      {term && (
        <span
          role="status"
          className="shrink-0 px-1 text-xs text-muted tabular-nums"
        >
          {count
            ? `${current + 1} / ${count}${result?.truncated ? "+" : ""}`
            : t("No matches")}
        </span>
      )}
      <button
        type="button"
        aria-label={t("Previous match")}
        title={t("Previous match (Shift+Enter)")}
        disabled={!count}
        onClick={() => move(-1)}
        className="icon-button disabled:opacity-40"
      >
        <span className="-rotate-90">
          <Icon name="chevron" size={14} />
        </span>
      </button>
      <button
        type="button"
        aria-label={t("Next match")}
        title={t("Next match (Enter)")}
        disabled={!count}
        onClick={() => move(1)}
        className="icon-button disabled:opacity-40"
      >
        <span className="rotate-90">
          <Icon name="chevron" size={14} />
        </span>
      </button>
      <button
        type="button"
        aria-label={t("Close find")}
        title={t("Close find (Esc)")}
        onClick={close}
        className="icon-button"
      >
        <Icon name="close" size={14} />
      </button>
    </div>
  );
}
