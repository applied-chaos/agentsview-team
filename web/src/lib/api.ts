import {
  boolean,
  object,
  oneOf,
  optional,
  string,
  tryParse,
  type Infer,
  type Schema,
} from './parse';

export const Me = object({
  id: string,
  email: string,
  name: string,
  isAdmin: boolean,
  /** Set while an Admin is Impersonating: this Me is then the User being acted as. */
  impersonatedBy: optional(object({ id: string, name: string, email: string })),
});
export type Me = Infer<typeof Me>;

export const InstanceStatus = object({
  state: oneOf('starting', 'running', 'failed'),
  url: string,
  version: string,
  error: string,
});
export type InstanceStatus = Infer<typeof InstanceStatus>;

export const Denied = object({ email: string });
export type Denied = Infer<typeof Denied>;
export const NameOnly = object({ name: string });

async function refresh(): Promise<boolean> {
  return (await fetch('/auth/refresh', { method: 'POST' })).ok;
}

export const toSignIn = () => location.replace('/sign-in');

export class HttpError extends Error {
  constructor(
    readonly status: number,
    path: string,
  ) {
    super(`${path}: ${status}`);
  }
}

// Pages are separate loads, so the stale-while-revalidate copies live in sessionStorage (per tab).
const PREFIX = 'avc:';
const inflight = new Map<string, Promise<string | null>>();
let generation = 0;

function recall(path: string): string | null {
  try {
    return sessionStorage.getItem(PREFIX + path);
  } catch {
    return null;
  }
}

/** Drops every cached GET and in-flight request: call on sign-out, sign-in, Impersonation and writes. */
export function clearCache(): void {
  generation++;
  inflight.clear();
  try {
    for (const k of Object.keys(sessionStorage))
      if (k.startsWith(PREFIX)) sessionStorage.removeItem(k);
  } catch {
    // storage unavailable: nothing cached
  }
}

function fetchDeduped(path: string): Promise<string | null> {
  const hit = inflight.get(path);
  if (hit) return hit;
  const gen = generation;
  const p: Promise<string | null> = (async () => {
    let res = await fetch(path);
    if (res.status === 401 && (await refresh())) res = await fetch(path);
    if (res.status === 401) {
      clearCache();
      return null;
    }
    if (!res.ok) throw new HttpError(res.status, path);
    const text = await res.text();
    if (gen === generation) {
      try {
        sessionStorage.setItem(PREFIX + path, text);
      } catch {
        // full or unavailable: skip caching
      }
    }
    return text;
  })().finally(() => {
    if (inflight.get(path) === p) inflight.delete(path);
  });
  inflight.set(path, p);
  return p;
}

/** GET a JSON endpoint; on 401 try one refresh, then retry. Returns null when signed out. */
export async function getJSON<T>(path: string, schema: Schema<T>): Promise<T | null> {
  const text = await fetchDeduped(path);
  return text === null ? null : schema(JSON.parse(text));
}

function recallParsed<T>(path: string, schema: Schema<T>): T | null {
  const text = recall(path);
  if (text === null) return null;
  try {
    return schema(JSON.parse(text));
  } catch {
    return null;
  }
}

/** Calls `set` with the last cached copy at once (if any), then with the fresh one. Resolves the fresh one.
 * While `serverStale` says the server is still reading newer data, asks again every 2s (up to 5 times). */
export async function swr<T>(
  path: string,
  schema: Schema<T>,
  set: (v: T) => void,
  serverStale?: (v: T) => boolean,
): Promise<T | null> {
  const cached = recallParsed(path, schema);
  if (cached !== null) set(cached);
  const fresh = await getJSON(path, schema);
  if (fresh !== null) set(fresh);
  if (fresh !== null && serverStale?.(fresh)) {
    let left = 5;
    const again = () =>
      setTimeout(
        () =>
          void getJSON(path, schema).then(
            (v) => {
              if (!v) return;
              set(v);
              if (serverStale(v) && --left > 0) again();
            },
            () => undefined,
          ),
        2000,
      );
    again();
  }
  return fresh;
}

/** Warms the cache for a likely next page. */
export const prefetch = (path: string): void => void fetchDeduped(path).catch(() => undefined);

export const getMe = () => getJSON('/api/me', Me);

/** Shows the cached Me at once, then the fresh one; sends the browser to sign in when signed out. */
export async function showMe(set: (m: Me) => void): Promise<void> {
  if (!(await swr('/api/me', Me, set))) toSignIn();
}

/** Resolves the signed-in Me, or sends the browser to sign in and resolves undefined. */
export async function requireMe(): Promise<Me | undefined> {
  const m = await getMe();
  if (!m) toSignIn();
  return m ?? undefined;
}

/** Ends Impersonation; the Admin then lands on that User's page. */
export async function stopImpersonating(userId: string): Promise<void> {
  const res = await fetch('/auth/impersonate/stop', { method: 'POST' });
  clearCache();
  if (res.ok) location.assign(`/admin/users/${userId}`);
}

export async function signOut(): Promise<void> {
  await fetch('/auth/logout', { method: 'POST' });
  clearCache();
  location.assign('/sign-in');
}

/** POST with no body and parse the JSON reply; null when signed out. */
export async function postJSON<T>(path: string, schema: Schema<T>): Promise<T | null> {
  let res = await fetch(path, { method: 'POST' });
  if (res.status === 401 && (await refresh())) res = await fetch(path, { method: 'POST' });
  clearCache();
  if (res.status === 401) return null;
  if (!res.ok) throw new Error(`${path}: ${res.status}`);
  return schema(await res.json());
}

/** Send a JSON body; resolves with the status and the body parsed by `schema` (null when no schema, empty, not JSON or a mismatch). */
export async function sendJSON<T = unknown>(
  method: 'POST' | 'PUT' | 'PATCH' | 'DELETE',
  path: string,
  body: unknown,
  schema?: Schema<T>,
): Promise<{ status: number; body: T | null }> {
  const init = {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  };
  let res = await fetch(path, init);
  if (res.status === 401 && (await refresh())) res = await fetch(path, init);
  clearCache();
  const raw: unknown = await res.json().catch(() => null);
  return { status: res.status, body: schema ? tryParse(schema, raw) : null };
}

export const clearHash = () => history.replaceState(null, '', location.pathname + location.search);
