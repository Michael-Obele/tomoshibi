package scraper

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/extract"
	"github.com/Michael-Obele/tomoshibi/internal/image"
	"github.com/Michael-Obele/tomoshibi/internal/recipe"
	"github.com/Michael-Obele/tomoshibi/pkg/logger"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
)

// cacheStore is the slice of the Redis client the scrape cache actually uses.
// Narrowing it to these two methods is what lets tests exercise the cache read
// and write paths against an in-memory fake instead of a live server;
// *redis.Client satisfies it as-is.
type cacheStore interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd
}

// Service acts as the main entry point and chooses the right scraper
type Service struct {
	colly    domain.Scraper
	chromedp domain.Scraper
	cache    cacheStore
	solver   domain.ChallengeSolver
	bridge   *BridgeTransport
}

// Option configures optional Service collaborators.
type Option func(*Service)

// WithSolver attaches an optional anti-bot addon. Without one, a challenge
// page is reported as a BlockedError instead of being returned as content.
func WithSolver(s domain.ChallengeSolver) Option {
	return func(svc *Service) { svc.solver = s }
}

// WithBridge attaches the browser bridge transport, enabling borrow-lane
// recipes. Without it, a borrow recipe fails loudly instead of silently
// falling back to an anonymous fetch the site would reject anyway.
func WithBridge(t *BridgeTransport) Option {
	return func(svc *Service) { svc.bridge = t }
}

func NewService(colly domain.Scraper, chromedp domain.Scraper, rdb *redis.Client, opts ...Option) *Service {
	s := &Service{
		colly:    colly,
		chromedp: chromedp,
	}
	for _, opt := range opts {
		opt(s)
	}
	// Assign only a real client. A nil *redis.Client stored in an interface
	// makes a non-nil interface holding a nil pointer, so the `s.cache != nil`
	// guards below would pass and then dereference it — and callers do pass nil
	// when Redis is not configured.
	if rdb != nil {
		s.cache = rdb
	}
	return s
}

// cacheKeyFor builds a deterministic cache key from the URL, mode, and the
// full scrape options so option changes never serve stale cached results.
func cacheKeyFor(url, mode string, opts domain.ScrapeOptions) string {
	// Normalize IncludeLinks nil -> true for stable keys; nil and true
	// now produce the same hash so legacy callers (empty opts) hit the
	// stored entry and old cache entries remain readable after the field addition.
	if opts.IncludeLinks == nil {
		t := true
		opts.IncludeLinks = &t
	}
	payload := struct {
		URL  string
		Mode string
		Opts domain.ScrapeOptions
	}{URL: url, Mode: mode, Opts: opts}
	data, err := json.Marshal(payload)
	if err != nil {
		// Marshal of these types cannot fail; fall back to the legacy key.
		return fmt.Sprintf("scrape:v2:%s:%s", url, mode)
	}
	sum := sha256.Sum256(data)
	// v2 invalidates entries written before challenge detection existed: those
	// may hold an interstitial body that the old code cached as a successful
	// scrape, and serving one would defeat the guard this version adds.
	return "scrape:v2:" + hex.EncodeToString(sum[:])
}

func (s *Service) Scrape(
	ctx context.Context,
	url string,
	mode string,
	opts domain.ScrapeOptions,
) (*domain.ScrapeResult, error) {
	// Default to smart if empty
	if mode == "" {
		mode = "smart"
	}

	// include_links defaults to true for Firecrawl parity.
	if opts.IncludeLinks == nil {
		t := true
		opts.IncludeLinks = &t
	}

	// Recipe routing. A borrow-lane recipe hands the fetch to the
	// user's own browser through the bridge; every other lane keeps the normal
	// engine pipeline, with the recipe's actions, schema and expectations applied
	// to it. An unknown host has no recipe and stays anonymous.
	var laneRecipe *recipe.Recipe
	if s.bridge != nil {
		rec, rerr := s.bridge.ResolveRecipe(url, opts.Recipe)
		if rerr != nil {
			return nil, rerr
		}
		if rec != nil {
			switch rec.Access.Lane {
			case recipe.LaneBorrow:
				// The borrow lane drives the user's own signed-in browser, so only a
				// direct scrape may use it. A crawl follows links nobody chose — and
				// one of them could be a sign-out link that ends the real session —
				// and a monitor repeats unattended. Those paths get a loud refusal
				// rather than a browser they should not have.
				if !opts.AllowBorrow {
					return nil, fmt.Errorf("recipe %s: the borrow lane is only available to a direct scrape; a crawl, monitor or queued job will not drive your browser", rec.ID)
				}
				// Options the borrow lane cannot honour are refused here rather
				// than accepted and dropped: the caller has to know that this
				// lane does not do them.
				if mode == "static" {
					return nil, fmt.Errorf("recipe %s: lane borrow needs a browser, so mode static is not available", rec.ID)
				}
				if opts.CaptureNetwork {
					return nil, fmt.Errorf("recipe %s: capture_network is not supported on the borrow lane", rec.ID)
				}
				return s.bridge.Scrape(ctx, url, rec, opts)
			case recipe.LaneOfficial, recipe.LaneImport:
				return nil, fmt.Errorf("recipe %s: lane %q is not implemented yet; use the plain lane or the site's own API", rec.ID, rec.Access.Lane)
			case recipe.LanePlain:
				// A plain recipe is still a recipe: its actions, extraction schema
				// and expectations apply to the ordinary pipeline. Without this a
				// pack's rules would be loaded, verified and then ignored.
				merged, merr := applyRecipeToOptions(rec, opts)
				if merr != nil {
					return nil, merr
				}
				opts = merged
				laneRecipe = rec
			}
		}
	} else if opts.Recipe != "" {
		return nil, fmt.Errorf("recipe %q requested but the bridge is not configured", opts.Recipe)
	}

	// Page actions require a real browser: force dynamic mode.
	if len(opts.Actions) > 0 {
		switch mode {
		case "dynamic":
		case "static":
			return nil, fmt.Errorf("actions require dynamic mode (got static)")
		default:
			mode = "dynamic"
		}
	}

	// Network capture observes the browser's own requests, so it has the same
	// mode requirement as page actions.
	if opts.CaptureNetwork {
		switch mode {
		case "dynamic":
		case "static":
			return nil, fmt.Errorf("capture_network requires dynamic mode (got static)")
		default:
			mode = "dynamic"
		}
	}

	// 1. Try Cache
	cacheKey := cacheKeyFor(url, mode, opts)
	if s.cache != nil {
		val, err := s.cache.Get(ctx, cacheKey).Result()
		if err == nil {
			// Try decompressing
			// Note: val is string, convert to byte
			b := bytes.NewReader([]byte(val))
			gz, err := gzip.NewReader(b)
			if err == nil {
				defer gz.Close()
				decompressed, err := io.ReadAll(gz)
				if err == nil {
					var result domain.ScrapeResult
					if err := json.Unmarshal(decompressed, &result); err == nil {
						if result.Metadata == nil {
							result.Metadata = make(map[string]string)
						}
						result.Metadata["cached"] = "true"
						return &result, nil
					}
				}
			} else {
				// Fallback: maybe it's legacy uncompressed data?
				// Try unmarshal directly
				var result domain.ScrapeResult
				if err := json.Unmarshal([]byte(val), &result); err == nil {
					if result.Metadata == nil {
						result.Metadata = make(map[string]string)
					}
					result.Metadata["cached"] = "true"
					return &result, nil
				}
			}
		}
	}

	// 2. Scrape
	var result *domain.ScrapeResult
	var err error

	// Helper to run dynamic
	runDynamic := func() (*domain.ScrapeResult, error) {
		if s.chromedp == nil {
			return nil, fmt.Errorf("dynamic scraper not configured")
		}
		return s.chromedp.Scrape(ctx, url, opts)
	}

	// Helper to run static
	runStatic := func() (*domain.ScrapeResult, error) {
		if s.colly == nil {
			return nil, fmt.Errorf("static scraper not configured")
		}
		return s.colly.Scrape(ctx, url, opts)
	}

	switch mode {
	case "dynamic":
		result, err = runDynamic()
	case "static":
		result, err = runStatic()
	case "smart":
		// A screenshot needs a real browser, so skip the static attempt.
		// The pipeline continues below (enrichment still applies); caching
		// is skipped for screenshot results at step 4.
		if opts.Screenshot {
			result, err = runDynamic()
			break
		}

		// 1. Try static first (fast & cheap)
		result, err = runStatic()

		// Decide whether dynamic is worth a second attempt.
		var needsDynamic bool
		var staticChallenge *Challenge
		var staticChallengeHTML string
		if err != nil {
			// Not every static failure is worth retrying: DNS and connection
			// errors fail identically in a browser, and retrying doubles the
			// latency of an already-failed request. Retry only the failures a
			// real browser plausibly fixes — bot blocks keyed on the absence
			// of a JS runtime, and empty bodies.
			var blocked *BlockedError
			if errors.As(err, &blocked) {
				// A challenge page is the strongest signal that a browser may
				// get through, so it always earns the one retry.
				staticChallenge = &Challenge{Vendor: blocked.Vendor}
				needsDynamic = true
			} else {
				needsDynamic = worthRetryingDynamic(err)
			}
			if needsDynamic {
				logger.Log.Info("Static scrape failed in a way a browser may fix; retrying dynamic",
					"url", url, "error", err)
			}
		} else if result != nil {
			if ch := detectChallenge([]byte(result.HTML), http.StatusOK); ch != nil {
				// Challenge pages are served with HTTP 200. Without this check
				// the "thin but real" fallback below would happily return the
				// block page as content.
				staticChallenge = ch
				staticChallengeHTML = result.HTML
				needsDynamic = true
				logger.Log.Info("Static scrape hit an anti-bot challenge; switching to Chromedp",
					"url", url, "vendor", ch.Vendor)
			} else if ShouldUseDynamic(result.HTML) {
				needsDynamic = true
				logger.Log.Info("Heuristics detected an SPA shell; switching to Chromedp", "url", url)
			}
		}

		if needsDynamic {
			dynamicResult, dynErr := runDynamic()
			switch {
			case dynErr == nil:
				result, err = dynamicResult, nil
			case err != nil:
				// Both engines failed. Report the static error: it is the
				// original cause, and the dynamic retry was speculative.
				logger.Log.Warn("Dynamic retry also failed", "url", url, "error", dynErr)
			default:
				if staticChallenge != nil {
					// The static "success" was a challenge page, so there is
					// nothing real to fall back to.
					err = &BlockedError{URL: url, Vendor: staticChallenge.Vendor, HTML: staticChallengeHTML, Err: dynErr}
				} else {
					// Static succeeded but looked like a shell, and dynamic
					// failed. The static result is thin but real, so return it
					// rather than failing a request we can partially answer.
					logger.Log.Warn("Dynamic retry failed; falling back to the static result",
						"url", url, "error", dynErr)
				}
			}
		}
	default:
		return nil, fmt.Errorf("unknown mode: %s", mode)
	}

	// A challenge can also arrive as a 200 from either engine. Give an optional
	// addon one chance, then fail loudly: returning block-page text as content
	// is the one failure mode an agent cannot detect for itself.
	if err == nil && result != nil {
		if ch := detectChallenge([]byte(result.HTML), http.StatusOK); ch != nil {
			solved, solveErr := s.trySolve(ctx, url, ch, http.StatusOK, result.HTML, opts)
			if solveErr != nil {
				logger.Log.Warn("Challenge solver failed", "url", url, "vendor", ch.Vendor, "error", solveErr)
			}
			if solved == nil {
				return nil, &BlockedError{URL: url, Vendor: ch.Vendor}
			}
			result = solved
		}
	}

	if err != nil {
		// A block reported as 403/429/503 still carries the challenge body, so
		// the addon gets the same opportunity as the 200 case above.
		var blocked *BlockedError
		if errors.As(err, &blocked) && blocked.HTML != "" {
			ch := &Challenge{Vendor: blocked.Vendor}
			solved, solveErr := s.trySolve(ctx, url, ch, blocked.StatusCode, blocked.HTML, opts)
			if solveErr != nil {
				logger.Log.Warn("Challenge solver failed", "url", url, "vendor", ch.Vendor, "error", solveErr)
			}
			if solved != nil {
				result, err = solved, nil
			}
		}
		if err != nil {
			return nil, err
		}
	}

	// 3a. Deterministic schema extraction (no LLM).
	if opts.ExtractSchema != nil && result.HTML != "" {
		extracted, extErr := extract.Apply(result.HTML, opts.ExtractSchema)
		if extErr != nil {
			logger.Log.Warn("Schema extraction failed", "url", url, "error", extErr)
		} else {
			result.Extracted = extracted
		}
	}

	// 3b. Extractive summary.
	if opts.Summary {
		result.Summary = extract.Summarize(result.Markdown, result.Metadata["excerpt"], opts.SummarySentences)
	}

	// 3c. PII redaction applied to markdown and summary.
	if opts.RedactPII {
		result.Markdown = extract.RedactPII(result.Markdown)
		if result.Summary != "" {
			result.Summary = extract.RedactPII(result.Summary)
		}
	}

	// 3d. Links extraction (after readability, deduped, absolute, isInternal).
	// Scrapers already fill Links via ExtractLinks on rc.ContentHTML; this fallback
	// covers mocks and cached paths where Links may be absent, re-deriving after
	// readability for parity with the per-engine extraction.
	if len(result.Links) == 0 && opts.IncludeLinks != nil && *opts.IncludeLinks && result.HTML != "" {
		if rc, _ := ExtractMainContent(result.HTML, url); rc != nil && rc.ContentHTML != "" {
			result.Links = ExtractLinks(rc.ContentHTML, url)
		} else {
			result.Links = ExtractLinks(result.HTML, url)
		}
		if len(result.Links) == 0 {
			// Ensure empty is nil for omitempty, but preserve explicit false vs empty distinction
			result.Links = nil
		}
	}
	if opts.IncludeLinks != nil && !*opts.IncludeLinks {
		result.Links = nil
	}

	// 3. Extract images if requested
	if opts.Images && result != nil && result.HTML != "" {
		maxImages := opts.MaxImages
		if maxImages <= 0 {
			maxImages = 10
		}
		result.Images = image.ExtractPageImages(result.HTML, url, maxImages)

		// Fetch and encode each image if blob format requested.
		// Fetches run concurrently (bounded) to avoid N serial round trips;
		// per-image failures are logged and skipped, preserving the response.
		if opts.ImageFormat == domain.ImageFormatBlob && len(result.Images) > 0 {
			proc := image.NewProcessor()
			maxBytes := int64(opts.MaxImageSizeKB) * 1024

			g, gctx := errgroup.WithContext(ctx)
			g.SetLimit(5)
			for i := range result.Images {
				img := &result.Images[i]
				if img.URL == "" {
					continue
				}
				g.Go(func() error {
					blob, fetchErr := proc.FetchAndEncodeLimit(img.URL, maxBytes)
					if fetchErr != nil {
						// Context cancellation is real; everything else is a
						// per-image skip.
						if gctx.Err() != nil {
							return fetchErr
						}
						logger.Log.Warn("Failed to fetch image for blob", "url", img.URL, "error", fetchErr)
						return nil
					}
					img.Blob = blob.DataURI
					if strings.HasPrefix(blob.MimeType, "image/") {
						img.Format = strings.TrimPrefix(blob.MimeType, "image/")
					}
					img.SizeBytes = int64(len(blob.RawBytes))
					return nil
				})
			}
			if gerr := g.Wait(); gerr != nil {
				return nil, fmt.Errorf("image fetch aborted: %w", gerr)
			}

			// Optional post-processing: resize/re-encode fetched blobs.
			if opts.ImageProcess != nil {
				for i := range result.Images {
					if result.Images[i].Blob == "" {
						continue
					}
					raw, _, decErr := image.DecodeDataURI(result.Images[i].Blob)
					if decErr != nil {
						logger.Log.Warn("Failed to decode image blob for processing", "url", result.Images[i].URL, "error", decErr)
						continue
					}
					processed, mime, procErr := image.Reencode(raw, *opts.ImageProcess)
					if procErr != nil {
						logger.Log.Warn("Failed to process image", "url", result.Images[i].URL, "error", procErr)
						continue
					}
					result.Images[i].Blob = image.NewProcessor().EncodeToDataURI(processed, mime)
					result.Images[i].Format = strings.TrimPrefix(mime, "image/")
					result.Images[i].SizeBytes = int64(len(processed))
				}
			}
		}
	}

	// 4. Save to Cache (screenshots and network captures are excluded: blobs
	// and API bodies are large, and both are time-sensitive — a cached one is
	// a liability). A recipe-routed fetch is never cached: its expectations are
	// checked per run, and a cached body would skip them.
	if s.cache != nil && !opts.Screenshot && !opts.CaptureNetwork && laneRecipe == nil {
		data, err := json.Marshal(result)
		if err == nil {
			// Compress data
			var b bytes.Buffer
			gz := gzip.NewWriter(&b)
			if _, err := gz.Write(data); err == nil {
				gz.Close()
				// Store for 7 days as requested (low storage usage allows this)
				s.cache.Set(ctx, cacheKey, b.Bytes(), 7*24*time.Hour)
			}
		}
	}

	// A plain-lane recipe is a contract about the page, so its expectations are
	// checked here too: a pack's rules must not be crowd-sourced away by falling
	// through to the generic pipeline.
	if laneRecipe != nil {
		if err := checkRecipeExpectations(laneRecipe, result); err != nil {
			return nil, err
		}
		result.Provenance = &domain.Provenance{
			Transport: "engine", Lane: laneRecipe.Access.Lane, Recipe: laneRecipe.Ref(),
			Pages: 1,
		}
	}

	return result, nil
}

// applyRecipeToOptions folds a plain-lane recipe's request settings into the
// scrape options. The caller's own values win: a recipe supplies defaults, not
// orders. The engine's action vocabulary is narrower than the bridge's (no
// typing, no keys, no navigation), so a recipe action it cannot express is an
// error naming the action — never a silently skipped step.
func applyRecipeToOptions(rec *recipe.Recipe, opts domain.ScrapeOptions) (domain.ScrapeOptions, error) {
	if len(opts.Actions) == 0 {
		for _, a := range rec.Request.Actions {
			if a.Text != "" || a.Key != "" || a.URL != "" || a.Px != 0 {
				return opts, fmt.Errorf("recipe %s: action %q needs fields the engine lane cannot express (text/key/url/px); use lane borrow for this recipe", rec.ID, a.Type)
			}
			opts.Actions = append(opts.Actions, domain.Action{
				Type: a.Type, Selector: a.Selector, Ms: a.Ms, Script: a.Script,
			})
		}
	}
	if len(opts.ExtractSchema) == 0 && len(rec.Extract.Schema) > 0 {
		opts.ExtractSchema = rec.Extract.Schema
	}
	return opts, nil
}

// checkRecipeExpectations runs a recipe's contains/absent/status rules against
// a completed result. Shared by the borrow lane and the engine pipeline so one
// recipe means the same thing whichever lane served it.
func checkRecipeExpectations(rec *recipe.Recipe, result *domain.ScrapeResult) error {
	if result == nil {
		return nil
	}
	text := result.Markdown
	if text == "" {
		text = result.HTML
	}
	for _, marker := range rec.Expect.Absent {
		if marker != "" && strings.Contains(text, marker) {
			return &BlockedError{URL: result.URL, Vendor: "challenge-marker", HTML: result.HTML,
				Err: fmt.Errorf("recipe %s: page contains a block marker %q", rec.ID, marker)}
		}
	}
	for _, needle := range rec.Expect.Contains {
		if needle != "" && !strings.Contains(text, needle) {
			return fmt.Errorf("recipe %s: expected text %q is missing; the recipe may be stale (tomoshibi recipes verify %s)", rec.ID, needle, rec.ID)
		}
	}
	return nil
}

// trySolve gives the optional addon one attempt at a challenge. It returns
// (nil, nil) when no solver is configured, which lets callers report the block
// without special-casing the absence of an addon.
func (s *Service) trySolve(
	ctx context.Context,
	url string,
	ch *Challenge,
	statusCode int,
	html string,
	opts domain.ScrapeOptions,
) (*domain.ScrapeResult, error) {
	if s.solver == nil {
		return nil, nil
	}

	solution, err := s.solver.Solve(ctx, domain.ChallengeRequest{
		URL:        url,
		Vendor:     ch.Vendor,
		StatusCode: statusCode,
		HTML:       html,
	})
	if err != nil {
		return nil, err
	}
	if solution == nil || solution.HTML == "" {
		return nil, fmt.Errorf("solver returned no HTML")
	}
	// An addon that hands back another interstitial has not solved anything;
	// returning it would reintroduce the silent failure this path prevents.
	if ch := detectChallenge([]byte(solution.HTML), http.StatusOK); ch != nil {
		return nil, fmt.Errorf("solver returned a %s challenge page", ch.Vendor)
	}

	solved, err := buildStaticResult(url, solution.HTML, opts)
	if err != nil {
		return nil, err
	}
	if solved.Metadata == nil {
		solved.Metadata = make(map[string]string)
	}
	solved.Metadata["challenge_solved"] = "true"
	return solved, nil
}
