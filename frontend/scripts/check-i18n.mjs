import fs from "node:fs";
import path from "node:path";
import ts from "typescript";

const root = path.resolve(import.meta.dirname, "../src");
const catalog = JSON.parse(
  fs.readFileSync(path.join(root, "zh-CN.json"), "utf8"),
);
const technical = new Set([
  "repsync",
  "CI/CD",
  "Skill",
  "MCP",
  "CLI",
  "English",
  "Codex ·",
  "/ AGENTS.md",
  "· PID",
]);
const errors = [];
for (const file of fs
  .readdirSync(root)
  .filter((file) => /\.tsx?$/.test(file) && file !== "mygo.ts")) {
  const source = ts.createSourceFile(
    file,
    fs.readFileSync(path.join(root, file), "utf8"),
    ts.ScriptTarget.Latest,
    true,
  );
  const fail = (node, message) =>
    errors.push(
      `${file}:${source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1}: ${message}`,
    );
  function visit(node) {
    if (
      ts.isCallExpression(node) &&
      ["t", "formatMessage"].includes(node.expression.getText(source))
    ) {
      const key = node.arguments[0];
      if (
        key &&
        ts.isStringLiteral(key) &&
        !Object.hasOwn(catalog, key.text) &&
        !technical.has(key.text)
      ) {
        fail(key, `Missing Chinese translation: ${JSON.stringify(key.text)}`);
      }
      if (key && ts.isStringLiteral(key) && !/[a-zA-Z]/.test(key.text)) {
        fail(
          key,
          "Translate a named interface message, not generic interpolation.",
        );
      }
    }
    if (ts.isJsxText(node)) {
      const text = node.text.replace(/\s+/g, " ").trim();
      if (/[a-zA-Z]/.test(text) && !technical.has(text)) {
        fail(node, `Untranslated interface text: ${JSON.stringify(text)}`);
      }
    }
    ts.forEachChild(node, visit);
  }
  visit(source);
}
for (const [source, translated] of Object.entries(catalog)) {
  if (typeof translated !== "string" || !translated.trim())
    errors.push(`Empty translation: ${source}`);
  const placeholders = new Set(source.match(/\{\d+\}/g) || []);
  for (const placeholder of translated.match(/\{\d+\}/g) || []) {
    if (!placeholders.has(placeholder))
      errors.push(`Unknown placeholder ${placeholder}: ${source}`);
  }
}
if (errors.length) {
  console.error(errors.join("\n"));
  process.exitCode = 1;
} else {
  console.log(
    `Interface translations checked (${Object.keys(catalog).length} messages).`,
  );
}
