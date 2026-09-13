import * as v from 'valibot';
import { query, form, command, getRequestEvent } from '$app/server';
import { PRIVATE_CINDER_BACKEND_URL, PRIVATE_CINDER_API_KEY } from '$env/static/private';
import { env } from '$env/dynamic/private';
import { error } from '@sveltejs/kit';
import { dev } from '$app/environment';
import { dailySearchLimiter } from '$lib/server/rate-limiter';

// --- Auth Helper ---
function isAuthenticated(): boolean {
	const event = getRequestEvent();
	if (!event) return false;
	const password = env.MASTRA_PASSWORD;
	const authCookie = event.cookies.get('mastra_auth');
	return !!(password && authCookie === password);
}

function getClientIP(): string {
	const event = getRequestEvent();
	try {
		return event?.getClientAddress() || 'unknown';
	} catch {
		return 'unknown';
	}
}

// --- Schemas ---

// Scrape — mirrors POST /v1/scrape (single url OR sync multi-URL urls[] max 10, exclusive).
// Backend: POST /v1/scrape with {urls: [...]} returns {results: [{url, markdown, ...}]} sync, errgroup limit 5, no Redis.
// include_links defaults to true (Firecrawl parity), block_ads/remove_base64_images default true.
const ScrapeOptionsSchema = v.object({
	url: v.optional(v.pipe(v.string(), v.url()), ''),
	urls: v.optional(v.string(), ''),
	mode: v.optional(v.union([v.picklist(['smart', 'static', 'dynamic']), v.string()]), 'smart'),
	screenshot: v.optional(
		v.union([
			v.pipe(
				v.string(),
				v.transform((v) => v === 'on' || v === 'true'),
				v.boolean()
			),
			v.boolean()
		]),
		false
	),
	images: v.optional(
		v.union([
			v.pipe(
				v.string(),
				v.transform((v) => v === 'on' || v === 'true'),
				v.boolean()
			),
			v.boolean()
		]),
		false
	),
	image_format: v.optional(v.union([v.picklist(['url', 'blob']), v.string()]), 'url'),
	max_images: v.optional(
		v.union([v.pipe(v.string(), v.transform(Number), v.number()), v.number()]),
		10
	),
	max_image_size_kb: v.optional(
		v.union([v.pipe(v.string(), v.transform(Number), v.number()), v.number()]),
		5120
	),
	// Advanced — exposed via OptionsSheet JSON / toggles
	summary: v.optional(
		v.union([
			v.pipe(
				v.string(),
				v.transform((v) => v === 'on' || v === 'true'),
				v.boolean()
			),
			v.boolean()
		]),
		false
	),
	summary_sentences: v.optional(
		v.union([v.pipe(v.string(), v.transform(Number), v.number()), v.number()]),
		5
	),
	redact_pii: v.optional(
		v.union([
			v.pipe(
				v.string(),
				v.transform((v) => v === 'on' || v === 'true'),
				v.boolean()
			),
			v.boolean()
		]),
		false
	),
	block_ads: v.optional(
		v.union([
			v.pipe(
				v.string(),
				v.transform((v) => v === 'on' || v === 'true'),
				v.boolean()
			),
			v.boolean()
		]),
		true
	),
	remove_base64_images: v.optional(
		v.union([
			v.pipe(
				v.string(),
				v.transform((v) => v === 'on' || v === 'true'),
				v.boolean()
			),
			v.boolean()
		]),
		true
	),
	include_links: v.optional(
		v.union([
			v.pipe(
				v.string(),
				v.transform((v) => v === 'on' || v === 'true'),
				v.boolean()
			),
			v.boolean()
		]),
		true
	),
	extract_schema: v.optional(v.string()),
	actions: v.optional(v.string())
});

// Crawl — POST /v1/crawl (async, Redis required)
const CrawlOptionsSchema = v.object({
	url: v.pipe(v.string(), v.url(), v.nonEmpty('URL is required')),
	mode: v.optional(v.union([v.picklist(['smart', 'static', 'dynamic']), v.string()]), 'smart'),
	maxDepth: v.optional(
		v.union([v.pipe(v.string(), v.transform(Number), v.number()), v.number()]),
		2
	),
	limit: v.optional(v.union([v.pipe(v.string(), v.transform(Number), v.number()), v.number()]), 10),
	screenshot: v.optional(
		v.union([
			v.pipe(
				v.string(),
				v.transform((v) => v === 'on' || v === 'true'),
				v.boolean()
			),
			v.boolean()
		]),
		false
	),
	images: v.optional(
		v.union([
			v.pipe(
				v.string(),
				v.transform((v) => v === 'on' || v === 'true'),
				v.boolean()
			),
			v.boolean()
		]),
		false
	),
	image_format: v.optional(v.union([v.picklist(['url', 'blob']), v.string()]), 'url'),
	include_paths: v.optional(v.string()),
	exclude_paths: v.optional(v.string()),
	webhook_url: v.optional(v.pipe(v.string(), v.url()), ''),
	webhook_secret: v.optional(v.string(), '')
});

// Search — POST /v1/search (SearXNG primary, Brave fallback)
// New: category (general|news|code) maps to SearXNG categories + Brave search_type, rerank (TF-IDF), highlights per hit.
const SearchOptionsSchema = v.object({
	query: v.pipe(v.string(), v.nonEmpty('Query is required')),
	mode: v.optional(v.union([v.string()]), 'default'),
	category: v.optional(v.union([v.picklist(['general', 'news', 'code']), v.string()]), ''),
	rerank: v.optional(
		v.union([
			v.pipe(
				v.string(),
				v.transform((v) => v === 'on' || v === 'true'),
				v.boolean()
			),
			v.boolean()
		]),
		false
	),
	limit: v.optional(v.union([v.pipe(v.string(), v.transform(Number), v.number()), v.number()]), 5),
	offset: v.optional(v.union([v.pipe(v.string(), v.transform(Number), v.number()), v.number()]), 0),
	maxAge: v.optional(v.union([v.pipe(v.string(), v.transform(Number), v.number()), v.number()]), 0),
	includeDomains: v.optional(v.union([v.array(v.string()), v.string()]), []),
	excludeDomains: v.optional(v.union([v.array(v.string()), v.string()]), []),
	requiredText: v.optional(v.union([v.array(v.string()), v.string()]), [])
});

// Map — POST /v1/map (sitemap → robots.txt → link fallback, no Redis)
const MapOptionsSchema = v.object({
	url: v.pipe(v.string(), v.url(), v.nonEmpty('URL is required')),
	search: v.optional(v.string(), ''),
	limit: v.optional(v.union([v.pipe(v.string(), v.transform(Number), v.number()), v.number()]), 100)
});

// Batch — POST /v1/batch/scrape (async, Redis, max 20 urls)
const BatchOptionsSchema = v.object({
	urls: v.pipe(v.string(), v.nonEmpty('At least one URL is required'))
});

// Monitor — POST /v1/monitor (change tracking, Redis, interval >= 3600s)
const MonitorOptionsSchema = v.object({
	url: v.pipe(v.string(), v.url(), v.nonEmpty('URL is required')),
	interval_seconds: v.optional(
		v.union([v.pipe(v.string(), v.transform(Number), v.number()), v.number()]),
		3600
	),
	webhook_url: v.optional(v.string(), ''),
	webhook_secret: v.optional(v.string(), '')
});

// --- Helper ---

async function fetchCinder(endpoint: string, method: string, body?: any) {
	if (!PRIVATE_CINDER_BACKEND_URL) {
		throw new Error('PRIVATE_CINDER_BACKEND_URL is not set');
	}

	// Normalize: strip trailing slash and any '/v1' suffix from base URL
	// so it works regardless of how the user configured it
	const baseUrl = PRIVATE_CINDER_BACKEND_URL.replace(/\/+$/, '').replace(/\/v1$/, '');
	const path = endpoint.startsWith('/') ? endpoint : `/${endpoint}`;
	const url = `${baseUrl}${path}`;
	const headers: Record<string, string> = {
		'Content-Type': 'application/json'
	};

	// Attach API key when configured (Go backend uses X-API-Key)
	if (PRIVATE_CINDER_API_KEY && PRIVATE_CINDER_API_KEY !== 'your_secret_key') {
		headers['X-API-Key'] = PRIVATE_CINDER_API_KEY;
	}

	const bodyStr = body ? JSON.stringify(body) : undefined;

	console.log('[Cinder] Fetching', method, url);
	if (body) console.log('[Cinder] Body:', JSON.stringify(body));

	try {
		const response = await fetch(url, {
			method,
			headers,
			body: bodyStr
		});

		// 204 No Content (e.g. DELETE /monitor/:id)
		if (response.status === 204) {
			console.log('[Cinder] Response 204 for', url);
			return { success: true };
		}

		if (!response.ok) {
			const errorText = await response.text();
			// Parse JSON error body for a clean message, fall back to raw text
			let errorMessage = `API Error: ${response.status} - ${errorText}`;
			try {
				const jsonError = JSON.parse(errorText);
				if (jsonError.error || jsonError.message) {
					errorMessage = jsonError.error || jsonError.message;
				}
			} catch {
				// errorText is not JSON — keep the raw fallback message
			}
			console.error('[Cinder] Response not OK:', response.status, errorMessage);

			// Detect common issues from the request body
			if (body?.render === true && errorMessage === 'Scraping failed') {
				throw new Error(
					'Scraping failed — headless browser mode (render) is not available on this server. ' +
						'Try turning OFF the "Browser Rendering" toggle in Advanced Options and scrape again.'
				);
			}

			throw new Error(errorMessage);
		}

		const result = await response.json();
		console.log('[Cinder] Response OK for', url);
		return result;
	} catch (err: any) {
		console.error('Cinder API Error:', err);
		// Include more detail in the error for debugging
		const detail = `Backend URL: ${PRIVATE_CINDER_BACKEND_URL} | Endpoint: ${endpoint} | Method: ${method}`;
		const errorMsg = dev ? `${err.message} — ${detail}` : err.message;
		// Preserve SvelteKit HttpError (e.g. 429) without wrapping
		if (err?.status && err.status >= 400 && err.status < 600) throw err;
		throw error(500, errorMsg || 'Internal Server Error');
	}
}

// --- Helpers for payload normalization ---

function parseJsonField(raw: string | undefined): any {
	if (!raw || !raw.trim()) return undefined;
	try {
		return JSON.parse(raw);
	} catch {
		throw error(400, `Invalid JSON: ${raw.slice(0, 120)}`);
	}
}

function splitCsv(raw: string | undefined): string[] | undefined {
	if (!raw || !raw.trim()) return undefined;
	return raw
		.split(',')
		.map((s) => s.trim())
		.filter(Boolean);
}

function splitLines(raw: string): string[] {
	return raw
		.split(/[\n,]+/)
		.map((s) => s.trim())
		.filter(Boolean);
}

// --- Remote Functions ---

// 1. Scrape — POST /v1/scrape (sync, no Redis)
// Supports single url OR sync multi-URL urls[] (max 10, exclusive) → {results: [...]} with per-entry error.
export const scrapeUrl = form(ScrapeOptionsSchema, async (data) => {
	const payload: Record<string, any> = { ...data };

	// Sync multi-URL: urls textarea (one per line or comma) → urls[] exclusive with url
	const rawUrls = typeof payload.urls === 'string' ? payload.urls.trim() : '';
	if (rawUrls) {
		const urls = splitLines(rawUrls);
		if (urls.length === 0) throw error(400, 'At least one URL is required in urls');
		if (urls.length > 10)
			throw error(400, 'Too many URLs (max 10) for sync scrape — use Batch tab for up to 20 async');
		for (const u of urls) {
			try {
				const parsed = new URL(u);
				if (!['http:', 'https:'].includes(parsed.protocol)) throw new Error();
			} catch {
				throw error(400, `Invalid URL in urls: ${u}`);
			}
		}
		delete payload.url;
		payload.urls = urls;
	} else {
		delete payload.urls;
		if (!payload.url) throw error(400, 'URL is required');
	}

	// Advanced JSON fields come as strings from hidden inputs
	if (typeof payload.extract_schema === 'string') {
		const parsed = parseJsonField(payload.extract_schema);
		if (parsed) payload.extract_schema = parsed;
		else delete payload.extract_schema;
	}
	if (typeof payload.actions === 'string') {
		const parsed = parseJsonField(payload.actions);
		if (parsed) payload.actions = parsed;
		else delete payload.actions;
	}
	// Backend defaults: include_links/block_ads/remove_base64_images default true — only send when explicitly false
	// Valibot defaults already set them to true, so we always send them; backend cache key includes include_links
	if (payload.block_ads === undefined) payload.block_ads = true;
	if (payload.remove_base64_images === undefined) payload.remove_base64_images = true;
	if (payload.include_links === undefined) payload.include_links = true;

	const result = await fetchCinder('/v1/scrape', 'POST', payload);
	return result;
});

// 2. Crawl — POST /v1/crawl (async, Redis required)
export const crawlUrl = form(CrawlOptionsSchema, async (data) => {
	const payload: Record<string, any> = { ...data };
	// Globs: comma-separated strings → arrays
	if (typeof payload.include_paths === 'string') {
		const arr = splitCsv(payload.include_paths);
		if (arr) payload.include_paths = arr;
		else delete payload.include_paths;
	}
	if (typeof payload.exclude_paths === 'string') {
		const arr = splitCsv(payload.exclude_paths);
		if (arr) payload.exclude_paths = arr;
		else delete payload.exclude_paths;
	}
	if (!payload.webhook_url) delete payload.webhook_url;
	if (!payload.webhook_secret) delete payload.webhook_secret;
	if (!payload.image_format) delete payload.image_format;

	const result = await fetchCinder('/v1/crawl', 'POST', payload);
	return result; // { id, url, maxDepth, limit, ... }
});

export const getCrawlStatus = query(v.string(), async (id) => {
	const res = await fetchCinder(`/v1/crawl/${id}`, 'GET');
	// Backend returns { id, queue, state, crawl?, failed_urls?, result? }
	// Older shape used `result` JSON string — handle both
	let parsedResult: any = res.crawl ?? null;
	if (!parsedResult && res.result) {
		try {
			parsedResult = JSON.parse(res.result);
		} catch {
			parsedResult = { message: res.result };
		}
	}
	const crawlFailed =
		parsedResult?.status === 'failed' ||
		(res.state === 'failed' && !parsedResult) ||
		(parsedResult?.failedUrls?.length > 0 && parsedResult?.total === 0);

	return {
		...res,
		parsedResult,
		crawlFailed,
		failedUrls: parsedResult?.failedUrls || res.failed_urls || []
	};
});

// 3. Search — POST /v1/search (SearXNG primary, Brave fallback)
// Now: category (general|news|code) + rerank (TF-IDF) + highlights/relevance per hit
export const searchWeb = form(SearchOptionsSchema, async (data) => {
	if (!isAuthenticated()) {
		try {
			const ip = getClientIP();
			await dailySearchLimiter.consume(ip);
		} catch {
			throw error(429, 'Daily search limit reached. Please try again tomorrow.');
		}
	}
	// Validate category enum server-side (backend also validates, but fail fast)
	if (data.category && !['general', 'news', 'code'].includes(data.category)) {
		throw error(400, 'Invalid category: must be one of general, news, code');
	}
	const processedData: Record<string, any> = {
		...data,
		includeDomains:
			typeof data.includeDomains === 'string' && data.includeDomains
				? data.includeDomains.split(',').map((s) => s.trim())
				: Array.isArray(data.includeDomains)
					? data.includeDomains
					: [],
		excludeDomains:
			typeof data.excludeDomains === 'string' && data.excludeDomains
				? data.excludeDomains.split(',').map((s) => s.trim())
				: Array.isArray(data.excludeDomains)
					? data.excludeDomains
					: [],
		requiredText:
			typeof data.requiredText === 'string' && data.requiredText
				? data.requiredText.split(',').map((s) => s.trim())
				: Array.isArray(data.requiredText)
					? data.requiredText
					: []
	};
	// Only send category/rerank when set so cache keys stay stable
	if (!processedData.category) delete processedData.category;
	if (!processedData.rerank) delete processedData.rerank;

	const result = await fetchCinder('/v1/search', 'POST', processedData);
	return result.results as {
		title: string;
		url: string;
		description: string;
		highlights?: string[];
		relevance?: number;
		domain?: string;
		id?: string;
	}[];
});

// 4. Map — POST /v1/map (no Redis, sitemap → robots.txt → link fallback)
export const mapSite = form(MapOptionsSchema, async (data) => {
	const payload: Record<string, any> = { url: data.url };
	if (data.search) payload.search = data.search;
	if (data.limit) payload.limit = data.limit;
	const result = await fetchCinder('/v1/map', 'POST', payload);
	return result as { url: string; count: number; links: { url: string; title?: string }[] };
});

// 5. Batch — POST /v1/batch/scrape (async, Redis, max 20)
export const batchScrape = form(BatchOptionsSchema, async (data) => {
	const urls = splitLines(data.urls);
	if (urls.length === 0) throw error(400, 'At least one URL is required');
	if (urls.length > 20) throw error(400, 'Too many URLs (max 20)');
	for (const u of urls) {
		try {
			const parsed = new URL(u);
			if (!['http:', 'https:'].includes(parsed.protocol)) throw new Error();
		} catch {
			throw error(400, `Invalid URL: ${u}`);
		}
	}
	const result = await fetchCinder('/v1/batch/scrape', 'POST', { urls });
	return result as { batch_id: string; tasks: { id: string; url: string }[] };
});

export const getBatchStatus = query(v.string(), async (id) => {
	const res = await fetchCinder(`/v1/batch/${id}`, 'GET');
	return res as {
		batch_id: string;
		total: number;
		completed: number;
		failed: number;
		tasks: { id: string; url: string }[];
	};
});

// 6. Monitor — POST /v1/monitor (change tracking, Redis, interval >= 3600)
export const createMonitor = form(MonitorOptionsSchema, async (data) => {
	const payload: Record<string, any> = {
		url: data.url,
		interval_seconds: data.interval_seconds || 3600
	};
	if (payload.interval_seconds < 3600) throw error(400, 'interval_seconds must be at least 3600');
	if (data.webhook_url) payload.webhook_url = data.webhook_url;
	if (data.webhook_secret) payload.webhook_secret = data.webhook_secret;
	const result = await fetchCinder('/v1/monitor', 'POST', payload);
	return result as { id: string; url: string; interval_seconds: number; next_check: string };
});

export const getMonitorStatus = query(v.string(), async (id) => {
	const res = await fetchCinder(`/v1/monitor/${id}`, 'GET');
	return res as {
		id: string;
		url: string;
		interval_seconds: number;
		webhook_url?: string;
		last_hash?: string;
		next_check: string;
	};
});

export const deleteMonitor = command(v.string(), async (id) => {
	const res = await fetchCinder(`/v1/monitor/${id}`, 'DELETE');
	return res;
});
