import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { clearCache, getJSON, HttpError, prefetch, sendJSON, swr } from './api';
import { boolean, number, object } from './parse';

const N = object({ n: number });
const reply = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });

let fetchMock: ReturnType<typeof vi.fn>;
beforeEach(() => {
  clearCache();
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
});
afterEach(() => vi.unstubAllGlobals());

describe('getJSON', () => {
  it('shares one in-flight request per path', async () => {
    fetchMock.mockImplementation(async () => reply({ n: 1 }));
    const [a, b] = await Promise.all([getJSON('/api/x', N), getJSON('/api/x', N)]);
    expect(a).toEqual({ n: 1 });
    expect(b).toEqual({ n: 1 });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await getJSON('/api/x', N);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('throws HttpError with the status', async () => {
    fetchMock.mockImplementation(async () => new Response('', { status: 404 }));
    await expect(getJSON('/api/x', N)).rejects.toMatchObject({ status: 404 });
    await expect(getJSON('/api/x', N)).rejects.toBeInstanceOf(HttpError);
  });

  it('resolves null and drops the cache when signed out', async () => {
    fetchMock.mockImplementation(async () => reply({ n: 1 }));
    await getJSON('/api/x', N);
    expect(sessionStorage.length).toBe(1);
    fetchMock.mockImplementation(async () => reply({}, 401));
    expect(await getJSON('/api/y', N)).toBeNull();
    expect(sessionStorage.length).toBe(0);
  });
});

describe('swr', () => {
  it('shows the cached copy first, then the fresh one', async () => {
    fetchMock.mockImplementation(async () => reply({ n: 1 }));
    await getJSON('/api/x', N);
    fetchMock.mockImplementation(async () => reply({ n: 2 }));
    const seen: number[] = [];
    await swr('/api/x', N, (v) => seen.push(v.n));
    expect(seen).toEqual([1, 2]);
  });

  it('calls set once on a cold cache and ignores an out-of-date cached shape', async () => {
    sessionStorage.setItem('avc:/api/x', '{"wrong":true}');
    fetchMock.mockImplementation(async () => reply({ n: 3 }));
    const seen: number[] = [];
    await swr('/api/x', N, (v) => seen.push(v.n));
    expect(seen).toEqual([3]);
  });

  it('asks again while the server says newer data is on its way', async () => {
    vi.useFakeTimers();
    try {
      const S = object({ n: number, stale: boolean });
      fetchMock.mockImplementationOnce(async () => reply({ n: 1, stale: true }));
      fetchMock.mockImplementationOnce(async () => reply({ n: 1, stale: true }));
      fetchMock.mockImplementation(async () => reply({ n: 2, stale: false }));
      const seen: number[] = [];
      await swr(
        '/api/x',
        S,
        (v) => seen.push(v.n),
        (v) => v.stale,
      );
      expect(seen).toEqual([1]);
      await vi.runAllTimersAsync();
      expect(seen).toEqual([1, 1, 2]);
      expect(fetchMock).toHaveBeenCalledTimes(3);
    } finally {
      vi.useRealTimers();
    }
  });
});

describe('clearCache', () => {
  it('keeps responses already in flight out of the cache', async () => {
    let release: (r: Response) => void = () => undefined;
    fetchMock.mockImplementation(() => new Promise<Response>((r) => (release = r)));
    const pending = getJSON('/api/x', N);
    clearCache();
    release(reply({ n: 1 }));
    await pending;
    expect(sessionStorage.getItem('avc:/api/x')).toBeNull();
  });

  it('runs after a write', async () => {
    fetchMock.mockImplementation(async () => reply({ n: 1 }));
    await getJSON('/api/x', N);
    expect(sessionStorage.getItem('avc:/api/x')).not.toBeNull();
    await sendJSON('PATCH', '/api/x', {});
    expect(sessionStorage.getItem('avc:/api/x')).toBeNull();
  });
});

describe('prefetch', () => {
  it('warms the cache and swallows errors', async () => {
    fetchMock.mockImplementation(async () => reply({ n: 4 }));
    prefetch('/api/x');
    await vi.waitFor(() => expect(sessionStorage.getItem('avc:/api/x')).toBe('{"n":4}'));
    fetchMock.mockRejectedValue(new Error('offline'));
    expect(() => prefetch('/api/z')).not.toThrow();
  });
});
