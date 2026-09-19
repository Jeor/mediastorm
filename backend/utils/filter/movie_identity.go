package filter

import (
	"strconv"
	"strings"
)

// A numbered installment is a different movie even when containment gives its
// title a near-perfect score and its year falls within the release-year tolerance.
// Compare against each metadata title separately so localized titles still work.
func movieInstallmentMismatch(releaseTitle, expectedTitle string) bool {
	releaseBase, releaseNumber := movieInstallment(releaseTitle)
	expectedBase, expectedNumber := movieInstallment(expectedTitle)
	return releaseBase != "" && releaseBase == expectedBase && releaseNumber != expectedNumber
}

func movieInstallment(title string) (string, int) {
	fields := strings.Fields(normalizeForContainment(title))
	if len(fields) < 2 {
		return strings.Join(fields, " "), 0
	}
	last := fields[len(fields)-1]
	number, err := strconv.Atoi(last)
	if err != nil {
		// Restrict Roman numerals to common sequel suffixes; ordinary words
		// elsewhere in the title must not be interpreted as numbers.
		number = map[string]int{"ii": 2, "iii": 3, "iv": 4, "v": 5, "vi": 6, "vii": 7, "viii": 8, "ix": 9, "x": 10}[last]
	}
	if number <= 0 || number >= 100 {
		return strings.Join(fields, " "), 0
	}
	return strings.Join(fields[:len(fields)-1], " "), number
}
