package engines

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// parseTimeLayouts covers the pubDate shapes seen in RSS feeds we use
// (RFC1123 with GMT, RFC1123Z, RFC822Z).
var parseTimeLayouts = []string{
	time.RFC1123Z,
	time.RFC1123,
	time.RFC822Z,
	time.RFC822,
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"2006-01-02T15:04:05Z07:00",
}

// parse dispatches on spec.Type. field values are extracted per-field and
// assembled into Results; items missing a title or URL are dropped.
func parse(body []byte, spec ParseSpec) ([]Result, error) {
	var items []map[string]string
	var err error
	switch spec.Type {
	case "css":
		items, err = parseCSS(body, spec)
	case "rss":
		items, err = parseRSS(body, spec)
	case "json":
		items, err = parseJSON(body, spec)
	default:
		err = fmt.Errorf("unknown parse type %q", spec.Type)
	}
	if err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(items))
	for _, m := range items {
		title := strings.TrimSpace(m["title"])
		rawURL := strings.TrimSpace(m["url"])
		if title == "" || rawURL == "" {
			continue
		}
		r := Result{Title: title, URL: rawURL, Description: strings.TrimSpace(m["content"])}
		if p := strings.TrimSpace(m["published"]); p != "" {
			if t, ok := parseTime(p); ok {
				r.Published = &t
			}
		}
		out = append(out, r)
	}
	return out, nil
}

func parseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range parseTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// --- css -------------------------------------------------------------------

func parseCSS(body []byte, spec ParseSpec) ([]map[string]string, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
	var out []map[string]string
	doc.Find(spec.Item).Each(func(_ int, node *goquery.Selection) {
		m := make(map[string]string, len(spec.Fields))
		for key, f := range spec.Fields {
			m[key] = cssField(node, f)
		}
		out = append(out, m)
	})
	return out, nil
}

func cssField(node *goquery.Selection, f FieldSpec) string {
	sel := node
	if f.Sel != "" {
		sel = node.Find(f.Sel).First()
	}
	if sel.Length() == 0 {
		return ""
	}
	if f.Exclude != "" {
		sel = sel.Clone()
		sel.Find(f.Exclude).Remove()
	}
	var raw string
	switch {
	case f.Attr == "" || f.Attr == "text":
		raw = sel.Text()
	case f.Attr == "html":
		h, _ := sel.Html()
		raw = h
	default:
		v, _ := sel.Attr(f.Attr)
		raw = v
	}
	raw = strings.TrimSpace(raw)
	if f.URLParam != "" {
		raw = decodeRedirect(raw, f.URLParam)
	}
	return raw
}

// decodeRedirect unwraps engine redirectors into the target URL:
//   - "uddg": //duckduckgo.com/l/?uddg=<pct-encoded target>
//   - "q":    /url?q=<pct-encoded target> (Google)
//   - "url":  .../apiclick.aspx?...&url=<pct-encoded target> (Bing News)
//   - "bing_u": https://www.bing.com/ck/a?...&u=a1<base64 target>
//
// Non-redirector values (already-direct URLs) pass through unchanged.
func decodeRedirect(raw, kind string) string {
	if raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if kind == "bing_u" {
		v := u.Query().Get("u")
		v = strings.TrimPrefix(v, "a1")
		v = strings.TrimRight(v, "=") // some results carry std padding
		if v == "" {
			return raw
		}
		// Bing mixes URL-safe and std alphabets across results.
		dec, err := base64.RawStdEncoding.DecodeString(v)
		if err != nil {
			dec, err = base64.RawURLEncoding.DecodeString(v)
			if err != nil {
				return raw
			}
		}
		target := string(dec)
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			return target
		}
		return "" // bing-internal link (e.g. /images/search?...) — drop
	}
	target := u.Query().Get(kind)
	if target == "" {
		return raw
	}
	if _, err := url.Parse(target); err != nil {
		return raw
	}
	return target
}

// --- rss -------------------------------------------------------------------

// rssDoc models RSS 2.0 (channel/item). Atom entry feeds are not used by
// the current roster.
type rssDoc struct {
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
	Items []rssItem `xml:"item"` // some feeds put items at the root
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	DCDate      string `xml:"date"` // dc:date fallback
	Content     string `xml:"encoded"`
}

func parseRSS(body []byte, spec ParseSpec) ([]map[string]string, error) {
	var doc rssDoc
	dec := xml.NewDecoder(bytes.NewReader(body))
	dec.Strict = false // feeds in the wild mis-escape ampersands
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse rss: %w", err)
	}
	items := doc.Channel.Items
	if len(items) == 0 {
		items = doc.Items
	}
	// rss field paths are element names, lowercased.
	elems := make([]map[string]string, 0, len(items))
	for _, it := range items {
		elems = append(elems, map[string]string{
			"title":       it.Title,
			"link":        it.Link,
			"description": it.Description,
			"pubdate":     firstNonEmpty(it.PubDate, it.DCDate),
			"content":     it.Content,
		})
	}
	return project(elems, spec.Fields), nil
}

// project applies field specs (path / template / urlparam / join) to
// already-string maps — shared by rss and by json once values stringify.
func project(items []map[string]string, fields map[string]FieldSpec) []map[string]string {
	out := make([]map[string]string, 0, len(items))
	for _, m := range items {
		row := make(map[string]string, len(fields))
		for key, f := range fields {
			v := ""
			if f.Path != "" {
				v = m[f.Path] // json paths keep their case (objectID, ...)
				if v == "" {
					v = m[strings.ToLower(f.Path)] // rss element names fold
				}
			}
			if v == "" && f.OrTemplate != "" {
				v = renderTemplate(f.OrTemplate, m, f.Encode)
			}
			if v == "" && f.Template != "" {
				v = renderTemplate(f.Template, m, f.Encode)
			}
			if f.StripHTML && v != "" {
				v = stripHTML(v)
			}
			if f.URLParam != "" && v != "" {
				v = decodeRedirect(v, f.URLParam)
			}
			row[key] = strings.TrimSpace(v)
		}
		out = append(out, row)
	}
	return out
}

// --- json ------------------------------------------------------------------

func parseJSON(body []byte, spec ParseSpec) ([]map[string]string, error) {
	var doc any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}
	var raw []any
	if spec.Item == "" {
		arr, ok := doc.([]any)
		if !ok {
			return nil, fmt.Errorf("json root is not an array")
		}
		raw = arr
	} else {
		v := dive(doc, spec.Item)
		arr, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("json path %q is not an array", spec.Item)
		}
		raw = arr
	}
	items := make([]map[string]string, 0, len(raw))
	for _, it := range raw {
		items = append(items, jsonFlatten(it))
	}
	return project(items, spec.Fields), nil
}

// jsonFlatten flattens one JSON item into dot-paths → string. Arrays of
// {key: string} objects are additionally exposed as joined text (mwmbl's
// segmented titles/descriptions).
func jsonFlatten(v any) map[string]string {
	m := map[string]string{}
	flattenInto(v, "", m)
	return m
}

func flattenInto(v any, prefix string, out map[string]string) {
	switch t := v.(type) {
	case map[string]any:
		for k, vv := range t {
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			flattenInto(vv, p, out)
		}
	case []any:
		// Join array-of-{value} / array-of-scalars into readable text.
		var sb strings.Builder
		for _, vv := range t {
			switch e := vv.(type) {
			case map[string]any:
				if s, ok := e["value"].(string); ok {
					sb.WriteString(s)
					continue
				}
				// Nested object in array: expose first string field.
				for _, ev := range e {
					if s, ok := ev.(string); ok && s != "" {
						sb.WriteString(s)
						break
					}
				}
			case string:
				sb.WriteString(e)
			}
		}
		if sb.Len() > 0 {
			out[prefix] = sb.String()
		}
	default:
		if prefix == "" {
			return
		}
		out[prefix] = stringify(v)
	}
}

func stringify(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

// dive walks a dot-path through nested JSON objects.
func dive(v any, path string) any {
	cur := v
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[part]
	}
	return cur
}

// renderTemplate substitutes {{field}} placeholders (dot-paths) from item
// values. encode applies path-safe escaping (spaces and # only) so wiki
// titles survive a path segment while package scopes keep their slashes.
func renderTemplate(tpl string, item map[string]string, encode bool) string {
	var sb strings.Builder
	for {
		open := strings.Index(tpl, "{{")
		if open < 0 {
			sb.WriteString(tpl)
			break
		}
		sb.WriteString(tpl[:open])
		close := strings.Index(tpl[open:], "}}")
		if close < 0 {
			sb.WriteString(tpl[open:])
			break
		}
		name := strings.TrimSpace(tpl[open+2 : open+close])
		v := item[name]
		if v == "" {
			v = diveString(item, name)
		}
		if encode {
			v = pathEscape(v)
		}
		sb.WriteString(v)
		tpl = tpl[open+close+2:]
	}
	return sb.String()
}

// diveString supports dotted names not pre-flattened (e.g. package.name
// when the source map only carries top-level keys).
func diveString(item map[string]string, name string) string {
	if v, ok := item[name]; ok {
		return v
	}
	if v, ok := item[strings.ToLower(name)]; ok {
		return v
	}
	return ""
}

func pathEscape(s string) string {
	s = strings.ReplaceAll(s, " ", "%20")
	s = strings.ReplaceAll(s, "#", "%23")
	return s
}

// stripHTML reduces an HTML fragment to plain text (entities decoded,
// block boundaries collapsed to spaces).
func stripHTML(s string) string {
	if !strings.ContainsAny(s, "<>&") {
		return strings.TrimSpace(s)
	}
	// Unescape first so escaped markup (&lt;span&gt;) is not treated as a
	// second document, then take visible text.
	s = html.UnescapeString(s)
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(s))
	if err != nil {
		return strings.TrimSpace(s)
	}
	text := doc.Text()
	text = strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(text)
	return strings.TrimSpace(collapseSpaces(text))
}

func collapseSpaces(s string) string {
	var sb strings.Builder
	space := false
	for _, r := range s {
		if r == ' ' {
			if space {
				continue
			}
			space = true
		} else {
			space = false
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// readLimited caps response bodies so a hostile upstream cannot balloon
// memory (4 MiB is far above any SERP we parse).
func readLimited(r io.Reader, limit int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, limit))
}
