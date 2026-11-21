#!/usr/bin/env bash
# Installs @protobase/realtime from the tarballs npm/pack.mjs packed, as a user would from npm, and checks that the
# relay it resolves for this machine runs and reports the version:
#   npm/verify-install.sh <packed dir> <version>
# The tarballs are published to a throwaway local registry, so npm picks the platform package by its os and cpu.
set -euo pipefail

packed=$(cd "$1" && pwd)
version=$2
work=$(mktemp -d)
registry=http://127.0.0.1:4873/

cat > "$work/config.yaml" <<YAML
storage: ./storage
auth: { htpasswd: { file: ./htpasswd } }
uplinks: {}
packages:
  '**': { access: \$all, publish: \$all }
log: { type: stdout, format: pretty, level: error }
YAML
# Installed and started by its own path, so the process this script stops is the registry itself.
npm install --prefix "$work/registry" verdaccio@7.1.2 --no-audit --no-fund --loglevel error
node "$work/registry/node_modules/verdaccio/bin/verdaccio" --config "$work/config.yaml" --listen 127.0.0.1:4873 \
  > "$work/registry.log" 2>&1 &
registry_pid=$!
trap 'kill "$registry_pid"; cd /; rm -rf "$work"' EXIT
for _ in $(seq 60); do
  curl -sf "${registry}-/ping" > /dev/null && break
  sleep 1
done
curl -sf "${registry}-/ping" > /dev/null

export npm_config_registry=$registry npm_config_userconfig=$work/npmrc
# A throwaway user of the throwaway registry, which publishes only with a token it issued.
token=$(curl -sf -X PUT -H 'content-type: application/json' -d '{"name":"local","password":"local-registry"}' \
  "${registry}-/user/org.couchdb.user:local" | node -p 'JSON.parse(require("fs").readFileSync(0)).token')
echo "//127.0.0.1:4873/:_authToken=$token" > "$work/npmrc"
for file in $(node -e 'for (const p of require(process.argv[1])) console.log(p.file)' "$packed/packages.json"); do
  npm publish "$packed/$file" --access public --tag local --loglevel error
done

mkdir "$work/app"
cd "$work/app"
npm init -y > /dev/null
npm install "@protobase/realtime@$version" --no-audit --no-fund
reported=$(npx protobase-realtime --version)
if [ "$reported" != "$version" ]; then
  echo "The relay reported version $reported, not $version" >&2
  exit 1
fi
node --input-type=module -e 'import { binaryPath } from "@protobase/realtime"; console.log(binaryPath())'
