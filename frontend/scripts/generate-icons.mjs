import { readFile, writeFile } from "node:fs/promises";
import { Resvg } from "@resvg/resvg-js";

// Preserve the original Figma mark; only the desktop tile adds a background.
const mark = await readFile(
  new URL("../../resources/mark.svg", import.meta.url),
);
const text = mark.toString();
const content = text.match(/<svg[^>]*>([\s\S]*?)<\/svg>/)?.[1];
const viewBox = text.match(/viewBox="([^"]+)"/)?.[1];
if (!content || !viewBox)
  throw new Error("The brand mark must be a valid SVG with a viewBox");
const icon =
  Buffer.from(`<svg xmlns="http://www.w3.org/2000/svg" width="1024" height="1024" viewBox="0 0 1024 1024">
<rect x="96" y="96" width="832" height="832" rx="192" fill="#f6f6f8" stroke="#e2e4e9" stroke-width="4"/>
<svg x="208" y="343" width="608" height="338" viewBox="${viewBox}">${content}</svg>
</svg>\n`);
const png = new Resvg(icon).render().asPng();

async function writeChanged(destination, data) {
  try {
    if ((await readFile(destination)).equals(data)) return;
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
  }
  await writeFile(destination, data);
}
await writeChanged(new URL("../../resources/icon.svg", import.meta.url), icon);
await writeChanged(new URL("../../resources/icon.png", import.meta.url), png);
await writeChanged(new URL("../public/app-icon.svg", import.meta.url), icon);
await writeChanged(new URL("../public/brand.svg", import.meta.url), mark);
