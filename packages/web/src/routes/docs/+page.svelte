<script lang="ts">
	import { ArrowRight, FileText } from "@lucide/svelte";

	let { data } = $props();
</script>

<div class="space-y-10">
	<header class="space-y-3">
		<h1 class="text-4xl font-bold tracking-tight">Documentation</h1>
		<p class="max-w-2xl text-muted-foreground">
			Everything published from this repository, generated from the
			tracked markdown at build time — the API, the MCP server, the
			browser bridge and the decisions behind them. Every page is also
			served as markdown, and <a
				href="/llms.txt"
				class="underline underline-offset-2">llms.txt</a
			>
			indexes the whole set for agents.
		</p>
	</header>

	{#each data.sections as section (section.title)}
		<section class="space-y-3">
			<h2
				class="text-[11px] font-bold tracking-wider text-muted-foreground uppercase"
			>
				{section.title}
			</h2>
			<div class="grid gap-3 md:grid-cols-2">
				{#each section.docs as doc (doc.slug)}
					<a
						href={`/docs/${doc.slug}`}
						class="group rounded-lg border p-4 transition-all hover:border-primary hover:bg-muted/50"
					>
						<div class="flex items-start gap-3">
							<FileText
								class="mt-0.5 h-4 w-4 shrink-0 text-primary"
							/>
							<div class="flex-1">
								<h3 class="mb-0.5 text-sm font-semibold">
									{doc.title}
								</h3>
								<p class="text-xs text-muted-foreground">
									{doc.summary}
								</p>
							</div>
							<ArrowRight
								class="h-4 w-4 text-muted-foreground transition-colors group-hover:text-primary"
							/>
						</div>
					</a>
				{/each}
			</div>
		</section>
	{/each}

	<section class="rounded-lg border bg-muted/30 p-6">
		<h2 class="mb-1 text-lg font-semibold">Running it yourself</h2>
		<p class="mb-4 text-sm text-muted-foreground">
			Start the playground with
			<code class="rounded bg-muted px-1 py-0.5 font-mono text-xs"
				>bun run dev:web</code
			>
			and the API with
			<code class="rounded bg-muted px-1 py-0.5 font-mono text-xs"
				>bun run dev:api</code
			>.
		</p>
		<a
			href="/playground"
			class="inline-flex items-center justify-center rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
		>
			Open Playground
		</a>
	</section>
</div>
