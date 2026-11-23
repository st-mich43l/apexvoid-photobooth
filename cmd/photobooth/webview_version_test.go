package main

import "testing"

func TestValidRuntimeVersion(t *testing.T) {
	for _, tc := range []struct {
		v    string
		want bool
	}{{"0.0.0.0", false}, {"", false}, {"1.2.3", false}, {"x.2.3.4", false}, {"123.0.1.99", true}, {" 123.0.1.99 ", true}} {
		if got := validRuntimeVersion(tc.v); got != tc.want {
			t.Errorf("%q: %v", tc.v, got)
		}
	}
}
