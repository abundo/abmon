package main

// GonemasterCheck itself shells out to the gonemaster binary, which isn't
// practical to exercise from a unit test. What's testable in isolation is
// the gonemaster JSON result parsing and the level map it's checked against.

import (
	"encoding/json"
	"testing"
)

func TestStrToLevelKnownLevels(t *testing.T) {
	cases := map[string]int{
		"INFO":     0,
		"NOTICE":   1,
		"WARNING":  2,
		"ERROR":    3,
		"CRITICAL": 4,
	}
	for level, want := range cases {
		got, ok := StrToLevel[level]
		if !ok {
			t.Errorf("StrToLevel missing entry for %q", level)
			continue
		}
		if got != want {
			t.Errorf("StrToLevel[%q] = %d, want %d", level, got, want)
		}
	}
}

func TestStrToLevelOrdering(t *testing.T) {
	// the check keeps the highest level seen, so INFO < NOTICE < WARNING <
	// ERROR < CRITICAL must hold for that "max" logic to pick the worst result.
	if !(StrToLevel["INFO"] < StrToLevel["NOTICE"] &&
		StrToLevel["NOTICE"] < StrToLevel["WARNING"] &&
		StrToLevel["WARNING"] < StrToLevel["ERROR"] &&
		StrToLevel["ERROR"] < StrToLevel["CRITICAL"]) {
		t.Fatalf("StrToLevel ordering broken: %+v", StrToLevel)
	}
}

func TestGonemasterResultUnmarshal(t *testing.T) {
	payload := `[
		{
			"level": "WARNING",
			"message": "Nameserver reply without EDNS0",
			"module": "CONNECTIVITY",
			"tag": "IPV4_ENABLED",
			"testcase": "connectivity01",
			"timestamp": 1.5,
			"args": {"ns": "ns1.example.com"}
		},
		{
			"level": "NOTICE",
			"message": "All good",
			"module": "DNSSEC",
			"tag": "SOME_TAG",
			"testcase": "dnssec01",
			"timestamp": 2.25
		}
	]`

	var results []GonemasterResultTest
	if err := json.Unmarshal([]byte(payload), &results); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	first := results[0]
	if first.Level != "WARNING" || first.Module != "CONNECTIVITY" || first.Tag != "IPV4_ENABLED" {
		t.Errorf("first result = %+v, unexpected fields", first)
	}
	if first.Args["ns"] != "ns1.example.com" {
		t.Errorf("first.Args[ns] = %v, want ns1.example.com", first.Args["ns"])
	}
	second := results[1]
	if second.Level != "NOTICE" || len(second.Args) != 0 {
		t.Errorf("second result = %+v, want NOTICE with no args", second)
	}
}

func TestLevelToReturnCode(t *testing.T) {
	cases := map[string]int{
		"INFO":     0,
		"NOTICE":   0,
		"WARNING":  1,
		"ERROR":    2,
		"CRITICAL": 2,
	}
	for level, want := range cases {
		got, ok := levelToReturnCode[level]
		if !ok {
			t.Errorf("levelToReturnCode missing entry for %q", level)
			continue
		}
		if got != want {
			t.Errorf("levelToReturnCode[%q] = %d, want %d", level, got, want)
		}
	}
	// nagios/icinga only understand 0=OK, 1=WARNING, 2=CRITICAL as a plugin
	// return code; gonemaster's ERROR/CRITICAL must not leak through as 3/4.
	for level, code := range levelToReturnCode {
		if code < 0 || code > 2 {
			t.Errorf("levelToReturnCode[%q] = %d, out of nagios return code range", level, code)
		}
	}
}

func TestGonemasterResultUnmarshalEmpty(t *testing.T) {
	var results []GonemasterResultTest
	if err := json.Unmarshal([]byte(`[]`), &results); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("len(results) = %d, want 0", len(results))
	}
}
