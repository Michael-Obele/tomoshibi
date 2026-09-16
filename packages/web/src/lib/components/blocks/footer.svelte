<script lang="ts">
  import Logo from "$lib/components/brand/Logo.svelte";
  import {
    Code2,
    Mail,
    Heart,
    ExternalLink,
    HeartHandshake,
    Container,
    Star,
    Box,
    FileText,
    MessageCircle,
  } from "@lucide/svelte";
  import * as Tooltip from "$lib/components/ui/tooltip";
  import { Button } from "$lib/components/ui/button";
</script>

<footer class="border-t border-border bg-background py-12">
  <div class="container mx-auto px-4">
    <!-- Sponsor banner -->
    <div class="mb-10 flex flex-col items-center justify-between gap-4 rounded-xl border border-amber-500/20 bg-amber-500/5 px-6 py-5 sm:flex-row">
      <div class="text-center sm:text-left">
        <p class="text-sm font-semibold">Keep Tomoshibi free & open</p>
        <p class="text-xs text-muted-foreground">Stars drive discovery · Sponsors fund maintenance</p>
      </div>
      <div class="flex gap-2">
        <Button href="https://github.com/sponsors/Michael-Obele" target="_blank" size="sm" class="gap-1.5 bg-[#ea4aaa] hover:bg-[#d63f98] text-white border-0">
          <HeartHandshake class="h-4 w-4" /> Sponsor
        </Button>
        <Button href="https://github.com/Michael-Obele/tomoshibi" target="_blank" variant="outline" size="sm" class="gap-1.5">
          <Star class="h-4 w-4" /> Star
        </Button>
      </div>
    </div>

    <div class="mb-10 grid grid-cols-1 gap-10 md:grid-cols-4 lg:grid-cols-5">
      <!-- Brand -->
      <div class="flex flex-col gap-4 lg:col-span-2">
        <div class="flex items-center gap-2">
          <Logo size="md" class="rounded-lg bg-primary/10 shadow-sm" />
          <span class="text-xl font-bold tracking-tight text-foreground">Tomoshibi</span>
        </div>
        <p class="max-w-xs text-sm leading-relaxed text-muted-foreground">
          Self-hosted scraping API. Turn any site into LLM-ready markdown in 200ms. One binary, $5/mo infra.
        </p>
        <div class="mt-1 flex flex-wrap gap-2">
          <Tooltip.Root>
            <Tooltip.Trigger>
              {#snippet child({ props })}
                <a
                  {...props}
                  href="https://github.com/Michael-Obele/tomoshibi"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="inline-flex h-9 w-9 items-center justify-center rounded-lg bg-muted/50 transition-all hover:bg-primary hover:text-primary-foreground"
                >
                  <Code2 class="h-4 w-4" />
                  <span class="sr-only">GitHub</span>
                </a>
              {/snippet}
            </Tooltip.Trigger>
            <Tooltip.Content side="top">Star on GitHub</Tooltip.Content>
          </Tooltip.Root>
          <Tooltip.Root>
            <Tooltip.Trigger>
              {#snippet child({ props })}
                <a
                  {...props}
                  href="https://github.com/sponsors/Michael-Obele"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="inline-flex h-9 w-9 items-center justify-center rounded-lg bg-muted/50 transition-all hover:bg-[#ea4aaa] hover:text-white"
                >
                  <HeartHandshake class="h-4 w-4" />
                  <span class="sr-only">Sponsor</span>
                </a>
              {/snippet}
            </Tooltip.Trigger>
            <Tooltip.Content side="top">Sponsor</Tooltip.Content>
          </Tooltip.Root>
          <Tooltip.Root>
            <Tooltip.Trigger>
              {#snippet child({ props })}
                <a
                  {...props}
                  href="mailto:contact@example.com"
                  class="inline-flex h-9 w-9 items-center justify-center rounded-lg bg-muted/50 transition-all hover:bg-primary hover:text-primary-foreground"
                >
                  <Mail class="h-4 w-4" />
                  <span class="sr-only">Email</span>
                </a>
              {/snippet}
            </Tooltip.Trigger>
            <Tooltip.Content side="top">Email</Tooltip.Content>
          </Tooltip.Root>
        </div>
      </div>

      <!-- Self-host -->
      <div class="flex flex-col gap-4">
        <h3 class="text-sm font-semibold tracking-wider text-foreground/80 uppercase">Self-host</h3>
        <nav class="flex flex-col gap-3">
          <a href="/docs/setup" class="flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-primary">
            <Container class="h-3.5 w-3.5" /> Docker Compose
          </a>
          <a href="https://hub.docker.com/r/michaelobele/tomoshibi-api" target="_blank" rel="noopener noreferrer" class="flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-primary">
            <Box class="h-3.5 w-3.5" /> Docker Hub <ExternalLink class="h-3 w-3" />
          </a>
          <a href="https://github.com/Michael-Obele/tomoshibi/pkgs/container/tomoshibi-api" target="_blank" rel="noopener noreferrer" class="flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-primary">
            GHCR <ExternalLink class="h-3 w-3" />
          </a>
          <a href="/docs/deployment" class="flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-primary">
            Deploy to Fly / Render
          </a>
        </nav>
      </div>

      <!-- Docs -->
      <div class="flex flex-col gap-4">
        <h3 class="text-sm font-semibold tracking-wider text-foreground/80 uppercase">Docs</h3>
        <nav class="flex flex-col gap-3">
          <a href="/docs/setup" class="text-sm text-muted-foreground transition-colors hover:text-primary">Quick Start</a>
          <a href="/docs/api" class="text-sm text-muted-foreground transition-colors hover:text-primary">API Reference</a>
          <a href="/docs/deployment" class="text-sm text-muted-foreground transition-colors hover:text-primary">Self-hosting</a>
          <a href="/playground" class="flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-primary">
            Playground <span class="rounded bg-muted px-1.5 py-0.5 text-[10px] font-mono">limited</span>
          </a>
        </nav>
      </div>

      <!-- Support -->
      <div class="flex flex-col gap-4">
        <h3 class="text-sm font-semibold tracking-wider text-foreground/80 uppercase">Support</h3>
        <nav class="flex flex-col gap-3">
          <a href="https://github.com/sponsors/Michael-Obele" target="_blank" rel="noopener noreferrer" class="flex items-center gap-2 text-sm font-medium text-[#ea4aaa] hover:text-[#d63f98]">
            <HeartHandshake class="h-4 w-4" /> Sponsor
          </a>
          <a href="https://github.com/Michael-Obele/tomoshibi" target="_blank" rel="noopener noreferrer" class="flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-primary">
            <Star class="h-4 w-4" /> Star on GitHub
          </a>
          <a href="https://github.com/Michael-Obele/tomoshibi/discussions" target="_blank" rel="noopener noreferrer" class="flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-primary">
            <MessageCircle class="h-4 w-4" /> Discussions
          </a>
          <a href="https://github.com/Michael-Obele/tomoshibi" target="_blank" rel="noopener noreferrer" class="flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-primary">
            <FileText class="h-4 w-4" /> MIT License
          </a>
        </nav>
      </div>
    </div>

    <div class="border-t border-border"></div>

    <div class="flex flex-col items-center justify-between gap-6 py-8 md:flex-row">
      <div class="flex flex-col items-center gap-1 md:items-start">
        <p class="text-sm text-muted-foreground">&copy; 2026 Tomoshibi. MIT licensed.</p>
        <p class="text-xs text-muted-foreground/60">SvelteKit 5 · Tailwind v4 · shadcn-svelte · Lucide</p>
      </div>
      <div class="flex items-center gap-1 text-sm text-muted-foreground">
        Made with <Heart class="mx-0.5 h-4 w-4 animate-pulse fill-red-500 text-red-500" /> by Tomoshibi Team
      </div>
    </div>
  </div>
</footer>
