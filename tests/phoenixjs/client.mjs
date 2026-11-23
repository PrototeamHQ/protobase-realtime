// Connects with the stock Phoenix client, joins a listed and an unlisted channel, and waits for one publish.
// Usage: node client.mjs <endpoint> <ticket>. Prints one JSON line per step for the Go test to read.
import { Socket } from "phoenix"

const [endpoint, ticket] = process.argv.slice(2)
const report = (step, detail) => console.log(JSON.stringify({ step, detail }))

const socket = new Socket(endpoint, { params: { ticket } })
socket.connect()

const listed = socket.channel("room", {})
listed.on("new", (payload) => {
  report("received", payload)
  socket.disconnect(() => process.exit(0))
})
listed.join()
  .receive("ok", () => {
    const unlisted = socket.channel("secret", {})
    unlisted.join()
      .receive("ok", () => report("unlisted", "ok"))
      .receive("error", (reply) => {
        unlisted.leave()
        report("unlisted", reply.reason)
        report("ready", "")
      })
  })
  .receive("error", (reply) => report("listed", reply.reason))
