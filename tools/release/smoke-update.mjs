import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import {
  mkdtemp,
  mkdir,
  readFile,
  readdir,
  rm,
  writeFile,
} from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { setTimeout } from "node:timers/promises";
import { promisify } from "node:util";

const exec = promisify(execFile);
assert.equal(
  process.platform,
  "darwin",
  "This smoke check exercises macOS bundle replacement",
);
const config = JSON.parse(await readFile("mygo.json", "utf8"));
const dir = path.resolve(process.argv[2] || "build/darwin-arm64");
const work = await mkdtemp(path.join(os.tmpdir(), "repsync-update-check-"));
const bundle = path.join(work, "repsync.app");
const executable = path.join(bundle, "Contents/MacOS/probe");
await mkdir(path.dirname(executable), { recursive: true });
const data = path.join(work, "project-data.json");
await writeFile(data, '{"preserve":true}\n');
try {
  await exec("go", [
    "build",
    "-o",
    executable,
    "-ldflags",
    `-X github.com/egoist/mygo.production=1 -X github.com/egoist/mygo.packageUpdateFeed=https://github.com/${config.updates.github}/releases/latest/download/update-darwin-arm64.json -X github.com/egoist/mygo.packageUpdateKey=${config.updates.publicKey}`,
    "./tools/release/smoke",
  ]);
  const result = await exec(executable, [dir], { timeout: 45000 });
  console.log(result.stdout.trim());
  const plist = path.join(bundle, "Contents/Info.plist");
  let installed = false;
  for (let attempt = 0; attempt < 100; attempt++) {
    try {
      const result = await exec("/usr/libexec/PlistBuddy", [
        "-c",
        "Print :CFBundleShortVersionString",
        plist,
      ]);
      installed = result.stdout.trim() === config.version;
      if (
        installed &&
        (await readdir(work)).every((name) => !name.includes(".update-"))
      )
        break;
    } catch {
      /* The helper is still replacing the bundle. */
    }
    await setTimeout(100);
  }
  assert(installed, "The signed update was not installed after process exit");
  assert.deepEqual((await readdir(work)).sort(), [
    "project-data.json",
    "repsync.app",
  ]);
  assert.equal(await readFile(data, "utf8"), '{"preserve":true}\n');
  await exec("codesign", ["--verify", "--deep", "--strict", bundle]);
  console.log(
    `Verified in-place upgrade to ${config.version}, preserved data, valid app signature, and no leftover app copies`,
  );
  await rm(work, { recursive: true });
} catch (error) {
  console.error(`Update smoke check failed; retained artifacts at ${work}`);
  throw error;
}
