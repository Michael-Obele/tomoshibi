<script lang="ts">
  import { IsMobile } from "$lib/hooks/is-mobile.svelte";
  import * as Sheet from "$lib/components/ui/sheet";
  import { Button } from "$lib/components/ui/button";
  import { BookOpen, Play, PanelLeft } from "@lucide/svelte";
  import { page } from "$app/state";
  import { documentationGroups } from "$lib/docs-content";

  const isMobile = new IsMobile();
  let sidebarOpen = $state(true);

  let { children } = $props();

  function isActive(url: string): boolean {
    return page.url.pathname === url;
  }
</script>

{#snippet docsNav()}
  <div class="flex h-full flex-col">
    <!-- Header — matches playground history header -->
    <div
      class="flex items-center gap-2 border-b bg-background/50 p-4 backdrop-blur"
    >
      <BookOpen class="size-4 text-muted-foreground" />
      <h2 class="text-sm font-semibold">Documentation</h2>
    </div>

    <!-- Nav — scrollable, like history list -->
    <div class="flex-1 space-y-6 overflow-y-auto p-3">
      {#each documentationGroups as group (group.title)}
        <div>
          <h3
            class="mb-2 px-2 text-[11px] font-bold tracking-wider text-muted-foreground uppercase"
          >
            {group.title}
          </h3>
          <nav class="space-y-1">
            {#each group.items as item (item.url)}
              <a
                href={item.url}
                class="flex items-center gap-2 rounded-lg px-2.5 py-2 text-sm font-medium transition-colors {isActive(
                  item.url,
                )
                  ? 'bg-primary text-primary-foreground shadow-sm'
                  : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'}"
              >
                <item.icon class="size-4 shrink-0" />
                {item.title}
              </a>
            {/each}
          </nav>
        </div>
      {/each}
    </div>

    <!-- Footer — matches playground Clear History footer -->
    <div class="border-t bg-background/50 p-3 backdrop-blur">
      <Button
        href="/playground"
        variant="outline"
        size="sm"
        class="w-full gap-2"
      >
        <Play class="size-3.5" />
        Go to Playground
      </Button>
    </div>
  </div>
{/snippet}

<div class="flex flex-1 flex-col bg-background md:flex-row">
  <!-- Desktop: sticky aside below navbar, never covers footer (like playground history) -->
  {#if !isMobile.current && sidebarOpen}
    <aside
      class="relative flex min-h-[50vh] w-72 shrink-0 flex-col self-start border-r border-b bg-muted/30 md:sticky md:top-16 md:min-h-[calc(100vh-4rem)] md:border-b-0"
    >
      {@render docsNav()}
    </aside>
  {/if}

  <!-- Main -->
  <main class="w-full min-w-0 flex-1">
    <div class="mx-auto max-w-6xl">
      <!-- Toolbar — matches playground workbench header -->
      <div class="flex items-center gap-2 border-b px-4 py-3 md:px-6">
        {#if isMobile.current}
          <Sheet.Root>
            <Sheet.Trigger>
              {#snippet child({ props })}
                <Button {...props} variant="outline" size="sm">
                  <PanelLeft class="mr-2 size-4" />
                  Docs
                </Button>
              {/snippet}
            </Sheet.Trigger>
            <Sheet.Content side="left" class="w-72 p-0">
              {@render docsNav()}
            </Sheet.Content>
          </Sheet.Root>
        {:else}
          <Button
            variant="outline"
            size="sm"
            onclick={() => (sidebarOpen = !sidebarOpen)}
          >
            <PanelLeft class="mr-2 size-4" />
            {sidebarOpen ? "Hide Docs" : "Show Docs"}
          </Button>
        {/if}
        <div class="mx-2 h-4 w-px bg-border"></div>
        <h1 class="text-sm font-medium">Documentation</h1>
      </div>

      <div class="p-6">
        {@render children()}
      </div>
    </div>
  </main>
</div>
