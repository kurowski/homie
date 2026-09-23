# Contributing

Contributions are welcome. You need Go (the version in `go.mod` or
newer) and `git`; nothing else runs for the unit suite.

```sh
go build ./... && go vet ./... && go test ./...
```

- **Adding a distro or a package manager:** the
  [contributing page](https://homie.sh/docs/contributing/) walks
  through both.
- **How the code is put together, and why:** [`CLAUDE.md`](CLAUDE.md).
- **Before tagging a release:** `make e2e`, which needs Docker.
- **New to the codebase?** This repository has a
  [Rolling Start](https://rollingstart.dev) map in `.rolling/`, and
  Claude Code can teach you the code from it, one task at a time, on
  your own checkout. In Claude Code, in this directory:

  ```
  /plugin marketplace add kurowski/rollingstart
  /plugin install rolling@rollingstart
  /clear
  /rolling:start
  ```

  The `/clear` starts a fresh session, so the plugin's hooks run
  before you begin.
