// Protocol v1, pinned on both halves of the seam.
//
// The Go side is `packages/api/internal/bridge/protocol.go` (`ProtocolVersion`).
// A mismatch is refused, not guessed, so bump both or neither.

export const PROTOCOL = 1;
export const DEFAULT_PORT = 7431;
export const BRIDGE_PATH = '/bridge';

// Built-in challenge hints. A page containing one of these stops the run as
// awaiting-human: the human clears the check in the open tab, then the fetch is
// retried. These are hints, not a solver; recipes may add their own via
// expect.absent (checked server-side).
export const BUILTIN_MARKERS = [
  'Just a moment...',
  'Enable JavaScript and cookies to continue',
  'Checking your browser before accessing',
  'Verifying you are human',
  'Attention Required!',
  'cf-challenge-running',
] as const;

/** The bridge's input layers. */
export type IO = 'scripting' | 'turbo';

/** Actions the server may name. The scripting layer refuses the last two. */
export type ActionType =
  | 'wait_ms'
  | 'wait_selector'
  | 'wait_for_function'
  | 'scroll_to_bottom'
  | 'scroll_by'
  | 'scroll_down'
  | 'click'
  | 'type'
  | 'press'
  | 'navigate'
  | 'evaluate';

export interface Action {
  type: ActionType;
  selector?: string;
  text?: string;
  key?: string;
  url?: string;
  script?: string;
  ms?: number;
  px?: number;
}

/** Named built-ins the server may ask for (D33). Closed vocabulary: no caller
 * JS crosses into a signed-in tab, and every one of these is a fixed function
 * the bridge ships. */
export interface SettleOptions {
  quiet_ms?: number;
  max_ms?: number;
  poll_ms?: number;
}

export interface ScrollCollectOptions {
  max_rounds?: number;
  quiet_rounds?: number;
  step_px?: number;
  pause_ms?: number;
  item_selector?: string;
}

export interface PageTreeOptions {
  max_nodes?: number;
  max_depth?: number;
  min_text_len?: number;
}

export interface FetchRequest {
  id: string;
  url: string;
  actions?: Action[];
  wait_selector?: string;
  settle_ms?: number;
  timeout_ms?: number;
  screenshot?: boolean;
  full_page?: boolean;
  io?: IO;
  /** Named built-ins (D33). Absent means "do not run it". */
  settle?: SettleOptions;
  scroll_collect?: ScrollCollectOptions;
  page_tree?: 'dom' | 'ax' | 'both';
  page_tree_options?: PageTreeOptions;
  motion?: boolean;
  recipe?: string;
  lane?: string;
}

export interface ActionResult {
  type: string;
  ok: boolean;
  error?: string;
}

export interface PageNode {
  tag: string;
  role: string;
  path: string;
  text_len: number;
  text?: string;
  box?: [number, number, number, number];
  visible: boolean;
  children?: PageNode[];
  truncated?: boolean;
}

export interface MotionReport {
  animations: Array<{
    play_state: string;
    infinite: boolean;
    duration_ms: number | null;
    target: string | null;
  }>;
  pinned: Array<{ path: string; position: string; box: [number, number, number, number] }>;
  scroll_container: string | null;
}

export interface ScrollHarvest {
  rounds: Array<{ round: number; scroll_y: number; text_len: number; items: number; grew: boolean }>;
  at_bottom: boolean;
  text_len: number;
  items: number;
}

export interface FetchResult {
  status: 'ok' | 'awaiting-human' | 'blocked' | 'paused' | 'cancelled' | 'error';
  final_url?: string;
  title?: string;
  text?: string;
  html?: string;
  html_truncated?: boolean;
  screenshot_b64?: string;
  markers?: string[];
  actions_run?: ActionResult[];
  io?: IO;
  http_status?: number;
  error?: string;
  message?: string;
  ms?: number;
  /** How long the page took to go quiet. -1 when settle was not requested. */
  settled_ms?: number;
  tree?: PageNode;
  motion?: MotionReport;
  collected?: ScrollHarvest;
}

/** Popup <-> background messages. */
export type PopupMessage =
  | { kind: 'status' }
  | { kind: 'pair'; code: string }
  | { kind: 'forget' }
  | { kind: 'pause'; on: boolean }
  | { kind: 'port'; port: number };

export interface BridgeState {
  connected: boolean;
  paired: boolean;
  paused: boolean;
  lastError: string;
  port: number;
  /** Origins the server says it will ask for. Empty on an older server. */
  allowed?: string[];
  lastRun?: { origin: string; ms: number; actions: number; at: number } | null;
}
