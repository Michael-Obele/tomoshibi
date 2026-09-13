<script lang="ts">
	import * as Alert from '$lib/components/ui/alert';
	import * as Card from '$lib/components/ui/card';
	import { Badge } from '$lib/components/ui/badge';
	import type { DocPage } from '$lib/docs-content';

	let { page }: { page: DocPage } = $props();
</script>

<div class="prose prose-sm dark:prose-invert max-w-none space-y-8">
	<div>
		<h1 class="mb-2 text-3xl font-bold tracking-tight">{page.title}</h1>
		<p class="text-lg text-muted-foreground">{page.description}</p>
	</div>

	{#each page.sections as section}
		{#if section.type === 'heading'}
			<h2 class="mt-8 mb-4 flex items-center gap-3 text-2xl font-bold">
				{#if section.icon}
					<section.icon class="h-6 w-6 {section.iconColor || ''}" />
				{/if}
				{section.text}
			</h2>
		{:else if section.type === 'text'}
			<p class="mb-4">{section.content}</p>
		{:else if section.type === 'alert'}
			<Alert.Root variant={section.variant || 'default'} class="mb-6">
				{#if section.icon}
					<section.icon class="h-4 w-4" />
				{/if}
				<Alert.Title>{section.title}</Alert.Title>
				<Alert.Description>{section.description}</Alert.Description>
			</Alert.Root>
		{:else if section.type === 'cards'}
			<div
				class="mb-6 grid gap-4 {section.columns === 1
					? 'md:grid-cols-1'
					: section.columns === 3
						? 'md:grid-cols-3'
						: 'md:grid-cols-2'}"
			>
				{#each section.items as item}
					<Card.Root class="overflow-hidden">
						<Card.Header class="pb-3">
							<Card.Title class="flex items-center gap-2 text-base">
								{#if item.icon}
									<item.icon class="h-4 w-4" />
								{/if}
								{item.title}
							</Card.Title>
						</Card.Header>
						<Card.Content class="space-y-2 text-sm">
							{#if item.badge}
								<div class="flex justify-between">
									<span class="font-medium">{item.title}</span>
									<Badge variant="outline">{item.badge}</Badge>
								</div>
							{/if}
							<p class="whitespace-pre-line text-muted-foreground">{item.description}</p>
						</Card.Content>
					</Card.Root>
				{/each}
			</div>
		{:else if section.type === 'steps'}
			<div class="space-y-4">
				{#each section.items as item, i}
					<div class="rounded-lg border p-6">
						<div class="flex items-start gap-4">
							<div
								class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-primary text-sm font-semibold text-primary-foreground"
							>
								{i + 1}
							</div>
							<div class="flex-1">
								<h3 class="mb-2 font-semibold">{item.title}</h3>
								<p class="mb-3 text-sm text-muted-foreground">{item.description}</p>
								{#if item.code}
									<pre class="overflow-x-auto rounded bg-muted p-3 text-xs"><code>{item.code}</code
										></pre>
								{/if}
								{#if item.note}
									<p class="mt-3 text-xs text-muted-foreground">
										<strong>Note:</strong>
										{item.note.replace('Note: ', '')}
									</p>
								{/if}
							</div>
						</div>
					</div>
				{/each}
			</div>
		{:else if section.type === 'code'}
			<div class="rounded-lg border p-4">
				{#if section.title}
					<h4 class="mb-2 font-semibold">{section.title}</h4>
				{/if}
				<pre class="overflow-x-auto rounded bg-muted p-3 text-xs"><code>{section.code}</code></pre>
			</div>
		{:else if section.type === 'flow'}
			<div class="mb-8 space-y-3">
				{#each section.items as item, i}
					<div class="flex items-start gap-4">
						<div
							class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-primary/20 text-sm font-semibold text-primary"
						>
							{i + 1}
						</div>
						<div class="flex-1">
							<h4 class="mb-1 font-semibold">{item.title}</h4>
							<p class="text-sm text-muted-foreground">{item.description}</p>
						</div>
					</div>
				{/each}
			</div>
		{:else if section.type === 'colors'}
			<div class="mb-8 grid gap-4 md:grid-cols-2">
				{#each section.items as item}
					<div class="flex items-center gap-3">
						<div class="h-12 w-12 rounded {item.class}"></div>
						<div>
							<p class="font-mono text-sm">{item.name}</p>
							<p class="text-xs text-muted-foreground">{item.value}</p>
						</div>
					</div>
				{/each}
			</div>
		{:else if section.type === 'features'}
			<div class="mb-6 grid gap-4 md:grid-cols-2">
				{#each section.items as item}
					<Card.Root class="overflow-hidden">
						<Card.Header class="pb-3">
							<Card.Title class="flex items-center gap-2 text-base">
								{#if item.icon}
									<item.icon class="h-4 w-4" />
								{/if}
								{item.title}
							</Card.Title>
						</Card.Header>
						<Card.Content class="space-y-2 text-sm text-muted-foreground">
							{#each item.features as feature}
								<p>✓ {feature}</p>
							{/each}
						</Card.Content>
					</Card.Root>
				{/each}
			</div>
		{:else if section.type === 'tree'}
			<div class="rounded-lg border bg-muted/30 p-6 font-mono text-sm">
				<pre>{section.content}</pre>
			</div>
			{#if section.note}
				<p class="mt-4 text-sm text-muted-foreground">{section.note}</p>
			{/if}
		{:else if section.type === 'list'}
			<div class="rounded-lg border bg-muted/30 p-4">
				{#if section.title}
					<p class="mb-2 text-sm font-medium">{section.title}</p>
				{/if}
				<ul class="space-y-1 text-sm text-muted-foreground">
					{#each section.items as item}
						<li>✓ {item}</li>
					{/each}
				</ul>
			</div>
		{:else if section.type === 'key-value'}
			<div class="space-y-3 rounded-lg border p-6">
				<div class="space-y-2 font-mono text-sm">
					{#each section.items as item}
						<div class="flex justify-between">
							<span>{item.key}</span>
							{#if item.badge}
								<Badge variant={item.badge as any}>{item.value}</Badge>
							{:else}
								<span class="text-muted-foreground">{item.value}</span>
							{/if}
						</div>
					{/each}
				</div>
			</div>
		{/if}
	{/each}
</div>
