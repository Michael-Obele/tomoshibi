<script lang="ts">
	import { scrapeUrl, crawlUrl, searchWeb, getCrawlStatus } from '$lib/remote/cinder.remote';
	import { IsMobile } from '$lib/hooks/is-mobile.svelte';
	import * as Sheet from '$lib/components/ui/sheet';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Badge } from '$lib/components/ui/badge';
	import { Tabs, TabsContent, TabsList, TabsTrigger } from '$lib/components/ui/tabs';
	import {
		Search,
		Globe,
		Layers,
		Map,
		Package,
		Monitor,
		History,
		ExternalLink,
		Loader2,
		AlertCircle,
		Trash2,
		Copy,
		ArrowRight,
		Clock
	} from '@lucide/svelte';
	import CodeViewer from '$lib/components/blocks/code-viewer.svelte';
	import ResultCard from '$lib/components/blocks/result-card.svelte';
	import OptionsSheet from '$lib/components/blocks/options-sheet.svelte';
	import ScrapeTab from '$lib/components/blocks/scrape-tab.svelte';
	import CrawlTab from '$lib/components/blocks/crawl-tab.svelte';
	import SearchTab from '$lib/components/blocks/search-tab.svelte';
	import MapTab from '$lib/components/blocks/map-tab.svelte';
	import BatchTab from '$lib/components/blocks/batch-tab.svelte';
	import MonitorTab from '$lib/components/blocks/monitor-tab.svelte';
	import { cn } from '$lib/utils';
	import { tick } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { createReactiveDB } from 'svelte-idb/svelte';
	import { PersistedState } from 'runed';

	const scrapeOptions = new PersistedState('cinder-scrape-options', {
		mode: 'smart',
		screenshot: false,
		images: false,
		image_format: 'url',
		max_images: 10,
		max_image_size_kb: 5120,
		summary: false,
		summary_sentences: 5,
		redact_pii: false,
		block_ads: true,
		remove_base64_images: true,
		include_links: true,
		extract_schema: '',
		actions: '',
		urls: ''
	});

	const crawlOptions = new PersistedState('cinder-crawl-options', {
		mode: 'smart',
		maxDepth: 2,
		limit: 10,
		screenshot: false,
		images: false,
		image_format: 'url',
		include_paths: '',
		exclude_paths: '',
		webhook_url: '',
		webhook_secret: ''
	});

	const searchOptions = new PersistedState('cinder-search-options', {
		mode: 'default',
		category: '',
		rerank: false,
		limit: 5,
		offset: 0,
		maxAge: 0,
		includeDomains: '',
		excludeDomains: '',
		requiredText: ''
	});

	const mapOptions = new PersistedState('cinder-map-options', {
		search: '',
		limit: 100
	});

	const monitorOptions = new PersistedState('cinder-monitor-options', {
		interval_seconds: 3600,
		webhook_url: '',
		webhook_secret: ''
	});

	const db = createReactiveDB({
		name: 'cinder-playground',
		version: 1,
		stores: {
			history: { keyPath: 'id' }
		}
	});

	const historyLive = db.history.liveAll();

	// Local State
	const isMobile = new IsMobile();
	let crawlId = $state<string | null>(null);
	const ALL_TABS = ['scrape', 'crawl', 'search', 'map', 'batch', 'monitor'] as const;
	let activeTab = $derived(
		(ALL_TABS as readonly string[]).includes(page.url.searchParams.get('tab') || '')
			? page.url.searchParams.get('tab')!
			: 'scrape'
	);

	function setActiveTab(tab: string) {
		const newUrl = new URL(page.url);
		newUrl.searchParams.set('tab', tab);
		goto(newUrl, { replaceState: true, keepFocus: true, noScroll: true });
	}

	let sidebarOpen = $state(true);

	// Error States
	let scrapeError = $state<string | null>(null);
	let crawlError = $state<string | null>(null);
	let searchError = $state<string | null>(null);
	let mapError = $state<string | null>(null);
	let batchError = $state<string | null>(null);
	let monitorError = $state<string | null>(null);

	// Derived query for crawl status
	const statusQuery = $derived(crawlId ? getCrawlStatus(crawlId) : null);
	let crawlCurrent = $state<any>(null);

	// History Management
	type HistoryItem = {
		id: string;
		type: 'scrape' | 'crawl' | 'search' | 'map' | 'batch' | 'monitor';
		title: string;
		url: string;
		timestamp: string;
		preview?: string;
		meta?: Record<string, any>;
		data?: any;
	};
	let selectedHistoryItem = $state<HistoryItem | null>(null);

	let searchHistory = $derived(
		historyLive.current
			? ([...historyLive.current].sort(
					(a: any, b: any) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime()
				) as HistoryItem[])
			: []
	);

	async function addToHistory(item: HistoryItem) {
		await db.history.put(item);
		// Keep only latest 20 items
		const allItems = (await db.history.getAll()) as HistoryItem[];
		if (allItems.length > 20) {
			const sorted = allItems.sort(
				(a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime()
			);
			const toDelete = sorted.slice(20);
			for (const old of toDelete) {
				await db.history.delete(old.id);
			}
		}
	}

	async function clearHistory() {
		await db.history.clear();
		selectedHistoryItem = null;
	}

	// Polling is handled strictly in CrawlTab, but isCurrentlyLoading needs crawlUrl loading bounds.
	let displayedScrapeResult = $derived(
		selectedHistoryItem?.type === 'scrape' && selectedHistoryItem.data
			? selectedHistoryItem.data
			: scrapeUrl.result
	);
	let displayedSearchResult = $derived(
		selectedHistoryItem?.type === 'search' && selectedHistoryItem.data
			? selectedHistoryItem.data
			: searchWeb.result
	);
	let isCurrentlyLoading = $derived(
		!!scrapeUrl.pending ||
			!!crawlUrl.pending ||
			!!searchWeb.pending ||
			(!!statusQuery?.loading && !!crawlId)
	);

	async function handleScrape(url: string) {
		setActiveTab('scrape');
		// Wait for tab switch and DOM update
		await tick();

		const input = document.getElementById('scrape-url') as HTMLInputElement;
		if (input) {
			input.value = url;
			input.dispatchEvent(new Event('input', { bubbles: true }));
		}
	}
</script>

{#snippet historyContent()}
	<div class="flex h-full flex-col">
		<!-- Header -->
		<div class="flex items-center gap-2 border-b bg-background/50 p-4 backdrop-blur">
			<History class="size-4 text-muted-foreground" />
			<h2 class="text-sm font-semibold">History</h2>
		</div>

		<!-- History List -->
		<div class="flex-1 space-y-2 overflow-y-auto p-3">
			{#each searchHistory as item (item.id)}
				<button
					class="group relative w-full rounded-lg border bg-card p-3 text-left shadow-xs transition-all hover:border-primary/50 hover:shadow-sm {selectedHistoryItem?.id ===
					item.id
						? 'border-primary/40 bg-primary/3 shadow-sm'
						: 'border-border/60'}"
					onclick={() => {
						setActiveTab(item.type);
						selectedHistoryItem = item;
						if (item.type === 'crawl' && item.data?.id) {
							crawlId = item.data.id;
						}
					}}
				>
					<div class="mb-1.5 flex items-center gap-2">
						{#if item.type === 'scrape'}
							<div class="flex size-5 items-center justify-center rounded bg-blue-500/10">
								<Globe class="size-3 text-blue-500" />
							</div>
						{:else if item.type === 'crawl'}
							<div class="flex size-5 items-center justify-center rounded bg-amber-500/10">
								<Layers class="size-3 text-amber-500" />
							</div>
						{:else if item.type === 'map'}
							<div class="flex size-5 items-center justify-center rounded bg-emerald-500/10">
								<Map class="size-3 text-emerald-500" />
							</div>
						{:else if item.type === 'batch'}
							<div class="flex size-5 items-center justify-center rounded bg-violet-500/10">
								<Package class="size-3 text-violet-500" />
							</div>
						{:else if item.type === 'monitor'}
							<div class="flex size-5 items-center justify-center rounded bg-rose-500/10">
								<Monitor class="size-3 text-rose-500" />
							</div>
						{:else}
							<div class="flex size-5 items-center justify-center rounded bg-primary/10">
								<Search class="size-3 text-primary" />
							</div>
						{/if}
						<span class="text-[10px] font-bold tracking-wider text-muted-foreground uppercase">
							{item.type}
						</span>
						{#if item.meta}
							{#if item.type === 'scrape' && item.meta.images > 0}
								<Badge variant="secondary" class="h-4 rounded px-1 text-[8px] font-bold"
									>{item.meta.images} img</Badge
								>
							{/if}
							{#if item.type === 'scrape' && item.meta.screenshot}
								<Badge variant="secondary" class="h-4 rounded px-1 text-[8px] font-bold">ss</Badge>
							{/if}
							{#if item.type === 'search' && item.meta.count}
								<Badge variant="secondary" class="h-4 rounded px-1 text-[8px] font-bold"
									>{item.meta.count}</Badge
								>
							{/if}
						{/if}
					</div>
					<div class="mb-0.5 truncate pr-4 text-xs font-semibold text-foreground">{item.title}</div>
					{#if item.type === 'scrape' && item.url && item.url !== 'Unknown'}
						{@const hostname = (() => {
							try {
								return new URL(item.url).hostname;
							} catch {
								return '';
							}
						})()}
						{#if hostname}
							<div class="truncate text-[11px] font-medium text-muted-foreground/80">
								{hostname}
							</div>
						{/if}
					{/if}
					{#if item.preview}
						<div class="mt-1 line-clamp-2 text-[11px] leading-relaxed text-muted-foreground/70">
							{item.preview}
						</div>
					{/if}
					<div
						class="mt-2 flex items-center gap-1 border-t border-border/40 pt-1.5 text-[9px] font-medium text-muted-foreground/60"
					>
						<Clock class="size-2.5" />
						{new Date(item.timestamp).toLocaleDateString([], { month: 'short', day: 'numeric' })}
						{new Date(item.timestamp).toLocaleTimeString([], {
							hour: '2-digit',
							minute: '2-digit'
						})}
					</div>
				</button>
			{:else}
				<div
					class="flex flex-col items-center justify-center h-40 text-muted-foreground opacity-50"
				>
					<History class="size-8 mb-2 stroke-[1px]" />
					<p class="text-xs">No history yet</p>
				</div>
			{/each}
		</div>

		<!-- Footer: Clear History Action -->
		<div class="border-t bg-background/50 p-3 backdrop-blur">
			<Button
				variant="destructive"
				size="sm"
				class="w-full"
				onclick={() => clearHistory()}
				title="Clear all history"
			>
				<Trash2 class="mr-2 size-3.5" />
				Clear History
			</Button>
		</div>
	</div>
{/snippet}

<div class="flex flex-1 flex-col bg-background md:flex-row">
	<!-- History Sidebar (Desktop Only) -->
	{#if !isMobile.current && sidebarOpen}
		<aside
			class="relative flex min-h-[50vh] w-80 shrink-0 flex-col self-start border-r border-b bg-muted/30 md:sticky md:top-16 md:min-h-[calc(100vh-4rem)] md:border-b-0"
		>
			{@render historyContent()}
		</aside>
	{/if}

	<!-- Main Workbench Area -->
	<main class="w-full min-w-0 flex-1">
		<div class="mx-auto max-w-6xl space-y-8 p-6">
			<div class="flex flex-col justify-between gap-4 border-b pb-6 md:flex-row md:items-end">
				<div>
					<h1 class="mb-1 text-3xl font-bold tracking-tight">Workbench</h1>
					<p class="text-muted-foreground">
						Master the web with precision crawling and extraction.
					</p>
				</div>
				<div class="flex items-center gap-2">
					{#if isMobile.current}
						<Sheet.Root>
							<Sheet.Trigger class="inline-flex">
								<Button variant="outline" size="sm">
									<History class="mr-2 size-4" />
									History
								</Button>
							</Sheet.Trigger>
							<Sheet.Content side="left" class="w-80 p-0">
								{@render historyContent()}
							</Sheet.Content>
						</Sheet.Root>
					{:else}
						<Button variant="outline" size="sm" onclick={() => (sidebarOpen = !sidebarOpen)}>
							<History class="mr-2 size-4" />
							{sidebarOpen ? 'Hide History' : 'Show History'}
						</Button>
					{/if}
					<div
						class="flex items-center gap-2 rounded-full border bg-muted/50 px-3 py-1.5 text-[10px] font-medium"
					>
						<div
							class={cn(
								'size-2 rounded-full',
								isCurrentlyLoading ? 'animate-pulse bg-primary' : 'bg-emerald-500'
							)}
						></div>
						{isCurrentlyLoading ? 'Processing' : 'System Ready'}
					</div>
				</div>
			</div>

			<Tabs value={activeTab} onValueChange={setActiveTab} class="w-full">
				<TabsList class="mb-6 grid w-full max-w-3xl grid-cols-3 gap-1 sm:grid-cols-6">
					<TabsTrigger value="scrape" class="gap-1.5 text-xs">
						<Globe class="size-3.5" />
						Scrape
					</TabsTrigger>
					<TabsTrigger value="crawl" class="gap-1.5 text-xs">
						<Layers class="size-3.5" />
						Crawl
					</TabsTrigger>
					<TabsTrigger value="search" class="gap-1.5 text-xs">
						<Search class="size-3.5" />
						Search
					</TabsTrigger>
					<TabsTrigger value="map" class="gap-1.5 text-xs">
						<Map class="size-3.5" />
						Map
					</TabsTrigger>
					<TabsTrigger value="batch" class="gap-1.5 text-xs">
						<Package class="size-3.5" />
						Batch
					</TabsTrigger>
					<TabsTrigger value="monitor" class="gap-1.5 text-xs">
						<Monitor class="size-3.5" />
						Monitor
					</TabsTrigger>
				</TabsList>

				<!-- Scrape Tab Content -->
				<TabsContent value="scrape" class="space-y-6 focus-visible:outline-none">
					<ScrapeTab
						bind:scrapeError
						bind:selectedHistoryItem
						{scrapeOptions}
						{displayedScrapeResult}
						{addToHistory}
					/>
				</TabsContent>

				<!-- Crawl Tab Content -->
				<TabsContent value="crawl" class="space-y-6 focus-visible:outline-none">
					<CrawlTab
						bind:crawlError
						bind:crawlId
						bind:selectedHistoryItem
						{crawlOptions}
						{addToHistory}
					/>
				</TabsContent>

				<!-- Search Tab Content -->
				<TabsContent value="search" class="space-y-6 focus-visible:outline-none">
					<SearchTab
						bind:searchError
						bind:selectedHistoryItem
						{searchOptions}
						{displayedSearchResult}
						{addToHistory}
						{handleScrape}
					/>
				</TabsContent>

				<!-- Map Tab Content -->
				<TabsContent value="map" class="space-y-6 focus-visible:outline-none">
					<MapTab bind:mapError bind:selectedHistoryItem {mapOptions} {addToHistory} />
				</TabsContent>

				<!-- Batch Tab Content -->
				<TabsContent value="batch" class="space-y-6 focus-visible:outline-none">
					<BatchTab bind:batchError bind:selectedHistoryItem {addToHistory} />
				</TabsContent>

				<!-- Monitor Tab Content -->
				<TabsContent value="monitor" class="space-y-6 focus-visible:outline-none">
					<MonitorTab bind:monitorError bind:selectedHistoryItem {monitorOptions} {addToHistory} />
				</TabsContent>
			</Tabs>
		</div>
	</main>
</div>
