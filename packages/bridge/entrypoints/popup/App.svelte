<script lang="ts">
  import { onMount } from 'svelte';
  import { browser } from 'wxt/browser';
  import { DEFAULT_PORT, type BridgeState } from '../../utils/protocol';

  const EMPTY: BridgeState = {
    connected: false,
    paired: false,
    paused: false,
    lastError: '',
    port: DEFAULT_PORT,
    allowed: [],
    lastRun: null,
  };

  let state = $state<BridgeState>({ ...EMPTY });
  let codeEl = $state<HTMLInputElement>();
  let portEl = $state<HTMLInputElement>();
  let localError = $state('');

  const dot = $derived(!state.connected ? 'off' : state.paired ? 'on' : 'pair');
  const headline = $derived(
    !state.connected
      ? 'Not connected — is Tomoshibi running?'
      : state.paired
        ? 'Connected and paired.'
        : 'Connected, not paired.',
  );
  const showPair = $derived(state.connected && !state.paired);
  const error = $derived(localError || state.lastError);

  async function refresh() {
    try {
      const next = (await browser.runtime.sendMessage({ kind: 'status' })) as BridgeState | undefined;
      if (next) state = { ...EMPTY, ...next };
      if (portEl && document.activeElement !== portEl) portEl.value = String(state.port || DEFAULT_PORT);
    } catch {
      /* the worker is asleep; the next broadcast wakes the popup */
    }
  }

  async function pair() {
    // Read the element rather than a bound copy: a paste, an autofill or a
    // scripted click must send what is actually in the box.
    const code = (codeEl?.value || '').trim();
    if (!/^\d{6}$/.test(code)) {
      localError = 'The code is the 6 digits from `tomoshibi bridge code`.';
      return;
    }
    localError = '';
    await browser.runtime.sendMessage({ kind: 'pair', code });
    setTimeout(refresh, 400);
  }

  async function togglePause(event: Event) {
    const on = (event.currentTarget as HTMLInputElement).checked;
    await browser.runtime.sendMessage({ kind: 'pause', on });
  }

  async function savePort() {
    const port = Number(portEl?.value) || DEFAULT_PORT;
    await browser.runtime.sendMessage({ kind: 'port', port });
    setTimeout(refresh, 600);
  }

  async function forget() {
    await browser.runtime.sendMessage({ kind: 'forget' });
    setTimeout(refresh, 600);
  }

  onMount(() => {
    refresh();
    const listener = (msg: { kind?: string; connected?: boolean }) => {
      if (msg?.kind === 'state') refresh();
    };
    browser.runtime.onMessage.addListener(listener as never);
    return () => browser.runtime.onMessage.removeListener(listener as never);
  });

  function ms(n: number | undefined) {
    if (!n) return '—';
    return n < 1000 ? `${n} ms` : `${(n / 1000).toFixed(1)} s`;
  }
</script>

<main>
  <h1><span id="dot" class="dot {dot}"></span> Tomoshibi Bridge</h1>
  <p id="state" class="muted">{headline}</p>

  <div id="pairBox" hidden={!showPair}>
    <p class="muted small">
      Enter the pairing code from Tomoshibi (run <code>tomoshibi bridge code</code>).
    </p>
    <input id="code" bind:this={codeEl} inputmode="numeric" maxlength="6" placeholder="6-digit code" />
    <button id="pairBtn" onclick={pair}>Pair this browser</button>
  </div>

  <label class="row">
    <input id="pause" type="checkbox" checked={state.paused} onchange={togglePause} />
    <span>Pause (stop answering fetches)</span>
  </label>

  {#if state.paired && state.allowed && state.allowed.length}
    <p class="muted small">
      Allowed origins: {state.allowed.slice(0, 4).join(', ')}{state.allowed.length > 4
        ? ` +${state.allowed.length - 4}`
        : ''}
    </p>
  {/if}

  {#if state.lastRun}
    <p class="muted small">
      Last run: {state.lastRun.origin} · {state.lastRun.actions} actions · {ms(state.lastRun.ms)}
    </p>
  {/if}

  <hr />

  <p class="muted small">Tomoshibi port</p>
  <input id="port" bind:this={portEl} inputmode="numeric" value={DEFAULT_PORT} />
  <div class="split">
    <button id="savePort" onclick={savePort}>Save port</button>
    <button id="forgetBtn" onclick={forget}>Forget</button>
  </div>
  <p class="muted small">
    Pair again after Tomoshibi's state was deleted or it was reinstalled.
  </p>
  <p id="err" class="err">{error}</p>
</main>

<style>
  :global(:root) {
    color-scheme: light dark;
  }
  :global(body) {
    font: 13px/1.45 system-ui, sans-serif;
    margin: 0;
  }
  main {
    width: 320px;
    padding: 12px;
    box-sizing: border-box;
  }
  h1 {
    font-size: 13px;
    margin: 0 0 8px;
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: #9ca3af;
    flex: none;
  }
  .dot.on {
    background: #16a34a;
  }
  .dot.off {
    background: #dc2626;
  }
  .dot.pair {
    background: #d97706;
  }
  p {
    margin: 6px 0;
  }
  .muted {
    opacity: 0.72;
  }
  .small {
    font-size: 11.5px;
  }
  .err {
    color: #dc2626;
    min-height: 1em;
  }
  input:not([type='checkbox']) {
    width: 100%;
    box-sizing: border-box;
    padding: 6px;
    margin: 4px 0;
    font: inherit;
    border-radius: 4px;
    border: 1px solid rgba(128, 128, 128, 0.45);
    background: transparent;
    color: inherit;
  }
  button {
    padding: 6px;
    font: inherit;
    cursor: pointer;
    margin: 4px 0;
    border-radius: 4px;
    border: 1px solid rgba(128, 128, 128, 0.45);
    background: rgba(128, 128, 128, 0.12);
    color: inherit;
  }
  button:hover {
    background: rgba(128, 128, 128, 0.22);
  }
  .split {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 6px;
  }
  .row {
    display: flex;
    gap: 6px;
    align-items: center;
    margin-top: 8px;
  }
  hr {
    border: none;
    border-top: 1px solid rgba(128, 128, 128, 0.3);
    margin: 10px 0;
  }
  code {
    font-size: 11.5px;
  }
</style>
