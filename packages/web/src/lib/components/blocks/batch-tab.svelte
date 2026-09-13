<script lang="ts">
	import { Package, Loader2, AlertCircle, ExternalLink, Copy, Check, Clock } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Label } from '$lib/components/ui/label';
	import * as Card from '$lib/components/ui/card';
	import { Textarea } from '$lib/components/ui/textarea';
	import { batchScrape, getBatchStatus } from '$lib/remote/cinder.remote';

	let {
		batchError = $bindable(),
		selectedHistoryItem = $bindable(),
		addToHistory
	} = $props<{
		batchError: string | null;
		selectedHistoryItem: any;
		addToHistory: (item: any) => Promise<void>;
	}>();

	let batchId = $state<string | null>(null);
	let copied = $state(false);

	// Poll batch status when we have an id
	const statusQuery = $derived(batchId ? getBatchStatus(batchId) : null);
	let status = $state<any>(null);

	$effect(() => {
		if (batchId && statusQuery) {
			status = statusQuery.current;
			const interval = setInterval(async () => {
				await statusQuery.refresh();
				status = statusQuery.current;
			}, 2500);
			return () => clearInterval(interval);
		}
	});

	let progress = $derived(
		status ? Math.round(((status.completed + status.failed) / Math.max(1, status.total)) * 100) : 0
	);
	let isDone = $derived(
		status && status.completed + status.failed >= status.total && status.total > 0
	);

	function copyTasks() {
		if (!status?.tasks) return;
		navigator.clipboard.writeText(status.tasks.map((t: any) => t.url).join('\n'));
		copied = true;
		setTimeout(() => (copied = false), 1500);
	}
</script>

<section class="overflow-hidden rounded-xl border bg-card/50 shadow-sm backdrop-blur-sm">
	<div class="p-6">
		<form
			{...batchScrape.enhance(async ({ submit }) => {
				batchError = null;
				selectedHistoryItem = null;
				batchId = null;
				status = null;
				try {
					await submit();
					const r = batchScrape.result as any;
					if (r?.batch_id) {
						batchId = r.batch_id;
						addToHistory({
							id: crypto.randomUUID(),
							type: 'batch',
							title: `Batch ${r.batch_id.slice(0, 8)}`,
							url: `${r.tasks?.length ?? 0} URLs`,
							timestamp: new Date().toISOString(),
							preview:
								r.tasks
									?.map((t: any) => t.url)
									.slice(0, 2)
									.join(', ') ?? '',
							meta: { count: r.tasks?.length ?? 0 },
							data: r
						});
					}
				} catch (e: any) {
					batchError = e.body?.message || e.message || 'Batch failed';
				}
			})}
			class="space-y-4"
		>
			<div class="flex flex-col gap-2">
				<Label
					for="batch-urls"
					class="pl-1 text-xs font-semibold tracking-wider text-muted-foreground uppercase"
				>
					URLs to Scrape (one per line or comma-separated, max 20)
				</Label>
				<Textarea
					id="batch-urls"
					{...batchScrape.fields.urls.as('text')}
					placeholder={'https://example.com\nhttps://example.org\nhttps://docs.example.com'}
					class="min-h-28 bg-background font-mono text-xs"
					rows={5}
				/>
				{#each batchScrape.fields.urls?.issues() || [] as issue (issue.message)}
					<p class="mt-1 flex items-center gap-1.5 pl-1 text-xs text-destructive">
						<AlertCircle class="size-3" />
						{issue.message}
					</p>
				{/each}
				<p class="pl-1 text-[11px] text-muted-foreground">
					Async via Redis + Asynq. Returns a <span class="font-mono">batch_id</span> — poll status below.
					For sync multi-URL (max 10, no Redis) use Scrape tab with advanced batch.
				</p>
			</div>
			<div class="flex justify-end">
				<Button type="submit" disabled={!!batchScrape.pending} class="h-11 px-8 shadow-md">
					{#if batchScrape.pending}
						<Loader2 class="mr-2 size-4 animate-spin" /> Enqueueing...
					{:else}
						<Package class="mr-2 size-4" /> Start Batch
					{/if}
				</Button>
			</div>
		</form>
	</div>
	<div
		class="flex items-center justify-between border-t bg-muted/10 p-4 px-6 text-[11px] font-medium text-muted-foreground"
	>
		<span class="flex items-center gap-1.5"
			><Badge variant="outline" class="h-5 rounded px-1.5 text-[9px] uppercase">Queue</Badge> Asynq /
			Redis — 202 Accepted</span
		>
		<span>Requires Redis</span>
	</div>
</section>

{#if batchScrape.pending}
	<div
		class="flex flex-col items-center justify-center rounded-xl border border-dashed bg-muted/20 p-12"
	>
		<Loader2 class="mb-3 size-8 animate-spin text-primary" />
		<p class="text-sm font-medium">Enqueueing batch...</p>
	</div>
{:else if batchError}
	<div
		class="flex gap-4 rounded-xl border border-destructive/20 bg-destructive/10 p-6 text-destructive"
	>
		<AlertCircle class="size-6 shrink-0" />
		<div>
			<h3 class="mb-1 font-bold">Batch Failed</h3>
			<p class="text-sm opacity-90">{batchError}</p>
		</div>
	</div>
{:else if batchId}
	<Card.Root class="overflow-hidden">
		<Card.Header class="pb-3">
			<Card.Title class="flex flex-wrap items-center gap-2 text-base">
				<Package class="size-4 text-primary" /> Batch
				<span class="font-mono text-xs font-normal text-muted-foreground">{batchId}</span>
				{#if status}
					<Badge variant={isDone ? 'default' : 'secondary'} class="ml-auto text-[10px]">
						{status.completed}/{status.total} done
						{#if status.failed > 0}· {status.failed} failed{/if}
					</Badge>
				{/if}
			</Card.Title>
			<Card.Description class="flex items-center gap-2 text-xs">
				<Clock class="size-3" /> Polling every 2.5s
				{#if status}
					<span class="ml-2 h-1.5 w-32 overflow-hidden rounded-full bg-muted">
						<span class="block h-full bg-primary transition-all" style="width: {progress}%"></span>
					</span>
					<span class="font-mono text-[11px]">{progress}%</span>
				{/if}
			</Card.Description>
		</Card.Header>
		<Card.Content>
			{#if !status}
				<p class="py-6 text-center text-sm text-muted-foreground">Waiting for status...</p>
			{:else}
				<div class="space-y-2">
					{#each status.tasks as task (task.id)}
						<div class="flex items-center justify-between rounded-lg border bg-card p-3">
							<div class="min-w-0 flex-1">
								<div class="truncate font-mono text-xs">{task.url}</div>
								<div class="truncate font-mono text-[10px] text-muted-foreground">
									{task.id.slice(0, 12)}…
								</div>
							</div>
							<Button
								variant="ghost"
								size="icon"
								class="size-7 shrink-0"
								href={task.url}
								target="_blank"
							>
								<ExternalLink class="size-3.5" />
							</Button>
						</div>
					{/each}
				</div>
				<div class="mt-4 flex flex-wrap gap-2">
					<Button variant="outline" size="sm" class="text-xs" onclick={copyTasks}>
						{#if copied}<Check class="mr-2 size-3" /> Copied{:else}<Copy class="mr-2 size-3" /> Copy URLs{/if}
					</Button>
					<Button
						variant="outline"
						size="sm"
						class="text-xs"
						onclick={() => statusQuery?.refresh()}
					>
						<Clock class="mr-2 size-3" /> Refresh Now
					</Button>
				</div>
			{/if}
		</Card.Content>
	</Card.Root>
{:else}
	<div
		class="flex flex-col items-center justify-center rounded-xl border border-dashed bg-muted/10 p-16 opacity-60"
	>
		<Package class="mb-3 size-10 stroke-[1px] text-muted-foreground" />
		<p class="text-sm font-medium">Batch scrape up to 20 URLs</p>
		<p class="mt-1 max-w-md text-center text-xs text-muted-foreground">
			Async — enqueues one Asynq task per URL. Poll <span class="font-mono">GET /v1/batch/:id</span> for
			aggregated status. Needs Redis.
		</p>
	</div>
{/if}
