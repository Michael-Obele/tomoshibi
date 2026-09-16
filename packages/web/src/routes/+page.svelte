<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import { Badge } from "$lib/components/ui/badge";
  import * as Card from "$lib/components/ui/card";
  import {
    Copy,
    Check,
    Terminal,
    Star,
    HeartHandshake,
    Zap,
    Container,
    ArrowRight,
    Shield,
    Clock,
    Layers,
    Sparkles,
    ExternalLink,
    Globe,
    Search,
    FileText,
    Code2,
    Box,
    Cpu,
  } from "@lucide/svelte";

  let copied = $state<string | null>(null);

  async function copy(text: string, id: string) {
    await navigator.clipboard.writeText(text);
    copied = id;
    setTimeout(() => (copied = null), 2000);
  }

  const dockerCompose = "git clone https://github.com/Michael-Obele/tomoshibi.git && cd tomoshibi\ndocker compose up -d";
  const dockerSingle = "docker run --rm -p 7431:7431 michaelobele/tomoshibi-api";
  const curlExample = `curl -X POST http://localhost:7431/v1/scrape \\
  -H "Content-Type: application/json" \\
  -d '{"url": "https://example.com"}'`;

  let activeSnippet = $state<"compose" | "single" | "curl">("compose");
</script>

<div class="flex flex-col">
  <!-- Eyebrow banner — urgency without spam -->
  <div class="border-b border-amber-500/20 bg-amber-500/10">
    <div class="container mx-auto flex items-center justify-center gap-2 px-4 py-2 text-center text-xs sm:text-sm">
      <span class="hidden h-2 w-2 animate-pulse rounded-full bg-amber-500 sm:inline-block"></span>
      <span class="text-muted-foreground">
        Free playground is <span class="font-semibold text-foreground">rate-limited & ephemeral</span> — self-host in 30s for unlimited use.
      </span>
      <a href="https://github.com/Michael-Obele/tomoshibi" target="_blank" rel="noopener noreferrer" class="hidden items-center gap-1 font-medium text-amber-600 hover:text-amber-700 dark:text-amber-400 sm:inline-flex">
        Star on GitHub <Star class="h-3 w-3" />
      </a>
    </div>
  </div>

  <!-- HERO — centered composition + code snippet (Evil Martians pattern) -->
  <section class="relative overflow-hidden">
    <!-- subtle glow -->
    <div class="pointer-events-none absolute inset-0 -z-10 bg-[radial-gradient(ellipse_at_top,_var(--tw-gradient-stops))] from-primary/15 via-transparent to-transparent"></div>
    <div class="pointer-events-none absolute inset-0 -z-10 bg-grid-slate-100 [mask-image:linear-gradient(to_bottom,white,transparent)] dark:bg-grid-slate-700/25"></div>

    <div class="container mx-auto px-4 py-12 sm:py-16 lg:py-20">
      <div class="mx-auto max-w-3xl text-center">
        <div class="mb-6 flex flex-wrap items-center justify-center gap-2">
          <Badge variant="secondary" class="gap-1.5 px-3 py-1 text-xs font-medium">
            <span class="h-2 w-2 rounded-full bg-emerald-500"></span>
            Open Source · MIT
          </Badge>
          <Badge variant="outline" class="gap-1.5 px-3 py-1 text-xs">
            <Box class="h-3 w-3" /> One binary · $5/mo
          </Badge>
          <Badge variant="outline" class="gap-1.5 px-3 py-1 text-xs">
            <Zap class="h-3 w-3 text-amber-500" /> ~200ms static
          </Badge>
        </div>

        <h1 class="text-4xl font-extrabold tracking-tight sm:text-5xl lg:text-6xl">
          Turn any website into
          <span class="bg-gradient-to-r from-primary to-amber-600 bg-clip-text text-transparent"> LLM-ready markdown</span>
          in 200ms
        </h1>
        <p class="mx-auto mt-6 max-w-2xl text-lg leading-relaxed text-muted-foreground sm:text-xl">
          Self-hosted Firecrawl alternative. Smart static → dynamic fallback, readability cleaning, and parallel crawling — without the per-token bill.
        </p>

        <!-- Dual CTA — bold primary + distinct secondary (Evil Martians) -->
        <div class="mt-8 flex flex-col items-center justify-center gap-3 sm:flex-row">
          <Button href="https://github.com/Michael-Obele/tomoshibi#quick-start" size="lg" class="group w-full gap-2 sm:w-auto">
            <Container class="h-4 w-4" />
            Run with Docker
            <ArrowRight class="h-4 w-4 transition-transform group-hover:translate-x-0.5" />
          </Button>
          <Button href="https://github.com/Michael-Obele/tomoshibi" target="_blank" variant="outline" size="lg" class="w-full gap-2 sm:w-auto">
            <Code2 class="h-4 w-4" />
            Star on GitHub
          </Button>
          <Button href="/playground" variant="ghost" size="lg" class="w-full gap-2 sm:w-auto">
            Try playground
            <span class="rounded bg-muted px-1.5 py-0.5 text-xs font-mono">free · limited</span>
          </Button>
        </div>
        <p class="mt-3 text-xs text-muted-foreground">
          No signup · No API key for self-host · Playground is just a preview
        </p>
      </div>

      <!-- Terminal — live code as hero visual (devtool best practice) -->
      <div class="mx-auto mt-10 max-w-3xl">
        <div class="overflow-hidden rounded-xl border bg-card shadow-xl">
          <div class="flex items-center justify-between border-b bg-muted/50 px-4 py-2.5">
            <div class="flex items-center gap-2">
              <div class="flex gap-1.5">
                <span class="h-3 w-3 rounded-full bg-red-500"></span>
                <span class="h-3 w-3 rounded-full bg-yellow-500"></span>
                <span class="h-3 w-3 rounded-full bg-green-500"></span>
              </div>
              <span class="ml-3 flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                <Terminal class="h-3.5 w-3.5" /> terminal
              </span>
            </div>
            <div class="flex gap-1">
              <button
                onclick={() => (activeSnippet = "compose")}
                class="rounded-md px-2.5 py-1 text-xs font-medium transition-colors {activeSnippet === 'compose' ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'}"
              >
                Compose
              </button>
              <button
                onclick={() => (activeSnippet = "single")}
                class="rounded-md px-2.5 py-1 text-xs font-medium transition-colors {activeSnippet === 'single' ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'}"
              >
                Docker
              </button>
              <button
                onclick={() => (activeSnippet = "curl")}
                class="rounded-md px-2.5 py-1 text-xs font-medium transition-colors {activeSnippet === 'curl' ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'}"
              >
                Scrape
              </button>
            </div>
          </div>
          <div class="relative">
            <pre class="overflow-x-auto p-4 text-sm leading-relaxed"><code class="font-mono text-foreground">{#if activeSnippet === "compose"}{dockerCompose}{:else if activeSnippet === "single"}{dockerSingle}{:else}{curlExample}{/if}</code></pre>
            <Button
              variant="ghost"
              size="icon"
              class="absolute right-2 top-2 h-7 w-7 bg-background/80 backdrop-blur"
              onclick={() => copy(activeSnippet === "compose" ? dockerCompose : activeSnippet === "single" ? dockerSingle : curlExample, activeSnippet)}
              aria-label="Copy command"
            >
              {#if copied === activeSnippet}
                <Check class="h-3.5 w-3.5 text-emerald-500" />
              {:else}
                <Copy class="h-3.5 w-3.5" />
              {/if}
            </Button>
          </div>
          <div class="flex items-center justify-between border-t bg-muted/30 px-4 py-2 text-xs text-muted-foreground">
            <span class="flex items-center gap-1.5">
              <Clock class="h-3 w-3" />
              {#if activeSnippet === "compose"}All 5 services on 7431–7435{:else if activeSnippet === "single"}Single service · needs Redis for crawl{:else}Smart mode is default — no config needed{/if}
            </span>
            <a href="/docs" class="hidden items-center gap-1 hover:text-foreground sm:inline-flex">
              Docs <ExternalLink class="h-3 w-3" />
            </a>
          </div>
        </div>
        <p class="mt-3 text-center text-xs text-muted-foreground">
          <span class="font-medium text-foreground">560 req/s p50 11ms</span> vs Firecrawl self-hosted 1.9 req/s p50 5.4s — ~300× throughput. Same $0.
          <a href="https://github.com/Michael-Obele/tomoshibi/blob/main/docs/SEARCH_COMPARISON.md" target="_blank" class="underline decoration-dotted underline-offset-2 hover:text-foreground">See benchmark</a>
        </p>
      </div>
    </div>
  </section>

  <!-- TRUST — numbers, not logos (individual-oriented, per research) -->
  <section class="border-y bg-muted/30">
    <div class="container mx-auto px-4 py-6">
      <div class="flex flex-wrap items-center justify-center gap-6 text-sm sm:gap-8">
        <a href="https://github.com/Michael-Obele/tomoshibi" target="_blank" rel="noopener noreferrer" class="flex items-center gap-2 hover:text-foreground">
          <Code2 class="h-4 w-4" />
          <span class="font-mono text-xs">GitHub</span>
          <img src="https://img.shields.io/github/stars/Michael-Obele/tomoshibi?style=social" alt="GitHub stars" class="h-5" loading="lazy" />
        </a>
        <span class="hidden h-4 w-px bg-border sm:inline-block"></span>
        <span class="flex items-center gap-2 text-muted-foreground">
          <span class="font-semibold text-foreground">Go 1.25+</span> · Gin + Chromedp + Colly
        </span>
        <span class="hidden h-4 w-px bg-border sm:inline-block"></span>
        <span class="flex items-center gap-2 text-muted-foreground">
          <Shield class="h-4 w-4 text-emerald-500" /> MIT · No telemetry
        </span>
        <span class="hidden h-4 w-px bg-border sm:inline-block"></span>
        <a href="https://www.npmjs.com/package/tomoshi" target="_blank" class="flex items-center gap-1.5 text-muted-foreground hover:text-foreground">
          <Box class="h-4 w-4" /> tomoshi CLI
          <img src="https://img.shields.io/npm/v/tomoshi?label=&color=cb0000" alt="npm version" class="h-5" loading="lazy" />
        </a>
      </div>
    </div>
  </section>

  <!-- PROBLEM → SOLUTION — not a feature list (Evil Martians: problem-oriented wins) -->
  <section class="container mx-auto px-4 py-12 sm:py-16">
    <div class="mx-auto max-w-3xl text-center">
      <Badge variant="outline" class="mb-4">Why self-host?</Badge>
      <h2 class="text-3xl font-bold tracking-tight sm:text-4xl">Stop paying per scrape</h2>
      <p class="mt-3 text-muted-foreground">Hosted APIs charge by the token and throttle you. Tomoshibi runs on your metal.</p>
    </div>

    <div class="mx-auto mt-10 grid max-w-5xl gap-4 sm:grid-cols-3">
      <Card.Root class="relative overflow-hidden">
        <Card.Header class="pb-3">
          <div class="mb-2 flex h-9 w-9 items-center justify-center rounded-lg bg-red-500/10 text-red-600 dark:text-red-400">
            <Clock class="h-5 w-5" />
          </div>
          <Card.Title class="text-base">Paying $0.01–0.10 per scrape</Card.Title>
          <Card.Description>Rate limits, quotas, and surprise bills when you scale RAG.</Card.Description>
        </Card.Header>
        <Card.Content class="pt-0">
          <div class="flex items-center gap-2 text-sm font-medium text-emerald-600 dark:text-emerald-400">
            <Check class="h-4 w-4" /> $0 self-hosted — one binary, hobby-tier RAM
          </div>
        </Card.Content>
      </Card.Root>

      <Card.Root class="relative overflow-hidden">
        <Card.Header class="pb-3">
          <div class="mb-2 flex h-9 w-9 items-center justify-center rounded-lg bg-amber-500/10 text-amber-600 dark:text-amber-400">
            <Cpu class="h-5 w-5" />
          </div>
          <Card.Title class="text-base">500ms Chrome spawn per request</Card.Title>
          <Card.Description>Spawning a browser per scrape kills throughput.</Card.Description>
        </Card.Header>
        <Card.Content class="pt-0">
          <div class="flex items-center gap-2 text-sm font-medium text-emerald-600 dark:text-emerald-400">
            <Check class="h-4 w-4" /> Shared allocator + recycled tabs → ~200ms
          </div>
        </Card.Content>
      </Card.Root>

      <Card.Root class="relative overflow-hidden">
        <Card.Header class="pb-3">
          <div class="mb-2 flex h-9 w-9 items-center justify-center rounded-lg bg-blue-500/10 text-blue-600 dark:text-blue-400">
            <Layers class="h-5 w-5" />
          </div>
          <Card.Title class="text-base">JS SPAs return empty HTML</Card.Title>
          <Card.Description>Static scrapers miss React/Vue content.</Card.Description>
        </Card.Header>
        <Card.Content class="pt-0">
          <div class="flex items-center gap-2 text-sm font-medium text-emerald-600 dark:text-emerald-400">
            <Check class="h-4 w-4" /> Smart mode: static first, Chromedp fallback
          </div>
        </Card.Content>
      </Card.Root>
    </div>

    <div class="mx-auto mt-6 grid max-w-5xl gap-4 sm:grid-cols-3">
      <Card.Root class="border-dashed">
        <Card.Header class="pb-2">
          <Card.Title class="flex items-center gap-2 text-sm">
            <Globe class="h-4 w-4 text-muted-foreground" /> Noisy HTML
          </Card.Title>
        </Card.Header>
        <Card.Content class="text-sm text-muted-foreground">
          Nav/ads/footer pollute the LLM context.
          <span class="font-medium text-foreground"> → Readability + ad block → clean markdown.</span>
        </Card.Content>
      </Card.Root>
      <Card.Root class="border-dashed">
        <Card.Header class="pb-2">
          <Card.Title class="flex items-center gap-2 text-sm">
            <Search class="h-4 w-4 text-muted-foreground" /> Crawl needs a fleet
          </Card.Title>
        </Card.Header>
        <Card.Content class="text-sm text-muted-foreground">
          Separate worker fleet per container.
          <span class="font-medium text-foreground"> → Monolith: Gin + Asynq in one process.</span>
        </Card.Content>
      </Card.Root>
      <Card.Root class="border-dashed">
        <Card.Header class="pb-2">
          <Card.Title class="flex items-center gap-2 text-sm">
            <FileText class="h-4 w-4 text-muted-foreground" /> Search costs extra
          </Card.Title>
        </Card.Header>
        <Card.Content class="text-sm text-muted-foreground">
          Another API, another bill.
          <span class="font-medium text-foreground"> → Built-in SearXNG, 560 req/s.</span>
        </Card.Content>
      </Card.Root>
    </div>
  </section>

  <!-- CONTRAST — Free hosted vs Self-hosted (anchoring) -->
  <section class="border-y bg-muted/30">
    <div class="container mx-auto px-4 py-12 sm:py-16">
      <div class="mx-auto max-w-5xl">
        <div class="text-center">
          <h2 class="text-2xl font-bold tracking-tight sm:text-3xl">Free playground vs self-hosted</h2>
          <p class="mx-auto mt-2 max-w-2xl text-sm text-muted-foreground">
            The hosted playground is a preview — rate-limited, ephemeral, and shared. For anything real, run it yourself.
          </p>
        </div>

        <div class="mt-8 grid gap-6 lg:grid-cols-2">
          <!-- Free — de-emphasized -->
          <Card.Root class="relative">
            <Card.Header>
              <div class="flex items-center justify-between">
                <Card.Title class="text-lg">Free playground</Card.Title>
                <Badge variant="secondary" class="text-xs">Preview only</Badge>
              </div>
              <Card.Description>Try it in the browser — no install.</Card.Description>
            </Card.Header>
            <Card.Content class="space-y-3 text-sm">
              <div class="flex items-center gap-2 text-muted-foreground">
                <span class="h-1.5 w-1.5 rounded-full bg-amber-500"></span> Rate-limited & shared
              </div>
              <div class="flex items-center gap-2 text-muted-foreground">
                <span class="h-1.5 w-1.5 rounded-full bg-amber-500"></span> No crawl / batch / monitor without Redis
              </div>
              <div class="flex items-center gap-2 text-muted-foreground">
                <span class="h-1.5 w-1.5 rounded-full bg-amber-500"></span> Data not persisted
              </div>
              <div class="flex items-center gap-2 text-muted-foreground">
                <span class="h-1.5 w-1.5 rounded-full bg-amber-500"></span> Best for quick tests
              </div>
              <Button href="/playground" variant="outline" class="mt-4 w-full">
                Open playground
              </Button>
            </Card.Content>
          </Card.Root>

          <!-- Self-hosted — recommended, smart default -->
          <Card.Root class="relative border-primary/50 shadow-lg shadow-primary/10">
            <div class="absolute -top-3 left-1/2 -translate-x-1/2">
              <Badge class="gap-1 px-3 py-1 shadow-md">
                <Sparkles class="h-3 w-3" /> Recommended · Most Popular
              </Badge>
            </div>
            <Card.Header class="pt-8">
              <div class="flex items-center justify-between">
                <Card.Title class="text-lg">Self-hosted</Card.Title>
                <span class="text-sm font-bold text-primary">$0 + $5/mo infra</span>
              </div>
              <Card.Description>Unlimited, private, and fast — your infra, your data.</Card.Description>
            </Card.Header>
            <Card.Content class="space-y-3 text-sm">
              <div class="flex items-center gap-2">
                <Check class="h-4 w-4 text-emerald-500" /> Unlimited scrape / crawl / batch / search
              </div>
              <div class="flex items-center gap-2">
                <Check class="h-4 w-4 text-emerald-500" /> All 5 services + SearXNG + Redis
              </div>
              <div class="flex items-center gap-2">
                <Check class="h-4 w-4 text-emerald-500" /> Private — no data leaves your VPC
              </div>
              <div class="flex items-center gap-2">
                <Check class="h-4 w-4 text-emerald-500" /> 512MB–1GB hobby tier · Fly / Render / Docker
              </div>
              <div class="mt-4 flex gap-2">
                <Button href="https://github.com/Michael-Obele/tomoshibi#quick-start" class="flex-1 gap-2">
                  <Container class="h-4 w-4" /> Docker Compose
                </Button>
                <Button variant="outline" class="gap-2" onclick={() => copy(dockerCompose, 'compare')}>
                  {#if copied === 'compare'}<Check class="h-4 w-4" /> Copied{:else}<Copy class="h-4 w-4" /> Copy{/if}
                </Button>
              </div>
              <p class="text-center text-xs text-muted-foreground">
                One command · Ports 7431–7435 · No clash with 3000/8080
              </p>
            </Card.Content>
          </Card.Root>
        </div>
      </div>
    </div>
  </section>

  <!-- HOW IT WORKS — goal gradient (3 steps) -->
  <section class="container mx-auto px-4 py-12 sm:py-16">
    <div class="mx-auto max-w-3xl text-center">
      <h2 class="text-2xl font-bold tracking-tight sm:text-3xl">From zero to markdown in 30 seconds</h2>
      <p class="mt-2 text-muted-foreground">No SDK, no dashboard, no API key dance.</p>
    </div>
    <div class="mx-auto mt-10 grid max-w-4xl gap-6 sm:grid-cols-3">
      <div class="relative text-center">
        <div class="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-primary text-primary-foreground font-bold">1</div>
        <h3 class="mt-4 font-semibold">Clone & up</h3>
        <p class="mt-1 text-sm text-muted-foreground"><code class="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">docker compose up -d</code></p>
        <p class="mt-1 text-xs text-muted-foreground">API 7431 · Web 7432 · MCP 7433 · Redis 7434 · SearXNG 7435</p>
      </div>
      <div class="relative text-center">
        <div class="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-primary text-primary-foreground font-bold">2</div>
        <h3 class="mt-4 font-semibold">Scrape anything</h3>
        <p class="mt-1 text-sm text-muted-foreground"><code class="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">POST /v1/scrape {"{url}"}</code></p>
        <p class="mt-1 text-xs text-muted-foreground">Smart mode auto-picks static or Chromedp</p>
      </div>
      <div class="relative text-center">
        <div class="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-primary text-primary-foreground font-bold">3</div>
        <h3 class="mt-4 font-semibold">Get LLM-ready data</h3>
        <p class="mt-1 text-sm text-muted-foreground">Markdown + metadata + links + optional screenshot</p>
        <p class="mt-1 text-xs text-muted-foreground">Crawl, batch, search, monitor — same API</p>
      </div>
    </div>
    <div class="mt-8 flex flex-wrap items-center justify-center gap-3">
      <Button href="https://github.com/Michael-Obele/tomoshibi#quick-start" variant="outline" class="gap-2">
        <FileText class="h-4 w-4" /> Read quick start
      </Button>
      <Button href="/docs" variant="ghost" class="gap-2">
        API reference <ArrowRight class="h-4 w-4" />
      </Button>
    </div>
  </section>

  <!-- SPONSOR — loss aversion + reciprocity -->
  <section class="border-y bg-gradient-to-b from-amber-500/10 via-transparent to-transparent">
    <div class="container mx-auto px-4 py-12 sm:py-16">
      <div class="mx-auto max-w-3xl text-center">
        <div class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-amber-500/15 text-amber-600 dark:text-amber-400">
          <HeartHandshake class="h-6 w-6" />
        </div>
        <h2 class="text-2xl font-bold tracking-tight sm:text-3xl">Keep Tomoshibi free & open</h2>
        <p class="mx-auto mt-3 max-w-2xl text-muted-foreground">
          Tomoshibi is MIT-licensed and built in the open. Sponsorship funds Chromedp upkeep, SearXNG tuning, and hobby-tier performance work — so self-hosting stays $0.
        </p>
        <div class="mt-6 flex flex-col items-center justify-center gap-3 sm:flex-row">
          <Button href="https://github.com/sponsors/Michael-Obele" target="_blank" size="lg" class="gap-2 bg-[#ea4aaa] hover:bg-[#d63f98] text-white border-0">
            <HeartHandshake class="h-4 w-4" /> Sponsor on GitHub
          </Button>
          <Button href="https://github.com/Michael-Obele/tomoshibi" target="_blank" variant="outline" size="lg" class="gap-2">
            <Star class="h-4 w-4" /> Star the repo
          </Button>
        </div>
        <p class="mt-3 text-xs text-muted-foreground">
          Stars drive discovery · Sponsors fund maintenance · Both keep the free tier alive
        </p>
      </div>
    </div>
  </section>

  <!-- FINAL CTA -->
  <section class="container mx-auto px-4 py-12 sm:py-16">
    <div class="mx-auto max-w-3xl rounded-2xl border bg-card p-8 text-center shadow-sm sm:p-10">
      <h2 class="text-2xl font-bold tracking-tight sm:text-3xl">Self-host Tomoshibi today</h2>
      <p class="mx-auto mt-2 max-w-xl text-muted-foreground">One binary, one compose file, no vendor lock-in. Your data never leaves your machine.</p>
      <div class="mt-6 flex flex-col items-center justify-center gap-3 sm:flex-row">
        <Button href="https://github.com/Michael-Obele/tomoshibi#quick-start" size="lg" class="w-full gap-2 sm:w-auto">
          <Container class="h-4 w-4" /> docker compose up -d
        </Button>
        <Button href="https://github.com/Michael-Obele/tomoshibi" target="_blank" variant="outline" size="lg" class="w-full gap-2 sm:w-auto">
          <Code2 class="h-4 w-4" /> View on GitHub
        </Button>
      </div>
      <div class="mt-6 flex flex-wrap items-center justify-center gap-4 text-xs text-muted-foreground">
        <span class="flex items-center gap-1.5"><Shield class="h-3.5 w-3.5" /> MIT License</span>
        <span class="flex items-center gap-1.5"><Box class="h-3.5 w-3.5" /> Docker Hub & GHCR</span>
        <span class="flex items-center gap-1.5"><Terminal class="h-3.5 w-3.5" /> tomoshi CLI on npm</span>
      </div>
    </div>
  </section>
</div>
