<script lang="ts">
	import { goto } from '$app/navigation';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { ArrowLeft, Construction } from '@lucide/svelte';
</script>

<div class="container flex min-h-[80vh] items-center justify-center py-12">
	<Card.Root
		class="mx-auto max-w-md border-slate-200/60 shadow-lg transition-all dark:border-slate-800/60"
	>
		<Card.Header class="pb-2 text-center">
			<div
				class="mx-auto mb-6 flex h-16 w-16 items-center justify-center rounded-full bg-amber-100 text-amber-600 dark:bg-amber-900/20 dark:text-amber-500"
			>
				<Construction size={32} />
			</div>
			<Card.Title class="text-3xl font-extrabold tracking-tighter text-balance">
				Settings Coming Soon
			</Card.Title>
			<Card.Description class="mt-4 text-base leading-relaxed text-balance text-muted-foreground">
				User account settings and advanced privacy controls are currently under development and will
				be available in an upcoming release.
			</Card.Description>
		</Card.Header>
		<Card.Content class="flex flex-col gap-4 pt-6">
			<Button
				onclick={() => goto('/')}
				size="lg"
				class="w-full bg-amber-600 font-semibold text-white transition-all hover:bg-amber-700 dark:bg-amber-600 dark:hover:bg-amber-500"
			>
				<ArrowLeft class="mr-2 h-4 w-4" />
				Back Home
			</Button>
		</Card.Content>
	</Card.Root>
</div>
