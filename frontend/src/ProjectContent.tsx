import type { Inspection } from "./mygo";
import { Empty } from "./components";
import ProjectDocuments from "./ProjectDocuments";

export type ProjectTab = "概览" | "命令" | "文档";

export default function ProjectContent({
  details,
  tab,
}: {
  details: Inspection;
  tab: ProjectTab;
}) {
  return (
    <>
      {details.warnings.map((warning, i) => (
        <p
          key={i}
          className="mb-4 break-words rounded-md bg-amber-50 p-3 text-sm leading-6 text-amber-900"
        >
          {warning}
        </p>
      ))}
      {tab === "文档" ? (
        <>
          <div className="section-heading">
            <h2>项目文档</h2>
            <p>项目根目录的 Markdown 文件 · 只读</p>
          </div>
          <ProjectDocuments
            key={details.project.path}
            path={details.project.path}
            names={details.documents}
          />
        </>
      ) : tab === "命令" ? (
        <>
          <div className="section-heading">
            <h2>项目命令</h2>
            <p>读取根目录 package.json · 命令执行与日志尚未接通。</p>
          </div>
          {Object.keys(details.scripts).length ? (
            <div className="grouped-list">
              {Object.entries(details.scripts).map(([name, command]) => (
                <div key={name} className="info-row">
                  <strong className="font-medium">{name}</strong>
                  <code className="text-muted">{command}</code>
                </div>
              ))}
            </div>
          ) : (
            <Empty icon="file" title="没有发现 package.json 命令">
              项目已有的其他命令入口暂未解析。
            </Empty>
          )}
        </>
      ) : (
        <>
          <div className="section-heading">
            <h2>项目概览</h2>
          </div>
          <dl className="grouped-list">
            {[
              ["目录", details.project.path],
              ["Git", details.git ? details.branch : "非 Git 项目"],
              [
                "工作区",
                !details.git
                  ? "不可用"
                  : !details.statusKnown
                    ? "状态读取失败"
                    : details.status
                      ? "有未提交改动"
                      : "工作区干净",
              ],
              ["项目文档", `${details.documents.length} 个 Markdown 文件`],
              [
                "命令",
                `${Object.keys(details.scripts).length} 个 package.json 脚本`,
              ],
            ].map(([label, value]) => (
              <div key={label} className="info-row">
                <dt className="text-muted">{label}</dt>
                <dd>{value}</dd>
              </div>
            ))}
          </dl>
        </>
      )}
    </>
  );
}
