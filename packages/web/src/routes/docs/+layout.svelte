<script lang="ts">
	import * as Sidebar from '$lib/components/ui/sidebar';
	import { page } from '$app/state';
	import { Play } from '@lucide/svelte';
	import { documentationGroups } from '$lib/docs-content';

	let { children } = $props();
</script>

<div class="flex h-full w-full flex-col">
	<Sidebar.Provider class="min-h-0! flex-1">
		<Sidebar.Root>
			<Sidebar.Header>
				<h2 class="flex items-center gap-2 px-4 py-2 text-lg font-bold">
					<div class="h-6 w-6 rounded bg-primary"></div>
					Cinder Docs
				</h2>
			</Sidebar.Header>
			<Sidebar.Content class="px-5">
				{#each documentationGroups as group (group.title)}
					<Sidebar.Group>
						<Sidebar.GroupLabel>{group.title}</Sidebar.GroupLabel>
						<Sidebar.GroupContent>
							<Sidebar.Menu>
								{#each group.items as item (item.url)}
									<Sidebar.MenuItem>
										<Sidebar.MenuButton isActive={page.url.pathname === item.url}>
											<item.icon />
											<a href={item.url}>{item.title}</a>
										</Sidebar.MenuButton>
									</Sidebar.MenuItem>
								{/each}
							</Sidebar.Menu>
						</Sidebar.GroupContent>
					</Sidebar.Group>
				{/each}
			</Sidebar.Content>
			<Sidebar.Footer>
				<Sidebar.Menu>
					<Sidebar.MenuItem>
						<Sidebar.MenuButton>
							<Play />
							<a href="/playground">Go to Playground</a>
						</Sidebar.MenuButton>
					</Sidebar.MenuItem>
				</Sidebar.Menu>
			</Sidebar.Footer>
		</Sidebar.Root>
		<Sidebar.Inset>
			<header
				class="sticky top-0 z-10 flex h-16 shrink-0 items-center gap-2 border-b bg-background px-4 transition-[width,height] ease-linear group-has-data-[collapsible=icon]/sidebar-wrapper:h-12"
			>
				<div class="flex items-center gap-2 px-4">
					<Sidebar.Trigger />
					<div class="mx-2 h-4 w-px bg-border"></div>
					<h1 class="text-sm font-medium">Documentation</h1>
				</div>
			</header>
			<div class="mx-auto w-full max-w-4xl p-6">
				{@render children()}
			</div>
		</Sidebar.Inset>
	</Sidebar.Provider>
</div>
