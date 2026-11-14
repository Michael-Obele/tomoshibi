// Functions injected into the page.
//
// These are serialised by `scripting.executeScript` (and by
// `Runtime.evaluate` in the turbo layer), so each one MUST be self-contained:
// no imports, no closures over module scope, no helper calls. Anything shared
// arrives as an argument. They are exported so unit tests can run them
// directly against a DOM.

/** Read the page the way the server expects it. Same fields and same caps as
 * the turbo layer's expression, so the two layers cannot disagree. */
export function readPage(htmlLimit: number, textLimit: number) {
  const html = document.documentElement ? document.documentElement.outerHTML : '';
  // The navigation's HTTP status when the browser will report it (same-origin
  // navigations do; cross-origin ones may not), 0 otherwise.
  const nav = (performance.getEntriesByType('navigation') || [])[0] as
    | PerformanceNavigationTiming
    | undefined;
  const body = document.body as (HTMLElement & { innerText?: string }) | null;
  // innerText is what a reader sees (it respects CSS); textContent is the
  // fallback for a document that does not implement it.
  const text = body ? (typeof body.innerText === 'string' ? body.innerText : body.textContent || '') : '';
  return {
    url: location.href,
    title: document.title,
    text: text.slice(0, textLimit),
    html: html.slice(0, htmlLimit),
    truncated: html.length > htmlLimit,
    status: (nav && (nav as unknown as { responseStatus?: number }).responseStatus) || 0,
  };
}

export function clickBySelector(selector: string) {
  const el = document.querySelector(selector);
  if (!el) throw new Error(`click: no element for ${selector}`);
  el.scrollIntoView({ block: 'center', inline: 'center' });
  const rect = el.getBoundingClientRect();
  const x = rect.left + rect.width / 2;
  const y = rect.top + rect.height / 2;
  const opts = { bubbles: true, cancelable: true, view: window, clientX: x, clientY: y, button: 0 };
  el.dispatchEvent(new PointerEvent('pointerdown', opts));
  el.dispatchEvent(new MouseEvent('mousedown', opts));
  el.dispatchEvent(new PointerEvent('pointerup', opts));
  el.dispatchEvent(new MouseEvent('mouseup', opts));
  el.dispatchEvent(new MouseEvent('click', opts));
}

export function typeIntoSelector(selector: string, text: string) {
  const el = document.querySelector(selector) as HTMLInputElement | HTMLTextAreaElement | null;
  if (!el) throw new Error(`type: no element for ${selector}`);
  el.focus();
  const proto =
    el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, 'value')?.set;
  if (setter) setter.call(el, text);
  else el.value = text;
  el.dispatchEvent(new Event('input', { bubbles: true }));
  el.dispatchEvent(new Event('change', { bubbles: true }));
}

export function readFieldAttrs(selector: string) {
  const el = document.querySelector(selector) as HTMLInputElement | null;
  if (!el) return { missing: true };
  return {
    tag: el.tagName,
    type: el.getAttribute('type'),
    name: el.getAttribute('name') || el.name,
    id: el.id,
    autocomplete: el.getAttribute('autocomplete'),
  };
}

export function pressKey(key: string) {
  const el = (document.activeElement || document.body) as HTMLElement;
  const opts = { key, code: key, bubbles: true, cancelable: true };
  const down = new KeyboardEvent('keydown', opts);
  const notPrevented = el.dispatchEvent(down);
  el.dispatchEvent(new KeyboardEvent('keyup', opts));
  // Synthetic events do not run browser defaults, so Enter's submit is done by
  // hand — and only when a real Enter would have submitted: a form with a
  // submit control, or exactly one field. Requesting submit unconditionally
  // would submit multi-field forms a human's Enter would not.
  if (key !== 'Enter' || !notPrevented) return;
  const form = (el.closest && el.closest('form')) as HTMLFormElement | null;
  if (!form || typeof form.requestSubmit !== 'function') return;
  const hasSubmit = !!form.querySelector('input[type=submit], button[type=submit], button:not([type])');
  const singleField = form.elements.length === 1;
  if (hasSubmit || singleField) form.requestSubmit();
}

/**
 * settlePage waits until the page stops moving, and reports how long it took.
 *
 * This replaces a fixed sleep with a real signal. Three conditions must hold at
 * once: no DOM mutation for `quiet_ms`, an unchanged scroll height and element
 * count, and no *finite* animation still running.
 *
 * The `iterations === Infinity` exclusion is the whole trick. Every spinner,
 * pulse and skeleton loader is an infinite animation; waiting for "zero running
 * animations" without it never returns on a page that has one, which is most
 * pages.
 */
export function settlePage(opts: {
  quiet_ms?: number;
  max_ms?: number;
  poll_ms?: number;
}): Promise<{ settled: boolean; reason: string; ms: number }> {
  const quietMs = opts.quiet_ms ?? 500;
  const maxMs = opts.max_ms ?? 10000;
  const pollMs = opts.poll_ms ?? 120;
  const started = performance.now();

  return new Promise((resolve) => {
    let lastMutation = performance.now();
    let lastHeight = document.documentElement ? document.documentElement.scrollHeight : 0;
    let lastNodes = document.getElementsByTagName('*').length;

    const finish = (settled: boolean, reason: string) => {
      try {
        observer.disconnect();
      } catch {
        /* detached */
      }
      resolve({ settled, reason, ms: Math.round(performance.now() - started) });
    };

    const observer = new MutationObserver(() => {
      lastMutation = performance.now();
    });
    try {
      observer.observe(document.documentElement || document, {
        subtree: true,
        childList: true,
        attributes: true,
        characterData: true,
      });
    } catch {
      /* document not observable; the other two signals still work */
    }

    const tick = () => {
      const now = performance.now();
      if (now - started >= maxMs) return finish(false, 'timeout');

      const height = document.documentElement ? document.documentElement.scrollHeight : 0;
      const nodes = document.getElementsByTagName('*').length;

      let animating = 0;
      if (typeof document.getAnimations === 'function') {
        try {
          for (const a of document.getAnimations()) {
            if (!a || a.playState !== 'running') continue;
            const timing = a.effect && a.effect.getComputedTiming ? a.effect.getComputedTiming() : null;
            // Infinity is ambient: a spinner that never stops is not motion
            // this page will finish.
            if (timing && timing.iterations === Infinity) continue;
            animating++;
          }
        } catch {
          animating = 0;
        }
      }

      const quiet = now - lastMutation >= quietMs;
      const stable = height === lastHeight && nodes === lastNodes;
      lastHeight = height;
      lastNodes = nodes;

      if (quiet && stable && animating === 0) return finish(true, 'stable');
      setTimeout(tick, pollMs);
    };

    setTimeout(tick, pollMs);
  });
}

/**
 * scrollAndCollect walks a page that grows as you scroll (an infinite feed, a
 * lazily-hydrated list) and reports what each round added.
 *
 * Growth is measured two ways: rendered text length, and the count of
 * `item_selector` matches when the caller supplies one. The selector is the
 * precise signal; text length is the fallback for feeds that recycle nodes.
 *
 * ponytail: this stops after `quiet_rounds` rounds with no growth, so a feed
 * that loads one item every four rounds is under-collected. Raise quiet_rounds
 * if that shows up; it is not worth a smarter heuristic yet.
 */
export async function scrollAndCollect(opts: {
  max_rounds?: number;
  quiet_rounds?: number;
  step_px?: number;
  pause_ms?: number;
  item_selector?: string;
}): Promise<{
  rounds: Array<{ round: number; scroll_y: number; text_len: number; items: number; grew: boolean }>;
  at_bottom: boolean;
  text_len: number;
  items: number;
}> {
  const maxRounds = opts.max_rounds ?? 8;
  const quietRounds = Math.max(1, opts.quiet_rounds ?? 3);
  const pauseMs = opts.pause_ms ?? 350;
  const itemSelector = opts.item_selector || '';

  const textLen = () => {
    const body = document.body as (HTMLElement & { innerText?: string }) | null;
    if (!body) return 0;
    return (typeof body.innerText === 'string' ? body.innerText : body.textContent || '').length;
  };
  const items = () => (itemSelector ? document.querySelectorAll(itemSelector).length : 0);

  const rounds: Array<{
    round: number;
    scroll_y: number;
    text_len: number;
    items: number;
    grew: boolean;
  }> = [];
  let lastText = textLen();
  let lastItems = items();
  let barren = 0;

  for (let i = 0; i < maxRounds; i++) {
    const step = opts.step_px || Math.max(200, Math.floor(window.innerHeight * 0.9));
    window.scrollBy(0, step);
    await new Promise((r) => setTimeout(r, pauseMs));

    const t = textLen();
    const n = items();
    const grew = t > lastText || n > lastItems;
    rounds.push({ round: i + 1, scroll_y: Math.round(window.scrollY), text_len: t, items: n, grew });

    if (grew) {
      barren = 0;
      lastText = t;
      lastItems = n;
    } else if (++barren >= quietRounds) {
      break;
    }
  }

  const doc = document.documentElement;
  const atBottom = !!doc && Math.ceil(window.scrollY + window.innerHeight) >= doc.scrollHeight - 4;
  // Back to the top: the text already carries everything, and the top is the
  // canonical view for a screenshot taken after this.
  window.scrollTo(0, 0);

  return { rounds, at_bottom: atBottom, text_len: lastText, items: lastItems };
}

/**
 * pageTreeDom returns a pruned outline of the rendered page: the block-level
 * skeleton, each node's accessible-ish role, a short selector path, its box and
 * whether it is actually visible.
 *
 * This is "the page tree" for the scripting layer, so it works on Firefox with
 * no debugger. Roles are inferred from the tag and `role=`; the fidelity of
 * `page_tree: "ax"` (turbo, real accessibility tree) is higher.
 */
export function pageTreeDom(opts: {
  max_nodes?: number;
  max_depth?: number;
  min_text_len?: number;
}): unknown {
  const maxNodes = opts.max_nodes ?? 1200;
  const maxDepth = opts.max_depth ?? 12;
  const minTextLen = opts.min_text_len ?? 0;

  const BLOCK = new Set([
    'DIV', 'SECTION', 'ARTICLE', 'MAIN', 'NAV', 'HEADER', 'FOOTER', 'ASIDE', 'FORM',
    'UL', 'OL', 'LI', 'TABLE', 'THEAD', 'TBODY', 'TR', 'TD', 'TH', 'DL', 'DT', 'DD',
    'FIGURE', 'FIGCAPTION', 'BLOCKQUOTE', 'PRE', 'FIELDSET', 'DETAILS', 'SUMMARY',
    'DIALOG', 'H1', 'H2', 'H3', 'H4', 'H5', 'H6', 'P', 'BUTTON', 'A', 'LABEL',
    'INPUT', 'SELECT', 'TEXTAREA', 'IMG', 'VIDEO', 'CANVAS', 'IFRAME', 'SVG',
  ]);

  const roleOf = (el: Element): string => {
    const explicit = el.getAttribute && el.getAttribute('role');
    if (explicit) return explicit;
    const tag = el.tagName;
    if (tag === 'A' && el.hasAttribute('href')) return 'link';
    if (tag === 'BUTTON') return 'button';
    if (/^H[1-6]$/.test(tag)) return 'heading';
    if (tag === 'NAV') return 'navigation';
    if (tag === 'MAIN') return 'main';
    if (tag === 'HEADER') return 'banner';
    if (tag === 'FOOTER') return 'contentinfo';
    if (tag === 'ASIDE') return 'complementary';
    if (tag === 'FORM') return 'form';
    if (tag === 'INPUT') {
      const t = String(el.getAttribute('type') || 'text').toLowerCase();
      if (t === 'search') return 'searchbox';
      if (t === 'submit' || t === 'button') return 'button';
      if (t === 'checkbox') return 'checkbox';
      return 'textbox';
    }
    if (tag === 'IMG') return 'img';
    if (tag === 'UL' || tag === 'OL') return 'list';
    if (tag === 'LI') return 'listitem';
    if (tag === 'TABLE') return 'table';
    if (tag === 'DETAILS') return 'group';
    if (tag === 'DIALOG') return 'dialog';
    return 'generic';
  };

  const pathOf = (el: Element): string => {
    const parts: string[] = [];
    let cur: Element | null = el;
    let hops = 0;
    while (cur && cur.nodeType === 1 && hops < 4) {
      const tag = cur.tagName.toLowerCase();
      if (cur.id) {
        parts.unshift(`${tag}#${cur.id}`);
        break;
      }
      const parent: Element | null = cur.parentElement;
      if (!parent) {
        parts.unshift(tag);
        break;
      }
      const sameTag = Array.from(parent.children).filter((c) => c.tagName === cur!.tagName);
      const idx = sameTag.indexOf(cur) + 1;
      parts.unshift(sameTag.length > 1 ? `${tag}:nth-of-type(${idx})` : tag);
      cur = parent;
      hops++;
    }
    return parts.join('>');
  };

  let count = 0;

  const walk = (el: Element, depth: number): unknown => {
    if (count >= maxNodes || depth > maxDepth) return null;
    const children: unknown[] = [];
    const rawText = (el.textContent || '').replace(/\s+/g, ' ').trim();
    const textLen = rawText.length;

    let rect: DOMRect | null = null;
    let visible = true;
    try {
      rect = el.getBoundingClientRect();
      const cs = getComputedStyle(el);
      visible =
        cs.display !== 'none' &&
        cs.visibility !== 'hidden' &&
        Number(cs.opacity) > 0 &&
        rect.width > 0 &&
        rect.height > 0;
    } catch {
      visible = true;
    }

    const keep = textLen >= minTextLen;
    if (keep) count++;

    for (const child of Array.from(el.children)) {
      if (!BLOCK.has(child.tagName)) {
        // A non-block wrapper is usually layout; descend without emitting a node
        // so the outline stays readable.
        const deep = walk(child, depth);
        if (deep && typeof deep === 'object' && 'children' in deep) {
          for (const g of (deep as { children: unknown[] }).children) children.push(g);
        }
        continue;
      }
      const node = walk(child, depth + 1);
      if (node) children.push(node);
    }

    if (!keep && children.length === 0) return null;

    const node: Record<string, unknown> = {
      tag: el.tagName.toLowerCase(),
      role: roleOf(el),
      path: pathOf(el),
      text_len: textLen,
      visible,
    };
    if (textLen > 0) node.text = rawText.slice(0, 200);
    if (rect && rect.width + rect.height > 0) {
      node.box = [
        Math.round(rect.left),
        Math.round(rect.top),
        Math.round(rect.width),
        Math.round(rect.height),
      ];
    }
    if (children.length) node.children = children;
    return node;
  };

  const root = document.body || document.documentElement;
  if (!root) return null;
  return walk(root, 0);
}

/**
 * pageMotion reports what is moving and what is pinned, which is the question
 * you must answer before parsing a page of carousels and sticky headers.
 *
 * ponytail: `getComputedStyle` on every element is the expensive part, so it is
 * capped. A page with more than a few thousand elements gets a partial pinned
 * list — the animation list is always complete.
 */
export function pageMotion(): unknown {
  const describe = (el: Element | null | undefined): string | null => {
    if (!el) return null;
    const tag = el.tagName.toLowerCase();
    if (el.id) return `${tag}#${el.id}`;
    const cls = String(el.getAttribute('class') || '').trim().split(/\s+/).filter(Boolean).slice(0, 2);
    return cls.length ? `${tag}.${cls.join('.')}` : tag;
  };

  const animations: Array<{
    play_state: string;
    infinite: boolean;
    duration_ms: number | null;
    target: string | null;
  }> = [];
  if (typeof document.getAnimations === 'function') {
    for (const a of document.getAnimations()) {
      const timing = a.effect && a.effect.getComputedTiming ? a.effect.getComputedTiming() : null;
      // `target` lives on the effect at runtime but is not in the DOM lib type.
      const target = ((a.effect as unknown as { target?: Element | null }) || null)?.target ?? null;
      animations.push({
        play_state: a.playState,
        infinite: !!(timing && timing.iterations === Infinity),
        duration_ms: timing && typeof timing.duration === 'number' ? Math.round(timing.duration) : null,
        target: describe(target),
      });
    }
  }

  const pinned: Array<{ path: string; position: string; box: [number, number, number, number] }> = [];
  let scrollContainer: string | null = null;
  let bestScrollable = 0;

  const all = document.getElementsByTagName('*');
  const cap = Math.min(all.length, 3000);
  for (let i = 0; i < cap; i++) {
    const el = all[i];
    if (!el) continue;
    let cs: CSSStyleDeclaration;
    try {
      cs = getComputedStyle(el);
    } catch {
      continue;
    }
    if ((cs.position === 'fixed' || cs.position === 'sticky') && pinned.length < 40) {
      const r = el.getBoundingClientRect();
      pinned.push({
        path: describe(el) || el.tagName.toLowerCase(),
        position: cs.position,
        box: [Math.round(r.left), Math.round(r.top), Math.round(r.width), Math.round(r.height)],
      });
    }
    const overflows =
      (cs.overflowY === 'auto' || cs.overflowY === 'scroll') &&
      el.scrollHeight > el.clientHeight + 32;
    if (overflows && el.scrollHeight > bestScrollable && el !== document.body) {
      bestScrollable = el.scrollHeight;
      scrollContainer = describe(el);
    }
  }

  return { animations, pinned, scroll_container: scrollContainer };
}
