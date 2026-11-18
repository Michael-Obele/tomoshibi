<script lang="ts">
	let { data } = $props();
</script>

<svelte:head>
	<title>{data.doc.title} · Tomoshibi</title>
	<meta name="description" content={data.doc.summary} />
	<link rel="alternate" type="text/markdown" href={data.doc.url} />
</svelte:head>

<article class="mx-auto max-w-3xl">
	<header class="mb-8 space-y-2">
		<p class="text-[11px] font-bold tracking-wider text-muted-foreground uppercase">
			{data.doc.section}
		</p>
		<h1 class="text-3xl font-bold tracking-tight">{data.doc.title}</h1>
		<p class="text-muted-foreground">{data.doc.summary}</p>
		<p class="text-xs text-muted-foreground">
			<a href={data.doc.url} class="underline-offset-2 hover:underline">Markdown</a>
			· source <code class="font-mono">{data.doc.source}</code>
		</p>
	</header>

	<!-- Build-time output from this repo's markdown, never user input. -->
	<div class="prose prose-sm dark:prose-invert max-w-none prose-pre:p-0">
		{@html data.html}
	</div>
</article>
