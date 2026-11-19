import { array, object, string, type Infer } from './parse';

export const PublicConfig = object({
  providerName: string,
  domains: array(string),
  group: string,
});
export type PublicConfig = Infer<typeof PublicConfig>;

/** Plain-English subject of the sign-in rule, e.g. "halcyon.example accounts in the staff group"; empty when anyone may sign in. */
export function describeRule({ domains, group }: Pick<PublicConfig, 'domains' | 'group'>): string {
  const who = domains.length ? `${domains.join(' or ')} accounts` : '';
  if (!group) return who;
  return who ? `${who} in the ${group} group` : `Users in the ${group} group`;
}
