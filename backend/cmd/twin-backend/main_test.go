package main

import "testing"

func TestStationsRejectInvalidOrDuplicateIDs(t *testing.T) {
	for _, value := range []string{"", "a,a", "a,", "a/b", "a,+"} {
		if _, err := parseStations(value); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
	got, err := parseStations("loader,printer")
	if err != nil || len(got) != 2 || got[1] != "printer" {
		t.Fatalf("%v %v", got, err)
	}
}
