<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { ArrowLeft } from '@lucide/svelte';
	import ComingSoon from '$lib/components/blocks/coming-soon.svelte';
</script>

<svelte:head>
	<title>General Settings - Cinder</title>
</svelte:head>

<div class="container mx-auto px-4 py-8">
	<div class="mx-auto max-w-2xl">
		<div class="mb-8 flex items-center gap-4">
			<Button variant="ghost" size="icon" title="Go back">
				<a href="/">
					<ArrowLeft class="h-5 w-5" />
				</a>
			</Button>
			<h1 class="text-3xl font-bold">General Settings</h1>
		</div>

		<ComingSoon featureName="General Settings" />
	</div>
</div>
