package main

// generateIcingaZonesConf does the actual file writing/reloading, so it's
// tested indirectly here by exercising the icinga2 config templates it
// renders, which is where a broken template or field rename would actually
// bite.

import (
	"strings"
	"testing"
	"text/template"

	abmon "github.com/abundo/abmon/internal"
)

func TestIcingaZoneTemplateRendersFields(t *testing.T) {
	zone := &abmon.ConfigZone{
		Name:                "example.com",
		Customer:            "acme",
		DnsNode:             true,
		DnsCheckPropagation: true,
		DnsCheckGonemaster:  false,
	}

	tmpl, err := template.New("template").Parse(icingaZoneTemplate)
	if err != nil {
		t.Fatalf("template parse error: %v", err)
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, zone); err != nil {
		t.Fatalf("template execute error: %v", err)
	}
	out := b.String()

	wantSubstrings := []string{
		`object Host "Zone - example.com"`,
		`vars.dns_zone = "example.com"`,
		`vars.dns_customer = "acme"`,
		`vars.dns_dnsnode = true`,
		`vars.dns_check_propagation = true`,
		`vars.dns_check_gonemaster = false`,
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("rendered template missing %q, got:\n%s", want, out)
		}
	}
}

func TestIcingaZoneHeaderParses(t *testing.T) {
	// the header is written verbatim (no template fields), but make sure it
	// at least defines the templates/groups the per-zone blocks rely on.
	for _, want := range []string{
		`template Host "dns-host-unmanaged"`,
		`template Service "dns-service"`,
		`ServiceGroup "DNS-propagation"`,
		`ServiceGroup "DNS-gonemaster"`,
	} {
		if !strings.Contains(icingaZoneHeader, want) {
			t.Errorf("icingaZoneHeader missing %q", want)
		}
	}
}
