<script lang="ts">
  import { showMe, swr, type Me } from '../lib/api';
  import HostList from '../lib/HostList.svelte';
  import { HostsList } from '../lib/hosts';
  import Icon from '../lib/Icon.svelte';
  import ListSkel from '../lib/ListSkel.svelte';
  import InstallTokens from '../lib/InstallTokens.svelte';
  import TopBar from '../lib/TopBar.svelte';

  let me: Me | undefined = $state();
  let data: HostsList | undefined = $state();
  $effect(() => {
    showMe((m) => (me = m));
    swr('/api/hosts', HostsList, (d) => (data = d));
  });
  const rows = $derived(
    [...(data?.hosts ?? [])].sort(
      (a, b) => Number(a.status === 'revoked') - Number(b.status === 'revoked'),
    ),
  );
</script>

<TopBar {me} tab="hosts" />
<main class="wrap">
  <div class="ph">
    <div>
      <h1>Hosts</h1>
      <p class="mu">Machines that send their agent sessions to your Instance.</p>
    </div>
    <div class="acts">
      <a class="btn primary" href="/hosts/new"><Icon name="plus" /> Add Host</a>
    </div>
  </div>
  {#if !data}
    <ListSkel
      cols="100px minmax(0,1.4fr) minmax(0,1.1fr) 120px 110px"
      heads={['Status', 'Name', 'OS', 'Last seen', 'Version']}
      rows={5}
    />
  {:else}
    {#if rows.length === 0}
      <p class="mu">No Hosts yet. Add one to send its sessions to your Instance.</p>
    {:else}
      <HostList
        hosts={rows}
        pinned={data.pinnedVersion}
        cols="100px minmax(0,1.4fr) minmax(0,1.1fr) 120px 110px"
      />
    {/if}
    <InstallTokens />
  {/if}
</main>
