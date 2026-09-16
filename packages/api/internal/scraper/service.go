package scraper

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Michael-Obele/tomoshibi/internal/domain"
	"github.com/Michael-Obele/tomoshibi/internal/extract"
	"github.com/Michael-Obele/tomoshibi/internal/image"
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
}

func NewService(colly domain.Scraper, chromedp domain.Scraper, rdb *redis.Client) *Service {
	s := &Service{
		colly:    colly,
		chromedp: chromedp,
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
		return fmt.Sprintf("scrape:%s:%s", url, mode)
	}
	sum := sha256.Sum256(data)
	return "scrape:" + hex.EncodeToString(sum[:])
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
		if err != nil {
			// Not every static failure is worth retrying: DNS and connection
			// errors fail identically in a browser, and retrying doubles the
			// latency of an already-failed request. Retry only the failures a
			// real browser plausibly fixes — bot blocks keyed on the absence
			// of a JS runtime, and empty bodies.
			needsDynamic = worthRetryingDynamic(err)
			if needsDynamic {
				logger.Log.Info("Static scrape failed in a way a browser may fix; retrying dynamic",
					"url", url, "error", err)
			}
		} else if result != nil {
			// Check heuristics
			if ShouldUseDynamic(result.HTML) {
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
				// Static succeeded but looked like a shell, and dynamic
				// failed. The static result is thin but real, so return it
				// rather than failing a request we can partially answer.
				logger.Log.Warn("Dynamic retry failed; falling back to the static result",
					"url", url, "error", dynErr)
			}
		}
	default:
		return nil, fmt.Errorf("unknown mode: %s", mode)
	}

	if err != nil {
		return nil, err
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

	// 4. Save to Cache (screenshots are excluded: blobs are large and every
	// capture is time-sensitive, so a cached one is a liability).
	if s.cache != nil && !opts.Screenshot {
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

	return result, nil
}
