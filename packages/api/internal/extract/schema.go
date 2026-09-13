// Package extract provides deterministic, LLM-free content intelligence:
// selector-based JSON extraction, extractive summaries, and PII redaction.
package extract

import (
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/Michael-Obele/tomoshibi/internal/domain"
)

// Apply evaluates an extraction schema against raw HTML using goquery.
// Field values come from the element's text (default), html, or a named
// attribute. Missing selectors omit the field; Multiple collects arrays.
func Apply(htmlBody string, schema map[string]domain.ExtractField) (map[string]any, error) {
	if len(schema) == 0 {
		return nil, nil
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlBody))
	if err != nil {
		return nil, fmt.Errorf("extraction failed to parse html: %w", err)
	}

	out := make(map[string]any, len(schema))
	for fieldName, spec := range schema {
		if spec.Selector == "" {
			continue
		}
		sel := doc.Find(spec.Selector)
		if sel.Length() == 0 {
			continue // missing selector → field omitted
		}

		value := func(s *goquery.Selection) string {
			switch spec.Attr {
			case "html":
				h, _ := goquery.OuterHtml(s)
				return h
			case "text", "":
				return strings.TrimSpace(s.Text())
			default:
				v, _ := s.Attr(spec.Attr)
				return v
			}
		}

		if spec.Multiple {
			items := make([]string, 0, sel.Length())
			sel.Each(func(_ int, s *goquery.Selection) {
				if v := value(s); v != "" {
					items = append(items, v)
				}
			})
			out[fieldName] = items
		} else {
			out[fieldName] = value(sel.First())
		}
	}
	return out, nil
}
