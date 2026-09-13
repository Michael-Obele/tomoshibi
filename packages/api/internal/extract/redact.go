package extract

import "regexp"

// Documented PII patterns. Replacement tokens keep structure while
// destroying the sensitive payload. Order matters: card-shaped digit runs
// must be masked before the phone pattern (which requires separators).
var (
	emailPattern = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	// Card-shaped: 13-19 digits, optionally separated by spaces or hyphens.
	cardPattern = regexp.MustCompile(`\b(?:\d[ -]?){13,19}\b`)
	// Phone-shaped: requires at least one separator (+, (, ), ., -, space)
	// so plain digit runs are never consumed.
	phonePattern = regexp.MustCompile(`(?:\+\d[\d\s().\-]{6,}\d|\(\d{3}\)[\s.\-]*\d{3}[\s.\-]*\d{4}|\d{3}[\s.\-]+\d{3}[\s.\-]+\d{4})`)
)

const (
	emailToken = "[EMAIL]"
	phoneToken = "[PHONE]"
	cardToken  = "[CARD]"
)

// RedactPII masks emails, phone numbers, and credit-card-shaped digit runs
// in the given text. Surrounding prose is preserved.
func RedactPII(text string) string {
	if text == "" {
		return ""
	}
	text = emailPattern.ReplaceAllString(text, emailToken)
	text = cardPattern.ReplaceAllString(text, cardToken)
	text = phonePattern.ReplaceAllString(text, phoneToken)
	return text
}
