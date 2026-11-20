# protobase-realtime

A multi-tenant realtime relay for Protobase apps. It holds the apps' WebSocket connections, checks client events
against the schemas each app declares, fans out what an app publishes with its server credential, and computes
presence (online, away, offline), so a quiet app and its database can sleep while people stay connected.

Lasting writes, such as a sent message, never go through the relay: they go through the app's own API, and the app
publishes the result.
