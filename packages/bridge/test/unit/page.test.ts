import { beforeEach, describe, expect, test } from 'bun:test';
import {
  pageMotion,
  pageTreeDom,
  readPage,
  scrollAndCollect,
  settlePage,
} from '../../utils/page';

// A stub animation in the exact shape the settle predicate inspects.
function animation(iterations: number, playState = 'running') {
  return {
    playState,
    effect: {
      getComputedTiming: () => ({ iterations, duration: 1000 }),
      target: document.body,
    },
  } as unknown as Animation;
}

function stubAnimations(list: Animation[]) {
  (document as unknown as { getAnimations: () => Animation[] }).getAnimations = () => list;
}

beforeEach(() => {
  document.body.innerHTML = '<h1>Hello</h1><p>Some text on the page.</p>';
  delete (document as unknown as { getAnimations?: unknown }).getAnimations;
});

describe('settlePage', () => {
  test('a quiet page settles', async () => {
    const r = await settlePage({ quiet_ms: 30, max_ms: 500, poll_ms: 10 });
    expect(r.settled).toBe(true);
    expect(r.reason).toBe('stable');
    expect(r.ms).toBeLessThan(500);
  });

  // The single most important line in the whole extraction upgrade: every page
  // has a spinner, a pulse or a skeleton, and all of them are infinite. A settle
  // that waits for "no running animations" without excluding those never returns
  // on a real page — so this asserts the exclusion, not just the happy path.
  test('an infinite ambient animation does not block settling', async () => {
    stubAnimations([animation(Infinity)]);
    const r = await settlePage({ quiet_ms: 30, max_ms: 500, poll_ms: 10 });
    expect(r.settled).toBe(true);
  });

  // The same exclusion must not become a blanket ignore: a finite animation is
  // real motion, and reading through it is how a carousel ends up half-rendered.
  test('a finite animation keeps the page unsettled', async () => {
    stubAnimations([animation(1)]);
    const r = await settlePage({ quiet_ms: 20, max_ms: 150, poll_ms: 10 });
    expect(r.settled).toBe(false);
    expect(r.reason).toBe('timeout');
  });

  test('a page that keeps mutating times out rather than lying', async () => {
    const timer = setInterval(() => {
      document.body.appendChild(document.createElement('span'));
    }, 10);
    try {
      const r = await settlePage({ quiet_ms: 60, max_ms: 140, poll_ms: 10 });
      expect(r.settled).toBe(false);
      expect(r.reason).toBe('timeout');
    } finally {
      clearInterval(timer);
    }
  });
});

describe('scrollAndCollect', () => {
  test('reports each round and stops when the page stops growing', async () => {
    let scrolls = 0;
    const original = window.scrollBy;
    (window as unknown as { scrollBy: (x: number, y: number) => void }).scrollBy = () => {
      scrolls += 1;
      // Three rounds of lazy content, then nothing: exactly the shape of an
      // infinite feed that has reached its end.
      if (scrolls <= 3) {
        const p = document.createElement('p');
        p.textContent = 'another item '.repeat(10);
        document.body.appendChild(p);
      }
    };
    try {
      const out = await scrollAndCollect({ max_rounds: 10, quiet_rounds: 2, pause_ms: 5 });
      const grew = out.rounds.filter((r) => r.grew).length;
      expect(out.rounds.length).toBeGreaterThanOrEqual(4);
      expect(out.rounds.length).toBeLessThanOrEqual(6);
      expect(grew).toBeGreaterThan(0);
      expect(out.text_len).toBeGreaterThan(0);
    } finally {
      (window as unknown as { scrollBy: typeof original }).scrollBy = original;
    }
  });

  test('counts items by selector when one is given', async () => {
    const list = document.createElement('div');
    list.innerHTML = '<div class="card">a</div><div class="card">b</div>';
    document.body.appendChild(list);
    const out = await scrollAndCollect({
      max_rounds: 2,
      quiet_rounds: 1,
      pause_ms: 5,
      item_selector: '.card',
    });
    expect(out.items).toBe(2);
  });
});

describe('pageTreeDom', () => {
  test('returns roles, paths and text for the readable structure', () => {
    document.body.innerHTML = `
      <nav><a href="/x">Home</a></nav>
      <main><h1>Title</h1><p>Body copy here.</p></main>`;

    const tree = pageTreeDom({ max_nodes: 100, max_depth: 8 }) as any;
    expect(tree).toBeTruthy();

    const seen: any[] = [];
    const walk = (n: any) => {
      if (!n) return;
      seen.push(n);
      (n.children || []).forEach(walk);
    };
    walk(tree);

    const roles = seen.map((n) => n.role);
    expect(roles).toContain('navigation');
    expect(roles).toContain('main');
    expect(roles).toContain('heading');
    expect(roles).toContain('link');
    expect(seen.find((n) => n.role === 'heading')?.text).toContain('Title');
    expect(seen.every((n) => typeof n.visible === 'boolean')).toBe(true);
    expect(seen.every((n) => typeof n.path === 'string' && n.path.length > 0)).toBe(true);
  });

  test('respects max_nodes', () => {
    document.body.innerHTML = Array.from({ length: 50 }, (_, i) => `<p>para ${i}</p>`).join('');
    const tree = pageTreeDom({ max_nodes: 5, max_depth: 8 }) as any;
    let count = 0;
    const walk = (n: any) => {
      if (!n) return;
      count++;
      (n.children || []).forEach(walk);
    };
    walk(tree);
    expect(count).toBeLessThanOrEqual(5);
  });
});

describe('pageMotion', () => {
  test('reports pinned elements and flags infinite animations', () => {
    document.body.innerHTML = '<div style="position: fixed; top: 0; width: 100px; height: 20px">bar</div>';
    stubAnimations([animation(Infinity), animation(1)]);

    const m = pageMotion() as any;
    expect(m.pinned.length).toBeGreaterThan(0);
    expect(m.pinned[0].position).toBe('fixed');
    expect(m.animations.length).toBe(2);
    expect(m.animations.filter((a: any) => a.infinite).length).toBe(1);
    expect(m.animations.find((a: any) => !a.infinite)?.duration_ms).toBe(1000);
  });
});

describe('readPage', () => {
  test('returns the text and the caps it was given', () => {
    const page = readPage(50, 10);
    expect(page.text.length).toBeLessThanOrEqual(10);
    expect(page.html.length).toBeLessThanOrEqual(50);
    expect(page.truncated).toBe(true);
  });
});
