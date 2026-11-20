#!/usr/bin/env node
// Runs the relay binary for this machine with the given arguments, passing on stop signals and its exit code.
import { spawn } from "node:child_process"
import { constants } from "node:os"
import { binaryPath } from "../index.js"

const child = spawn(binaryPath(), process.argv.slice(2), { stdio: "inherit" })
for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => child.kill(signal))
}
child.on("exit", (code, signal) => {
  if (signal) {
    process.exit(128 + constants.signals[signal])
    return
  }
  process.exit(code)
})
