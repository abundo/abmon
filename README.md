# monitoring_plugins_go

Collection of useful monitoring plugins, for nagios, naemon, icinga etc written in go

# Available checks

Command line arguments, valid for all checks:

| Argument      | Type    | Default  | Required | Description                 |
| --------      | ----    | -------  | -------- |---------------------------- |
| -h            | flag    |          |          | Show detailed help on arguments |
| --unknown_as  | choice  | unknown  |          | How to return UNKNOWN       |
| --warning_as  | choice  | warning  |          | How to return WARNING       |
| --critical_as | choice  | critical |          | How to return CRITICAL      |
| --verbose  -v | flag    | no       |          | Show details                |
| --debug       | flag    | no       |          | Print debug output, used mostly during development |

choice is one of 'ok', 'warning', 'critical' or 'unknown'


## check_file_status

Check age on file, if older than specified values, return error
Check content of file, first line should be one of OK, WARNING, CRITICAL, UNKNOWN and return it

Additional command line arguments:

| Argument       | Type    | Default  | Required | Description                 |
| --------       | ----    | -------  | -------- | --------------------------- |
| --file         | text    |          | Yes      | File to check               |
| --age_warning  | number  | 48       |          | Max age on file in hours, before WARNING  |
| --age_critical | number  | 36       |          | Max age on file in hours, before CRITICAL |

age_warning and age_critical is of type float, so fractional values can be specified

Example:

    check_file_status --statfile /tmp/file_to_check.txt  --age_critical 25


## check_http_redirect

Check that a web site implements proper redirect, and that the redirect target is correct

Additional command line arguments:

| Argument     | Type    | Default  | Required | Description                 |
| --------     | ----    | -------  | -------- | --------------------------- |
| --host    -H | text    |          |          | Host to check               |
| --ipv4    -4 | flag    | no       |          | Use IPv4 connection         |
| --ipv6    -6 | flag    | no       |          | Use IPv6 connection         |
| --url     -U | text    |          | Yes      | URL to retrieve             |
| --redir   -R | text    |          | Yes      | Expected redirect URL       |
| --timeout -t | number  | 10       |          | Timeout waiting for a response in seconds |

Example:

    check_http_redirect.py --host webserver1.example.com --url http://example.com --redir https://example.com

If --host is an IPv6 address, put the IPv6 address inside []


## check_imap_message_age

Check age on messages on a IMAP message store. If too old, generate errors.

Additional command line arguments:

| Argument      | Type    | Default  | Required | Description                 |
| --------      | ----    | -------  | -------- | --------------------------- |
| --host     -H | text    |          | Yes      | IMAP server                 |
| --port     -p | number  | 443      |          | Port on IMAP server         |
| --ssl      -S | flag    | no       |          | Use IMAPS (default port 993) |
| --username -U | text    |          | Yes      | IMAP account username       |
| --password -P | text    |          | Yes      | IMAP account password       |
| --folder   -f | text    | INBOX    |          | IMAP folder to check        |
| --timeout  -t | number  | 10       |          | Timeout waiting for a response in seconds |

Example:

    check_imap_message_age.py


## check_ntp_peers

Check that NTP is correctly syncing time towards at least one NTP server.

Additional command line arguments:

| Argument      | Type    | Default  | Required | Description                 |
| --------      | ----    | -------  | -------- | --------------------------- |
| --max_offset  | Range   | 250:500  |          | Max offset in milliseconds before warning/critical. Specify as warning:critical |
| --max_jitter  | Range   | 250:500  |          | Max offset in milliseconds before warning/critical. Specify as warning:critical |

Example:

    check_ntp_peers.py
    

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

    check_rrsig_expiry.py --host ns1.example.com --zone example.com


## check_zonemaster

Check zone(s) for errors, using the tool zonemaster.


# Installation

Use the makefile to compile and install the binaries

make build
make install
