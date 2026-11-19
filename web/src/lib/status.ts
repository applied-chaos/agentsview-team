export const STATUS_LABEL = {
  online: ['', 'Online'],
  offline: ['off', 'Offline'],
  revoked: ['rev', 'Revoked'],
  active: ['', 'Active'],
  disabled: ['off', 'Disabled'],
  instance_failed: ['bad', 'Instance failed'],
} as const satisfies Record<string, readonly [string, string]>;
