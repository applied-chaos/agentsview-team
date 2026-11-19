<script lang="ts">
  import AdminGate from '../lib/AdminGate.svelte';
  import AdminHead from '../lib/AdminHead.svelte';
  import { AuditPage, formatDateTime } from '../lib/admin';
  import { getJSON } from '../lib/api';
  import { num, shortDate } from '../lib/format';
  import Icon from '../lib/Icon.svelte';

  const TYPES = [
    ['all', 'All actions'],
    ['impersonation', 'Impersonation'],
    ['instance_view', 'Instance View'],
    ['admin_grants', 'Admin grants'],
    ['user_changes', 'User changes'],
    ['hosts', 'Hosts'],
    ['install_tokens', 'Install Tokens'],
  ] as const;
  const RANGES = [
    ['7d', 'Last 7 days'],
    ['30d', 'Last 30 days'],
    ['90d', 'Last 90 days'],
    ['all', 'All time'],
  ] as const;

  let query = $state('');
  let type = $state('all');
  let range = $state('7d');
  let page = $state(1);
  let data: AuditPage | undefined = $state();

  const filters = $derived(`${query}|${type}|${range}`);
  let pageFilters = '';
  $effect(() => {
    if (filters !== pageFilters) {
      pageFilters = filters;
      page = 1;
    }
    const p = new URLSearchParams({ q: query.trim(), type, range, page: String(page) });
    const timer = setTimeout(
      () => getJSON(`/api/admin/audit?${p}`, AuditPage).then((r) => (data = r ?? undefined)),
      query ? 200 : 0,
    );
    return () => clearTimeout(timer);
  });

  const last = $derived(data ? Math.ceil(data.total / data.pageSize) : 1);
</script>

<AdminGate>
  <main class="wrap">
    <AdminHead current="audit" lead="Every Admin action and every Impersonation." />
    <div class="toolbar">
      <div class="search">
        <label class="vh" for="q">Search the Audit Log</label>
        <Icon name="search" />
        <input
          id="q"
          class="input"
          type="search"
          placeholder="Search the Audit Log"
          bind:value={query}
        />
      </div>
      <div style="width:190px">
        <label class="vh" for="at">Action type</label>
        <select id="at" class="input" bind:value={type}>
          {#each TYPES as [value, label] (value)}<option {value}>{label}</option>{/each}
        </select>
      </div>
      <div style="width:170px">
        <label class="vh" for="dr">Date range</label>
        <select id="dr" class="input" bind:value={range}>
          {#each RANGES as [value, label] (value)}<option {value}>{label}</option>{/each}
        </select>
      </div>
    </div>
    <div class="list" style="--cols:130px 130px minmax(0,1.2fr) minmax(0,1.1fr) minmax(0,1.5fr)">
      <div class="r hd">
        <span>Time</span><span>Admin</span><span>Action</span><span>Target</span><span>Details</span
        >
      </div>
      {#each data?.entries ?? [] as e (e.id)}
        <div class="r">
          <span class="hm nowrap">{formatDateTime(e.at)}</span>
          <span class="hm">{e.actor}</span>
          <span class="nm">{e.action}<span class="sub om">{e.target} · {e.actor}</span></span>
          <span class="hm">{e.target}</span>
          <span class="mu hm">{e.details}</span>
          <span class="om end mu nowrap">{shortDate(e.at)}</span>
        </div>
      {/each}
    </div>
    {#if data}
      <div
        style="display:flex;justify-content:space-between;align-items:center;gap:var(--s4);margin-top:var(--s5)"
      >
        <span class="mu">Showing {num(data.entries.length)} of {num(data.total)}</span>
        <span style="display:flex;gap:var(--s2)">
          <button class="btn" disabled={page <= 1} onclick={() => page--}>Previous</button>
          <button class="btn" disabled={page >= last} onclick={() => page++}>Next</button>
        </span>
      </div>
    {/if}
  </main>
</AdminGate>
