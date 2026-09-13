<script lang="ts">
	import {
		Search,
		Loader2,
		AlertCircle,
		ArrowRight,
		ExternalLink,
		Globe,
		Sparkles
	} from '@lucide/svelte';
	import { Input } from '$lib/components/ui/input';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import OptionsSheet from '$lib/components/blocks/options-sheet.svelte';
	import { searchWeb } from '$lib/remote/cinder.remote';

	let {
		searchError = $bindable(),
		selectedHistoryItem = $bindable(),
		searchOptions,
		displayedSearchResult,
		addToHistory,
		handleScrape
	} = $props();
</script>

<section class="overflow-hidden rounded-xl border bg-card/50 shadow-sm backdrop-blur-sm">
	<div class="p-6">
		<form
			{...searchWeb.enhance(async ({ submit }) => {
				searchError = null;
				selectedHistoryItem = null;
				try {
					await submit();
					const result = searchWeb.result as any[];
					if (result?.length) {
						const query = searchWeb.fields.query.value();
						addToHistory({
							id: crypto.randomUUID(),
							type: 'search',
							title: query,
							url: 'Search Results',
							timestamp: new Date().toISOString(),
							preview: `${result.length} result${result.length > 1 ? 's' : ''} — ${result[0]?.title || ''}`,
							meta: { count: result.length },
							data: result
						});
					}
				} catch (e: any) {
					searchError = e.body?.message || e.message || 'An unexpected error occurred';
				}
			})}
			class="space-y-4"
		>
			<div class="flex flex-col gap-2">
				<label
					for="search-query"
					class="pl-1 text-xs font-semibold tracking-wider text-muted-foreground uppercase"
				>
					Search Intent
				</label>
				<div class="flex gap-2">
					<div class="relative flex-1">
						<Search class="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
						<Input
							id="search-query"
							{...searchWeb.fields.query.as('text')}
							placeholder="e.g. 'latest openai news'"
							class="h-11 bg-background pl-10"
							aria-invalid={(searchWeb.fields.query?.issues()?.length || 0) > 0}
						/>
					</div>
					<input type="hidden" name="mode" value={searchOptions.current.mode} />
					{#if searchOptions.current.category}
						<input type="hidden" name="category" value={searchOptions.current.category} />
					{/if}
					{#if searchOptions.current.rerank}
						<input type="hidden" name="rerank" value="on" />
					{/if}
					<input type="hidden" name="limit" value={searchOptions.current.limit} />
					<input type="hidden" name="offset" value={searchOptions.current.offset} />
					<input type="hidden" name="maxAge" value={searchOptions.current.maxAge} />
					{#if searchOptions.current.includeDomains}
						<input
							type="hidden"
							name="includeDomains"
							value={searchOptions.current.includeDomains}
						/>
					{/if}
					{#if searchOptions.current.excludeDomains}
						<input
							type="hidden"
							name="excludeDomains"
							value={searchOptions.current.excludeDomains}
						/>
					{/if}
					{#if searchOptions.current.requiredText}
						<input type="hidden" name="requiredText" value={searchOptions.current.requiredText} />
					{/if}
					<OptionsSheet bind:options={searchOptions.current} type="search" />
					<Button type="submit" disabled={!!searchWeb.pending} class="h-11 px-8 shadow-md">
						{#if searchWeb.pending}
							<Loader2 class="mr-2 size-4 animate-spin" />
							Searching...
						{:else}
							Search
							<ArrowRight class="ml-2 size-4" />
						{/if}
					</Button>
				</div>
				{#each searchWeb.fields.query?.issues() || [] as issue (issue.message)}
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
			<span class="flex items-center gap-1.5">
				<Badge variant="outline" class="h-5 rounded px-1.5 text-[9px] uppercase">Engine</Badge> SearXNG
				→ Brave fallback
			</span>
			{#if searchOptions.current.category}
				<span class="flex items-center gap-1.5"
					><Badge variant="secondary" class="h-5 rounded px-1.5 text-[9px] uppercase"
						>{searchOptions.current.category}</Badge
					></span
				>
			{/if}
			{#if searchOptions.current.rerank}
				<span class="flex items-center gap-1.5"><Sparkles class="size-3" /> TF-IDF rerank</span>
			{/if}
		</div>
		<span>Highlights + relevance per hit</span>
	</div>
</section>

{#if searchWeb.pending}
	<div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
		{#each Array(6) as _, i (i)}
			<div
				class="flex h-32 animate-pulse flex-col justify-between rounded-xl border bg-card/40 p-4"
			>
				<div class="space-y-2">
					<div class="h-4 w-3/4 rounded bg-muted"></div>
					<div class="h-3 w-full rounded bg-muted"></div>
				</div>
				<div class="h-3 w-1/2 rounded bg-muted"></div>
			</div>
		{/each}
	</div>
{:else if searchError}
	<div
		class="flex animate-in gap-4 rounded-xl border border-destructive/20 bg-destructive/10 p-6 text-destructive slide-in-from-top-2"
	>
		<AlertCircle class="size-6 shrink-0" />
		<div>
			<h3 class="mb-1 font-bold">Search Missed</h3>
			<p class="text-sm opacity-90">{searchError}</p>
		</div>
	</div>
{:else if displayedSearchResult}
	<div
		class="grid animate-in grid-cols-1 gap-4 duration-500 slide-in-from-bottom-4 md:grid-cols-2 lg:grid-cols-3"
	>
		{#each (displayedSearchResult as any[]) || [] as item (item.url)}
			<div
				class="group flex flex-col justify-between rounded-xl border bg-card p-4 transition-all hover:border-primary/50 hover:shadow-lg"
			>
				<div>
					<h4
						class="mb-2 line-clamp-2 text-sm leading-snug font-bold transition-colors group-hover:text-primary"
					>
						{item.title}
					</h4>
					{#if item.highlights?.length}
						<div
							class="mb-3 rounded-md border border-amber-200 bg-amber-50 px-2.5 py-1.5 text-[11px] leading-relaxed text-amber-900 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-200"
						>
							{#each item.highlights as h}
								<span class="italic">"{h}"</span>
							{/each}
						</div>
					{/if}
					<p class="mb-3 line-clamp-3 text-xs leading-relaxed text-muted-foreground">
						{item.description || 'No description provided.'}
					</p>
					{#if item.relevance !== undefined}
						<div class="mb-2 flex items-center gap-1.5">
							<Badge variant="outline" class="h-5 rounded px-1.5 font-mono text-[9px]"
								>rel {Number(item.relevance).toFixed(2)}</Badge
							>
							{#if item.domain}<span class="font-mono text-[10px] text-muted-foreground"
									>{item.domain}</span
								>{/if}
						</div>
					{/if}
				</div>
				<div class="mt-auto flex items-center justify-between border-t pt-3">
					<span class="max-w-30 truncate font-mono text-[10px] text-muted-foreground"
						>{new URL(item.url || 'http://localhost').hostname}</span
					>
					<Button
						variant="ghost"
						size="icon"
						class="size-7"
						onclick={() => window.open(item.url, '_blank')}
					>
						<ExternalLink class="size-3.5" />
					</Button>
				</div>
				<Button
					variant="secondary"
					size="sm"
					class="mt-2 w-full text-[10px] font-medium"
					onclick={() => handleScrape(item.url)}
				>
					<Globe class="mr-2 size-3" />
					Scrape This
				</Button>
			</div>
		{/each}
	</div>
{:else}
	<div
		class="flex flex-col items-center justify-center rounded-xl border border-dashed bg-muted/10 p-20 opacity-60"
	>
		<Search class="mb-4 size-12 stroke-[1px] text-muted-foreground" />
		<p class="text-sm font-medium">Search across the entire indexed web with AI precision</p>
	</div>
{/if}
