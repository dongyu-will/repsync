import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";

export default function MarkdownPreview({
  content,
  names,
  onDocument,
}: {
  content: string;
  names: string[];
  onDocument: (name: string) => void;
}) {
  return (
    <Markdown
      remarkPlugins={[remarkGfm]}
      skipHtml
      components={{
        a({ href, children }) {
          let name = "";
          try {
            name = decodeURIComponent((href || "").replace(/^\.\//, ""));
          } catch {
            /* Keep malformed URLs as text. */
          }
          if (names.includes(name))
            return (
              <button
                className="text-accent underline underline-offset-2"
                onClick={() => onDocument(name)}
              >
                {children}
              </button>
            );
          if (href && /^(https?:|mailto:)/i.test(href))
            return (
              <a href={href} target="_blank" rel="noreferrer">
                {children}
              </a>
            );
          return <span>{children}</span>;
        },
        img({ alt, src }) {
          return typeof src === "string" && /^https?:\/\//i.test(src) ? (
            <a href={src} target="_blank" rel="noreferrer">
              {alt || "查看图片"}
            </a>
          ) : (
            <span className="text-muted">{alt || "本地图片暂不展示"}</span>
          );
        },
        table({ children }) {
          return (
            <div className="overflow-x-auto">
              <table>{children}</table>
            </div>
          );
        },
      }}
    >
      {content}
    </Markdown>
  );
}
