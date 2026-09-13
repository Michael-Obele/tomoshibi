<script lang="ts">
	import {
		Code2,
		Mail,
		Send,
		Heart,
		ExternalLink,
		Globe,
		Shield,
		MessageCircle
	} from '@lucide/svelte';
	import * as Tooltip from '$lib/components/ui/tooltip';
</script>

<footer class="border-t border-border bg-background py-16">
	<div class="container mx-auto px-4">
		<!-- Main Footer Content -->
		<div class="mb-12 grid grid-cols-1 gap-12 md:grid-cols-4 lg:grid-cols-5">
			<!-- Brand Section -->
			<div class="flex flex-col gap-4 lg:col-span-2">
				<div class="flex items-center gap-2">
					<div
						class="flex h-10 w-10 items-center justify-center rounded-lg bg-primary font-bold text-white shadow-sm"
					>
						C
					</div>
					<span class="text-xl font-bold tracking-tight text-foreground">Cinder</span>
				</div>
				<p class="max-w-xs text-sm leading-relaxed text-muted-foreground">
					The ultimate tool for modern web exploration. Scrape, crawl, and analyze the web with ease
					and precision.
				</p>
				<div class="mt-2 flex gap-3">
					<Tooltip.Root>
						<Tooltip.Trigger>
							{#snippet child({ props })}
								<a
									{...props}
									href="https://github.com/Michael-Obele/cinder-sv"
									target="_blank"
									rel="noopener noreferrer"
									class="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-muted/50 transition-all hover:bg-primary hover:text-primary-foreground hover:shadow-md"
								>
									<Code2 class="h-5 w-5" />
									<span class="sr-only">GitHub</span>
								</a>
							{/snippet}
						</Tooltip.Trigger>
						<Tooltip.Content side="top">GitHub</Tooltip.Content>
					</Tooltip.Root>
					<Tooltip.Root>
						<Tooltip.Trigger>
							{#snippet child({ props })}
								<a
									{...props}
									href="https://twitter.com"
									target="_blank"
									rel="noopener noreferrer"
									class="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-muted/50 transition-all hover:bg-primary hover:text-primary-foreground hover:shadow-md"
								>
									<Send class="h-5 w-5" />
									<span class="sr-only">Twitter</span>
								</a>
							{/snippet}
						</Tooltip.Trigger>
						<Tooltip.Content side="top">Twitter</Tooltip.Content>
					</Tooltip.Root>
					<Tooltip.Root>
						<Tooltip.Trigger>
							{#snippet child({ props })}
								<a
									{...props}
									href="mailto:contact@example.com"
									class="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-muted/50 transition-all hover:bg-primary hover:text-primary-foreground hover:shadow-md"
								>
									<Mail class="h-5 w-5" />
									<span class="sr-only">Email</span>
								</a>
							{/snippet}
						</Tooltip.Trigger>
						<Tooltip.Content side="top">Email</Tooltip.Content>
					</Tooltip.Root>
				</div>
			</div>

			<!-- Documentation -->
			<div class="flex flex-col gap-4">
				<h3 class="text-sm font-semibold tracking-wider text-foreground/80 uppercase">
					Documentation
				</h3>
				<nav class="flex flex-col gap-3">
					<a
						href="/docs/setup"
						class="flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-primary"
					>
						Setup Guide
					</a>
					<a
						href="/docs/architecture"
						class="flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-primary"
					>
						Architecture
					</a>
					<a
						href="/docs/features"
						class="flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-primary"
					>
						Features
					</a>
					<a
						href="/docs/theme"
						class="flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-primary"
					>
						Theme System
					</a>
				</nav>
			</div>

			<!-- Resources -->
			<div class="flex flex-col gap-4">
				<h3 class="text-sm font-semibold tracking-wider text-foreground/80 uppercase">Resources</h3>
				<nav class="flex flex-col gap-3">
					<a
						href="/playground"
						class="flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-primary"
					>
						Interactive Playground
					</a>
					<a
						href="/docs/user-flow"
						class="flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-primary"
					>
						User Workflow
					</a>
					<a
						href="https://github.com/Michael-Obele/cinder-sv"
						target="_blank"
						rel="noopener noreferrer"
						class="flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-primary"
					>
						Frontend Source <ExternalLink class="h-3 w-3" />
					</a>
					<a
						href="https://github.com/Michael-Obele/cinder"
						target="_blank"
						rel="noopener noreferrer"
						class="flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-primary"
					>
						Backend Source <ExternalLink class="h-3 w-3" />
					</a>
					<a
						href="/docs/api"
						class="flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-primary"
					>
						API Reference
					</a>
				</nav>
			</div>

			<!-- Support & Legal -->
			<div class="flex flex-col gap-4">
				<h3 class="text-sm font-semibold tracking-wider text-foreground/80 uppercase">Support</h3>
				<nav class="flex flex-col gap-3">
					<a
						href="https://github.com/Michael-Obele/cinder-sv/discussions"
						target="_blank"
						rel="noopener noreferrer"
						class="flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-primary"
					>
						<MessageCircle class="h-4 w-4" /> Community Forum
					</a>
					<a
						href="https://github.com/Michael-Obele/cinder-sv"
						target="_blank"
						rel="noopener noreferrer"
						class="flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-primary"
					>
						<Globe class="h-4 w-4" /> Status Page
					</a>
					<a
						href="/docs/theme"
						class="flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-primary"
					>
						<Shield class="h-4 w-4" /> Privacy Policy
					</a>
				</nav>
			</div>
		</div>

		<!-- Divider -->
		<div class="border-t border-border"></div>

		<!-- Bottom Section -->
		<div class="flex flex-col items-center justify-between gap-6 py-8 md:flex-row">
			<div class="flex flex-col items-center gap-1 md:items-start">
				<p class="text-sm text-muted-foreground">&copy; 2026 Cinder. All rights reserved.</p>
				<p class="text-xs text-muted-foreground/60">
					Built with SvelteKit 5, Tailwind CSS v4, and Lucide Icons.
				</p>
			</div>
			<div class="flex items-center gap-6">
				<div
					class="flex items-center gap-1 text-sm text-muted-foreground transition-colors hover:text-foreground"
				>
					Made with
					<Heart class="mx-0.5 h-4 w-4 animate-pulse fill-red-500 text-red-500" />
					by Cinder Team
				</div>
			</div>
		</div>
	</div>
</footer>
