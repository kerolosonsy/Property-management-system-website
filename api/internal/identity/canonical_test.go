package identity

import (
	"testing"
)

func TestHasInvisibleOrBidi(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"محمد", false},
		{"user", false},
		{"user\u200B", true},
		{"user\u200E", true},
		{"user\u202E", true},
		{"user\uFEFF", true},
		{"user\u2067", true},
	}
	for _, c := range cases {
		got := HasInvisibleOrBidi(c.in)
		if got != c.want {
			t.Errorf("HasInvisibleOrBidi(%q) = %v; want %v", c.in, got, c.want)
		}
	}
}

func TestEasternToWesternDigits(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"admin٠١٢", "admin012"},
		{"١٢٣abc٤٥٦", "123abc456"},
		{"محمد٠١", "محمد01"},
		{"", ""},
	}
	for _, c := range cases {
		got := EasternToWesternDigits(c.in)
		if got != c.want {
			t.Errorf("EasternToWesternDigits(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestCanonicalCaseInsensitive(t *testing.T) {
	if Canonical("Admin") != Canonical("admin") {
		t.Errorf("case-insensitive canonicalisation failed")
	}
}

func TestCanonicalDiacritics(t *testing.T) {
	// مَحَمّد (with fatha on mīm, shadda on dāl) vs محمّد
	if Canonical("مَحَمّد") != Canonical("محمّد") {
		t.Errorf("diacritic-insensitive canonicalisation failed")
	}
}

func TestCanonicalTatweel(t *testing.T) {
	// عـabd vs abd — tatweel stripped
	if Canonical("عـمر") != Canonical("عمر") {
		t.Errorf("tatweel-insensitive canonicalisation failed")
	}
}

func TestCanonicalAlefVariants(t *testing.T) {
	alefs := []string{"أحمد", "إحمد", "آحمد", "احمد"}
	base := Canonical("احمد")
	for _, a := range alefs {
		if Canonical(a) != base {
			t.Errorf("alef-variant canonicalisation failed: %q != %q", Canonical(a), base)
		}
	}
}

func TestCanonicalYaAndTaMarbuta(t *testing.T) {
	if Canonical("موسى") != Canonical("موسي") {
		t.Errorf("alef maqsura canonicalisation failed")
	}
	if Canonical("مدرسة") != Canonical("مدرسه") {
		t.Errorf("ta marbuta canonicalisation failed")
	}
}

func TestCanonicalEasternDigits(t *testing.T) {
	// Eastern digits converted before case/letter rules.
	if Canonical("admin٠١٢") != Canonical("admin012") {
		t.Errorf("Eastern-digit canonicalisation failed")
	}
}

func TestCanonicalPreservesLatin(t *testing.T) {
	// Mixed Arabic + Latin — Latin case folded but Arabic letters untouched.
	got := Canonical("User_01")
	want := "user_01"
	if got != want {
		t.Errorf("Canonical(%q) = %q; want %q", "User_01", got, want)
	}
}
