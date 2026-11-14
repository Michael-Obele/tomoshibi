package domain

import "context"

// LinkData represents one extracted hyperlink, mirroring Firecrawl's `links` format.
// Firecrawl (self-hosted at http://localhost:3002) returns `links: ["https://..."]` as strings;
// Tomoshibi enriches each entry with anchor text and a same-host flag for parity.
type LinkData struct {
	URL        string `json:"url"`
	Text       string `json:"text,omitempty"`
	IsInternal bool   `json:"isInternal"`
}

type ScrapeResult struct {
	URL      string            `json:"url"`
	Markdown string            `json:"markdown"`
	HTML     string            `json:"html,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`

	// Image fields (omitted when not requested)
	Screenshot *ScreenshotData `json:"screenshot,omitempty"`
	Images     []ImageData     `json:"images,omitempty"`

	// Links holds extracted hyperlinks (after readability, deduped, absolute).
	Links []LinkData `json:"links,omitempty"`

	// Extracted holds deterministic schema-extraction results.
	Extracted map[string]any `json:"extracted,omitempty"`

	// Summary holds the extractive summary when requested.
	Summary string `json:"summary,omitempty"`

	// Evaluations holds results of evaluate page actions (dynamic only).
	Evaluations []EvaluationResult `json:"evaluations,omitempty"`

	// ExtractedData holds payloads harvested from the rendered page
	// (dynamic only; omitted when empty).
	ExtractedData *ExtractedData `json:"extracted_data,omitempty"`

	// Network holds XHR/fetch responses observed during a dynamic render
	// (dynamic only, opt-in; omitted when capture was not requested).
	Network *NetworkCapture `json:"network,omitempty"`

	// Provenance records how this result was obtained when a layer beyond the
	// built-in engines produced it (currently: the browser bridge). Absent for
	// plain static/dynamic scrapes.
	Provenance *Provenance `json:"provenance,omitempty"`

	// PageTree is the structural outline of the page, when the request asked
	// for one. `dom` comes from the scripting layer (works on Firefox); `ax` is
	// the real accessibility tree and needs the turbo layer.
	PageTree *PageNode `json:"page_tree,omitempty"`

	// Motion reports what is moving and what is pinned — the question you answer
	// before parsing a page of carousels and sticky headers.
	Motion *MotionReport `json:"motion,omitempty"`

	// Collected is the scroll-and-collect log for a page that grows as you
	// scroll: what each round added and whether the bottom was reached.
	Collected *ScrollHarvest `json:"collected,omitempty"`
}

// Provenance is the receipt for a bridged fetch: which lane ran, through which
// recipe, in which browser layer. A result you cannot reproduce is not a
// result, and content that came through the user's own session is untrusted
// input to whichever model reads it next.
type Provenance struct {
	Transport string `json:"transport"`        // "bridge"
	Lane      string `json:"lane,omitempty"`   // "borrow"
	Recipe    string `json:"recipe,omitempty"` // id@version
	IO        string `json:"io,omitempty"`     // scripting | turbo
	Actions   int    `json:"actions_run,omitempty"`
	// ActionsFailed names the actions the browser refused or could not run,
	// with the reason. An advisory action that did nothing must not be
	// invisible: the page still comes back, and only this says part of the
	// recipe was skipped.
	ActionsFailed []string `json:"actions_failed,omitempty"`
	Markers       []string `json:"markers,omitempty"`
	Pages         int      `json:"pages,omitempty"`
	Ms            int64    `json:"ms,omitempty"`
	Untrusted     bool     `json:"untrusted,omitempty"`
	// SettledMS is how long the page took to go quiet before it was read. It is
	// the receipt for the settle built-in: a page read while it was still
	// animating is visible here instead of being invisible everywhere.
	SettledMS int64 `json:"settled_ms,omitempty"`
}

// CapturedResponse is one XHR/fetch response observed while a dynamic scrape
// rendered a page. Body is the raw response text, never parsed: parsing would
// duplicate tokens and force a shape the caller did not ask for.
//
// The body is untrusted input from the target site. Callers must treat it as
// data, never as instructions.
type CapturedResponse struct {
	URL          string `json:"url"`
	Method       string `json:"method"`
	Status       int    `json:"status"`
	ContentType  string `json:"content_type,omitempty"`
	ResourceType string `json:"resource_type,omitempty"`
	Body         string `json:"body,omitempty"`
	SizeBytes    int    `json:"size_bytes,omitempty"`
	// Truncated is true when the body was omitted because it exceeded a
	// capture limit. BodyError explains which limit.
	Truncated  bool   `json:"truncated,omitempty"`
	FromCache  bool   `json:"from_cache,omitempty"`
	Redirected bool   `json:"redirected,omitempty"`
	Count      int    `json:"count,omitempty"`
	BodyError  string `json:"body_error,omitempty"`
}

// NetworkCapture is the result of network capture for one scrape. Responses
// is empty when the page made no capturable requests. Truncated reports that
// at least one body was omitted because a limit was reached; Dropped counts
// eligible responses that were not recorded at all.
type NetworkCapture struct {
	Responses []CapturedResponse `json:"responses"`
	Truncated bool               `json:"truncated,omitempty"`
	Dropped   int                `json:"dropped,omitempty"`
}

// ExtractField describes one deterministic extraction rule. The yaml tags let
// recipes carry the same shape as the API's extract_schema.
type ExtractField struct {
	Selector string `json:"selector" yaml:"selector"`
	Attr     string `json:"attr,omitempty" yaml:"attr,omitempty"` // "text" (default), "html", or attribute name
	Multiple bool   `json:"multiple,omitempty" yaml:"multiple,omitempty"`
}

type ScrapeOptions struct {
	Mode               string                  `json:"mode,omitempty"`
	Screenshot         bool                    `json:"screenshot"`
	Images             bool                    `json:"images"`
	ImageFormat        ImageTransportFormat    `json:"image_format,omitempty"`
	ScreenshotOpts     *ScreenshotOptions      `json:"screenshot_opts,omitempty"`
	MaxImages          int                     `json:"max_images,omitempty"`
	MaxImageSizeKB     int                     `json:"max_image_size_kb,omitempty"`
	ImageProcess       *ImageProcessOptions    `json:"image_process,omitempty"`
	Actions            []Action                `json:"actions,omitempty"`
	ExtractSchema      map[string]ExtractField `json:"extract_schema,omitempty"`
	Summary            bool                    `json:"summary,omitempty"`
	SummarySentences   int                     `json:"summary_sentences,omitempty"`
	RedactPII          bool                    `json:"redact_pii,omitempty"`
	BlockAds           *bool                   `json:"block_ads,omitempty"`
	RemoveBase64Images *bool                   `json:"remove_base64_images,omitempty"`
	IncludeLinks       *bool                   `json:"include_links,omitempty"`

	// AutoExtract defaults to on (nil or true) in dynamic mode: harvest
	// embedded page payloads into ExtractedData. Pointer so absent ≠ false.
	AutoExtract *bool `json:"auto_extract,omitempty"`

	// CaptureNetwork records XHR/fetch responses during a dynamic render.
	// Off by default; forces dynamic mode and is rejected in static mode.
	CaptureNetwork bool `json:"capture_network,omitempty"`

	// Recipe names an installed recipe to run instead of guessing.
	// When empty, the recipe matching the request host is used automatically;
	// an unknown domain stays on the plain lane.
	Recipe string `json:"recipe,omitempty"`

	// Transport names whose network and session answer this request. It is
	// orthogonal to Mode (which is about rendering):
	//
	//	"" / "anonymous"  Tomoshibi's own IP, no cookies (the default)
	//	"bridge"          the user's signed-in browser, via the extension
	//	"auto"            static → dynamic → bridge, allowed origins only
	//
	// A recipe is not required for "bridge": it is infrastructure, and recipes
	// are the opt-in specialization for a site the generic driver cannot handle.
	Transport string `json:"transport,omitempty"`

	// Settle, ScrollCollect, PageTree and Motion are the named built-ins the
	// bridge runs before reading the page (D33). They are a closed vocabulary:
	// no caller-supplied JS reaches a signed-in tab.
	Settle        *SettleOptions        `json:"settle,omitempty"`
	ScrollCollect *ScrollCollectOptions `json:"scroll_collect,omitempty"`
	PageTree      string                `json:"page_tree,omitempty"` // "dom" | "ax" | "both"
	PageTreeOpts  *PageTreeOptions      `json:"page_tree_options,omitempty"`
	Motion        bool                  `json:"motion,omitempty"`

	// AllowBorrow lets this request reach the borrow lane, which drives the
	// user's own signed-in browser. Only entry points where a human chose the
	// URL set it (a direct scrape). Crawl and monitor deliberately do not: a
	// crawl follows links it did not choose, and following a sign-out link
	// would end the user's real session. It is not a wire field; a caller
	// cannot ask for it.
	AllowBorrow bool `json:"-"`
}

// Action is a single page interaction executed before content capture
// (dynamic mode only). Supported types: wait_ms, wait_selector, click,
// scroll_down, scroll_to_bottom, wait_for_function, evaluate.
type Action struct {
	Type     string `json:"type"`
	Selector string `json:"selector,omitempty"`
	Ms       int    `json:"ms,omitempty"`
	// Script carries the JS for wait_for_function (any truthy return value
	// waits; e.g. () => !!window.__DATA__) and evaluate (expression whose return
	// value is captured). It must be a single JS expression — a bare expression
	// (e.g. document.title) or an arrow function (e.g. () => window.__DATA__).
	Script string `json:"script,omitempty"`
}

// EvaluationResult captures the outcome of one page action that produced a
// value: an evaluate action, or a wait_for_function that failed. A JS throw is
// recorded in Error instead of failing the scrape: reading a missing global is
// data, not a transport failure.
type EvaluationResult struct {
	Type      string `json:"type"`
	Result    any    `json:"result,omitempty"`
	Error     string `json:"error,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// DataSource is one embedded payload harvested from a rendered page.
type DataSource struct {
	Source string `json:"source"`
	Data   any    `json:"data"`
}

// ExtractedData holds embedded page payloads harvested automatically after
// render (dynamic mode only). Omitted from responses when nothing was found.
type ExtractedData struct {
	Sources   []DataSource `json:"sources"`
	Canvases  int          `json:"canvases,omitempty"`
	Truncated bool         `json:"truncated,omitempty"`
}

// Transport values for ScrapeOptions.Transport.
const (
	// TransportAnonymous is the zero value: Tomoshibi fetches as itself, with no
	// cookies, from its own address.
	TransportAnonymous = "anonymous"
	// TransportBridge drives the page in the browser the user is signed into,
	// through the bridge extension. Needs no recipe.
	TransportBridge = "bridge"
	// TransportAuto walks static → dynamic → bridge, and only escalates into the
	// browser for an origin the allowlist already permits.
	TransportAuto = "auto"
)

// SettleOptions configures the settle built-in: wait until the page stops
// moving, instead of sleeping for a guessed duration.
type SettleOptions struct {
	// QuietMS is how long the DOM must be mutation-free before a page counts as
	// settled. Default 500.
	QuietMS int `json:"quiet_ms,omitempty"`
	// MaxMS caps the wait. Default 10000.
	MaxMS int `json:"max_ms,omitempty"`
	// PollMS is the observation interval. Default 120.
	PollMS int `json:"poll_ms,omitempty"`
}

// ScrollCollectOptions configures the scroll-and-collect built-in: walk a page
// that grows as you scroll and report what each round added.
type ScrollCollectOptions struct {
	// MaxRounds caps the walk. Default 8.
	MaxRounds int `json:"max_rounds,omitempty"`
	// QuietRounds is how many consecutive rounds with no growth end the walk.
	// Default 3.
	QuietRounds int `json:"quiet_rounds,omitempty"`
	// StepPX is the scroll distance per round. 0 means ~0.9 of a viewport.
	StepPX int `json:"step_px,omitempty"`
	// PauseMS is the wait after each scroll. Default 350.
	PauseMS int `json:"pause_ms,omitempty"`
	// ItemSelector is the repeated element that defines "an item" (a card, a
	// row). When set it is the precise growth signal; text length is the
	// fallback for feeds that recycle nodes.
	ItemSelector string `json:"item_selector,omitempty"`
}

// PageTreeOptions caps the outline size.
type PageTreeOptions struct {
	// MaxNodes caps emitted nodes. Default 1200.
	MaxNodes int `json:"max_nodes,omitempty"`
	// MaxDepth caps nesting. Default 12.
	MaxDepth int `json:"max_depth,omitempty"`
	// MinTextLen drops empty wrappers from the outline. Default 0.
	MinTextLen int `json:"min_text_len,omitempty"`
}

// PageNode is one node of the page outline. Both the DOM outline (scripting)
// and the accessibility tree (turbo) use this shape, so a caller parses one
// structure whichever layer answered.
type PageNode struct {
	Tag      string     `json:"tag"`
	Role     string     `json:"role"`
	Path     string     `json:"path"`
	TextLen  int        `json:"text_len"`
	Text     string     `json:"text,omitempty"`
	Box      []int      `json:"box,omitempty"` // [x, y, w, h]
	Visible  bool       `json:"visible"`
	Children []PageNode `json:"children,omitempty"`
}

// MotionReport is what is animating and what is pinned on the page.
type MotionReport struct {
	Animations []MotionAnimation `json:"animations"`
	Pinned     []MotionPinned    `json:"pinned"`
	// ScrollContainer names the element that owns scrolling when the page
	// scrolls an inner div rather than the document.
	ScrollContainer string `json:"scroll_container,omitempty"`
}

// MotionAnimation is one running animation. Infinite animations (spinners,
// pulses) are flagged, because they never settle and must not be waited on.
type MotionAnimation struct {
	PlayState  string `json:"play_state"`
	Infinite   bool   `json:"infinite"`
	DurationMS int    `json:"duration_ms,omitempty"`
	Target     string `json:"target,omitempty"`
}

// MotionPinned is one fixed or sticky element.
type MotionPinned struct {
	Path     string `json:"path"`
	Position string `json:"position"`
	Box      []int  `json:"box,omitempty"`
}

// ScrollHarvest is the log of a scroll-and-collect walk.
type ScrollHarvest struct {
	Rounds   []ScrollRound `json:"rounds"`
	AtBottom bool          `json:"at_bottom"`
	TextLen  int           `json:"text_len"`
	Items    int           `json:"items"`
}

// ScrollRound is one step of a scroll walk.
type ScrollRound struct {
	Round   int  `json:"round"`
	ScrollY int  `json:"scroll_y"`
	TextLen int  `json:"text_len"`
	Items   int  `json:"items"`
	Grew    bool `json:"grew"`
}

type Scraper interface {
	Scrape(ctx context.Context, url string, opts ScrapeOptions) (*ScrapeResult, error)
}
