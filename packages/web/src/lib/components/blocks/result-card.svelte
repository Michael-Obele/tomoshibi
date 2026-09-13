<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';
	import { ExternalLink, Image, Camera } from '@lucide/svelte';

	let { result } = $props();

	let imageCount = $derived(result?.images?.length || 0);
	let hasScreenshot = $derived(!!result?.screenshot?.blob || !!result?.screenshot?.url);
</script>

<Card.Root>
	<Card.Header>
		<Card.Title class="text-lg font-medium">
			<a href={result.url} target="_blank" class="flex items-center gap-2 hover:underline">
				{result.metadata?.title || result.title || result.url}
				<ExternalLink class="h-4 w-4 text-muted-foreground" />
			</a>
		</Card.Title>
		<Card.Description>{result.url}</Card.Description>
	</Card.Header>
	<Card.Content>
		<p class="line-clamp-3 text-sm text-muted-foreground">
			{result.metadata?.description || result.description || 'No description available.'}
		</p>
	</Card.Content>
	<Card.Footer class="flex-wrap gap-2">
		{#if result.metadata}
			{#each Object.entries(result.metadata) as [key, value]}
				<Badge variant="outline">{key}: {value}</Badge>
			{/each}
		{/if}
	</Card.Footer>
</Card.Root>
