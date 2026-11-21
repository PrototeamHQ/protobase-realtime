import { createRequire } from "node:module"
import path from "node:path"
import { binaryName, packageName, platformFor } from "./platforms.js"

const require = createRequire(import.meta.url)

// The path of the relay binary for this machine, from the platform package npm installed beside this one.
export function binaryPath(os = process.platform, cpu = process.arch, resolve = require.resolve) {
  const platform = platformFor(os, cpu)
  const name = packageName(platform)
  let manifest
  try {
    manifest = resolve(`${name}/package.json`)
  } catch (error) {
    if (error.code !== "MODULE_NOT_FOUND") throw error
    throw new Error(`${name} is not installed: install @protobase/realtime with its optional dependencies`, {
      cause: error,
    })
  }
  return path.join(path.dirname(manifest), "bin", binaryName(platform))
}
