# abmon

[![CI](https://github.com/abundo/abmon/actions/workflows/ci.yml/badge.svg)](https://github.com/abundo/abmon/actions/workflows/ci.yml)

Collection of useful monitoring plugins, for nagios, naemon, icinga, dns and more - written in go

# Available checks

Command line arguments, valid for all checks:

| Argument       | Type    | Default              | Required | Description                 |
| --------       | ----    | -------              | -------- |---------------------------- |
| -h             | flag    |                      |          | Show detailed help on arguments |
| --config    -c | text    | /etc/abmon/abmon.yaml |         | Path to configuration file  |
| --unknown-as   | choice  | unknown              |          | How to return UNKNOWN       |
| --warning-as   | choice  | warning              |          | How to return WARNING       |
| --critical-as  | choice  | critical             |          | How to return CRITICAL      |
| --loglevel  -l | choice  | info                 |          | Log level                   |
| --verbose   -v | flag    | no                   |          | Show details                |
| --debug        | flag    | no                   |          | Print debug output, used mostly during development |

choice for --unknown-as/--warning-as/--critical-as is one of 'ok', 'warning', 'critical' or 'unknown'

choice for --loglevel is one of 'error', 'warning', 'info' or 'debug'


## check_dns_propagation

Passive check, running as a daemon. Listens for DNS NOTIFY messages (on UDP port 1053) from a zone's primary/hidden/dist nameservers, and when one arrives, measures how long it takes for the new SOA serial to propagate. If the zone is hosted at DnsNode, propagation to each anycast site/dist-primary is checked via the DnsNode API; otherwise each of the zone's NS records is queried directly over DNS. The result is sent to Icinga/Nagios itself (via `abmon.Notify`), it is not invoked directly by Icinga.

Zones are read from `abmon.yaml` (see `examples/abmon-example.yaml`); only zones with `dns_check_propagation: true` and `dnsnode: true` trigger a check on NOTIFY.

No additional command line arguments.

The daemon also exposes a small telnet-like console on `localhost:10023` for inspecting/controlling it at runtime (`check <zone>`, `check all`, `debug`/`undebug <all|check|dnsnode>`, `loglevel <level>`, `show clients|jobs|status|zone <name>|zones`, `help`, `quit`). Running the binary from a terminal connects to this console automatically; something needs to be listening on it too (e.g. `telnet localhost 10023`) from another session to drive it interactively.

Typically run as a systemd service, see `examples/check_dns_propagation.service`. Something must forward real DNS NOTIFY messages for the zone to this host's port 1053 (e.g. the zone's actual nameserver).


## check_file_status

Check age on file, if older than specified values, return error
Check content of file, first line should be one of OK, WARNING, CRITICAL, UNKNOWN and return it

Additional command line arguments:

| Argument       | Type    | Default  | Required | Description                 |
| --------       | ----    | -------  | -------- | --------------------------- |
| --age_warning  | number  | 48       |          | Max age on file in hours, before WARNING  |
| --age_critical | number  | 36       |          | Max age on file in hours, before CRITICAL |
| --file         | text    |          | Yes      | File to check               |

age_warning and age_critical is of type float, so fractional values can be specified

Example:

    check_file_status --statfile /tmp/file_to_check.txt  --age_critical 25


## check_http_redirect

Check that a web site implements proper redirect, and that the redirect target is correct

Additional command line arguments:

| Argument     | Type    | Default  | Required | Description                 |
| --------     | ----    | -------  | -------- | --------------------------- |
| --host    -H | text    |          | Yes      | Host to check               |
| --ipv4    -4 | flag    | no       |          | Connect using IPv4          |
| --ipv6    -6 | flag    | no       |          | Connect using IPv6          |
| --url     -U | text    |          | Yes      | URL to retrieve             |
| --redir   -R | text    |          | Yes      | Expected redirect URL       |
| --timeout -t | number  | 10       |          | Timeout waiting for a response in seconds |

Example:

    check_http_redirect --host webserver1.example.com --url http://example.com --redir https://example.com

If --host is an IPv6 address, put the IPv6 address inside []


## check_imap_message_age

Check age on oldest message in a IMAP message store. If too old, generate errors.

Additional command line arguments:

| Argument      | Type    | Default  | Required | Description                  |
| --------      | ----    | -------  | -------- | ---------------------------  |
| --host     -H | text    |          | Yes      | IMAP server                  |
| --port     -p | number  | 143      |          | Port on IMAP server, 993 if --ssl and not set |
| --ssl      -S | flag    | no       |          | Use IMAPS (default port 993) |
| --username -U | text    |          |          | IMAP account username        |
| --password -P | text    |          |          | IMAP account password        |
| --credentials | text    |          |          | .INI file with username/password, overrides --username/--password |
| --folder   -f | text    | INBOX    |          | IMAP folder to check         |
| --critical -c | number  |          | Yes      | Return CRITICAL if oldest message is older than this many seconds |
| --warning  -w | number  |          | Yes      | Return WARNING if oldest message is older than this many seconds |
| --timeout  -t | number  | 10       |          | Timeout waiting for a response in seconds |

Example:

    check_imap_message_age --host 192.168.1.10 --username username --password password --warning 10 --critical 20


## check_ldap_auth

Check that authentication against an LDAP server works, by connecting and
performing a bind with a username/password.

The password is read from the environment variable `LDAP_PASSWORD`, never
from the command line, so it doesn't leak through the process list or shell
history.

Additional command line arguments:

| Argument      | Type    | Default  | Required | Description                 |
| --------      | ----    | -------  | -------- | --------------------------- |
| --host    -H  | text    |          | Yes      | LDAP server, as an URL (ldap://host:389 or ldaps://host:636) |
| --username -u | text    |          | Yes      | LDAP bind username (DN)     |
| --timeout -t  | number  | 10       |          | Timeout waiting for a response in seconds |

Example:

    LDAP_PASSWORD=secret check_ldap_auth --host ldap://ldap.example.com:389 --username "cn=monitor,dc=example,dc=com"


## check_ntp_peers

Check that NTP is correctly syncing time towards at least one NTP server.

Additional command line arguments:

| Argument         | Type    | Default  | Required | Description                 |
| --------         | ----    | -------  | -------- | --------------------------- |
| --maxoffset -o   | Range   | 250:500  |          | Max offset in milliseconds before warning/critical. Specify as warning:critical |
| --maxjitter -j   | Range   | 250:500  |          | Max jitter in milliseconds before warning/critical. Specify as warning:critical |

Reads peer status from `ntpq -p` (requires `/usr/bin/ntpq` on PATH).

Example:

    check_ntp_peers
    

## check_radius_auth

Check that authentication against a RADIUS server works, by sending an Access-Request (PAP) with a username/password and verifying the server replies with Access-Accept.

The password is read from the environment variable `RADIUS_PASSWORD`, never from the command line, so it doesn't leak through the process list or shell history.

Additional command line arguments:

| Argument      | Type    | Default  | Required | Description                 |
| --------      | ----    | -------  | -------- | --------------------------- |
| --host    -H  | text    |          | Yes      | RADIUS server hostname or IP |
| --port    -P  | number  | 1812     |          | RADIUS server port          |
| --username -u | text    |          | Yes      | RADIUS username              |
| --secret  -s  | text    |          | Yes      | RADIUS shared secret         |
| --timeout -t  | number  | 10       |          | Timeout waiting for a response in seconds |

Example:

    RADIUS_PASSWORD=secret check_radius_auth --host radius.example.com --username testuser --secret sharedsecret


## check_rrsig_expiry

Check all RRSIGs in a zone, validating that the age is above certain limits.

Zone can either be transfered using AXFR, or read from a lical file.


Additional command line arguments:

| Argument      | Type    | Default  | Required | Description                 |
| --------      | ----    | -------  | -------- | --------------------------- |
| --host     -H | text    |          | Yes      | Host to transfer zone from  |
| --zone        | text    |          |          | zone to transfer            |
| --tsig        | text    |          |          | path to file with tsig      |
| --warning  -w | number  | 8.0      |          | Minimim age in days on RRSIG before warning |
| --critical -c | number  | 6.0      |          | Minimim age in days on RRSIG before critical |
| --zonefile    | text    |          |          | Read zone from file instead of AXFR |

warning and critical is of type float so parts of days can be specified

Example:

    check_rrsig_expiry --host ns1.example.com --zone example.com


## check_zonemaster

Check zone(s) for errors, using the tool [zonemaster](https://github.com/zonemaster/zonemaster). Zonemaster is run via `docker run zonemaster/cli` (requires `/usr/bin/docker` and network access to pull `zonemaster/cli`), so this is implemented as a passive check that sends its result to Icinga/Nagios itself instead of relying on the process exit code.

Only zones in `abmon.yaml` with `dns_check_zonemaster: true` are checked; tests can be excluded per zone with the zone's `exclude` list (either a full `module/test` name or just a module name to exclude all its tests).

Additional command line arguments:

| Argument      | Type    | Default | Required | Description                 |
| --------      | ----    | ------- | -------- | --------------------------- |
| --zone    -z  | text    |         |          | Zones to check (repeatable). If not specified, checks all zones with `dns_check_zonemaster: true` |
| --exclude -e  | text    | []      |          | Not currently used by the check itself; excludes are read per-zone from `abmon.yaml` instead |

Intended to run periodically via cron, see `examples/check_zonemaster.cron`.

Example:

    check_zonemaster --zone example.com


## check_becs_dhcp_scope

Check number of free addresses in BECS DHCP scopes. Optionally (re)writes an
icinga2 configuration file with one passive service check per scope, and
sends the current utilization to icinga as a passive check result.

Scopes are configured in `abmon.yaml`, under `dhcp_scopes` (see
`examples/abmon-example.yaml`).

> **Not yet functional:** this check does not vendor a BECS ExtAPI client -
> the SOAP operation used by the original Python check to fetch DHCP scope
> utilization is not available in this repository. See `BecsClient` in
> `cmd/check_becs_dhcp_scope/check_becs_dhcp_scope.go`.

Additional command line arguments:

| Argument             | Type    | Default | Required | Description                 |
| --------             | ----    | ------- | -------- | --------------------------- |
| --scope       -s     | text    |         |          | DHCP scopes to check (repeatable). If not specified, checks all |
| --exclude     -e     | text    |         |          | DHCP scopes to exclude (repeatable) |
| --free_warning       | number  | 20      |          | Generate WARNING if number of free addresses is below |
| --free_critical      | number  | 5       |          | Generate CRITICAL if number of free addresses is below |
| --icinga             | flag    | no      |          | Write icinga2 config file and send result as a passive check |
| --icinga_config_file | text    |         |          | Path to the generated icinga2 config file |
| --icinga_host        | text    |         |          | host.name used in the generated icinga2 'assign where' clause |

Example:

    check_becs_dhcp_scope --scope office-lan


## create_icinga_zones_conf

Not a check. Generates an Icinga2 configuration file (one `Host` and passive `Service` per DNS zone, grouped by customer) from the zones defined in `abmon.yaml`, and reloads Icinga2 (`systemctl reload icinga2.service`) if the generated file changed.

Writes to `/tmp/factum-zones.conf`, then, if it differs from `/etc/icinga2/conf.d/factum-zones.conf`, copies it there and reloads Icinga2. These paths are currently hardcoded.

No additional command line arguments.

Intended to run periodically (e.g. via cron), alongside `check_dns_propagation` and `check_zonemaster`, so newly added/removed zones get picked up by Icinga2.

Example:

    create_icinga_zones_conf


# Installation

Use the makefile to compile and install the binaries

make build
make install

Note: `check_radius_auth` and `check_ntp_peers` do not yet have Makefile build/install targets (see `DEV.md`); build them directly instead, e.g.:

    go build -o build/check_radius_auth ./cmd/check_radius_auth

Alternatively, download a prebuilt release from the [Releases page](https://github.com/abundo/abmon/releases) - each release has a `.tar.gz` per OS/architecture containing all the check binaries. Releases are built by [goreleaser](https://goreleaser.com) (config: `.goreleaser.yaml`); see `DEV.md` for how to cut one.
