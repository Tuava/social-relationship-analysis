package domain

import "testing"

func TestCleanDisplayText(t *testing.T) {
	for input, want := range map[string]string{
		"<$\u00ff\u0100\x11\x10>Wind Garden": "Wind Garden",
		"  normal  name  ":                   "normal name",
		"line\tname":                         "line name",
		"&quot;hello&amp;world&quot;":        "\"hello&world\"",
		"[em]e100[/em] test":                 "[表情] test",
		"\u2062hidden":                       "hidden",
	} {
		if got := CleanDisplayText(input); got != want {
			t.Fatalf("CleanDisplayText(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestIsOpaqueIdentifier(t *testing.T) {
	if !IsOpaqueIdentifier("03e9a8e9-8796-4b78-a17f-17496c5098dc") {
		t.Fatal("expected UUID to be opaque")
	}
	if IsOpaqueIdentifier("10000001") || IsOpaqueIdentifier("Wind Garden") {
		t.Fatal("normal display values must not be opaque")
	}
}
