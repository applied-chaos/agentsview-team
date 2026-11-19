import { describe, expect, it } from 'vitest';
import {
  array,
  boolean,
  nullable,
  number,
  object,
  oneOf,
  optional,
  string,
  tryParse,
  type Infer,
} from './parse';

const Row = object({
  id: string,
  n: number,
  ok: boolean,
  at: nullable(string),
  tag: optional(string),
  kind: oneOf('a', 'b'),
});

describe('parse', () => {
  const good = { id: 'x', n: 1, ok: true, at: null, kind: 'a', extra: 1 };

  it('accepts a match, drops unknown keys, omits absent optionals', () => {
    const r: Infer<typeof Row> = Row(good);
    expect(r).toEqual({ id: 'x', n: 1, ok: true, at: null, kind: 'a' });
    expect('tag' in r).toBe(false);
    expect(Row({ ...good, tag: 't' }).tag).toBe('t');
  });

  it.each([
    [{ ...good, n: '1' }, 'n: expected number, got string'],
    [{ ...good, at: undefined }, 'at: expected string, got undefined'],
    [{ ...good, kind: 'c' }, 'kind: expected a | b, got string'],
    [null, 'expected object, got null'],
  ])('rejects %j', (v, msg) => {
    expect(() => Row(v)).toThrow(msg);
  });

  it('reports the array index and returns null from tryParse', () => {
    expect(() => array(Row)([good, {}])).toThrow('[1]: id: expected string, got undefined');
    expect(array(Row)([good])).toHaveLength(1);
    expect(tryParse(array(string), 'no')).toBeNull();
  });
});
