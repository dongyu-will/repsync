import { useEffect, useState } from "react";
import { isMyGo } from "mygo-runtime";
import { Service } from "./mygo";
import { Button, Empty, Icon } from "./components";
import { previewDocuments } from "./preview";

export default function ProjectDocuments({
  path,
  names,
}: {
  path: string;
  names: string[];
}) {
  const [choice, setChoice] = useState("");
  const [attempt, setAttempt] = useState(0);
  const selected = names.includes(choice) ? choice : names[0];

  if (!selected) {
    return (
      <Empty icon="file" title="没有发现 Markdown 文档">
        当前仅查看项目根目录的 .md 和 .markdown 文件。
      </Empty>
    );
  }

  return (
    <div className="grid min-w-0 grid-cols-[clamp(112px,22%,144px)_minmax(0,1fr)] gap-3">
      <nav aria-label="项目文档" className="min-w-0">
        <ul className="space-y-1">
          {names.map((name) => (
            <li key={name}>
              <button
                aria-current={name === selected ? "true" : undefined}
                onClick={() => setChoice(name)}
                className={`flex min-h-9 w-full items-start gap-2 rounded-md px-2 py-2 text-left text-sm ${name === selected ? "bg-selection font-medium text-accent" : "hover:bg-sidebar"}`}
              >
                <Icon name="file" size={16} />
                <span className="min-w-0 break-all">{name}</span>
              </button>
            </li>
          ))}
        </ul>
      </nav>
      <section className="min-w-0" aria-label={`${selected} 文档`}>
        <h3 className="mb-3 break-all text-sm font-semibold">{selected}</h3>
        <DocumentContent
          key={`${path}:${selected}:${attempt}`}
          path={path}
          name={selected}
          onRetry={() => setAttempt((value) => value + 1)}
        />
      </section>
    </div>
  );
}

function DocumentContent({
  path,
  name,
  onRetry,
}: {
  path: string;
  name: string;
  onRetry: () => void;
}) {
  const [result, setResult] = useState<{
    content?: string;
    error?: string;
  } | null>(null);

  useEffect(() => {
    let active = true;
    async function load() {
      try {
        const content = isMyGo()
          ? await Service.readDocument(path, name)
          : previewDocuments[path]?.[name];
        if (content === undefined) throw new Error("示例文档不存在");
        if (active) setResult({ content });
      } catch (error) {
        if (active) setResult({ error: String(error) });
      }
    }
    void load();
    return () => {
      active = false;
    };
  }, [path, name]);

  if (!result) return <p role="status">正在读取 {name}…</p>;
  if (result.error) {
    return (
      <div role="alert" className="text-sm">
        <p className="mb-3 break-words text-red-800">
          无法读取 {name}：{result.error}
        </p>
        <Button onClick={onRetry}>重试</Button>
      </div>
    );
  }
  return (
    <pre
      className="source-panel"
      tabIndex={0}
      role="region"
      aria-label={`${name} 内容`}
    >
      {result.content || "文件为空"}
    </pre>
  );
}
