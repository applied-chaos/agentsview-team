<script lang="ts">
  import Icon from './Icon.svelte';

  let { current, lead }: { current: 'users' | 'instance' | 'audit'; lead: string } = $props();

  const TABS = [
    ['users', 'Users', '/admin/users'],
    ['instance', 'Admin Instance', '/admin/instance'],
    ['audit', 'Audit Log', '/admin/audit'],
  ] as const;
</script>

<div class="ph top">
  <div>
    <h1>Admin</h1>
    <p class="mu">{lead}</p>
  </div>
  <div class="acts">
    <a class="btn" href="/api/admin/instance/open">Open Admin Instance <Icon name="ext" /></a>
  </div>
</div>
<nav class="subtabs" aria-label="Admin">
  {#each TABS as [key, label, href] (key)}
    <a {href} aria-current={key === current ? 'page' : undefined}>{label}</a>
  {/each}
</nav>
