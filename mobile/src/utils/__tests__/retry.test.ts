import { fetchWithRetry, isRetryableStatus, withRetry } from "../retry";

const noSleep = jest.fn().mockResolvedValue(undefined);

beforeEach(() => {
  noSleep.mockClear();
  jest.spyOn(console, "warn").mockImplementation(() => undefined);
});
afterEach(() => jest.restoreAllMocks());

describe("isRetryableStatus", () => {
  it("retries timeouts, rate limiting and server errors, never other client errors", () => {
    for (const s of [408, 429, 500, 502, 503, 504]) expect(isRetryableStatus(s)).toBe(true);
    for (const s of [200, 301, 400, 401, 403, 404, 422]) expect(isRetryableStatus(s)).toBe(false);
  });
});

describe("withRetry", () => {
  it("returns the first success without sleeping", async () => {
    const fn = jest.fn().mockResolvedValue("ok");
    expect(await withRetry(fn, { sleep: noSleep })).toBe("ok");
    expect(fn).toHaveBeenCalledTimes(1);
    expect(noSleep).not.toHaveBeenCalled();
  });

  it("retries a transient failure and then succeeds", async () => {
    const fn = jest.fn().mockRejectedValueOnce(new Error("blip")).mockResolvedValue("ok");
    expect(await withRetry(fn, { sleep: noSleep })).toBe("ok");
    expect(fn).toHaveBeenCalledTimes(2);
    expect(noSleep).toHaveBeenCalledTimes(1);
  });

  it("gives up after the attempt budget and throws the last error", async () => {
    const fn = jest.fn().mockRejectedValue(new Error("down"));
    await expect(withRetry(fn, { attempts: 3, sleep: noSleep })).rejects.toThrow("down");
    expect(fn).toHaveBeenCalledTimes(3);
  });

  it("does not retry when shouldRetry says the error is permanent", async () => {
    const fn = jest.fn().mockRejectedValue(new Error("403"));
    await expect(
      withRetry(fn, { shouldRetry: () => false, sleep: noSleep }),
    ).rejects.toThrow("403");
    expect(fn).toHaveBeenCalledTimes(1);
  });

  it("backs off exponentially, capped at maxDelayMs", async () => {
    const fn = jest.fn().mockRejectedValue(new Error("down"));
    await expect(
      withRetry(fn, { attempts: 4, baseDelayMs: 1000, maxDelayMs: 3000, sleep: noSleep }),
    ).rejects.toThrow();
    const delays = noSleep.mock.calls.map((c) => c[0] as number);
    expect(delays).toHaveLength(3);
    expect(delays[0]).toBeGreaterThanOrEqual(1000);
    expect(delays[1]).toBeGreaterThanOrEqual(2000);
    for (const d of delays) expect(d).toBeLessThanOrEqual(3000);
  });
});

function res(status: number, headers: Record<string, string> = {}) {
  return { status, ok: status < 400, headers: { get: (k: string) => headers[k] ?? null } } as unknown as Response;
}

describe("fetchWithRetry", () => {
  const originalFetch = globalThis.fetch;
  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  it("retries a 503 and returns the eventual 200", async () => {
    globalThis.fetch = jest
      .fn()
      .mockResolvedValueOnce(res(503))
      .mockResolvedValue(res(200)) as unknown as typeof fetch;
    const out = await fetchWithRetry("https://x", undefined, { sleep: noSleep });
    expect(out.status).toBe(200);
    expect(globalThis.fetch).toHaveBeenCalledTimes(2);
  });

  it("does not retry a 404", async () => {
    globalThis.fetch = jest.fn().mockResolvedValue(res(404)) as unknown as typeof fetch;
    const out = await fetchWithRetry("https://x", undefined, { sleep: noSleep });
    expect(out.status).toBe(404);
    expect(globalThis.fetch).toHaveBeenCalledTimes(1);
  });

  it("returns the last response, not an exception, when a 500 never clears", async () => {
    globalThis.fetch = jest.fn().mockResolvedValue(res(500)) as unknown as typeof fetch;
    const out = await fetchWithRetry("https://x", undefined, { attempts: 3, sleep: noSleep });
    expect(out.status).toBe(500);
    expect(globalThis.fetch).toHaveBeenCalledTimes(3);
  });

  it("retries a network failure and throws once the budget is spent", async () => {
    globalThis.fetch = jest.fn().mockRejectedValue(new TypeError("Network request failed")) as unknown as typeof fetch;
    await expect(
      fetchWithRetry("https://x", undefined, { attempts: 3, sleep: noSleep }),
    ).rejects.toThrow("Network request failed");
    expect(globalThis.fetch).toHaveBeenCalledTimes(3);
  });

  it("honors a short Retry-After on 429 instead of the default backoff", async () => {
    globalThis.fetch = jest
      .fn()
      .mockResolvedValueOnce(res(429, { "Retry-After": "2" }))
      .mockResolvedValue(res(200)) as unknown as typeof fetch;
    await fetchWithRetry("https://x", undefined, { baseDelayMs: 100, sleep: noSleep });
    expect(noSleep).toHaveBeenCalledWith(2000);
  });

  it("aborts an attempt that outlives the timeout and retries it", async () => {
    let calls = 0;
    globalThis.fetch = jest.fn((_url: unknown, init?: RequestInit) => {
      calls += 1;
      if (calls === 1) {
        return new Promise((_resolve, reject) => {
          init?.signal?.addEventListener("abort", () => reject(new Error("aborted")));
        });
      }
      return Promise.resolve(res(200));
    }) as unknown as typeof fetch;
    const out = await fetchWithRetry("https://x", undefined, { timeoutMs: 20, sleep: noSleep });
    expect(out.status).toBe(200);
    expect(calls).toBe(2);
  });

  it("does not call fetch, or retry, once the caller's own signal has aborted", async () => {
    globalThis.fetch = jest.fn() as unknown as typeof fetch;
    const controller = new AbortController();
    controller.abort();
    await expect(
      fetchWithRetry("https://x", { signal: controller.signal }, { sleep: noSleep }),
    ).rejects.toThrow("aborted");
    expect(globalThis.fetch).not.toHaveBeenCalled();
  });

  it("stops retrying when the caller aborts mid-request", async () => {
    const controller = new AbortController();
    globalThis.fetch = jest.fn((_url: unknown, init?: RequestInit) => {
      controller.abort();
      return Promise.reject(new Error(init?.signal?.aborted ? "aborted" : "other"));
    }) as unknown as typeof fetch;
    await expect(
      fetchWithRetry("https://x", { signal: controller.signal }, { sleep: noSleep }),
    ).rejects.toThrow("aborted");
    expect(globalThis.fetch).toHaveBeenCalledTimes(1);
  });
});
