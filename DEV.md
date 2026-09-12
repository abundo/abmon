# Development

## Prerequisites

- Go 1.25+
- A sibling checkout of `github.com/abundo/dnsnode` next to this repo, i.e. at `../dnsnode`. The published module is currently just an empty scaffold, so `go.mod` points at the local checkout via a `replace` directive:

      replace github.com/abundo/dnsnode v0.0.0 => ../dnsnode

- `gonemaster` (https://codeberg.org/pawal/gonemaster), only needed to run `check_gonemaster` (it shells out to the `gonemaster` binary; no docker/perl dependencies, unlike its predecessor zonemaster).
- `ntpq` (from the ntp/chrony tooling), only needed to run `check_ntp_peers`.

## Building

    make build      # builds all checks with a Makefile target into build/
    make install    # installs those binaries to /usr/local/bin

## Project layout

- `cmd/` - one directory per binary. `cmd/cmd_base.go` holds `cmdbase.Run`, a shared `main()` wrapper (see below).
- `internal/` - shared code: config file loading (`ConfigFile`/`ConfigZone`), `MonitoringCheck` (CLI parsing, nagios/icinga return codes, `Notify`/`Exit`), `Range` (nagios-style `warning:critical` range parsing), misc utilities in `util.go`.
- `examples/` - example `abmon.yaml`, plus the systemd/cron unit files used by the checks that run as a daemon or from cron (`check_dns_propagation.service`, `check_gonemaster.cron`).

## Check conventions

- Every check's `Opts` struct embeds `abmon.CheckOpts` (see `internal/check.go`) for the common flags (`--config`, `--debug`, `--verbose`, `--unknown-as`, `--warning-as`, `--critical-as`, `--loglevel`), calls `kong.Parse(&opts, ...)` itself to parse CLI args, then `abmon.NewCheck(opts.CheckOpts)` to set up logging/return-code remapping and load the config file. (CLI parsing is a `kong.Parse` call per binary rather than centralized in `NewCheck`, because kong parses one concrete struct via compile-time reflection - unlike the old `go-flags`-based `NewCheck`, it can't merge an arbitrary `interface{}` of extra flags into the common ones at runtime.)
- Two `main()` patterns exist side by side:
  - Newer checks (`check_radius_auth`, `check_ldap_auth`, `check_http_redirect`, `check_imap_message_age`, `check_file_status`, `check_becs_dhcp_scope`) call `cmdbase.Run(func() error { ... })`, so a runtime error prints a single `Error: ...` line to stderr and exits 1, instead of also dumping usage/help. Prefer this pattern for new checks.
  - Older checks (`check_dns_propagation`, `check_ntp_peers`, `check_gonemaster`) call `check.Exit(status, msg)` directly, or `os.Exit`. `check.Exit` prints the nagios/icinga-formatted result line (`OK ...`, `WARNING ...`, etc, with perfdata and details) and exits with the matching status code.
- Passive checks (`check_dns_propagation`, `check_gonemaster`) send their result to Icinga/Nagios themselves via `abmon.Notify`, instead of relying on the process exit code - they aren't invoked directly by Icinga, so their exit code doesn't matter to it.

## Testing

    go test ./...
    go test -race ./...

Every check has a test file. Two patterns are used, depending on what the check's `main()` does:

- **In-process unit tests** for pure logic that doesn't call `check.Exit`/`os.Exit` - e.g. `ParsePeer` in `check_ntp_peers`, `loadCredentials` in `check_imap_message_age`, the `becsClient` JSON-RPC logic and `writeIcingaConfig` in `check_becs_dhcp_scope`, `CLI()` in `check_dns_propagation` (driven over a `net.Pipe`), and the icinga2 config templates in `check_dns_propagation`'s `generateIcingaZonesConf`.
- **Black-box subprocess tests** for checks whose logic terminates the process via `check.Exit`: a `TestMain` builds the check's binary once into a temp dir, and tests run it with different arguments/env vars (using `httptest` servers or closed local ports to fake the remote service), asserting on stdout and the exit code. `cmd/cmd_base_run_test.go` and `internal/check_test.go` (`TestCheckExit*`) use a lighter variant of this - re-exec the test binary itself with an env var switch - for testing `cmdbase.Run`/`MonitoringCheck.Exit` directly.

`check_gonemaster` (shells out to the `gonemaster` binary) and `check_rrsig_expiry`'s zone transfer, and `check_dns_propagation`'s live DNS/telnet servers, aren't exercised end-to-end for that reason; only their pure logic (JSON parsing, the unknown-zone error path, `setSitePropagation`/`debugFlagsToString`) is covered.

`abmon.ReloadIcinga` (in `internal/check.go`) is a package-level `var`, not a `func`, specifically so tests that exercise `writeIcingaConfig`/`generateIcingaZonesConf` can stub it out instead of actually shelling out to `systemctl reload icinga2.service`.

## CI/CD

`.github/workflows/ci.yml` runs on every push/PR: checks out this repo and a sibling checkout of `github.com/abundo/dnsnode` (see Prerequisites above), then runs `gofmt -l`, `go vet`, `go build` and `go test -race`.

`.github/workflows/release.yml` runs on `v*` tags: it checks out both repos the same way, then runs [goreleaser](https://goreleaser.com) (config: `.goreleaser.yaml`) to cross-build every `cmd/` binary for linux/darwin amd64/arm64, and publishes a GitHub Release with one tarball per OS/arch (all check binaries bundled together, plus `README.md`/`DEV.md`/`examples/`) and a `checksums.txt`.

To cut a release: `git tag v0.1.0 && git push origin v0.1.0`.

To test the goreleaser config locally without publishing: `goreleaser release --snapshot --clean`.

## Adding a new check

1. Create `cmd/check_x/check_x.go`.
2. Define an `Opts` struct embedding `abmon.CheckOpts` plus its extra flags (kong struct tags: `help`, `short`, `required`, `default`, `enum`, `name` to override the kebab-cased default flag name). In `main()`, call `kong.Parse(&opts, kong.Name(...), kong.Description(...))`, then `abmon.NewCheck(opts.CheckOpts)`.
3. Add a build (and install) target to `Makefile`.
4. Add a `builds:` entry for it in `.goreleaser.yaml`.
5. Add tests - see Testing above for which pattern fits.
6. Document it in `README.md`: what it checks, its additional CLI arguments, and an example invocation.
