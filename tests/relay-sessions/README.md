# Recorded relay sessions

Every frame of real sessions on a changes service's relay connection, in both directions, recorded against real
Postgres by the integration tests of [protobase-changes](https://github.com/PrototeamHQ/protobase-changes) at commit
`b4375babe663968a7d611ef6a8684ba9cb0d18fb`, from its `tests/relay-sessions/`. They are copied unchanged: record them
again there and copy them over, never edit them here.

`internal/changes/changestest` replays the service's side of a session, and the tests here and in `internal/changes`
check the relay's side against it.
