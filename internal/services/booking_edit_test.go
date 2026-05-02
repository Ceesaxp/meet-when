package services

import (
	"reflect"
	"testing"
)

func TestNormalizeGuestList(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"trims and dedupes case-insensitively",
			[]string{"  alice@example.com ", "ALICE@example.com", "bob@example.com"},
			[]string{"alice@example.com", "bob@example.com"},
		},
		{"drops empty and non-email lines",
			[]string{"", "   ", "not-an-email", "carol@example.com"},
			[]string{"carol@example.com"},
		},
		{"preserves first-seen casing",
			[]string{"Dave@Example.com", "dave@example.com"},
			[]string{"Dave@Example.com"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeGuestList(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("normalizeGuestList(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestSameStringSlice(t *testing.T) {
	cases := []struct {
		a, b []string
		want bool
	}{
		{nil, nil, true},
		{[]string{}, nil, true},
		{[]string{"a"}, []string{"a"}, true},
		{[]string{"a", "b"}, []string{"a", "b"}, true},
		{[]string{"a", "b"}, []string{"b", "a"}, false}, // order matters — guests are an ordered list
		{[]string{"a"}, []string{"a", "b"}, false},
	}
	for i, tc := range cases {
		if got := sameStringSlice(tc.a, tc.b); got != tc.want {
			t.Errorf("case %d: sameStringSlice(%v, %v) = %v, want %v", i, tc.a, tc.b, got, tc.want)
		}
	}
}
