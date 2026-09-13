<script>
	import { Button } from '$lib/components/ui/button';
	import { ArrowRight } from '@lucide/svelte';
</script>

<div class="flex min-h-[calc(100vh-4rem)] flex-col items-center justify-center p-4 text-center">
	<h1 class="mb-4 text-4xl font-extrabold tracking-tight text-primary lg:text-5xl">Cinder</h1>
	<p class="mb-8 max-w-150 text-xl text-muted-foreground">
		The ultimate scraping and crawling platform. Turn websites into LLM-ready data.
	</p>

	<div class="flex gap-4">
		<Button href="/playground" size="lg" class="gap-2">
			Open Playground <ArrowRight class="h-4 w-4" />
		</Button>
		<Button href="/docs" variant="outline" size="lg">Documentation</Button>
	</div>
</div>
