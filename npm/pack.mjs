// Builds the relay for every platform and packs the npm packages at one version:
//   node npm/pack.mjs <version> <out dir>
// It writes one tarball per platform package and one for @protobase/realtime, which depends on each platform package
// as optional, and <out dir>/packages.json, the packages in the order they publish: the main package last, so it never
// points at a platform package that is not on npm yet.
import { execFileSync } from "node:child_process"
import { cpSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import path from "node:path"
import semver from "semver"
import { binaryName, packageName, platforms } from "./realtime/platforms.js"

const [version, out] = process.argv.slice(2)
if (!version || !out) throw new Error("usage: node npm/pack.mjs <version> <out dir>")
if (semver.valid(version) !== version) throw new Error(`${version} is not a semver version such as 1.2.3`)

const root = path.resolve(import.meta.dirname, "..")
const outDir = path.resolve(out)
const staging = mkdtempSync(path.join(tmpdir(), "realtime-pack-"))
mkdirSync(outDir, { recursive: true })

// The tarball's file name, from the report npm pack prints with --json.
function pack(dir) {
  const report = execFileSync("npm", ["pack", "--json", "--pack-destination", outDir], { cwd: dir, encoding: "utf8" })
  return JSON.parse(report)[0].filename
}

const packed = []
for (const platform of platforms) {
  const name = packageName(platform)
  const dir = path.join(staging, `${platform.os}-${platform.cpu}`)
  mkdirSync(path.join(dir, "bin"), { recursive: true })
  execFileSync(
    "go",
    [
      "build",
      "-trimpath",
      "-ldflags",
      `-s -w -X main.version=${version}`,
      "-o",
      path.join(dir, "bin", binaryName(platform)),
      "./cmd/relay",
    ],
    {
      cwd: root,
      stdio: "inherit",
      env: { ...process.env, CGO_ENABLED: "0", GOOS: platform.goos, GOARCH: platform.goarch },
    },
  )
  const manifest = {
    name,
    version,
    description: `The realtime relay for ${platform.os}-${platform.cpu}; install @protobase/realtime instead`,
    repository: "github:PrototeamHQ/protobase-realtime",
    os: [platform.os],
    cpu: [platform.cpu],
    files: ["bin"],
  }
  writeFileSync(path.join(dir, "package.json"), `${JSON.stringify(manifest, null, 2)}\n`)
  packed.push({ name, version, file: pack(dir) })
}

const mainDir = path.join(staging, "realtime")
cpSync(path.join(import.meta.dirname, "realtime"), mainDir, { recursive: true })
const manifest = JSON.parse(readFileSync(path.join(mainDir, "package.json"), "utf8"))
manifest.version = version
manifest.optionalDependencies = Object.fromEntries(platforms.map((p) => [packageName(p), version]))
writeFileSync(path.join(mainDir, "package.json"), `${JSON.stringify(manifest, null, 2)}\n`)
packed.push({ name: manifest.name, version, file: pack(mainDir) })

writeFileSync(path.join(outDir, "packages.json"), `${JSON.stringify(packed, null, 2)}\n`)
rmSync(staging, { recursive: true })
