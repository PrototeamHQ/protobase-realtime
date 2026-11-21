import assert from "node:assert/strict"
import path from "node:path"
import { test } from "node:test"
import { binaryPath } from "./index.js"

const installed = (name) => path.join("/app/node_modules", name)

test("resolves the binary from the platform package", () => {
  const resolve = (id) => installed(id)
  assert.equal(
    binaryPath("linux", "arm64", resolve),
    path.join("/app/node_modules/@protobase/realtime-linux-arm64/bin/relay"),
  )
  assert.equal(
    binaryPath("win32", "x64", resolve),
    path.join("/app/node_modules/@protobase/realtime-win32-x64/bin/relay.exe"),
  )
})

test("refuses a platform without a binary", () => {
  assert.throws(() => binaryPath("freebsd", "x64", installed), /no relay for freebsd-x64; it supports darwin-arm64/)
})

test("names the platform package when it is not installed", () => {
  const resolve = () => {
    throw Object.assign(new Error("Cannot find module"), { code: "MODULE_NOT_FOUND" })
  }
  assert.throws(() => binaryPath("darwin", "x64", resolve), /@protobase\/realtime-darwin-x64 is not installed/)
})

test("rethrows other resolution errors", () => {
  const resolve = () => {
    throw Object.assign(new Error("permission denied"), { code: "EACCES" })
  }
  assert.throws(() => binaryPath("darwin", "x64", resolve), /permission denied/)
})
