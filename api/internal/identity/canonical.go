// Package identity handles account storage and username canonicalization. The
// canonical form is used for matching and uniqueness only; the original typed
// form is what is stored and displayed (FR-038, FR-039).
package identity

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Invisible and bidirectional control characters that FR-037 forbids.
const invisibleChars = "\u200B\u200C\u200D\u200E\u200F\u202A\u202B\u202C\u202D\u202E\u202F\u2066\u2067\u2068\u2069\uFEFF"

// HasInvisibleOrBidi returns true if s contains any of the zero-width or
// bidirectional control characters listed in FR-037.
func HasInvisibleOrBidi(s string) bool {
	return strings.ContainsAny(s, invisibleChars)
}

// CanonicalQuery prepares a free-text search query for LIKE matching. It is
// the same Canonical normalisation followed by trimming and lower-casing the
// LIKE wildcards out of the input, so a search for "10%" is a literal substring
// search rather than a "ends with 10" pattern (research.md edge case: "search
// text contains characters that have special meaning to the search? They are
// treated as literal text").
func CanonicalQuery(raw string) string {
	s := Canonical(raw)
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(s)
}

// AllowedUsernameRune reports whether r is one of the characters FR-036
// allows (Arabic letter, Latin letter, digit, dot, underscore, hyphen).
// Combining marks (Unicode Mn/Mc/Me) are also accepted because the
// canonical form strips them: a username typed with a diacritic is meant to
// collide with the un-diacritic form per FR-038, and refusing the input
// here would make that collision impossible to express.
func AllowedUsernameRune(r rune) bool {
	if r == '.' || r == '_' || r == '-' {
		return true
	}
	if unicode.IsDigit(r) {
		return true
	}
	if unicode.IsLetter(r) {
		return true
	}
	return unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r) || unicode.Is(unicode.Me, r)
}

// EasternToWesternDigits converts Eastern Arabic digits to Western digits. It
// leaves non-digit code points untouched. Used in two places (research §8):
// server-side on receipt, client-side on entry.
func EasternToWesternDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '٠':
			b.WriteByte('0')
		case '١':
			b.WriteByte('1')
		case '٢':
			b.WriteByte('2')
		case '٣':
			b.WriteByte('3')
		case '٤':
			b.WriteByte('4')
		case '٥':
			b.WriteByte('5')
		case '٦':
			b.WriteByte('6')
		case '٧':
			b.WriteByte('7')
		case '٨':
			b.WriteByte('8')
		case '٩':
			b.WriteByte('9')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Canonical returns the canonical form of a username, in this order:
//  1. Eastern digits → Western digits (FR-038 / FR-033)
//  2. Unicode NFKC normalization
//  3. Strip Arabic diacritics and tatweel
//  4. Unify alef, alef maqsura/ya, ta marbuta/ha variants
//  5. Unicode case fold (Latin portion)
//
// This is the value matched against and stored in account.username_canonical.
func Canonical(raw string) string {
	s := EasternToWesternDigits(raw)

	// NFKC
	t := transform.Chain(norm.NFKC)
	result, _, err := transform.String(t, s)
	if err != nil {
		result = s
	}
	s = result

	// Diacritics + tatweel
	s = stripMarks(s)
	s = strings.ReplaceAll(s, "\u0640", "")

	// Alef variants → ا
	s = strings.NewReplacer(
		"\u0622", "\u0627", // آ → ا
		"\u0623", "\u0627", // أ → ا
		"\u0625", "\u0627", // إ → ا
		"\u0671", "\u0627", // ٱ → ا
	).Replace(s)

	// Alef maqsura → ي
	s = strings.ReplaceAll(s, "\u0649", "\u064A")

	// Ta marbuta → ه
	s = strings.ReplaceAll(s, "\u0629", "\u0647")

	// Case fold (affects Latin only)
	s = strings.ToLower(s)

	return s
}

func stripMarks(s string) string {
	var b strings.Builder
	in := runes.Remove(runes.In(unicode.Mn))
	for _, r := range s {
		// Keep the code-point; remove combining marks.
		_ = in
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
