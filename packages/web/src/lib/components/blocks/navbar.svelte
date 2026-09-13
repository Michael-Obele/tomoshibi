<script lang="ts">
	import { Menu, X, Code2, LayoutDashboard, FileText, Play, Crown, LogIn } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button';
	import ModeToggle from './mode-toggle.svelte';
	import { page } from '$app/state';
	import { getAuthStatus } from '$lib/remote/auth.remote';

	let auth = getAuthStatus();
	let user = $derived(auth.current?.user);

	let isOpen = $state(false);

	function toggleMenu() {
		isOpen = !isOpen;
	}

	function closeMenu() {
		isOpen = false;
	}

	const navItems = [
		{ href: '/', label: 'Home', icon: LayoutDashboard },
		{ href: '/docs', label: 'Docs', icon: FileText },
		{ href: '/playground', label: 'Playground', icon: Play }
	];

	function isActive(href: string): boolean {
		if (href === '/') {
			return page.url.pathname === '/';
		}
		if (href === '/docs') {
			return page.url.pathname.startsWith('/docs');
		}
		return page.url.pathname === href;
	}
</script>

<nav
	class="sticky top-0 z-50 w-full border-b border-border bg-background/80 backdrop-blur supports-backdrop-filter:bg-background/60"
>
	<div class="container mx-auto px-4">
		<div class="flex h-16 items-center justify-between">
			<!-- Logo -->
			<div class="flex items-center gap-2">
				<a
					href="/"
					class="flex items-center gap-2 text-lg font-bold text-foreground transition-colors hover:text-primary"
				>
					<div
						class="flex h-8 w-8 items-center justify-center rounded-md bg-primary font-bold text-white"
					>
						C
					</div>
					<span class="hidden sm:inline">Cinder</span>
				</a>
			</div>

			<!-- Desktop Navigation -->
			<div class="hidden items-center gap-1 md:flex">
				{#each navItems as item (item.href)}
					<Button
						variant="ghost"
						size="sm"
						href={item.href}
						class="gap-2 {isActive(item.href) ? 'bg-accent text-accent-foreground' : ''}"
					>
						<item.icon class="h-4 w-4" />
						<span class="text-sm font-medium">{item.label}</span>
					</Button>
				{/each}
			</div>

			<!-- Desktop Actions -->
			<div class="hidden items-center gap-2 md:flex">
				{#if user}
					<div class="mr-2 flex items-center gap-2 rounded-full border border-amber-500/30 bg-amber-500/10 py-1.5 pl-2 pr-3 text-xs font-bold text-amber-500 shadow-[0_0_15px_-3px_rgba(245,158,11,0.2)] animate-in fade-in zoom-in">
						<Crown class="h-3.5 w-3.5 fill-amber-500/20" />
						<span>MASTER ACCESS</span>
					</div>
				{:else}
					<Button href="/login" variant="ghost" size="sm" class="gap-2 text-muted-foreground hover:text-foreground hover:bg-accent mr-2">
						<LogIn class="h-4 w-4" />
						<span>Elevate</span>
					</Button>
				{/if}

				<Button variant="ghost" size="icon" class="text-muted-foreground hover:text-foreground">
					<a
						href="https://github.com/Michael-Obele/cinder-sv"
						target="_blank"
						rel="noopener noreferrer"
						title="GitHub"
						class="inline-flex"
					>
						<Code2 class="h-5 w-5" />
					</a>
				</Button>
				<ModeToggle />
			</div>

			<!-- Mobile Menu Button -->
			<Button variant="ghost" size="icon" onclick={toggleMenu} class="md:hidden">
				{#if isOpen}
					<X class="h-6 w-6" />
				{:else}
					<Menu class="h-6 w-6" />
				{/if}
			</Button>
		</div>

		<!-- Mobile Navigation -->
		{#if isOpen}
			<div class="border-t border-border bg-background pb-4 md:hidden">
				<div class="flex flex-col gap-1 pt-4">
					{#each navItems as item (item.href)}
						<Button
							variant="ghost"
							href={item.href}
							class="w-full justify-start gap-2 {isActive(item.href)
								? 'bg-accent text-accent-foreground'
								: ''}"
							onclick={closeMenu}
						>
							<item.icon class="h-4 w-4" />
							<span class="text-sm font-medium">{item.label}</span>
						</Button>
					{/each}

					<div class="my-2 border-t border-border"></div>
					<div class="flex items-center justify-between px-2">
						<div class="flex gap-1">
							<Button
								variant="ghost"
								size="icon"
								class="text-muted-foreground hover:text-foreground"
							>
								<a
									href="https://github.com/Michael-Obele/cinder-sv"
									target="_blank"
									rel="noopener noreferrer"
									class="inline-flex"
								>
									<Code2 class="h-5 w-5" />
								</a>
							</Button>
							<ModeToggle />
						</div>
					</div>
				</div>
			</div>
		{/if}
	</div>
</nav>
