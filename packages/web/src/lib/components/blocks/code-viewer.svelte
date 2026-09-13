<script lang="ts">
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Card from '$lib/components/ui/card';
	import { ScrollArea } from '$lib/components/ui/scroll-area';
	import { Badge } from '$lib/components/ui/badge';
	import { Copy, Check, Image, Camera } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button';
	import ImageGallery from './image-gallery.svelte';

	let { result } = $props();
	// Result can be ScrapeResult (markdown, html, images, screenshot, etc.)

	let copied = $state(false);

	// Determine available tabs
	let hasImages = $derived(!!result?.images?.length);
	let hasScreenshot = $derived(!!result?.screenshot?.blob || !!result?.screenshot?.url);
	let tabCount = $derived(4 + (hasImages ? 1 : 0) + (hasScreenshot ? 1 : 0));

	// Default to images when available, then screenshot, else markdown
	// Writable $derived: assigning to it temporarily overrides the computed value
	// until the expression re-evaluates (when result/hasImages/hasScreenshot change)
	let activeTab = $derived(
		!result ? 'markdown' : hasImages ? 'images' : hasScreenshot ? 'screenshot' : 'markdown'
	);

	let isMulti = $derived(!!result?.results && Array.isArray(result.results));

	function formatLinks(links: any): string {
		if (!links || links.length === 0) return '';
		if (typeof links[0] === 'string') return links.join('\n');
		return links
			.map((l: any) =>
				l.url
					? `${l.url}${l.text ? ` — ${l.text}` : ''}${l.isInternal ? ' (internal)' : ''}`
					: JSON.stringify(l)
			)
			.join('\n');
	}

	function copyToClipboard() {
		let content = '';
		if (activeTab === 'markdown') {
			content = isMulti
				? result.results
						.map((r: any) => `# ${r.url}\n\n${r.markdown || r.error || ''}`)
						.join('\n\n---\n\n')
				: result.markdown;
		} else if (activeTab === 'html') {
			content = isMulti ? result.results.map((r: any) => r.html || '').join('\n\n') : result.html;
		} else if (activeTab === 'json') content = JSON.stringify(result, null, 2);
		else if (activeTab === 'links') {
			const links = isMulti
				? result.results.flatMap((r: any) => r.links || [])
				: result.links || [];
			content = formatLinks(links);
		}

		navigator.clipboard.writeText(content || '');
		copied = true;
		setTimeout(() => (copied = false), 2000);
	}

	function getScreenshotSrc(): string {
		const sc = result.screenshot;
		if (!sc) return '';
		if (sc.blob) return `data:image/${sc.format || 'jpeg'};base64,${sc.blob}`;
		return sc.url || '';
	}
</script>

<Card.Root class="flex h-full flex-col border-0 bg-muted/50 shadow-none">
	<Tabs.Root value={activeTab} onValueChange={(v) => (activeTab = v)} class="flex flex-1 flex-col">
		<div
			class="sticky top-0 z-10 flex items-center justify-between border-b bg-background/50 px-4 py-2 backdrop-blur"
		>
			<Tabs.List class="grid w-fit gap-0.5" style="grid-template-columns: repeat({tabCount}, auto)">
				<Tabs.Trigger value="markdown">Markdown</Tabs.Trigger>
				<Tabs.Trigger value="html">HTML</Tabs.Trigger>
				<Tabs.Trigger value="json">JSON</Tabs.Trigger>
				<Tabs.Trigger value="links">Links</Tabs.Trigger>
				{#if hasImages}
					<Tabs.Trigger value="images" class="gap-1.5"
						><Image class="size-3.5" /> Images</Tabs.Trigger
					>
				{/if}
				{#if hasScreenshot}
					<Tabs.Trigger value="screenshot" class="gap-1.5"
						><Camera class="size-3.5" /> Screenshot</Tabs.Trigger
					>
				{/if}
			</Tabs.List>

			<div class="flex items-center gap-2">
				{#if result.statusCode}
					<Badge
						variant={result.statusCode >= 200 && result.statusCode < 300
							? 'default'
							: 'destructive'}
					>
						{result.statusCode}
					</Badge>
				{/if}
				<Button variant="ghost" size="icon" onclick={copyToClipboard}>
					{#if copied}
						<Check class="h-4 w-4 text-green-500" />
					{:else}
						<Copy class="h-4 w-4" />
					{/if}
				</Button>
			</div>
		</div>

		<div class="relative min-h-0 flex-1">
			<ScrollArea class="h-144 p-4">
				{#if isMulti}
					<div class="mb-3 flex flex-wrap items-center gap-2">
						<Badge variant="secondary" class="text-[10px]">{result.results.length} URLs</Badge>
						<Badge variant="outline" class="text-[10px]"
							>{result.results.filter((r: any) => !r.error).length} ok</Badge
						>
						{#if result.results.some((r: any) => r.error)}<Badge
								variant="destructive"
								class="text-[10px]"
								>{result.results.filter((r: any) => r.error).length} failed</Badge
							>{/if}
					</div>
				{/if}
				<Tabs.Content value="markdown" class="mt-0">
					{#if isMulti}
						<div class="space-y-6">
							{#each result.results as item (item.url)}
								<div class="rounded-lg border bg-background p-3">
									<div class="mb-2 flex items-center gap-2">
										<Badge variant={item.error ? 'destructive' : 'outline'} class="text-[10px]"
											>{item.url}</Badge
										>
										{#if item.word_count}<span class="text-[10px] text-muted-foreground"
												>{item.word_count} words</span
											>{/if}
										{#if item.error}<span class="text-xs text-destructive">{item.error}</span>{/if}
									</div>
									<pre
										class="font-mono text-sm whitespace-pre-wrap text-foreground/80">{item.markdown ||
											'No markdown'}</pre>
								</div>
							{/each}
						</div>
					{:else}
						<pre
							class="font-mono text-sm whitespace-pre-wrap text-foreground/80">{result.markdown ||
								'No Markdown content'}</pre>
					{/if}
				</Tabs.Content>
				<Tabs.Content value="html" class="mt-0">
					{#if isMulti}
						<div class="space-y-4">
							{#each result.results as item (item.url)}
								<div class="rounded-lg border bg-background p-3">
									<div class="mb-2 font-mono text-xs text-muted-foreground">{item.url}</div>
									<pre
										class="font-mono text-sm whitespace-pre-wrap text-foreground/80">{item.html ||
											'No HTML'}</pre>
								</div>
							{/each}
						</div>
					{:else}
						<pre class="font-mono text-sm whitespace-pre-wrap text-foreground/80">{result.html ||
								'No HTML content'}</pre>
					{/if}
				</Tabs.Content>
				<Tabs.Content value="json" class="mt-0">
					<pre class="font-mono text-sm whitespace-pre-wrap text-foreground/80">{JSON.stringify(
							result,
							null,
							2
						)}</pre>
				</Tabs.Content>
				<Tabs.Content value="links" class="mt-0">
					{#if isMulti}
						{@const allLinks = result.results.flatMap((r: any) => r.links || [])}
						<pre class="font-mono text-sm whitespace-pre-wrap text-foreground/80">{formatLinks(
								allLinks
							) || 'No links found'}</pre>
					{:else}
						<pre class="font-mono text-sm whitespace-pre-wrap text-foreground/80">{formatLinks(
								result.links || []
							) || 'No links found'}</pre>
					{/if}
				</Tabs.Content>
				{#if hasImages}
					<Tabs.Content value="images" class="mt-0">
						<ImageGallery images={result.images} className="p-0" />
					</Tabs.Content>
				{/if}
				{#if hasScreenshot}
					<Tabs.Content value="screenshot" class="mt-0">
						{@const screenshotSrc = getScreenshotSrc()}
						<div>
							<div class="mb-3 flex flex-wrap items-center gap-2">
								{#if result.screenshot.width && result.screenshot.height}
									<Badge variant="outline" class="text-[10px]"
										>{result.screenshot.width}×{result.screenshot.height}px</Badge
									>
								{/if}
								{#if result.screenshot.format}
									<Badge variant="outline" class="text-[10px]"
										>{result.screenshot.format.toUpperCase()}</Badge
									>
								{/if}
								{#if result.screenshot.size_bytes}
									<Badge variant="outline" class="text-[10px]"
										>{(result.screenshot.size_bytes / 1024).toFixed(1)} KB</Badge
									>
								{/if}
								{#if result.screenshot.full_page}
									<Badge variant="secondary" class="text-[10px]">Full Page</Badge>
								{/if}
							</div>
							{#if screenshotSrc}
								<div class="overflow-hidden rounded-lg border">
									<img
										src={screenshotSrc}
										alt="Full page screenshot of {result.url}"
										class="w-full"
									/>
								</div>
							{:else}
								<p class="text-sm text-muted-foreground">No screenshot data available.</p>
							{/if}
						</div>
					</Tabs.Content>
				{/if}
			</ScrollArea>
		</div>
	</Tabs.Root>
</Card.Root>
