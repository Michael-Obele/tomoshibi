<script lang="ts">
	import { Layers, Loader2, AlertCircle, ExternalLink, Globe } from '@lucide/svelte';
	import { Input } from '$lib/components/ui/input';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import OptionsSheet from '$lib/components/blocks/options-sheet.svelte';
	import { cn } from '$lib/utils';
	import { crawlUrl, getCrawlStatus } from '$lib/remote/cinder.remote';

	let {
		crawlId = $bindable(),
		crawlError = $bindable(),
		selectedHistoryItem = $bindable(),
		crawlOptions,
		addToHistory
	} = $props();

	let crawlCurrent = $state<any>(null);

	// Dedicated polling status query isolated safely to this component
	const statusQuery = $derived(crawlId ? getCrawlStatus(crawlId) : null);

	$effect(() => {
		if (crawlId && statusQuery) {
			crawlCurrent = statusQuery.current;
			const interval = setInterval(async () => {
				await statusQuery.refresh();
				crawlCurrent = statusQuery.current;
			}, 3000);
			return () => clearInterval(interval);
		}
	});

	let crawlProgress = $derived(
		crawlCurrent?.state === 'completed' ? 100 : crawlCurrent?.state === 'active' ? 50 : 25
	);
	let crawlFailed = $derived(crawlCurrent?.crawlFailed ?? false);
	let crawlFailedUrls = $derived(crawlCurrent?.failedUrls || []);
	let crawlList = $derived(
		crawlCurrent?.crawl?.pages ||
			crawlCurrent?.parsedResult?.pages ||
			crawlCurrent?.parsedResult?.data ||
			crawlCurrent?.parsedResult?.urls ||
			(Array.isArray(crawlCurrent?.parsedResult) ? crawlCurrent?.parsedResult : [])
	);
	let crawlMessage = $derived(crawlCurrent?.parsedResult?.message || null);

	// Helpers for processing and displaying titles elegantly
	function extractDisplayTitle(urlStr: string) {
		try {
			const u = new URL(urlStr);
			let p = u.pathname;
			if (p.endsWith('/')) p = p.slice(0, -1);
			const segments = p.split('/');
			const last = segments[segments.length - 1];
			if (!last) return 'Root Index';
			return last.replace(/[-_]/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
		} catch {
			return 'Discovered Node';
		}
	}

	function getPageTitle(page: any) {
		const rawUrl = typeof page === 'string' ? page : page?.url || '';
		const title = typeof page === 'string' ? null : page?.metadata?.title || page?.title;

		if (title && typeof title === 'string' && title.trim() !== '') {
			if (title === rawUrl) return extractDisplayTitle(rawUrl);
			return title;
		}
		return extractDisplayTitle(rawUrl);
	}

	function getPageUrl(page: any) {
		if (typeof page === 'string') return page;
		return page?.url || '';
	}

	function getParsedUrl(urlStr: string) {
		if (!urlStr) return { host: 'unknown', path: '', valid: false };
		try {
			const u = new URL(urlStr);
			return { host: u.hostname, path: u.pathname + u.search, valid: true };
		} catch {
			return { host: urlStr, path: '', valid: false };
		}
	}
</script>

<section class="overflow-hidden rounded-xl border bg-card/50 shadow-sm backdrop-blur-sm">
	<div class="p-6">
		<form
			{...crawlUrl.enhance(async ({ submit }) => {
				crawlError = null;
				selectedHistoryItem = null;
				try {
					await submit();
					const result = crawlUrl.result as any;
					if (result?.id) {
						crawlId = result.id;
						const crawlUrlValue = crawlUrl.fields.url.value();
						addToHistory({
							id: crypto.randomUUID(),
							type: 'crawl',
							title: crawlUrlValue || 'URL Crawl',
							url: 'Crawl Job',
							timestamp: new Date().toISOString(),
							preview: `Crawl ID: ${result.id}`,
							meta: { depth: result.maxDepth, limit: result.limit },
							data: { id: result.id, url: crawlUrlValue }
						});
					}
				} catch (e: any) {
					crawlError = e.message || 'An unexpected error occurred';
				}
			})}
			class="space-y-4"
		>
			<div class="flex flex-col gap-2">
				<label
					for="crawl-url"
					class="pl-1 text-xs font-semibold tracking-wider text-muted-foreground uppercase"
				>
					Crawl Boundary (Sitemap/Domain)
				</label>
				<div class="flex gap-2">
					<div class="relative flex-1">
						<Layers class="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
						<Input
							id="crawl-url"
							{...crawlUrl.fields.url.as('text')}
							placeholder="https://example.com/*"
							class="h-11 bg-background pl-10"
							aria-invalid={(crawlUrl.fields.url?.issues()?.length || 0) > 0}
						/>
					</div>
					<input type="hidden" name="mode" value={crawlOptions.current.mode || 'smart'} />

					<input type="hidden" name="maxDepth" value={crawlOptions.current.maxDepth || 2} />
					<input type="hidden" name="limit" value={crawlOptions.current.limit || 10} />
					{#if crawlOptions.current.screenshot}
						<input type="hidden" name="screenshot" value="on" />
					{/if}
					{#if crawlOptions.current.images}
						<input type="hidden" name="images" value="on" />
					{/if}
					<input type="hidden" name="image_format" value={crawlOptions.current.image_format || 'url'} />
					{#if crawlOptions.current.include_paths}
						<input type="hidden" name="include_paths" value={crawlOptions.current.include_paths} />
					{/if}
					{#if crawlOptions.current.exclude_paths}
						<input type="hidden" name="exclude_paths" value={crawlOptions.current.exclude_paths} />
					{/if}
					{#if crawlOptions.current.webhook_url}
						<input type="hidden" name="webhook_url" value={crawlOptions.current.webhook_url} />
					{/if}
					{#if crawlOptions.current.webhook_secret}
						<input type="hidden" name="webhook_secret" value={crawlOptions.current.webhook_secret} />
					{/if}

					<OptionsSheet bind:options={crawlOptions.current} type="crawl" />
					<Button type="submit" disabled={!!crawlUrl.pending} class="h-11 px-8 shadow-md">
						{#if crawlUrl.pending}
							<Loader2 class="mr-2 size-4 animate-spin" />
							Initializing...
						{:else}
							Start Crawl
							<Layers class="ml-2 size-4" />
						{/if}
					</Button>
				</div>
				{#each crawlUrl.fields.url?.issues() || [] as issue (issue.message)}
					<p class="mt-1 flex items-center gap-1.5 pl-1 text-xs text-destructive">
						<AlertCircle class="size-3" />
						{issue.message}
					</p>
				{/each}
			</div>
		</form>
	</div>
	<div
		class="flex items-center justify-between border-t bg-muted/10 p-4 px-6 text-[11px] font-medium text-muted-foreground"
	>
		<div class="flex items-center gap-4">
			<span class="flex items-center gap-1.5 text-amber-500">
				<span class="size-1.5 animate-pulse rounded-full bg-amber-500"></span>
				Max Depth: {crawlOptions.current.maxDepth || 2}
			</span>
			<span class="flex items-center gap-1.5">Limit: {crawlOptions.current.limit || 10} pages</span>
		</div>
		<span>Crawl engine idle</span>
	</div>
</section>

{#if crawlId}
	<div class="animate-in space-y-6 duration-500 fade-in">
		<div
			class="relative overflow-hidden rounded-2xl border bg-card/60 p-6 shadow-sm backdrop-blur-md"
		>
			<!-- Subtle Background Glow -->
			<div class="absolute -top-24 -right-24 h-48 w-48 rounded-full bg-primary/5 blur-[80px]"></div>

			<div class="relative mb-6 flex flex-col justify-between gap-4 sm:flex-row sm:items-center">
				<div class="flex items-center gap-4">
					<div
						class="relative flex size-12 shrink-0 items-center justify-center rounded-xl bg-linear-to-br from-primary/20 to-primary/5 ring-1 ring-primary/20 ring-inset"
					>
						<div
							class="absolute inset-0 rounded-xl bg-primary/10 {crawlCurrent?.state === 'active'
								? 'animate-pulse'
								: ''}"
						></div>
						<Layers class="relative z-10 size-5 text-primary" />
					</div>
					<div>
						<h3 class="text-lg font-semibold tracking-tight">Active Crawl Mission</h3>
						<p
							class="font-mono text-[11px] font-medium tracking-tight text-muted-foreground uppercase opacity-80"
						>
							ID: <span class="text-foreground/80">{crawlId}</span>
						</p>
					</div>
				</div>
				<Badge
					class={cn(
						'self-start border-0 px-3.5 py-1.5 text-[10px] font-bold tracking-wider uppercase shadow-sm sm:self-auto',
						crawlCurrent?.state === 'completed' && !crawlFailed
							? 'bg-emerald-500/15 text-emerald-600 ring-1 ring-emerald-500/20 ring-inset hover:bg-emerald-500/20'
							: crawlCurrent?.state === 'failed' || crawlCurrent?.state === 'retry' || crawlFailed
								? 'bg-destructive/15 text-destructive ring-1 ring-destructive/20 ring-inset hover:bg-destructive/20'
								: 'animate-pulse bg-primary/15 text-primary ring-1 ring-primary/20 ring-inset hover:bg-primary/20'
					)}
				>
					{crawlFailed
						? 'Failed'
						: crawlCurrent?.state === 'active'
							? 'Crawling...'
							: crawlCurrent?.state || 'Processing'}
				</Badge>
			</div>

			<div class="space-y-4">
				<div class="mb-1 flex justify-between text-xs font-bold tracking-tight">
					<span class="text-muted-foreground">Crawl Progress</span>
					<span class="text-emerald-600 dark:text-emerald-400"
						>{crawlList.length || 0} Pages Found</span
					>
				</div>
				<div class="h-2 w-full overflow-hidden rounded-full border bg-muted/50 p-0.5">
					<div
						class={cn(
							'h-full rounded-full transition-all duration-700 ease-out',
							crawlFailed
								? 'bg-destructive shadow-[0_0_10px_rgba(239,68,68,0.5)]'
								: 'bg-primary shadow-[0_0_10px_hsla(var(--primary)/0.5)]'
						)}
						style="width: {crawlProgress}%"
					></div>
				</div>
				<p class="pb-2 text-[10px] font-medium tracking-wide text-muted-foreground">
					Current Mission: <span class="text-foreground/80 capitalize"
						>{crawlCurrent?.state || 'Initializing'}</span
					>
					exploration on {crawlUrl.fields.url.value() || 'target'}
				</p>
			</div>
		</div>

		{#if crawlList?.length > 0}
			<div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
				{#each crawlList as page, i (getPageUrl(page) + i)}
					{@const parsed = getParsedUrl(getPageUrl(page))}
					<div
						class="group relative flex flex-col justify-between overflow-hidden rounded-xl border border-border/40 bg-background/50 p-5 transition-all duration-300 hover:border-primary/40 hover:bg-card hover:shadow-xl hover:shadow-primary/5"
					>
						<div
							class="pointer-events-none absolute inset-0 bg-linear-to-br from-primary/5 to-transparent opacity-0 transition-opacity duration-300 group-hover:opacity-100"
						></div>

						<div class="relative z-10 mb-4">
							<div class="mb-3 flex items-start justify-between gap-4">
								<div class="flex items-center gap-2 text-primary/70">
									<Layers class="size-3.5" />
									<span class="font-mono text-[10px] font-bold tracking-wider uppercase">Page</span>
								</div>
								<Button
									variant="ghost"
									size="icon"
									href={getPageUrl(page)}
									target="_blank"
									class="size-6 shrink-0 -translate-x-2 opacity-0 transition-all duration-300 group-focus-within:translate-x-0 group-focus-within:opacity-100 group-hover:translate-x-0 group-hover:opacity-100"
								>
									<ExternalLink class="size-3.5 text-muted-foreground hover:text-primary" />
								</Button>
							</div>

							<h4
								class="line-clamp-2 text-sm font-semibold tracking-tight text-foreground/90 transition-colors group-hover:text-foreground"
							>
								{getPageTitle(page)}
							</h4>
						</div>

						<div class="relative z-10 mt-auto border-t border-border/40 pt-3">
							<div class="flex items-center gap-1.5 truncate font-mono text-[11px]">
								<Globe class="size-3 shrink-0 text-muted-foreground/60" />
								{#if parsed.valid}
									<span class="text-foreground/70">{parsed.host}</span>
									<span class="truncate text-muted-foreground/60">{parsed.path}</span>
								{:else}
									<span class="truncate text-muted-foreground/60">{parsed.host}</span>
								{/if}
							</div>
						</div>
					</div>
				{/each}
			</div>
		{:else if crawlFailed && crawlFailedUrls.length > 0}
			<div class="animate-in space-y-3 duration-500 fade-in">
				<div
					class="flex items-center gap-4 rounded-xl border border-destructive/20 bg-destructive/10 p-6"
				>
					<AlertCircle class="size-6 shrink-0 text-destructive" />
					<div>
						<h3 class="mb-1 font-bold text-destructive">Crawl Failed</h3>
						<p class="text-sm text-muted-foreground">
							{crawlFailedUrls.length} URL{crawlFailedUrls.length > 1 ? 's' : ''} failed to crawl.
						</p>
					</div>
				</div>
				{#each crawlFailedUrls as failedUrl (failedUrl.url)}
					<div class="rounded-lg border border-destructive/10 bg-destructive/5 p-4">
						<p class="mb-1 truncate font-mono text-xs font-medium">{failedUrl.url}</p>
						<p class="text-xs text-muted-foreground">{failedUrl.error}</p>
					</div>
				{/each}
			</div>
		{:else if crawlMessage && crawlCurrent?.state === 'completed' && !crawlFailed}
			<div
				class="flex animate-in items-center gap-4 rounded-xl border border-emerald-500/20 bg-emerald-500/10 p-6 duration-500 fade-in"
			>
				<div
					class="flex size-10 shrink-0 items-center justify-center rounded-full bg-emerald-500/20"
				>
					<Layers class="size-5 text-emerald-500" />
				</div>
				<div>
					<h3 class="mb-1 font-bold text-emerald-400">Crawl Complete</h3>
					<p class="text-sm text-muted-foreground">{crawlMessage}</p>
				</div>
			</div>
		{/if}
	</div>
{:else if crawlError}
	<div
		class="flex animate-in gap-4 rounded-xl border border-destructive/20 bg-destructive/10 p-6 text-destructive slide-in-from-top-2"
	>
		<AlertCircle class="size-6 shrink-0" />
		<div>
			<h3 class="mb-1 font-bold">Crawl Failed</h3>
			<p class="text-sm opacity-90">{crawlError}</p>
		</div>
	</div>
{:else}
	<div
		class="group flex flex-col items-center justify-center rounded-2xl border border-dashed border-muted/50 bg-linear-to-b from-muted/20 to-muted/5 p-24 opacity-80 transition-all hover:border-primary/30 hover:opacity-100"
	>
		<div
			class="mb-6 rounded-full bg-muted/30 p-4 ring-1 ring-muted-foreground/20 transition-transform ring-inset group-hover:scale-110"
		>
			<Layers
				class="size-10 stroke-[1.5px] text-muted-foreground transition-colors group-hover:text-primary"
			/>
		</div>
		<p class="text-center text-lg font-medium tracking-tight">
			Define a boundary to start a <span class="text-primary italic">massive</span> discovery mission.
		</p>
		<p class="mt-2 text-sm text-muted-foreground/80">
			Sitemap exploration and recursive discovery.
		</p>
	</div>
{/if}
