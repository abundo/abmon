package main

// ZonemasterCheck itself shells out to docker, which isn't practical to
// exercise from a unit test. What's testable in isolation is the zonemaster
// JSON result parsing and the level map it's checked against.

import (
	"encoding/json"
	"testing"
)

func TestStrToLevelKnownLevels(t *testing.T) {
	cases := map[string]int{
		"NOTICE":  0,
		"WARNING": 1,
		"ERROR":   2,
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
	// the check keeps the highest level seen, so NOTICE < WARNING < ERROR
	// must hold for that "max" logic to pick the worst result.
	if !(StrToLevel["NOTICE"] < StrToLevel["WARNING"] && StrToLevel["WARNING"] < StrToLevel["ERROR"]) {
		t.Fatalf("StrToLevel ordering broken: %+v", StrToLevel)
	}
}

func TestZonemasterResultUnmarshal(t *testing.T) {
	payload := `{
		"results": [
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
		]
	}`

	var result ZonemasterResult
	if err := json.Unmarshal([]byte(payload), &result); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if len(result.Results) != 2 {
		t.Fatalf("len(Results) = %d, want 2", len(result.Results))
	}
	first := result.Results[0]
	if first.Level != "WARNING" || first.Module != "CONNECTIVITY" || first.Tag != "IPV4_ENABLED" {
		t.Errorf("first result = %+v, unexpected fields", first)
	}
	if first.Args["ns"] != "ns1.example.com" {
		t.Errorf("first.Args[ns] = %v, want ns1.example.com", first.Args["ns"])
	}
	second := result.Results[1]
	if second.Level != "NOTICE" || len(second.Args) != 0 {
		t.Errorf("second result = %+v, want NOTICE with no args", second)
	}
}

func TestZonemasterResultUnmarshalEmpty(t *testing.T) {
	var result ZonemasterResult
	if err := json.Unmarshal([]byte(`{"results": []}`), &result); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if len(result.Results) != 0 {
		t.Fatalf("len(Results) = %d, want 0", len(result.Results))
	}
}
