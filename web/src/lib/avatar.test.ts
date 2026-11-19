import { describe, expect, it } from 'vitest';
import { gravatarUrl, initialsOf } from './avatar';

describe('avatar', () => {
  it('takes up to two initials', () => {
    expect(initialsOf('jess okafor smith')).toBe('JO');
    expect(initialsOf('Ada')).toBe('A');
  });

  it('hashes the trimmed lowercased email with SHA-256', async () => {
    const url = await gravatarUrl('  Test@Example.COM ', 52);
    expect(url).toBe(
      'https://www.gravatar.com/avatar/973dfe463ec85785f5f95af5ba3906eedb2d931c24e69824a89ea65dba4e813b?s=52&d=404',
    );
  });
});
