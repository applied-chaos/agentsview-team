/** Runtime schemas for API replies; static types are derived from them with Infer. */
export type Schema<T> = (v: unknown) => T;
export type Infer<S> = S extends Schema<infer T> ? T : never;

type Optional<T> = Schema<T | undefined> & { optional: true };
type Shape = Record<string, Schema<unknown>>;
type OptKeys<S extends Shape> = {
  [K in keyof S]: S[K] extends { optional: true } ? K : never;
}[keyof S];
type Flat<T> = { [K in keyof T]: T[K] };
type Obj<S extends Shape> = Flat<
  { [K in Exclude<keyof S, OptKeys<S>>]: Infer<S[K]> } & {
    [K in OptKeys<S>]?: Exclude<Infer<S[K]>, undefined>;
  }
>;

const fail = (want: string, v: unknown): never => {
  throw new TypeError(`expected ${want}, got ${v === null ? 'null' : typeof v}`);
};
const at = (where: string, e: unknown): never => {
  throw new TypeError(`${where}: ${e instanceof Error ? e.message : String(e)}`);
};

const prim =
  <T>(name: string): Schema<T> =>
  (v) =>
    typeof v === name ? (v as T) : fail(name, v);

export const string = prim<string>('string');
export const number = prim<number>('number');
export const boolean = prim<boolean>('boolean');

export const nullable =
  <T>(s: Schema<T>): Schema<T | null> =>
  (v) =>
    v === null ? null : s(v);

export const optional = <T>(s: Schema<T>): Optional<T> =>
  Object.assign((v: unknown) => (v === undefined ? undefined : s(v)), { optional: true as const });

export const array =
  <T>(s: Schema<T>): Schema<T[]> =>
  (v) => {
    if (!Array.isArray(v)) return fail('array', v);
    return v.map((x, i) => {
      try {
        return s(x);
      } catch (e) {
        return at(`[${i}]`, e);
      }
    });
  };

export const object =
  <S extends Shape>(shape: S): Schema<Obj<S>> =>
  (v) => {
    if (typeof v !== 'object' || v === null || Array.isArray(v)) return fail('object', v);
    const src = v as Record<string, unknown>;
    const out: Record<string, unknown> = {};
    for (const [k, s] of Object.entries(shape)) {
      try {
        const x = s(src[k]);
        if (x !== undefined) out[k] = x;
      } catch (e) {
        at(k, e);
      }
    }
    return out as Obj<S>;
  };

export const oneOf =
  <const L extends readonly string[]>(...options: L): Schema<L[number]> =>
  (v) =>
    options.includes(v as string) ? (v as L[number]) : fail(options.join(' | '), v);

export function tryParse<T>(s: Schema<T>, v: unknown): T | null {
  try {
    return s(v);
  } catch {
    return null;
  }
}
