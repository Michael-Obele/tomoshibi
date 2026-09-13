<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import { Button } from '$lib/components/ui/button';
	import { Construction, Rocket, ArrowLeft, Bell } from '@lucide/svelte';
	import { goto } from '$app/navigation';

	interface Props {
		featureName: string;
		description?: string;
	}

	let {
		featureName,
		description = "We're currently building the authentication and user management systems for Cinder. This feature will be available once the infrastructure is secure and ready for production."
	}: Props = $props();
</script>

<div class="flex min-h-[70vh] flex-col items-center justify-center p-6 text-center">
	<Card.Root
		class="w-full max-w-xl border-dashed bg-muted/30 py-12 shadow-sm transition-all hover:bg-muted/40 dark:bg-muted/10"
	>
		<Card.Header class="space-y-6">
			<div
				class="mx-auto flex h-20 w-20 items-center justify-center rounded-3xl bg-amber-500/10 text-amber-600 shadow-inner dark:bg-amber-400/10 dark:text-amber-500"
			>
				<Construction size={40} />
			</div>
			<div class="space-y-2">
				<Card.Title class="text-4xl font-extrabold tracking-tighter sm:text-5xl">
					{featureName} <span class="text-primary/80">Coming Soon</span>
				</Card.Title>
				<Card.Description
					class="mx-auto max-w-lg px-2 text-lg leading-relaxed text-muted-foreground sm:text-xl"
				>
					{description}
				</Card.Description>
			</div>
		</Card.Header>
		<Card.Content class="pt-8">
			<div class="flex flex-col items-center justify-center gap-4 sm:flex-row">
				<Button
					variant="outline"
					size="lg"
					class="min-w-40 gap-2 rounded-full border-muted-foreground/20 hover:bg-accent"
					onclick={() => history.back()}
				>
					<ArrowLeft class="h-5 w-5" />
					Go Back
				</Button>
				<Button
					size="lg"
					class="min-w-40 gap-2 rounded-full bg-amber-600 text-white shadow-lg hover:bg-amber-700 dark:bg-amber-700 dark:hover:bg-amber-800"
				>
					<Bell class="h-5 w-5" />
					Stay Updated
				</Button>
			</div>
		</Card.Content>
		<Card.Footer class="pt-12">
			<div
				class="mx-auto flex items-center gap-3 rounded-full bg-accent/50 px-4 py-2 text-sm font-medium text-muted-foreground backdrop-blur-sm"
			>
				<Rocket class="h-4 w-4 animate-pulse text-amber-500" />
				Target Release: Q1 2026
			</div>
		</Card.Footer>
	</Card.Root>
</div>
