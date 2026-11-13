import { DEFAULT_PORT, api } from './lib.js';

const $ = (id) => document.getElementById(id);
const dot = $('dot');
const state = $('state');

function render(s) {
  dot.className = 'dot ' + (!s.connected ? 'off' : s.paired ? 'on' : 'pair');
  state.textContent = !s.connected
    ? 'Not connected — is Tomoshibi running?'
    : s.paired
      ? 'Connected and paired.'
      : 'Connected, not paired.';
  $('pairBox').hidden = !s.connected || s.paired;
  $('pause').checked = !!s.paused;
  $('err').textContent = s.lastError || '';
  if (document.activeElement !== $('port')) $('port').value = s.port || DEFAULT_PORT;
}

async function refresh() {
  const s = await api.runtime.sendMessage({ kind: 'status' });
  if (s) render(s);
}

api.runtime.onMessage.addListener((msg) => {
  if (msg.kind === 'state') render(msg);
});

$('pairBtn').addEventListener('click', async () => {
  const code = $('code').value.trim();
  if (!/^\d{6}$/.test(code)) {
    $('err').textContent = 'The code is the 6 digits from `tomoshibi bridge code`.';
    return;
  }
  await api.runtime.sendMessage({ kind: 'pair', code });
  setTimeout(refresh, 400);
});

$('pause').addEventListener('change', async (e) => {
  await api.runtime.sendMessage({ kind: 'pause', on: e.target.checked });
});

$('savePort').addEventListener('click', async () => {
  await api.runtime.sendMessage({ kind: 'port', port: Number($('port').value) || DEFAULT_PORT });
  setTimeout(refresh, 600);
});

$('forgetBtn').addEventListener('click', async () => {
  await api.runtime.sendMessage({ kind: 'forget' });
  setTimeout(refresh, 600);
});

refresh();
