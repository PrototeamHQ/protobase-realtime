# Protobase Realtime

Multi-tenant WebSocket relay for Protobase apps: server-to-client push, plus filtered, verified user-to-user events.

## How messages flow

### App to people

The app tells the relay what happened, and the relay pushes it to the sockets of a channel's members. This is for
anything users should see as it happens. For example, someone posts in a conversation: the app saves the post and
publishes it to the conversation's channel.

```http
POST /apps/chat/publish
Authorization: Bearer <api key>

{ "id": "1", "channel": "room", "event": "new", "payload": { "n": 1 } }
```

The other app commands narrow a publish to some of the members, add or remove people in channels, and close every
socket of a sign-in session when someone signs out. All are idempotent, so the app can retry. See [App API](#app-api).

### People to app

This path is not part of the relay. Lasting writes go through the app's own API, as they would without the relay: the
app checks them, stores them and publishes the result. Nothing a client sends over the socket reaches the app, and the
relay makes no request to the app except [presence reports](#presence). For example, sending a message is a request to
the app, which then publishes `message_created` to the conversation, the sender's other devices included.

### Between people

Events that only matter in the moment, such as typing or call signalling, go from person to person through the relay
without reaching the app. The app declares each event clients may send, with a JSON schema, a size cap and a rate per
person, and the relay refuses anything else. An app may let every event through instead with `allow_any`, which is not
recommended. For example, a typing event declared for `conversations/...` channels:

```json
"typing": {
  "schema": { "type": "object", "properties": { "thread": { "type": "string" } }, "required": ["thread"] },
  "max_bytes": 256, "per_second": 2, "burst": 4
}
```

- A channel event is sent on a joined channel and reaches the members joined to it, the sender's other devices
  included.
- A direct event is sent on the `relay` topic to one person the sender shares a channel with, and reaches each of their
  devices.
- The app supplies each person's block list and whether they take direct events from friends only; the relay drops
  what these rules refuse without telling the sender.
- The relay stamps each event with the sender, the channel or recipient, and the server time, all verified; whatever
  the payload claims is carried untouched and never trusted.
- An anonymous session receives but never sends.

See [Client events](#client-events).

### Presence

Presence is the one thing the relay reports to the app: a person came online, went away or went offline, combined over
their devices, debounced, batched and sent at least once. A report carries only the person, their status and when it
began, never anything a client sent. The relay rebroadcasts nothing: who hears of it is the app's to decide. For
example, once every socket of `u1` said it is away, the app gets:

```json
{ "started": 1795190000000, "changes": [{ "user": "u1", "status": "away", "at": 1795200000000 }] }
```

See [Presence reports](#presence-reports).

## Requirements

- **Multi-tenant.** One relay serves many apps. Each app has its own API keys, ticket keys, allowed origins and limits,
  and its own hub: equal channel names in two apps never meet.
- **Phoenix channels.** Clients speak the Phoenix channels V2 protocol over WebSocket, so stock Phoenix clients work.
  A join is answered from the person's channel list; clients never manage membership.
- **Tickets.** A client connects with a short-lived, single-use ticket the app issues. It names the person or an
  anonymous session, their sign-in session and the channels they belong to. The connection is the session: there is
  no token refresh, and the relay keeps no expiry timers. A socket ends when the client leaves or the app closes it.
- **Interest.** A client may say what it views in a channel; that only narrows delivery, never grants anything.
- **Nothing kept.** No history, no replay, no database, no app logic. The only timers and retries the relay runs are
  those of presence reports.

<gap>The relay cannot run as several instances sharing connections because clustering is not implemented.</gap>
<gap>The relay cannot drop a half-open connection that never sends again because liveness checks are not implemented.</gap>
<gap>The relay cannot report per-app message counts because metrics are not implemented.</gap>

## Running

```sh
PROTOBASE_REALTIME_APPS_FILE=apps.json PROTOBASE_REALTIME_ADDR=:4000 go run ./cmd/protobase-realtime
```

| Variable                        | Meaning                                                                          |
| ------------------------------- | -------------------------------------------------------------------------------- |
| `PROTOBASE_REALTIME_APPS_FILE`  | The apps file, required.                                                         |
| `PROTOBASE_REALTIME_ADDR`       | The listen address; `:4000` when unset.                                          |
| `PROTOBASE_REALTIME_SENTRY_DSN` | The DSN of an error tracker that speaks the Sentry protocol; no reporting unset. |

The apps file lists every app the relay serves; the relay reads it at startup and refuses unknown fields and a file
without an `apps` key. An empty list, `{"apps": []}`, is valid: the relay then serves no app, so it can start before
the first app exists and keep running after the last one goes.

On `SIGHUP` the relay reads the apps file again and swaps it in whole, without a restart. Apps still listed keep their
sockets, channels and presence, and take changed keys, origins, client events, presence and limits from the next event
on; a lower `limits.connections` applies to new sockets only. A removed app's sockets close with 1000. A file that fails
to load is refused: the relay keeps serving the apps it had and writes the reason to stderr.

### Health

`GET /health` answers `200` while the relay serves, with no auth and nothing about any app, for a container health
check or an uptime monitor. It checks nothing else. `started` is when the relay started, in milliseconds, so a monitor
sees a restart:

```json
{ "ok": true, "started": 1795190000000 }
```

### Error reporting

With `PROTOBASE_REALTIME_SENTRY_DSN` set, the relay reports its errors to that tracker over the Sentry protocol; a DSN
that does not parse stops startup. It reports:

- an apps file that fails to load, at startup or on `SIGHUP`;
- a panic in a socket or an app command, after which that socket or request ends and the relay serves on;
- an app's presence report that still fails after the relay waited a full minute, once until a report succeeds again.

An event holds the error, its stack, the app id as the `app` tag, the relay's version as the release, and the host and
Go runtime. It never holds a request, its headers or query string, a user, a ticket, a key or a payload.

<gap>The relay cannot keep serving after a panic in an app's presence reporter because recovering that goroutine is not implemented.</gap>

```json
{
  "apps": [
    {
      "id": "chat",
      "api_keys": ["at least 32 characters, any one is valid"],
      "ticket_keys": ["at least 32 characters, any one is valid"],
      "origins": ["chat.example.com"],
      "client_events": {
        "events": {
          "ring": { "schema": { "type": "object" }, "max_bytes": 256, "per_second": 1, "burst": 2 }
        },
        "channels": {
          "conversations": {
            "events": {
              "typing": {
                "schema": { "type": "object", "properties": { "thread": { "type": "string" } }, "required": ["thread"] },
                "max_bytes": 256, "per_second": 2, "burst": 4
              }
            }
          }
        }
      },
      "presence": { "url": "https://chat.example.com/relay/presence", "key": "at least 32 characters", "grace_seconds": 30 },
      "limits": { "connections": 10000, "channels_per_ticket": 200, "request_bytes": 65536 }
    }
  ]
}
```

- `id`: 1 to 64 of `a-z`, `0-9`, `_` and `-`.
- `api_keys`, `ticket_keys`: lists, so a key rotates without downtime: add the new key, move the app over, drop the old.
- `origins`: the browser origins allowed to connect, as host patterns (`*.example.com`) or `scheme://host`. The relay's
  own host is always allowed, and clients that send no `Origin`, such as native apps, are not checked.
- `client_events` (optional): the events signed-in clients may send each other (see below); without it, clients send
  nothing.
  - `events`: the direct events, sent on the `relay` topic, by event name: the event's `schema` (a JSON schema, draft
    2020-12 unless it names another; it may not load other documents), `max_bytes` (the payload's size, at most
    65536), and `per_second` and `burst` (how often one person may send it, over all their sockets).
  - `channels`: the channel events, by resource: the part of the channel's name before the first `/`, so an event on
    `conversations/7` is checked against `channels.conversations.events`. Each resource's `events` takes the same
    fields as above, and a person's rate is counted per resource. An event not declared for the channel's resource is
    refused, as is every event on a channel whose name holds no `/`.
  - `allow_any`: also let through every other valid event name, on any channel and as a direct event, with a 4096-byte
    cap and 10 a second (bursts of 20) per person over all undeclared events. Not recommended: the app then vouches for
    nothing its clients send.
- `presence` (optional): turns on presence reports (see below). `url` receives them, with `key` as the bearer token;
  `grace_seconds` (0 to 3600) is how long a person who closed their last socket still counts as present.
- `limits.connections`: open sockets for the app; `channels_per_ticket`: channels one ticket may list;
  `request_bytes`: the largest app command body.

The relay terminates no TLS: run it behind a reverse proxy that does.

### As a container

The `Dockerfile` builds a static relay into a minimal image that runs as a non-root user, carries CA certificates for
presence reports over https, listens on `:4000` and reads its apps file from `/etc/protobase-realtime/apps.json`:

```sh
docker pull ghcr.io/prototeamhq/protobase-realtime:1.2.3
docker run -p 4000:4000 -v "$PWD/apps.json:/etc/protobase-realtime/apps.json:ro" ghcr.io/prototeamhq/protobase-realtime:1.2.3
docker run --rm ghcr.io/prototeamhq/protobase-realtime:1.2.3 -version   # 1.2.3
```

Each release publishes the image for linux/amd64 and linux/arm64 under its version without the `v`, a prerelease
included; no tag is `latest`, so pin a version, or its digest, which the publishing run prints in its summary. To build
it yourself instead: `docker build --build-arg VERSION=1.2.3 -t protobase-realtime .`

`examples/apps.json` is a minimal apps file to try it with; replace its keys before serving anyone. The image has no
shell, so it declares no health check: probe `GET /health` from outside.

### From a release

Each GitHub release carries the relay prebuilt for Linux, macOS and Windows, so it runs without Go:

| Platform    | Asset                               |
| ----------- | ----------------------------------- |
| Linux x64   | `protobase-realtime-linux-amd64`       |
| Linux arm64 | `protobase-realtime-linux-arm64`       |
| macOS x64   | `protobase-realtime-darwin-amd64`      |
| macOS arm64 | `protobase-realtime-darwin-arm64`      |
| Windows x64 | `protobase-realtime-windows-amd64.exe` |

An asset downloads from `https://github.com/PrototeamHQ/protobase-realtime/releases/download/v<version>/<asset>`, and
`SHA256SUMS` beside it holds every asset's SHA-256 digest in `sha256sum` format:

```sh
base=https://github.com/PrototeamHQ/protobase-realtime/releases/download/v1.2.3
curl -fL -O "$base/protobase-realtime-linux-amd64" -O "$base/SHA256SUMS"
sha256sum -c --ignore-missing SHA256SUMS
chmod +x protobase-realtime-linux-amd64 && ./protobase-realtime-linux-amd64 -version   # 1.2.3
```

<gap>Release assets cannot be downloaded anonymously because public access to the repository is not enabled yet.</gap>

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
| `blocked`   | Optional: people whose events never reach this person, at most 1000.          |
| `friends_only` | Optional: `true` when this person takes direct events from `friends` only. |
| `friends`   | Optional: the people whose direct events reach a `friends_only` person, at most 1000. |

The ticket travels in the socket URL, which proxies cap in length: for long `blocked` or `friends` lists, send them
with `people/rules` once the person is connected.

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
- `channel`: 1 to 256 bytes, not `relay` or `phoenix`, the relay's and the protocol's own topics.
- `event`: 1 to 64 bytes, not `heartbeat`, `interest`, `presence`, `client_event` or `phx_*`.
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

### `POST /apps/{app}/people/rules`

```json
{ "user": "u1", "blocked": ["u7"], "friends_only": true, "friends": ["u2", "u3"] }
```

Replaces the delivery rules of the person's open sockets, as the ticket's claims of the same names set them. Tickets
issued later carry the app's own rules. Lists hold at most 1000 people.

## Presence reports

With `presence` set, the relay posts to its `url`, with `Authorization: Bearer <key>`:

```json
{ "started": 1795190000000, "changes": [{ "user": "u1", "status": "away", "at": 1795200000000 }] }
```

- A person is `online` while one of their sockets is, `away` while every socket said it is away, and `offline` once
  they had no socket for `grace_seconds`; `at` is when the status began (milliseconds). A phone that drops and
  reconnects within the grace is never reported offline.
- A status is reported once it held for 2 seconds, so a quick flap is not reported at all, and only when it differs
  from what the app last accepted. Changes are batched, at most 500 to a request.
- A report is sent until the app answers `2xx`, at least once: after a failure the relay waits twice as long as before,
  up to a minute, before it tries again. Each change carries its own `at`, so a repeat is harmless.
- `started` is when the relay started. When it changes, the relay restarted and lost what it knew: people it has not
  reported since are offline.
- Anonymous sessions have no presence.

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
| any other event on a joined channel  | A channel event, below.                                                       |
| `phx_join` on `relay`                | `ok` for a signed-in person, else `error` `receive_only`. The relay topic carries direct events and presence. |
| `presence` `{"status": "away"}` on `relay` | `ok`; `away` before the client is suspended, `online` when it is back.    |
| any other event on `relay`, `{"to": "u2", "payload": {...}}` | A direct event, below.                            |
| anything on a channel not joined     | `error` `unmatched topic`.                                                    |

The relay closes a socket with `1001` when it shuts down and `1013` when the socket falls too far behind; Phoenix
clients reconnect after both.

### Client events

A signed-in person's channel event on a joined channel reaches the channel's other joined sockets, the sender's other
devices included, but not the sending socket. A direct event on the relay topic reaches every socket of the person
named in `to` that joined the relay topic, when the sender shares a channel with them. Both arrive as:

```json
[null, null, "conversations/7", "client_event", { "from": { "user": "u1", "connection": "Zk3..." }, "channel": "conversations/7", "at": 1795200000000, "event": "typing", "payload": { "...": "as sent" } }]
[null, null, "relay", "client_event", { "from": { "user": "u1", "connection": "Zk3..." }, "to": "u2", "at": 1795200000000, "event": "ring", "payload": { "...": "as sent" } }]
```

`from.user`, `channel`, `to` and `at` (milliseconds) are set by the relay; `from.connection` is an id the relay minted
for the sending socket, never the sign-in session. Nobody whose `blocked` list names the sender gets their events,
and a `friends_only` person gets direct events from `friends` only; the sender is not told. A refusal is an `error`
reply with one of these reasons, and a refused event reaches nobody:

| Reason              | When                                                                               |
| ------------------- | ---------------------------------------------------------------------------------- |
| `receive_only`      | The session is anonymous.                                                          |
| `invalid_event`     | The event is reserved or longer than 64 bytes.                                     |
| `event_not_allowed` | The app did not declare the event for the channel's resource, or as a direct event, and does not allow any. |
| `too_large`         | The payload is over the event's `max_bytes`.                                       |
| `rate_limited`      | The person sent the event more often than its rate, counted over all their sockets. |
| `invalid_payload`   | The payload does not match the event's schema.                                     |
| `invalid_direct`    | A direct event is not `{"to": "<person>", "payload": {...}}`.                      |
| `unreachable`       | The sender shares no channel with the person, or the person has no socket open.    |

An event that fails its schema still counts against the rate, so a flood of invalid events stays cheap.

## Development

```sh
npm ci --prefix tests/phoenixjs   # the stock Phoenix client the integration tests use
go vet ./... && go test ./...
```

Unit tests sit next to the code; `tests/` holds the integration tests, which serve the relay and connect real
WebSocket clients, phoenix.js among them.


### Releasing

Cutting a release is pushing a tag `vX.Y.Z` from `main`. `.github/workflows/release.yml` then builds every asset with
`scripts/build-release.sh`, checks them against `SHA256SUMS` and creates the release `vX.Y.Z` with them attached, using
only the workflow's own token; a tag such as `v1.3.0-rc.1` makes a prerelease. The assets are
`protobase-realtime-<GOOS>-<GOARCH>` (`.exe` on Windows) for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64 and
windows/amd64, plus `SHA256SUMS`, each at
`https://github.com/PrototeamHQ/protobase-realtime/releases/download/v<version>/<asset>`. Pull requests run the same build and
check, publishing nothing.

The same tag runs `.github/workflows/image.yml`, which pushes the container image
`ghcr.io/prototeamhq/protobase-realtime:<version>` for linux/amd64 and linux/arm64 with the workflow's own token and
refuses a version already published. Running it by hand with an existing tag publishes that tag. Pull requests build
both platforms, pushing nothing.

The release build and check, as pull requests run them:

```sh
scripts/build-release.sh 0.0.0-dev /tmp/release && (cd /tmp/release && sha256sum -c SHA256SUMS)
```
