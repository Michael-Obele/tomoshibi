<script lang="ts">
	import { Map, Globe, Loader2, AlertCircle, ExternalLink, Search, Copy } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Badge } from '$lib/components/ui/badge';
	import * as Card from '$lib/components/ui/card';
	import { mapSite } from '$lib/remote/cinder.remote';

	let {
		mapError = $bindable(),
		selectedHistoryItem = $bindable(),
		mapOptions,
		addToHistory
	} = $props<{
		mapError: string | null;
		selectedHistoryItem: any;
		mapOptions: { current: Record<string, any> };
		addToHistory: (item: any) => Promise<void>;
	}>();

	let result = $derived(mapSite.result as any);
	let pending = $derived(!!mapSite.pending);
	let links = $derived(result?.links ?? []);
	let count = $derived(result?.count ?? links.length);

	function hostnameOf(url: string) {
		try {
			return new URL(url).hostname;
		} catch {
			return url;
		}
	}
</script>

<section class="overflow-hidden rounded-xl border bg-card/50 shadow-sm backdrop-blur-sm">
	<div class="p-6">
		<form
			{...mapSite.enhance(async ({ submit }) => {
				mapError = null;
				selectedHistoryItem = null;
				try {
					await submit();
					const r = mapSite.result as any;
					if (r?.url) {
						addToHistory({
							id: crypto.randomUUID(),
							type: 'map',
							title: hostnameOf(r.url),
							url: r.url,
							timestamp: new Date().toISOString(),
							preview: `${r.count ?? r.links?.length ?? 0} URLs discovered`,
							meta: { count: r.count ?? r.links?.length ?? 0 },
							data: r
						});
					}
				} catch (e: any) {
					mapError = e.body?.message || e.message || 'Map failed';
				}
			})}
			class="space-y-4"
		>
			<div class="flex flex-col gap-2">
				<label
					for="map-url"
					class="pl-1 text-xs font-semibold tracking-wider text-muted-foreground uppercase"
				>
					Site URL to Map
				</label>
				<div class="flex gap-2">
					<div class="relative flex-1">
						<Globe class="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
						<Input
							id="map-url"
							{...mapSite.fields.url.as('text')}
							placeholder="https://example.com"
							class="h-11 bg-background pl-10"
							aria-invalid={(mapSite.fields.url?.issues()?.length || 0) > 0}
						/>
					</div>
					<input type="hidden" name="search" value={mapOptions.current.search ?? ''} />
					<input type="hidden" name="limit" value={mapOptions.current.limit ?? 100} />
					<Button type="submit" disabled={pending} class="h-11 px-8 shadow-md">
						{#if pending}
							<Loader2 class="mr-2 size-4 animate-spin" /> Mapping...
						{:else}
							<Map class="mr-2 size-4" /> Map Site
						{/if}
					</Button>
				</div>
				{#each mapSite.fields.url?.issues() || [] as issue (issue.message)}
					<p class="mt-1 flex items-center gap-1.5 pl-1 text-xs text-destructive">
						<AlertCircle class="size-3" />
						{issue.message}
					</p>
				{/each}
				<div class="flex flex-wrap gap-2 pt-1">
					<div class="flex items-center gap-2">
						<Search class="size-3.5 text-muted-foreground" />
						<Input
							placeholder="Filter (search substring)"
							value={mapOptions.current.search ?? ''}
							oninput={(e) => (mapOptions.current.search = (e.target as HTMLInputElement).value)}
							class="h-8 w-56 text-xs"
						/>
					</div>
					<div class="flex items-center gap-2">
						<Badge variant="outline" class="h-6 text-[10px] uppercase">Limit</Badge>
						<Input
							type="number"
							min="1"
							max="5000"
							value={mapOptions.current.limit ?? 100}
							oninput={(e) =>
								(mapOptions.current.limit = Number((e.target as HTMLInputElement).value) || 100)}
							class="h-8 w-24 text-xs"
						/>
					</div>
				</div>
			</div>
		</form>
	</div>
	<div
		class="flex items-center justify-between border-t bg-muted/10 p-4 px-6 text-[11px] font-medium text-muted-foreground"
	>
		<span class="flex items-center gap-1.5"
			><Badge variant="outline" class="h-5 rounded px-1.5 text-[9px] uppercase">Engine</Badge> Sitemap
			→ robots.txt → Links</span
		>
		<span>No Redis required</span>
	</div>
</section>

{#if pending}
	<div
		class="flex flex-col items-center justify-center rounded-xl border border-dashed bg-muted/20 p-12"
	>
		<Loader2 class="mb-3 size-8 animate-spin text-primary" />
		<p class="text-sm font-medium">Discovering URLs...</p>
		<p class="text-xs text-muted-foreground">Checking sitemap.xml, robots.txt and page links</p>
	</div>
{:else if mapError}
	<div
		class="flex gap-4 rounded-xl border border-destructive/20 bg-destructive/10 p-6 text-destructive"
	>
		<AlertCircle class="size-6 shrink-0" />
		<div>
			<h3 class="mb-1 font-bold">Map Failed</h3>
			<p class="text-sm opacity-90">{mapError}</p>
		</div>
	</div>
{:else if result}
	<Card.Root>
		<Card.Header class="pb-3">
			<Card.Title class="flex items-center gap-2 text-base">
				<Map class="size-4 text-primary" /> Discovered URLs
				<Badge variant="secondary" class="ml-2 text-[10px]">{count} found</Badge>
			</Card.Title>
			<Card.Description class="truncate font-mono text-xs">{result.url}</Card.Description>
		</Card.Header>
		<Card.Content>
			{#if links.length === 0}
				<p class="py-8 text-center text-sm text-muted-foreground">
					No URLs found. Try increasing limit or removing filter.
				</p>
			{:else}
				<div class="grid gap-2 md:grid-cols-2">
					{#each links.slice(0, 100) as link (link.url)}
						<a
							href={link.url}
							target="_blank"
							rel="noopener noreferrer"
							class="group flex items-center justify-between rounded-lg border bg-card p-3 transition-colors hover:border-primary/40 hover:bg-muted/50"
						>
							<div class="min-w-0 flex-1">
								<div class="truncate text-xs font-medium group-hover:text-primary">
									{link.title || link.url}
								</div>
								<div class="truncate font-mono text-[10px] text-muted-foreground">{link.url}</div>
							</div>
							<ExternalLink
								class="ml-2 size-3.5 shrink-0 text-muted-foreground group-hover:text-primary"
							/>
						</a>
					{/each}
				</div>
				{#if links.length > 100}
					<p class="mt-3 text-center text-xs text-muted-foreground">
						Showing 100 of {links.length} — increase limit to see more.
					</p>
				{/if}
				<div class="mt-4 flex gap-2">
					<Button
						variant="outline"
						size="sm"
						class="text-xs"
						onclick={() => navigator.clipboard.writeText(links.map((l: any) => l.url).join('\n'))}
					>
						<Copy class="mr-2 size-3" /> Copy URLs
					</Button>
					<Button
						variant="outline"
						size="sm"
						class="text-xs"
						onclick={() => navigator.clipboard.writeText(JSON.stringify(result, null, 2))}
					>
						<Copy class="mr-2 size-3" /> Copy JSON
					</Button>
				</div>
			{/if}
		</Card.Content>
	</Card.Root>
{:else}
	<div
		class="flex flex-col items-center justify-center rounded-xl border border-dashed bg-muted/10 p-16 opacity-60"
	>
		<Map class="mb-3 size-10 stroke-[1px] text-muted-foreground" />
		<p class="text-sm font-medium">Map any site's URL graph</p>
		<p class="mt-1 max-w-md text-center text-xs text-muted-foreground">
			Discovers URLs via sitemap.xml → robots.txt → link traversal. Filter with search, cap with
			limit (max 5000).
		</p>
	</div>
{/if}
