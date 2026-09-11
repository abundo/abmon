module github.com/abundo/abmon

go 1.25.0

// github.com/abundo/dnsnode has not been published yet (the repo on GitHub
// is an empty scaffold). Until it is, build abmon with a checkout of
// dnsnode as a sibling directory (../dnsnode).
replace github.com/abundo/dnsnode v0.0.0 => ../dnsnode

require (
	github.com/abundo/dnsnode v0.0.0
	github.com/alecthomas/kong v1.16.1
	github.com/emersion/go-imap v1.2.1
	github.com/go-ldap/ldap/v3 v3.4.14
	github.com/mattn/go-isatty v0.0.20
	github.com/miekg/dns v1.1.61
	github.com/sirupsen/logrus v1.9.4
	gopkg.in/yaml.v2 v2.4.0
	layeh.com/radius v0.0.0-20231213012653-1006025d24f8
)

require (
	github.com/Azure/go-ntlmssp v0.1.1 // indirect
	github.com/emersion/go-sasl v0.0.0-20200509203442-7bfe0ed36a21 // indirect
	github.com/go-asn1-ber/asn1-ber v1.5.8 // indirect
	github.com/google/uuid v1.6.0 // indirect
	golang.org/x/crypto v0.54.0 // indirect
	golang.org/x/mod v0.37.0 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	golang.org/x/tools v0.47.0 // indirect
)
