# @protobase/realtime

The protobase-realtime relay, prebuilt for macOS (arm64, x64), Linux (arm64, x64) and Windows (x64). npm installs the
binary for the machine from the matching optional package, `@protobase/realtime-<os>-<arch>`.

```sh
RELAY_APPS_FILE=apps.json npx protobase-realtime
```

```js
import { binaryPath } from "@protobase/realtime"
```

`binaryPath()` is the path of the relay binary, for tools that start it themselves. The relay's configuration and API
are described in the repository's README.
