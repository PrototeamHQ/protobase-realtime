// The platforms the relay is published for, each as its own optional package that npm installs only where its os and
// cpu match. os and cpu are Node's process.platform and process.arch; goos and goarch the Go build target.
export const platforms = [
  { os: "darwin", cpu: "arm64", goos: "darwin", goarch: "arm64" },
  { os: "darwin", cpu: "x64", goos: "darwin", goarch: "amd64" },
  { os: "linux", cpu: "arm64", goos: "linux", goarch: "arm64" },
  { os: "linux", cpu: "x64", goos: "linux", goarch: "amd64" },
  { os: "win32", cpu: "x64", goos: "windows", goarch: "amd64" },
]

export function packageName(platform) {
  return `@protobase/realtime-${platform.os}-${platform.cpu}`
}

export function binaryName(platform) {
  return platform.os === "win32" ? "relay.exe" : "relay"
}

export function platformFor(os, cpu) {
  const platform = platforms.find((p) => p.os === os && p.cpu === cpu)
  if (!platform) {
    const supported = platforms.map((p) => `${p.os}-${p.cpu}`).join(", ")
    throw new Error(`@protobase/realtime has no relay for ${os}-${cpu}; it supports ${supported}`)
  }
  return platform
}
