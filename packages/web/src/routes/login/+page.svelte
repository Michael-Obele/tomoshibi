<script lang="ts">
	import { goto } from '$app/navigation';
	import { loginUser } from '$lib/remote/auth.remote';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Button } from '$lib/components/ui/button';
	import { Label } from '$lib/components/ui/label';
	import { Crown, Lock, Eye, EyeOff } from '@lucide/svelte';

	let form = loginUser;
	let isSubmitting = $state(false);
	let submitError = $state<string | null>(null);
	let showPassword = $state(false);

	$effect(() => {
		if (form.result?.success) {
			goto('/');
		}
	});
</script>

<div class="flex min-h-[80vh] w-full items-center justify-center p-4">
	<Card.Root
		class="w-full max-w-md border-amber-500/20 bg-slate-950/50 shadow-[0_0_40px_-10px_rgba(245,158,11,0.1)] backdrop-blur-sm transition-all duration-500 hover:border-amber-500/40 hover:shadow-[0_0_60px_-10px_rgba(245,158,11,0.2)]"
	>
		<Card.Header class="space-y-4 text-center">
			<div
				class="mx-auto flex h-16 w-16 items-center justify-center rounded-full bg-amber-500/10 ring-1 ring-amber-500/50 transition-transform duration-500 hover:scale-110 hover:bg-amber-500/20"
			>
				<Crown class="h-8 w-8 text-amber-500 drop-shadow-[0_0_8px_rgba(245,158,11,0.5)]" />
			</div>

			<div class="space-y-2">
				<Card.Title class="font-mono text-3xl font-bold tracking-tight text-white">
					Identify Yourself
				</Card.Title>
				<Card.Description class="text-base text-slate-400">
					Welcome <span
						class="bg-linear-to-r from-amber-500 to-orange-400 bg-clip-text font-bold text-transparent"
						>masta</span
					>
					. Enter your password to
					<span
						class="bg-linear-to-r from-amber-500 via-pink-500 to-orange-400 bg-clip-text font-bold text-transparent animation-duration-[500ms] hover:animate-pulse"
						>evlivate</span
					> yourself above the masses.
				</Card.Description>
			</div>
		</Card.Header>

		<Card.Content>
			<form
				{...form.enhance(async ({ form: formEl, submit }) => {
					isSubmitting = true;
					submitError = null;
					try {
						await submit();
					} catch (error) {
						submitError = error instanceof Error ? error.message : 'An error occurred';
					} finally {
						isSubmitting = false;
					}
				})}
				class="space-y-6"
			>
				<div class="space-y-2">
					<Label for="password" class="text-slate-200">Secret Key</Label>
					<div class="group relative">
						<Input
							{...form.fields.password.as('password')}
							id="password"
							type={showPassword ? 'text' : 'password'}
							placeholder="••••••••••••"
							class="border-slate-800 bg-slate-950/60 pr-10 pl-10 text-white transition-all group-hover:border-slate-700 focus-visible:border-amber-500 focus-visible:ring-amber-500/50"
						/>
						<Lock
							class="absolute top-2.5 left-3 h-4 w-4 text-slate-500 transition-colors group-hover:text-amber-500/70"
						/>
						<button
							type="button"
							onclick={() => (showPassword = !showPassword)}
							class="absolute top-2.5 right-3 text-slate-500 transition-colors hover:text-amber-500/70 focus:outline-none focus-visible:ring-2 focus-visible:ring-amber-500/50"
							aria-label={showPassword ? 'Hide password' : 'Show password'}
						>
							{#if showPassword}
								<EyeOff class="h-4 w-4" />
							{:else}
								<Eye class="h-4 w-4" />
							{/if}
						</button>
					</div>

					<Button
						type="submit"
						class="w-full bg-linear-to-r from-amber-600 to-orange-600 font-bold text-white shadow-[0_0_20px_-5px_rgba(245,158,11,0.4)] transition-all hover:scale-[1.02] hover:from-amber-500 hover:to-orange-500 hover:shadow-[0_0_30px_-5px_rgba(245,158,11,0.6)] active:scale-[0.98]"
						disabled={isSubmitting}
					>
						{#if isSubmitting}
							<div class="flex items-center gap-2">
								<div
									class="h-4 w-4 animate-spin rounded-full border-2 border-white/20 border-t-white"
								></div>
								<span>Verifying Status...</span>
							</div>
						{:else}
							Elevate
						{/if}
					</Button>

					{#if submitError}
						<div
							class="group animate-in rounded-md border border-red-500/20 bg-red-500/10 p-4 text-center text-sm text-red-400 slide-in-from-top-2"
						>
							<span class="font-bold group-hover:underline">Error:</span>
							{submitError}
						</div>
					{/if}
				</div>
			</form>
		</Card.Content>

		<Card.Footer class="justify-center border-t border-slate-800/50 pt-6">
			<p
				class="font-mono text-xs tracking-[0.2em] text-amber-600/75 uppercase transition-colors hover:text-amber-100"
			>
				Restricted Access Zone
			</p>
		</Card.Footer>
	</Card.Root>
</div>
