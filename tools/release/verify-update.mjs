import assert from "node:assert/strict";
import { createHash, createPublicKey, verify } from "node:crypto";
import { readFile } from "node:fs/promises";
import path from "node:path";

// Verify the artifacts before publishing a release, independently of the builder.
const config = JSON.parse(await readFile("mygo.json", "utf8"));
const dir = path.resolve(process.argv[2] || "build/darwin-arm64");
const manifest = JSON.parse(
  await readFile(path.join(dir, "update-darwin-arm64.json"), "utf8"),
);
assert.equal(
  manifest.version,
  config.version,
  "Update version differs from app",
);
const name = `${config.slug}-${config.version}-darwin-arm64.tar.gz`;
assert.equal(
  manifest.url,
  `https://github.com/${config.updates.github}/releases/download/v${config.version}/${name}`,
  "Update URL differs from release",
);
const archive = await readFile(path.join(dir, name));
assert.equal(archive.length, manifest.size, "Update archive size differs");
const key = createPublicKey({
  key: Buffer.concat([
    Buffer.from("302a300506032b6570032100", "hex"),
    Buffer.from(config.updates.publicKey, "base64"),
  ]),
  format: "der",
  type: "spki",
});
assert(
  verify(
    null,
    createHash("sha256").update(archive).digest(),
    key,
    Buffer.from(manifest.signature, "base64"),
  ),
  "Update archive signature is invalid",
);
console.log(
  `Verified signed update ${manifest.version} (${archive.length} bytes)`,
);
