<script lang="ts">
	import * as Card from '$lib/components/ui/card';
	import * as Alert from '$lib/components/ui/alert';
	import { Badge } from '$lib/components/ui/badge';
	import { ArrowRight } from '@lucide/svelte';
	import {
		hero,
		quickStart,
		techStack,
		principles,
		documentationSections
	} from '$lib/docs-content';
</script>

<div class="space-y-12">
	<!-- Hero Section -->
	<div class="prose prose-sm dark:prose-invert max-w-none">
		<h1 class="mb-4 text-4xl font-bold tracking-tight">{hero.title}</h1>
		<p class="text-xl text-muted-foreground">
			{hero.description}
		</p>
	</div>

	<!-- Quick Start Cards -->
	<section class="not-prose space-y-4">
		<h2 class="text-2xl font-bold">Quick Start</h2>
		<div class="grid gap-4 md:grid-cols-3">
			{#each quickStart as item}
				<a
					href={item.href}
					class="group relative rounded-lg border p-6 transition-all hover:border-primary hover:bg-muted/50"
				>
					<div class="mb-3 flex items-center gap-2">
						<item.icon class="h-5 w-5 {item.iconColor}" />
						<h3 class="font-semibold">{item.title}</h3>
						<ArrowRight
							class="ml-auto h-4 w-4 -translate-x-2 opacity-0 transition-all group-hover:translate-x-0 group-hover:opacity-100"
						/>
					</div>
					<p class="text-sm text-muted-foreground">
						{item.description}
					</p>
				</a>
			{/each}
		</div>
	</section>

	<!-- Tech Stack Overview -->
	<section class="not-prose space-y-4">
		<h2 class="text-2xl font-bold">Technology Stack</h2>
		<div class="grid gap-4 md:grid-cols-2">
			{#each techStack as stack}
				<Card.Root class="overflow-hidden">
					<Card.Header class="pb-3">
						<Card.Title class="flex items-center gap-2 text-base">
							<stack.icon class="h-4 w-4" />
							{stack.title}
						</Card.Title>
					</Card.Header>
					<Card.Content class="space-y-2 text-sm">
						{#each stack.items as item}
							<div class="flex items-center justify-between">
								<span>{item.name}</span>
								<Badge
									variant={item.variant as 'default' | 'secondary' | 'outline' | 'destructive'}
								>
									{item.badge}
								</Badge>
							</div>
						{/each}
					</Card.Content>
				</Card.Root>
			{/each}
		</div>
	</section>

	<!-- Key Principles -->
	<section class="not-prose space-y-4">
		<h2 class="text-2xl font-bold">Key Principles</h2>

		{#each principles as principle}
			{#if principle.highlight}
				<Alert.Root class="mb-6">
					{#if principle.icon}
						<principle.icon class="h-4 w-4" />
					{/if}
					<Alert.Title>{principle.title}</Alert.Title>
					<Alert.Description>
						{principle.description}
					</Alert.Description>
				</Alert.Root>
			{/if}
		{/each}

		<div class="grid gap-4 md:grid-cols-2">
			{#each principles as principle}
				{#if !principle.highlight}
					<Card.Root>
						<Card.Header>
							<Card.Title class="text-base">{principle.title}</Card.Title>
						</Card.Header>
						<Card.Content class="text-sm text-muted-foreground">
							{principle.description}
						</Card.Content>
					</Card.Root>
				{/if}
			{/each}
		</div>
	</section>

	<!-- Documentation Navigation -->
	<section class="not-prose space-y-4">
		<h2 class="text-2xl font-bold">Documentation</h2>
		<div class="grid gap-3">
			{#each documentationSections as section}
				<a
					href={section.href}
					class="group rounded-lg border p-4 transition-all hover:border-primary hover:bg-muted/50"
				>
					<div class="flex items-start gap-3">
						<section.icon class="mt-0.5 h-5 w-5 shrink-0 text-primary" />
						<div class="flex-1">
							<h3 class="mb-0.5 text-sm font-semibold">{section.title}</h3>
							<p class="text-xs text-muted-foreground">{section.description}</p>
						</div>
						<ArrowRight
							class="h-4 w-4 text-muted-foreground transition-colors group-hover:text-primary"
						/>
					</div>
				</a>
			{/each}
		</div>
	</section>

	<!-- Get Help -->
	<section class="not-prose space-y-4 rounded-lg border bg-muted/30 p-6">
		<h2 class="text-xl font-bold">Need Help?</h2>
		<p class="text-sm text-muted-foreground">
			Check out the setup guide to get started, explore the architecture to understand how
			everything works, or browse the features to see what's possible.
		</p>
		<div class="flex flex-wrap gap-3">
			<a
				href="/playground"
				class="inline-flex items-center justify-center rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
			>
				Open Playground
			</a>
			<a
				href="/docs/setup"
				class="inline-flex items-center justify-center rounded-md border border-input bg-background px-4 py-2 text-sm font-medium transition-colors hover:bg-accent hover:text-accent-foreground"
			>
				View Setup Guide
			</a>
		</div>
	</section>
</div>
