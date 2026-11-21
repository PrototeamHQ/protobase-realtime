// Publishes the tarballs npm/pack.mjs packed, in the order its packages.json lists them:
//   node npm/publish.mjs <packed dir>
// A version npm already has is skipped, so a run that stopped halfway can run again. A prerelease, such as 1.3.0-rc.1,
// goes under the next dist-tag, so latest stays on the newest release.
import { execFileSync } from "node:child_process"
import { readFileSync } from "node:fs"
import path from "node:path"
import semver from "semver"

const [dir] = process.argv.slice(2)
if (!dir) throw new Error("usage: node npm/publish.mjs <packed dir>")

const registry = new URL(execFileSync("npm", ["config", "get", "registry"], { encoding: "utf8" }).trim())
const packed = JSON.parse(readFileSync(path.join(dir, "packages.json"), "utf8"))
for (const { name, version, file } of packed) {
  const response = await fetch(new URL(`${name}/${version}`, registry))
  if (response.ok) continue
  if (response.status !== 404) throw new Error(`npm answered ${response.status} for ${name}@${version}`)
  const distTag = semver.prerelease(version) ? "next" : "latest"
  execFileSync("npm", ["publish", path.join(dir, file), "--access", "public", "--tag", distTag], { stdio: "inherit" })
}
