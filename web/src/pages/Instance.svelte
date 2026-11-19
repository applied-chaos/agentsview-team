<script lang="ts">
  import { getJSON, InstanceStatus, NameOnly, requireMe, toSignIn, type Me } from '../lib/api';
  import { ICONS } from '../lib/icons';
  import PlatBar from '../lib/PlatBar.svelte';

  let me: Me | undefined = $state();
  let status: InstanceStatus | undefined = $state();
  let details = $state(false);
  let viewing: { name: string; id: string } | undefined = $state();
  const pathId = location.pathname.split('/')[2] ?? '';

  async function poll() {
    const s = await getJSON(`/api/instance${viewing ? `?user=${viewing.id}` : ''}`, InstanceStatus);
    if (!s) return toSignIn();
    // The proxy serves this page only while the Instance is down; reload once it is up.
    if (s.state === 'running') return location.reload();
    status = s;
    if (s.state === 'starting') setTimeout(poll, 1500);
  }

  async function retry() {
    await fetch(`/api/instance/retry${viewing ? `?user=${viewing.id}` : ''}`, { method: 'POST' });
    status = {
      state: 'starting',
      url: status?.url ?? '',
      version: status?.version ?? '',
      error: '',
    };
    setTimeout(poll, 1500);
  }

  async function start() {
    const m = await requireMe();
    if (!m) return;
    if (m.isAdmin && pathId !== 'me' && pathId !== m.id) {
      const u = await getJSON(`/api/admin/users/${pathId}`, NameOnly).catch(() => null);
      if (u) viewing = { name: u.name, id: pathId };
    }
    me = m;
    poll();
  }

  $effect(() => {
    start();
  });
</script>

{#if me}
  <PlatBar {me} {viewing} />
  <main class="fail">
    {#if status?.state === 'failed'}
      <div class="empty">
        <svg class="i big-i" role="img" aria-label="Error" viewBox="0 0 24 24">
          {#each ICONS.alert as d (d)}<path {d} />{/each}
        </svg>
        <h1>Your Instance didn't start</h1>
        <p>
          The agentsview upgrade to {status.version} failed. Your sessions are safe, and your Hosts keep
          queueing uploads until your Instance is back.
        </p>
        <div class="acts">
          {#if !viewing}<button class="btn primary" onclick={retry}>Try again</button>{/if}
          <a
            class="btn"
            href="#status"
            onclick={(e) => {
              e.preventDefault();
              details = !details;
            }}>View status details</a
          >
        </div>
        {#if details}<p class="sm" role="status">{status.error || 'State: failed'}</p>{/if}
        <p class="sm">Admins have been notified.</p>
      </div>
    {:else}
      <div class="empty">
        <h1>Starting your Instance</h1>
        <p>This takes a few seconds the first time.</p>
        <span class="spin" role="status" aria-label="Starting"></span>
      </div>
    {/if}
  </main>
{/if}

<style>
  .fail {
    min-height: calc(100vh - 36px);
    display: grid;
    place-items: center;
    padding: var(--s5);
  }
  .fail .empty {
    margin: 0;
  }
  .acts {
    display: flex;
    gap: var(--s2);
    justify-content: center;
    flex-wrap: wrap;
    margin-bottom: var(--s5);
  }
  .acts .btn {
    margin: 0;
  }
  .empty p.sm {
    margin: 0;
  }
  .big-i {
    width: 32px;
    height: 32px;
    color: var(--danger);
    margin-bottom: var(--s4);
  }
  .spin {
    display: inline-block;
  }
</style>
