<script lang="ts">
	import {
		Monitor,
		Clock,
		Trash2,
		Loader2,
		AlertCircle,
		Copy,
		Check,
		Eye,
		ExternalLink
	} from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Badge } from '$lib/components/ui/badge';
	import * as Card from '$lib/components/ui/card';
	import { createMonitor, getMonitorStatus, deleteMonitor } from '$lib/remote/cinder.remote';

	let {
		monitorError = $bindable(),
		selectedHistoryItem = $bindable(),
		monitorOptions,
		addToHistory
	} = $props<{
		monitorError: string | null;
		selectedHistoryItem: any;
		monitorOptions: { current: Record<string, any> };
		addToHistory: (item: any) => Promise<void>;
	}>();

	let monitorId = $state<string | null>(null);
	let monitorStatus = $state<any>(null);
	let statusQuery = $derived(monitorId ? getMonitorStatus(monitorId) : null);
	let deleting = $state(false);
	let copied = $state(false);

	$effect(() => {
		if (monitorId && statusQuery) {
			monitorStatus = statusQuery.current;
			const interval = setInterval(async () => {
				await statusQuery.refresh();
				monitorStatus = statusQuery.current;
			}, 5000);
			return () => clearInterval(interval);
		}
	});

	async function handleDelete() {
		if (!monitorId) return;
		deleting = true;
		try {
			await deleteMonitor(monitorId);
			monitorStatus = null;
			monitorId = null;
		} catch (e: any) {
			monitorError = e.body?.message || e.message || 'Delete failed';
		} finally {
			deleting = false;
		}
	}

	function copyId() {
		if (!monitorId) return;
		navigator.clipboard.writeText(monitorId);
		copied = true;
		setTimeout(() => (copied = false), 1500);
	}
</script>

<section class="overflow-hidden rounded-xl border bg-card/50 shadow-sm backdrop-blur-sm">
	<div class="p-6">
		<form
			{...createMonitor.enhance(async ({ submit }) => {
				monitorError = null;
				selectedHistoryItem = null;
				try {
					await submit();
					const r = createMonitor.result as any;
					if (r?.id) {
						monitorId = r.id;
						monitorStatus = null;
						addToHistory({
							id: crypto.randomUUID(),
							type: 'monitor',
							title: r.url || monitorOptions.current.url || 'Monitor',
							url: r.url || '',
							timestamp: new Date().toISOString(),
							preview: `Interval ${r.interval_seconds}s · next ${r.next_check}`,
							meta: { interval: r.interval_seconds },
							data: r
						});
					}
				} catch (e: any) {
					monitorError = e.body?.message || e.message || 'Monitor creation failed';
				}
			})}
			class="space-y-4"
		>
			<div class="grid gap-4 md:grid-cols-[1fr_180px]">
				<div class="flex flex-col gap-2">
					<Label
						for="monitor-url"
						class="pl-1 text-xs font-semibold tracking-wider text-muted-foreground uppercase"
						>URL to Watch</Label
					>
					<Input
						id="monitor-url"
						{...createMonitor.fields.url.as('text')}
						placeholder="https://example.com"
						class="h-11 bg-background"
						aria-invalid={(createMonitor.fields.url?.issues()?.length || 0) > 0}
					/>
					{#each createMonitor.fields.url?.issues() || [] as issue (issue.message)}
						<p class="flex items-center gap-1.5 pl-1 text-xs text-destructive">
							<AlertCircle class="size-3" />
							{issue.message}
						</p>
					{/each}
				</div>
				<div class="flex flex-col gap-2">
					<Label
						for="monitor-interval"
						class="pl-1 text-xs font-semibold tracking-wider text-muted-foreground uppercase"
						>Interval (seconds, ≥3600)</Label
					>
					<Input
						id="monitor-interval"
						{...createMonitor.fields.interval_seconds.as('number')}
						type="number"
						min="3600"
						placeholder="3600"
						class="h-11 bg-background"
					/>
				</div>
			</div>

			<div class="grid gap-4 md:grid-cols-2">
				<div class="flex flex-col gap-2">
					<Label
						for="monitor-webhook"
						class="pl-1 text-xs font-semibold tracking-wider text-muted-foreground uppercase"
						>Webhook URL (optional)</Label
					>
					<Input
						id="monitor-webhook"
						{...createMonitor.fields.webhook_url.as('text')}
						placeholder="https://myapp.example.com/hooks/cinder"
						class="h-10 bg-background text-xs"
					/>
				</div>
				<div class="flex flex-col gap-2">
					<Label
						for="monitor-secret"
						class="pl-1 text-xs font-semibold tracking-wider text-muted-foreground uppercase"
						>Webhook Secret (HMAC-SHA256)</Label
					>
					<Input
						id="monitor-secret"
						{...createMonitor.fields.webhook_secret.as('text')}
						placeholder="s3cret"
						class="h-10 bg-background text-xs"
						type="password"
					/>
				</div>
			</div>

			<p class="pl-1 text-[11px] text-muted-foreground">
				Scrapes on interval, hashes markdown, fires signed <span class="font-mono"
					>X-Cinder-Signature</span
				> webhook on change. First check records baseline. Requires Redis.
			</p>

			<div class="flex justify-end">
				<Button type="submit" disabled={!!createMonitor.pending} class="h-11 px-8 shadow-md">
					{#if createMonitor.pending}
						<Loader2 class="mr-2 size-4 animate-spin" /> Creating...
					{:else}
						<Monitor class="mr-2 size-4" /> Create Monitor
					{/if}
				</Button>
			</div>
		</form>
	</div>
	<div
		class="flex items-center justify-between border-t bg-muted/10 p-4 px-6 text-[11px] font-medium text-muted-foreground"
	>
		<span class="flex items-center gap-1.5"
			><Badge variant="outline" class="h-5 rounded px-1.5 text-[9px] uppercase"
				>Change Tracking</Badge
			> Hash + Webhook</span
		>
		<span>Polls every 5s</span>
	</div>
</section>

{#if createMonitor.pending}
	<div
		class="flex flex-col items-center justify-center rounded-xl border border-dashed bg-muted/20 p-12"
	>
		<Loader2 class="mb-3 size-8 animate-spin text-primary" />
		<p class="text-sm font-medium">Creating monitor...</p>
	</div>
{:else if monitorError}
	<div
		class="flex gap-4 rounded-xl border border-destructive/20 bg-destructive/10 p-6 text-destructive"
	>
		<AlertCircle class="size-6 shrink-0" />
		<div>
			<h3 class="mb-1 font-bold">Monitor Error</h3>
			<p class="text-sm opacity-90">{monitorError}</p>
		</div>
	</div>
{:else if monitorId}
	<Card.Root>
		<Card.Header class="pb-3">
			<Card.Title class="flex flex-wrap items-center gap-2 text-base">
				<Monitor class="size-4 text-primary" /> Monitor
				<span class="max-w-48 truncate font-mono text-xs font-normal text-muted-foreground"
					>{monitorId}</span
				>
				<Button variant="ghost" size="icon" class="size-7" onclick={copyId} title="Copy monitor ID">
					{#if copied}<Check class="size-3.5 text-emerald-500" />{:else}<Copy
							class="size-3.5"
						/>{/if}
				</Button>
				<Badge variant="secondary" class="ml-auto gap-1.5 text-[10px]"
					><Clock class="size-3" /> Active</Badge
				>
			</Card.Title>
			<Card.Description class="flex flex-wrap items-center gap-2 text-xs">
				{#if monitorStatus}
					<span
						>Next check: <span class="font-mono text-foreground">{monitorStatus.next_check}</span
						></span
					>
					{#if monitorStatus.last_hash}
						<Badge variant="outline" class="font-mono text-[10px]"
							>hash {monitorStatus.last_hash.slice(0, 12)}…</Badge
						>
					{:else}
						<Badge variant="outline" class="text-[10px]">baseline pending</Badge>
					{/if}
				{:else}
					<span class="text-muted-foreground">Loading status...</span>
				{/if}
			</Card.Description>
		</Card.Header>
		<Card.Content class="space-y-4">
			{#if monitorStatus}
				<div class="grid gap-3 rounded-lg border bg-muted/30 p-4 text-xs">
					<div class="flex justify-between">
						<span class="text-muted-foreground">URL</span><a
							href={monitorStatus.url}
							target="_blank"
							class="flex items-center gap-1 font-mono text-primary hover:underline"
							>{monitorStatus.url} <ExternalLink class="size-3" /></a
						>
					</div>
					<div class="flex justify-between">
						<span class="text-muted-foreground">Interval</span><span class="font-mono"
							>{monitorStatus.interval_seconds}s</span
						>
					</div>
					{#if monitorStatus.webhook_url}<div class="flex justify-between">
							<span class="text-muted-foreground">Webhook</span><span
								class="max-w-64 truncate font-mono">{monitorStatus.webhook_url}</span
							>
						</div>{/if}
					<div class="flex justify-between">
						<span class="text-muted-foreground">Next Check</span><span class="font-mono"
							>{monitorStatus.next_check}</span
						>
					</div>
					{#if monitorStatus.last_hash}<div class="flex justify-between">
							<span class="text-muted-foreground">Last Hash</span><span
								class="max-w-64 truncate font-mono text-[11px]">{monitorStatus.last_hash}</span
							>
						</div>{/if}
				</div>
				<div class="flex flex-wrap gap-2">
					<Button variant="outline" size="sm" class="text-xs" onclick={() => statusQuery?.refresh()}
						><Eye class="mr-2 size-3" /> Refresh</Button
					>
					<Button
						variant="destructive"
						size="sm"
						class="text-xs"
						onclick={handleDelete}
						disabled={deleting}
					>
						{#if deleting}<Loader2 class="mr-2 size-3 animate-spin" /> Deleting...{:else}<Trash2
								class="mr-2 size-3"
							/> Delete Monitor{/if}
					</Button>
				</div>
			{:else}
				<p class="py-4 text-center text-sm text-muted-foreground">Fetching monitor status...</p>
			{/if}
		</Card.Content>
	</Card.Root>
{:else}
	<div
		class="flex flex-col items-center justify-center rounded-xl border border-dashed bg-muted/10 p-16 opacity-60"
	>
		<Monitor class="mb-3 size-10 stroke-[1px] text-muted-foreground" />
		<p class="text-sm font-medium">Change-tracking monitors</p>
		<p class="mt-1 max-w-md text-center text-xs text-muted-foreground">
			Create a monitor to hash markdown on interval and fire a signed webhook when content changes.
			Baseline recorded on first check.
		</p>
	</div>
{/if}
