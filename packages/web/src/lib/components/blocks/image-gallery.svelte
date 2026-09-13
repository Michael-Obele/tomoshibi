<script lang="ts">
	import * as Dialog from '$lib/components/ui/dialog';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Image, X, Download, ExternalLink, AlertCircle } from '@lucide/svelte';
	import { cn } from '$lib/utils';

	let {
		images = [],
		className = ''
	}: {
		images: Array<{
			url?: string;
			blob?: string;
			alt?: string;
			title?: string;
			width?: number;
			height?: number;
			format?: string;
			size_bytes?: number;
			source?: string;
		}>;
		className?: string;
	} = $props();

	let selectedImage = $state<any>(null);

	function getImageSrc(img: any): string {
		if (img.blob) {
			return `data:image/${img.format || 'jpeg'};base64,${img.blob}`;
		}
		return img.url || '';
	}

	function formatSize(bytes: number | undefined): string {
		if (!bytes) return '';
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	}
</script>

{#if images.length === 0}
	<div class={cn('flex flex-col items-center justify-center rounded-xl border border-dashed bg-muted/10 p-16 opacity-60', className)}>
		<Image class="mb-4 size-12 stroke-[1px] text-muted-foreground" />
		<p class="text-sm font-medium">No images extracted</p>
		<p class="mt-1 text-xs text-muted-foreground">Enable image extraction in options to capture images</p>
	</div>
{:else}
	<div class={cn('space-y-4', className)}>
		<div class="flex items-center justify-between">
			<div class="flex items-center gap-2">
				<Image class="size-4 text-primary" />
				<h3 class="text-sm font-semibold">Extracted Images</h3>
				<Badge variant="secondary" class="text-[10px]">{images.length} found</Badge>
			</div>
		</div>

		<div class="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5">
			{#each images as img, i (getImageSrc(img) + i)}
				{@const src = getImageSrc(img)}
				<button
					class="group relative overflow-hidden rounded-lg border bg-card transition-all hover:border-primary/50 hover:shadow-lg"
					onclick={() => (selectedImage = img)}
				>
					<div class="aspect-square">
						{#if src.startsWith('data:') || src.startsWith('http')}
							<img
								src={src}
								alt={img.alt || img.title || `Image ${i + 1}`}
								loading="lazy"
								class="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105"
							/>
						{:else}
							<div class="flex h-full w-full items-center justify-center bg-muted/30">
								<AlertCircle class="size-6 text-muted-foreground/50" />
							</div>
						{/if}
					</div>
					<div class="absolute inset-x-0 bottom-0 bg-linear-to-t from-black/60 to-transparent p-2 opacity-0 transition-opacity group-hover:opacity-100">
						<span class="block truncate text-[10px] font-medium text-white">{img.alt || img.title || `Image ${i + 1}`}</span>
						{#if img.width && img.height}
							<span class="text-[9px] text-white/70">{img.width}×{img.height}{img.format ? ` · ${img.format.toUpperCase()}` : ''}</span>
						{/if}
					</div>
				</button>
			{/each}
		</div>
	</div>

	<!-- Lightbox Dialog -->
	<Dialog.Root open={!!selectedImage} onOpenChange={(open) => { if (!open) selectedImage = null; }}>
		<Dialog.Content class="max-w-4xl border-0 bg-background/95 p-0 backdrop-blur-xl">
			<Dialog.Close class="absolute top-3 right-3 z-10">
				<Button variant="ghost" size="icon" class="size-8 rounded-full bg-background/50">
					<X class="size-4" />
				</Button>
			</Dialog.Close>

			{#if selectedImage}
				{@const src = getImageSrc(selectedImage)}
				<div class="flex flex-col">
					<div class="flex items-center justify-center bg-black/5 p-8 dark:bg-white/5">
						{#if src.startsWith('data:') || src.startsWith('http')}
							<img
								src={src}
								alt={selectedImage.alt || selectedImage.title || 'Image preview'}
								class="max-h-[70vh] max-w-full rounded-lg object-contain"
							/>
						{:else}
							<div class="flex items-center justify-center p-20">
								<AlertCircle class="size-12 text-muted-foreground/50" />
							</div>
						{/if}
					</div>

					<div class="border-t p-4">
						<Dialog.Header class="p-0">
							<Dialog.Title class="text-base">{selectedImage.alt || selectedImage.title || 'Image Details'}</Dialog.Title>
						</Dialog.Header>
						<div class="mt-3 flex flex-wrap gap-3 text-xs text-muted-foreground">
							{#if selectedImage.width && selectedImage.height}
								<Badge variant="outline" class="text-[10px]">{selectedImage.width}×{selectedImage.height}px</Badge>
							{/if}
							{#if selectedImage.format}
								<Badge variant="outline" class="text-[10px]">{selectedImage.format.toUpperCase()}</Badge>
							{/if}
							{#if selectedImage.size_bytes}
								<Badge variant="outline" class="text-[10px]">{formatSize(selectedImage.size_bytes)}</Badge>
							{/if}
							{#if selectedImage.source}
								<Badge variant="outline" class="text-[10px]">{selectedImage.source}</Badge>
							{/if}
						</div>
						<div class="mt-4 flex gap-2">
							{#if selectedImage.url}
								<Button variant="outline" size="sm" class="gap-1.5 text-[11px]" onclick={() => window.open(selectedImage.url, '_blank')}>
									<ExternalLink class="size-3" />
									Open Original
								</Button>
							{/if}
							{#if src}
								<Button variant="outline" size="sm" class="gap-1.5 text-[11px]" onclick={() => {
									const a = document.createElement('a');
									a.href = src;
									a.download = selectedImage.alt || selectedImage.title || 'image';
									a.click();
								}}>
									<Download class="size-3" />
									Download
								</Button>
							{/if}
						</div>
					</div>
				</div>
			{/if}
		</Dialog.Content>
	</Dialog.Root>
{/if}
