<script lang="ts">
	import * as Sheet from '$lib/components/ui/sheet';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Settings } from '@lucide/svelte';

	let {
		options = $bindable(),
		type = 'scrape'
	}: {
		options: Record<string, any>;
		type?: 'scrape' | 'crawl' | 'search' | 'map' | 'monitor';
	} = $props();
</script>

<Sheet.Root>
	<Sheet.Trigger>
		{#snippet child({ props })}
			<Button variant="outline" class="mt-1" size="icon" {...props}>
				<Settings class="size-5" />
			</Button>
		{/snippet}
	</Sheet.Trigger>
	<Sheet.Content class="w-100 overflow-y-auto px-6 sm:w-135">
		<Sheet.Header>
			<Sheet.Title>Advanced Options</Sheet.Title>
			<Sheet.Description>Configure advanced {type} parameters.</Sheet.Description>
		</Sheet.Header>
		<div class="grid gap-6 py-6">
			{#if type === 'scrape'}
				<div class="grid grid-cols-4 items-center gap-4">
					<Label class="text-right text-xs">Extraction Mode</Label>
					<div class="col-span-3 flex shrink-0 items-center justify-start gap-1">
						<Button
							size="sm"
							variant={options.mode === 'smart' ? 'default' : 'outline'}
							onclick={() => (options.mode = 'smart')}>Smart</Button
						>
						<Button
							size="sm"
							variant={options.mode === 'static' ? 'default' : 'outline'}
							onclick={() => (options.mode = 'static')}>Static</Button
						>
						<Button
							size="sm"
							variant={options.mode === 'dynamic' ? 'default' : 'outline'}
							onclick={() => (options.mode = 'dynamic')}>Dynamic</Button
						>
					</div>
				</div>

				<div class="space-y-4">
					<div class="grid grid-cols-4 items-center gap-4">
						<Label class="text-right text-xs">Screenshot</Label>
						<div class="col-span-3">
							<Switch bind:checked={options.screenshot} />
						</div>
					</div>
					<div class="grid grid-cols-4 items-center gap-4">
						<Label class="text-right text-xs">Extract Images</Label>
						<div class="col-span-3">
							<Switch bind:checked={options.images} />
						</div>
					</div>
					{#if options.images}
						<div class="grid grid-cols-4 items-center gap-4">
							<Label class="text-right text-xs">Image Format</Label>
							<div class="col-span-3 flex shrink-0 items-center justify-start gap-1">
								<Button
									size="sm"
									variant={options.image_format === 'url' ? 'default' : 'outline'}
									onclick={() => (options.image_format = 'url')}>URL</Button
								>
								<Button
									size="sm"
									variant={options.image_format === 'blob' ? 'default' : 'outline'}
									onclick={() => (options.image_format = 'blob')}>Blob</Button
								>
							</div>
						</div>
						<div class="grid grid-cols-4 items-center gap-4">
							<Label class="text-right">Max Images</Label>
							<Input
								type="number"
								bind:value={options.max_images}
								class="col-span-3"
								placeholder="10"
								min="1"
								max="100"
							/>
						</div>
						<div class="grid grid-cols-4 items-center gap-4">
							<Label class="text-right leading-tight">Max Image Size (KB)</Label>
							<Input
								type="number"
								bind:value={options.max_image_size_kb}
								class="col-span-3"
								placeholder="5120"
								min="1"
							/>
						</div>
					{/if}
					<div class="grid grid-cols-4 items-center gap-4">
						<Label class="text-right text-xs">Summary</Label>
						<div class="col-span-3"><Switch bind:checked={options.summary} /></div>
					</div>
					{#if options.summary}
						<div class="grid grid-cols-4 items-center gap-4">
							<Label class="text-right text-xs">Sentences</Label>
							<Input
								type="number"
								bind:value={options.summary_sentences}
								class="col-span-3"
								placeholder="5"
								min="1"
								max="20"
							/>
						</div>
					{/if}
					<div class="grid grid-cols-4 items-center gap-4">
						<Label class="text-right text-xs">Redact PII</Label>
						<div class="col-span-3"><Switch bind:checked={options.redact_pii} /></div>
					</div>
					<div class="grid grid-cols-4 items-center gap-4">
						<Label class="text-right text-xs">Block Ads</Label>
						<div class="col-span-3"><Switch bind:checked={options.block_ads} /></div>
					</div>
					<div class="grid grid-cols-4 items-center gap-4">
						<Label class="text-right text-xs">Remove Base64</Label>
						<div class="col-span-3"><Switch bind:checked={options.remove_base64_images} /></div>
					</div>
					<div class="grid grid-cols-4 items-center gap-4">
						<Label class="text-right text-xs">Include Links</Label>
						<div class="col-span-3"><Switch bind:checked={options.include_links} /></div>
					</div>
					<div class="grid grid-cols-4 items-start gap-4">
						<Label class="pt-2 text-right text-xs leading-tight">Extract Schema (JSON)</Label>
						<Textarea
							bind:value={options.extract_schema}
							class="col-span-3 font-mono text-xs"
							placeholder={'{"title":{"selector":"h1"}}'}
							rows={3}
						/>
					</div>
					<div class="grid grid-cols-4 items-start gap-4">
						<Label class="pt-2 text-right text-xs leading-tight">Actions (JSON)</Label>
						<Textarea
							bind:value={options.actions}
							class="col-span-3 font-mono text-xs"
							placeholder={'[{"type":"scroll_to_bottom"}]'}
							rows={3}
						/>
					</div>
					<div class="grid grid-cols-4 items-start gap-4">
						<Label class="pt-2 text-right text-xs leading-tight"
							>Sync Multi-URL (one per line)</Label
						>
						<Textarea
							bind:value={options.urls}
							class="col-span-3 font-mono text-xs"
							placeholder={'https://example.com\nhttps://example.org'}
							rows={3}
						/>
					</div>
					<p class="pl-2 text-[11px] text-muted-foreground">
						When filled, <span class="font-mono">urls[]</span> is sent (max 10, exclusive with single
						URL) — sync, no Redis, limit 5 parallel. Leave empty for single-URL scrape.
					</p>
				</div>
			{/if}

			{#if type === 'crawl'}
				<div class="space-y-4">
					<div class="grid grid-cols-4 items-center gap-4">
						<Label class="text-right text-xs">Crawl Mode</Label>
						<div class="col-span-3 flex shrink-0 items-center justify-start gap-1">
							<Button
								size="sm"
								variant={options.mode === 'smart' ? 'default' : 'outline'}
								onclick={() => (options.mode = 'smart')}>Smart</Button
							>
							<Button
								size="sm"
								variant={options.mode === 'static' ? 'default' : 'outline'}
								onclick={() => (options.mode = 'static')}>Static</Button
							>
							<Button
								size="sm"
								variant={options.mode === 'dynamic' ? 'default' : 'outline'}
								onclick={() => (options.mode = 'dynamic')}>Dynamic</Button
							>
						</div>
					</div>
					{#if options.mode === 'dynamic'}
						<div
							class="rounded-md border border-amber-500/20 bg-amber-500/10 p-3 text-xs text-amber-600 dark:text-amber-400"
						>
							Dynamic mode requires Chrome/Chromium on the backend server. If unavailable, scraping
							will fail.
						</div>
					{/if}
					<div class="grid grid-cols-4 items-center gap-4">
						<Label class="text-right">Max Depth</Label>
						<Input
							type="number"
							bind:value={options.maxDepth}
							class="col-span-3"
							placeholder="2"
							min="1"
							max="10"
						/>
					</div>
					<div class="grid grid-cols-4 items-center gap-4">
						<Label class="text-right">Page Limit</Label>
						<Input
							type="number"
							bind:value={options.limit}
							class="col-span-3"
							placeholder="10"
							min="1"
							max="100"
						/>
					</div>
					<div class="grid grid-cols-4 items-center gap-4">
						<Label class="text-right text-xs">Screenshots</Label>
						<div class="col-span-3">
							<Switch bind:checked={options.screenshot} />
						</div>
					</div>
					<div class="grid grid-cols-4 items-center gap-4">
						<Label class="text-right text-xs">Extract Images</Label>
						<div class="col-span-3">
							<Switch bind:checked={options.images} />
						</div>
					</div>
					{#if options.images}
						<div class="grid grid-cols-4 items-center gap-4">
							<Label class="text-right text-xs">Image Format</Label>
							<div class="col-span-3 flex shrink-0 items-center justify-start gap-1">
								<Button
									size="sm"
									variant={options.image_format === 'url' ? 'default' : 'outline'}
									onclick={() => (options.image_format = 'url')}>URL</Button
								>
								<Button
									size="sm"
									variant={options.image_format === 'blob' ? 'default' : 'outline'}
									onclick={() => (options.image_format = 'blob')}>Blob</Button
								>
							</div>
						</div>
					{/if}
					<div class="grid grid-cols-4 items-center gap-4">
						<Label class="text-right leading-tight">Include Paths</Label>
						<Input
							bind:value={options.include_paths}
							class="col-span-3 font-mono text-xs"
							placeholder="/blog/*, /docs/**"
						/>
					</div>
					<div class="grid grid-cols-4 items-center gap-4">
						<Label class="text-right leading-tight">Exclude Paths</Label>
						<Input
							bind:value={options.exclude_paths}
							class="col-span-3 font-mono text-xs"
							placeholder="/admin/*, /login"
						/>
					</div>
					<div class="grid grid-cols-4 items-center gap-4">
						<Label class="text-right leading-tight">Webhook URL</Label>
						<Input
							bind:value={options.webhook_url}
							class="col-span-3 font-mono text-xs"
							placeholder="https://myapp.example.com/hooks/cinder"
						/>
					</div>
					<div class="grid grid-cols-4 items-center gap-4">
						<Label class="text-right leading-tight">Webhook Secret</Label>
						<Input
							bind:value={options.webhook_secret}
							class="col-span-3 font-mono text-xs"
							placeholder="s3cret"
							type="password"
						/>
					</div>
				</div>
			{/if}

			{#if type === 'map'}
				<div class="space-y-4">
					<div class="grid grid-cols-4 items-center gap-4">
						<Label class="text-right leading-tight">Search Filter</Label>
						<Input bind:value={options.search} class="col-span-3" placeholder="blog" />
					</div>
					<div class="grid grid-cols-4 items-center gap-4">
						<Label class="text-right">Limit</Label>
						<Input
							type="number"
							bind:value={options.limit}
							class="col-span-3"
							placeholder="100"
							min="1"
							max="5000"
						/>
					</div>
					<p class="pl-2 text-[11px] text-muted-foreground">
						Map uses sitemap.xml → robots.txt → link fallback. No Redis required.
					</p>
				</div>
			{/if}

			{#if type === 'monitor'}
				<div class="space-y-4">
					<div class="grid grid-cols-4 items-center gap-4">
						<Label class="text-right leading-tight">Interval (s)</Label>
						<Input
							type="number"
							bind:value={options.interval_seconds}
							class="col-span-3"
							placeholder="3600"
							min="3600"
						/>
					</div>
					<div class="grid grid-cols-4 items-center gap-4">
						<Label class="text-right leading-tight">Webhook URL</Label>
						<Input
							bind:value={options.webhook_url}
							class="col-span-3 font-mono text-xs"
							placeholder="https://myapp.example.com/hooks/cinder"
						/>
					</div>
					<div class="grid grid-cols-4 items-center gap-4">
						<Label class="text-right leading-tight">Webhook Secret</Label>
						<Input
							bind:value={options.webhook_secret}
							class="col-span-3 font-mono text-xs"
							placeholder="s3cret"
							type="password"
						/>
					</div>
					<p class="pl-2 text-[11px] text-muted-foreground">
						Hashes markdown on interval, fires signed webhook on change. Min 3600s. Requires Redis.
					</p>
				</div>
			{/if}

			{#if type === 'search'}
				<div class="grid grid-cols-4 items-center gap-4">
					<Label class="text-right text-xs">Search Mode</Label>
					<div class="col-span-3 flex shrink-0 items-center justify-start gap-1">
						<Button
							size="sm"
							variant={options.mode === 'default' ? 'default' : 'outline'}
							onclick={() => (options.mode = 'default')}>Default</Button
						>
						<Button
							size="sm"
							variant={options.mode === 'news' ? 'default' : 'outline'}
							onclick={() => (options.mode = 'news')}>News</Button
						>
					</div>
				</div>
				<div class="grid grid-cols-4 items-center gap-4">
					<Label class="text-right text-xs">Category</Label>
					<div class="col-span-3 flex shrink-0 flex-wrap items-center justify-start gap-1">
						<Button
							size="sm"
							variant={!options.category ? 'default' : 'outline'}
							onclick={() => (options.category = '')}>All</Button
						>
						<Button
							size="sm"
							variant={options.category === 'general' ? 'default' : 'outline'}
							onclick={() => (options.category = 'general')}>General</Button
						>
						<Button
							size="sm"
							variant={options.category === 'news' ? 'default' : 'outline'}
							onclick={() => (options.category = 'news')}>News</Button
						>
						<Button
							size="sm"
							variant={options.category === 'code' ? 'default' : 'outline'}
							onclick={() => (options.category = 'code')}>Code</Button
						>
					</div>
				</div>
				<p class="pl-2 text-[11px] text-muted-foreground">
					Category maps to SearXNG <span class="font-mono">categories</span> (general/news/it) +
					Brave <span class="font-mono">search_type</span>. Validated enum.
				</p>
				<div class="grid grid-cols-4 items-center gap-4">
					<Label class="text-right text-xs">TF-IDF Rerank</Label>
					<div class="col-span-3"><Switch bind:checked={options.rerank} /></div>
				</div>
				<p class="pl-2 text-[11px] text-muted-foreground">
					Lightweight pure-Go rerank (no ONNX) — reorders by query-term TF-IDF. Opt-in.
				</p>
				<div class="grid grid-cols-4 items-center gap-4">
					<Label class="text-right">Result Limit</Label>
					<Input type="number" bind:value={options.limit} class="col-span-3" placeholder="5" />
				</div>
				<div class="grid grid-cols-4 items-center gap-4">
					<Label class="text-right">Max Age (Days)</Label>
					<Input type="number" bind:value={options.maxAge} class="col-span-3" placeholder="0" />
				</div>
				<div class="grid grid-cols-4 items-center gap-4">
					<Label class="text-right leading-tight">Include Domains</Label>
					<Input
						bind:value={options.includeDomains}
						class="col-span-3"
						placeholder="wikipedia.org, github.com"
					/>
				</div>
				<div class="grid grid-cols-4 items-center gap-4">
					<Label class="text-right leading-tight">Exclude Domains</Label>
					<Input
						bind:value={options.excludeDomains}
						class="col-span-3"
						placeholder="pinterest.com"
					/>
				</div>
				<div class="grid grid-cols-4 items-center gap-4">
					<Label class="text-right leading-tight">Required Text</Label>
					<Input
						bind:value={options.requiredText}
						class="col-span-3"
						placeholder="open source, code"
					/>
				</div>
			{/if}
		</div>
	</Sheet.Content>
</Sheet.Root>
