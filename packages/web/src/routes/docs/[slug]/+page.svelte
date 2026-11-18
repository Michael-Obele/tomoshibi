<script lang="ts">
	import { Check, Copy } from '@lucide/svelte';

	let { data } = $props();

	const COPIED_MS = 1600;
	const iconKeys = ['copy', 'check'] as const;
	const resetTimers = new WeakMap<HTMLElement, number>();

	/** The rendered HTML carries the buttons; one delegated listener drives them. */
	function copyBlocks(node: HTMLElement) {
		const labelOf = (button: HTMLElement) => button.querySelector('.code-block__label');

		// lucide-svelte renders the two icons once; every button gets a clone.
		const icons = iconKeys.map((key) => ({
			key,
			icon: node.querySelector<SVGElement>(`[data-icon-source='${key}'] svg`)
		}));

		for (const button of node.querySelectorAll<HTMLElement>('[data-copy]')) {
			const label = labelOf(button);
			if (!label) continue;
			for (const { key, icon } of icons) {
				if (!icon) continue;
				const clone = icon.cloneNode(true) as SVGElement;
				clone.setAttribute('data-icon', key);
				button.insertBefore(clone, label);
			}
		}

		function reset(button: HTMLElement) {
			delete button.dataset.copied;
			button.setAttribute('aria-label', 'Copy code');
			const label = labelOf(button);
			if (label) label.textContent = 'Copy';
		}

		async function onClick(event: MouseEvent) {
			const button = (event.target as Element | null)?.closest<HTMLElement>('[data-copy]');
			if (!button || !node.contains(button)) return;

			const code = button.closest('.code-block')?.querySelector('pre code');
			if (!code) return;

			try {
				await navigator.clipboard.writeText(code.textContent ?? '');
			} catch {
				return;
			}

			button.dataset.copied = 'true';
			button.setAttribute('aria-label', 'Copied');
			const label = labelOf(button);
			if (label) label.textContent = 'Copied';

			window.clearTimeout(resetTimers.get(button));
			resetTimers.set(button, window.setTimeout(() => reset(button), COPIED_MS));
		}

		// Unhides the buttons only once clicking one does something.
		node.dataset.copyReady = 'true';
		node.addEventListener('click', onClick);

		return {
			destroy() {
				node.removeEventListener('click', onClick);
			}
		};
	}
</script>

<svelte:head>
	<title>{data.doc.title} · Tomoshibi</title>
	<meta name="description" content={data.doc.summary} />
	<link rel="alternate" type="text/markdown" href={data.doc.url} />
</svelte:head>

<article class="mx-auto max-w-3xl" use:copyBlocks>
	<!-- Icon sources for the build-time copy buttons, cloned in on mount. -->
	<span class="hidden" aria-hidden="true">
		<span data-icon-source="copy"><Copy class="code-block__icon" /></span>
		<span data-icon-source="check"><Check class="code-block__icon" /></span>
	</span>

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
	<div class="prose prose-sm dark:prose-invert max-w-none">
		{@html data.html}
	</div>
</article>
