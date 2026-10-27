/**
 * Bounded retry with exponential backoff, plus a fetch wrapper that adds a
 * per-attempt timeout. Only for idempotent reads: retrying a login, a
 * paid LLM call or any other write would risk doing it twice.
 *
 * Every retry path here is capped by an attempt count -- no unbounded loops
 * (the backend's TTS worker once looped forever on a permanent error).
 */

const DEFAULT_ATTEMPTS = 3;
const DEFAULT_BASE_DELAY_MS = 500;
const DEFAULT_MAX_DELAY_MS = 8000;
const DEFAULT_TIMEOUT_MS = 15000;

export type RetryOptions = {
  /** Total tries, including the first. Default 3. */
  attempts?: number;
  /** First backoff; doubles each retry, with up to one extra `baseDelayMs` of jitter. Default 500. */
  baseDelayMs?: number;
  maxDelayMs?: number;
  /** Return false for errors another try cannot fix. Default: retry everything. */
  shouldRetry?: (err: unknown) => boolean;
  /** Injectable so tests don't wait. */
  sleep?: (ms: number) => Promise<void>;
};

const defaultSleep = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms));

function backoffMs(retryNumber: number, baseDelayMs: number, maxDelayMs: number): number {
  const exponential = baseDelayMs * 2 ** (retryNumber - 1);
  return Math.min(exponential + Math.random() * baseDelayMs, maxDelayMs);
}

export function isRetryableStatus(status: number): boolean {
  return status === 408 || status === 429 || status >= 500;
}

export async function withRetry<T>(fn: () => Promise<T>, opts: RetryOptions = {}): Promise<T> {
  const {
    attempts = DEFAULT_ATTEMPTS,
    baseDelayMs = DEFAULT_BASE_DELAY_MS,
    maxDelayMs = DEFAULT_MAX_DELAY_MS,
    shouldRetry = () => true,
    sleep = defaultSleep,
  } = opts;

  for (let attempt = 1; ; attempt++) {
    try {
      return await fn();
    } catch (err) {
      if (attempt >= attempts || !shouldRetry(err)) throw err;
      console.warn(`retry ${attempt}/${attempts - 1} after error:`, err);
      await sleep(backoffMs(attempt, baseDelayMs, maxDelayMs));
    }
  }
}

/** Seconds from a Retry-After header, in ms, only when it is short enough to wait for. */
function retryAfterMs(res: Response, maxDelayMs: number): number | null {
  const raw = res.headers?.get?.("Retry-After");
  const seconds = raw ? Number(raw) : NaN;
  if (!Number.isFinite(seconds) || seconds < 0) return null;
  const ms = seconds * 1000;
  return ms <= maxDelayMs ? ms : null;
}

/**
 * fetch with a per-attempt timeout, retrying network failures, timeouts and
 * 408/429/5xx. Other statuses (404, 403...) are returned straight away for the
 * caller to handle as before. When a retryable status never clears, the LAST
 * response is returned rather than thrown, so callers keep their existing
 * status handling; only a network failure on the final attempt throws.
 *
 * The timeout covers the wait for the response headers, not a body that then
 * stalls while being read. A caller-supplied `init.signal` is honoured: once it
 * aborts, the request fails at once and is not retried.
 */
export async function fetchWithRetry(
  input: RequestInfo | URL,
  init?: RequestInit,
  opts: Omit<RetryOptions, "shouldRetry"> & { timeoutMs?: number } = {},
): Promise<Response> {
  const {
    timeoutMs = DEFAULT_TIMEOUT_MS,
    attempts = DEFAULT_ATTEMPTS,
    baseDelayMs = DEFAULT_BASE_DELAY_MS,
    maxDelayMs = DEFAULT_MAX_DELAY_MS,
    sleep = defaultSleep,
  } = opts;
  const callerSignal = init?.signal;

  for (let attempt = 1; ; attempt++) {
    if (callerSignal?.aborted) throw new Error("request aborted by caller");
    const last = attempt >= attempts;
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeoutMs);
    const abortWithCaller = () => controller.abort();
    callerSignal?.addEventListener("abort", abortWithCaller);
    try {
      const res = await fetch(input, { ...init, signal: controller.signal });
      if (!isRetryableStatus(res.status) || last) return res;
      console.warn(`retry ${attempt}/${attempts - 1}: ${String(input)} answered ${res.status}`);
      // Free the connection held by a response we are about to discard.
      await res.body?.cancel?.().catch(() => undefined);
      await sleep(retryAfterMs(res, maxDelayMs) ?? backoffMs(attempt, baseDelayMs, maxDelayMs));
    } catch (err) {
      if (last || callerSignal?.aborted) throw err;
      console.warn(`retry ${attempt}/${attempts - 1}: ${String(input)} failed:`, err);
      await sleep(backoffMs(attempt, baseDelayMs, maxDelayMs));
    } finally {
      clearTimeout(timer);
      callerSignal?.removeEventListener("abort", abortWithCaller);
    }
  }
}
