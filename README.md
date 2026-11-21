# protobase-realtime

A multi-tenant realtime relay. It holds the WebSocket connections of many apps' users, so an app and its database can
sleep while people stay connected: the app talks to the relay only when something happens, and the relay fans it out.

Lasting writes, such as a sent message, never go through the relay: they go through the app's own API, and the app
publishes the result.

## Requirements

- **Multi-tenant.** One relay serves many apps. Each app has its own API keys, ticket keys, allowed origins and limits,
  and its own hub: equal channel names in two apps never meet.
- **Phoenix channels.** Clients speak the Phoenix channels V2 protocol over WebSocket, so stock Phoenix clients work.
  A join is answered from the person's channel list; clients never manage membership.
- **Tickets.** A client connects with a short-lived, single-use ticket the app issues. It names the person or an
  anonymous session, their sign-in session and the channels they belong to. The connection is the session: there is
  no token refresh, and the relay keeps no expiry timers. A socket ends when the client leaves or the app closes it.
- **App commands.** Publish one message to a channel's members (optionally narrowed to some people), add or remove
  people in channels, close the sockets of a sign-in session. All are idempotent, so the app can retry.
- **Stamped messages.** A member's message is delivered with the sender, channel and server time the relay verified;
  whatever the payload claims is carried untouched and never trusted.
- **Receive-only anonymous sessions.** An anonymous session receives but never sends.
- **Interest.** A client may say what it views in a channel; that only narrows delivery, never grants anything.
- **Nothing kept.** No history, no replay, no database, no app logic, no timers or retries on the app's behalf.

<gap>The relay cannot run as several instances sharing connections because clustering is not implemented.</gap>
<gap>Clients cannot send app-declared events checked against schemas, rates and block lists because declared events are not implemented.</gap>
<gap>The relay cannot report presence because presence is not implemented.</gap>
<gap>The relay cannot drop a half-open connection that never sends again because liveness checks are not implemented.</gap>
<gap>The relay cannot report per-app message counts because metrics are not implemented.</gap>

## Running

```sh
RELAY_APPS_FILE=apps.json RELAY_ADDR=:4000 go run ./cmd/relay
```

| Variable          | Meaning                                         |
| ----------------- | ----------------------------------------------- |
| `RELAY_APPS_FILE` | The apps file, required.                        |
| `RELAY_ADDR`      | The listen address; `:4000` when unset.         |

The apps file lists every app the relay serves; the relay reads it at startup and refuses unknown fields.

```json
{
  "apps": [
    {
      "id": "chat",
      "api_keys": ["at least 32 characters, any one is valid"],
      "ticket_keys": ["at least 32 characters, any one is valid"],
      "origins": ["chat.example.com"],
      "client_messages": false,
      "limits": { "connections": 10000, "channels_per_ticket": 200, "request_bytes": 65536 }
    }
  ]
}
```

- `id`: 1 to 64 of `a-z`, `0-9`, `_` and `-`.
- `api_keys`, `ticket_keys`: lists, so a key rotates without downtime: add the new key, move the app over, drop the old.
- `origins`: the browser origins allowed to connect, as host patterns (`*.example.com`) or `scheme://host`. The relay's
  own host is always allowed, and clients that send no `Origin`, such as native apps, are not checked.
- `client_messages`: whether signed-in members may send messages to each other (see below). Off unless set.
- `limits.connections`: open sockets for the app; `channels_per_ticket`: channels one ticket may list;
  `request_bytes`: the largest app command body.

The relay terminates no TLS: run it behind a reverse proxy that does.

### From npm

The relay is published prebuilt on npm for macOS (arm64, x64), Linux (arm64, x64) and Windows (x64), so it runs without
Go:

```sh
npm install @protobase/realtime
RELAY_APPS_FILE=apps.json npx protobase-realtime
```

`@protobase/realtime` depends on one optional package per platform, `@protobase/realtime-<os>-<arch>` with Node's
`process.platform` and `process.arch` names, and npm installs only the one that matches the machine; an install that
omits optional dependencies has no relay. A tool that starts the relay itself takes the binary's path from
`binaryPath()`:

```js
import { binaryPath } from "@protobase/realtime"
```

`protobase-realtime --version` prints the version, which is the release's.

## Tickets

A ticket is a JWT signed with HS256 by one of the app's ticket keys; any stock JWT library issues it.

| Claim       | Meaning                                                                       |
| ----------- | ----------------------------------------------------------------------------- |
| `iss`       | The app id.                                                                   |
| `sub`       | The person's id. Absent for an anonymous session.                             |
| `anonymous` | `true` for an anonymous session, which has no `sub`.                          |
| `sid`       | The sign-in session (or the anonymous session's id), as `sessions/close` names it. |
| `jti`       | A random id. Each ticket connects once.                                       |
| `exp`       | Expiry, at most 120 seconds ahead.                                            |
| `channels`  | The channels the person belongs to.                                           |

Every connection, reconnects included, needs a fresh ticket. The Phoenix client's `params` may be a function, called on
each connect: keep a fresh ticket from the app ready for it.

## App API

Each command is a `POST` with a JSON body and `Authorization: Bearer <api key>`; `{app}` is the app id. A wrong key and
an unknown app both get `401`. A refused request gets `400` (or `413` over `request_bytes`) with
`{"error": "<code>", "message": "<why>"}`. A command reports how many open sockets it reached, `{"sockets": 2}`;
reaching none is no error.

### `POST /apps/{app}/publish`

```json
{ "id": "msg-42", "channel": "room:1", "users": ["u1"], "event": "message_created", "payload": { "id": 42 }, "about": "thread:7" }
```

Sends `[null, null, channel, event, payload]`, the same bytes to every socket joined to `channel`.

- `id` (required, at most 128 bytes): a repeat of the same `id` within 10 minutes sends nothing again and answers
  `{"sockets": 0, "duplicate": true}`.
- `users` (optional): only these people's sockets. A direct message goes to a channel every ticket lists, narrowed to
  the recipient.
- `event`: 1 to 64 bytes, not `heartbeat`, `interest`, `client_message` or `phx_*`.
- `payload`: a JSON object.
- `about` (optional): only sockets that declared no interest, or declared this key.

### `POST /apps/{app}/channels/add` and `/channels/remove`

```json
{ "channel": "room:1", "users": ["u1"], "sessions": ["anon-9"] }
```

Changes the channel list of the people's (or sessions') open sockets. Removing closes the channel on sockets that
joined it with `phx_close`. Tickets issued later carry the app's own list.

### `POST /apps/{app}/sessions/close`

```json
{ "session": "s-123" }
```

Closes every socket of the sign-in session with close code `1000`, which Phoenix clients take as final, and refuses
tickets for the session for 120 seconds, so one issued before the close cannot reopen it. The person's other sessions
stay open.

## Client protocol

Connect to `wss://<relay>/socket/websocket?vsn=2.0.0&ticket=<ticket>`; with phoenix.js:

```js
const socket = new Socket("wss://relay.example.com/socket", { params: () => ({ ticket: nextTicket }) })
```

Frames are Phoenix V2 JSON text frames, `[join_ref, ref, topic, event, payload]`; binary frames close the socket.

| Client sends                         | Relay answers                                                                 |
| ------------------------------------ | ----------------------------------------------------------------------------- |
| `heartbeat` on `phoenix`             | `ok`.                                                                         |
| `phx_join` on a channel              | `ok` when the channel is in the person's list, else `error` `unauthorized`. An optional `{"viewing": [...]}` declares interest. |
| `phx_leave`                          | `ok`.                                                                         |
| `interest` `{"viewing": [...]}`      | `ok`; replaces what the socket views in that channel (at most 32 keys of 128 bytes). |
| any other event on a joined channel  | A member's message, below.                                                    |
| anything on a channel not joined     | `error` `unmatched topic`.                                                    |

The relay closes a socket with `1001` when it shuts down and `1013` when the socket falls too far behind; Phoenix
clients reconnect after both.

### Members' messages

With the app's `client_messages` on, a signed-in member's message on a joined channel reaches the channel's other
sockets, the sender's other devices included, but not the sending socket. It arrives as:

```json
[null, null, "room:1", "client_message", { "from": { "user": "u1", "connection": "Zk3..." }, "channel": "room:1", "at": 1795200000000, "event": "typing", "payload": { "...": "as sent" } }]
```

`from.user`, `channel` and `at` (milliseconds) are set by the relay; `from.connection` is an id the relay minted for
the sending socket, never the sign-in session. A refusal is an `error` reply with one of these reasons:

| Reason                     | When                                                       |
| -------------------------- | ---------------------------------------------------------- |
| `receive_only`             | The session is anonymous.                                  |
| `client_messages_disabled` | The app has not turned members' messages on.               |
| `invalid_event`            | The event is reserved or longer than 64 bytes.             |
| `too_large`                | The payload is over 4096 bytes.                            |
| `rate_limited`             | The person sent over 10 a second (bursts of 20), counted over all their sockets. |

## Development

```sh
npm ci --prefix tests/phoenixjs   # the stock Phoenix client the integration tests use
go vet ./... && go test ./...
```

Unit tests sit next to the code; `tests/` holds the integration tests, which serve the relay and connect real
WebSocket clients, phoenix.js among them.

### Releasing

Publishing a GitHub release tagged `v1.2.3` publishes the npm packages at `1.2.3` (`.github/workflows/npm.yml`); a
prerelease such as `v1.3.0-rc.1` goes under npm's `next` dist-tag. The workflow cross-compiles the relay
(`npm/pack.mjs`), installs the packages from a local registry on each platform and runs the relay npm picked
(`npm/verify-install.sh`), then publishes through npm's trusted publishing, with no stored npm token. Run by hand with
an existing tag, it publishes that tag again, skipping versions npm already has. Pull requests that touch the packaging
run every step but the publish.

npm trusts the workflow only for a package that already exists, so a new package is published once by hand, then
`.github/workflows/npm.yml` in this repository is registered as its trusted publisher in the package's settings on npm.

```sh
npm ci --prefix npm && npm test --prefix npm   # the launcher's unit tests
node npm/pack.mjs 0.0.0-dev /tmp/packages && npm/verify-install.sh /tmp/packages 0.0.0-dev
```
