export const initialsOf = (name: string) =>
  name
    .split(/\s+/)
    .map((p) => p[0])
    .join('')
    .slice(0, 2)
    .toUpperCase();

/** Gravatar URL for an email; d=404 so a missing Gravatar errors and initials stay. */
export async function gravatarUrl(email: string, px: number): Promise<string | undefined> {
  if (!globalThis.crypto?.subtle) return undefined;
  const bytes = new TextEncoder().encode(email.trim().toLowerCase());
  const hash = [...new Uint8Array(await crypto.subtle.digest('SHA-256', bytes))]
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('');
  return `https://www.gravatar.com/avatar/${hash}?s=${px}&d=404`;
}
