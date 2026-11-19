<script lang="ts">
  import { showMe, swr, type Me } from '../lib/api';
  import Calendar from '../lib/Calendar.svelte';
  import { formatBytes } from '../lib/admin';
  import { num, tokenLabel, placeholders } from '../lib/format';
  import HostList from '../lib/HostList.svelte';
  import ListSkel from '../lib/ListSkel.svelte';
  import { Overview } from '../lib/hosts';
  import Icon from '../lib/Icon.svelte';
  import TopBar from '../lib/TopBar.svelte';

  let me: Me | undefined = $state();
  let ov: Overview | undefined = $state();
  $effect(() => {
    showMe((m) => (me = m));
    swr(
      '/api/overview',
      Overview,
      (o) => (ov = o),
      (o) => o.usageStale,
    );
  });

  const live = $derived(ov?.hosts.filter((h) => h.status !== 'revoked') ?? []);
  const online = $derived(live.filter((h) => h.status === 'online').length);
  const range = (u: { from: string; to: string }) => {
    const f = (s: string, y: boolean) =>
      new Date(`${s}T00:00:00Z`).toLocaleDateString('en-GB', {
        day: 'numeric',
        month: 'long',
        ...(y ? { year: 'numeric' } : {}),
        timeZone: 'UTC',
      });
    return `${f(u.from, false)} to ${f(u.to, false)}`;
  };
  const dash = '–';
</script>

<TopBar {me} tab="overview" />
{#if !ov}
  <main class="wrap" aria-busy="true">
    <div class="ph">
      <div><span class="sk h1"></span></div>
    </div>
    <div class="rail">
      <section aria-label="Hosts">
        <div class="sec"><span class="sk" style="width:140px"></span></div>
        <ListSkel
          cols="100px minmax(0,1.3fr) minmax(0,1.1fr) 120px 100px"
          heads={['Status', 'Name', 'OS', 'Last seen', 'Version']}
          rows={5}
        />
      </section>
      <aside aria-label="Your usage">
        <div class="figs">
          {#each placeholders(4) as i (i)}
            <div><span class="sk fig"></span></div>
          {/each}
        </div>
      </aside>
    </div>
  </main>
{:else}
  <main class="wrap">
    {#if live.length === 0}
      <div class="empty">
        <h1>Add your first Host</h1>
        <p>Install the agent on a machine and its sessions appear in your Instance.</p>
        <a class="btn primary big" href="/hosts/new">Add Host</a>
        <p style="margin: var(--s5) 0 0">
          <a href="/i/me/">Open AgentsView <Icon name="ext" /></a>
        </p>
      </div>
    {:else}
      <div class="ph">
        <div>
          <h1>Your month</h1>
          {#if ov.usage}<p class="mu">{range(ov.usage)}</p>{/if}
        </div>
        <div class="acts">
          <a class="btn primary big" href="/i/me/">Open AgentsView <Icon name="ext" /></a>
        </div>
      </div>
      <div class="rail">
        <section aria-labelledby="hosts-h">
          <div class="sec">
            <h2 id="hosts-h">Hosts <span class="mu">{online} of {live.length} online</span></h2>
            <a class="more" href="/hosts">All Hosts <Icon name="right" /></a>
          </div>
          <HostList
            hosts={live.slice(0, 5)}
            pinned={ov.pinnedVersion}
            ephemeralTag={false}
            cols="100px minmax(0,1.3fr) minmax(0,1.1fr) 120px 100px"
          />
        </section>
        <aside aria-label="Your usage">
          <div class="figs">
            <div>
              <span class="fig">{ov.usage ? num(ov.usage.sessions) : dash}</span><span class="lbl"
                >sessions</span
              >
            </div>
            <div>
              <span class="fig">{ov.usage ? tokenLabel(ov.usage.tokens) : dash}</span><span
                class="lbl">tokens</span
              >
            </div>
            <div>
              <span class="fig">{ov.usage ? `$${Math.round(ov.usage.costUsd)}` : dash}</span><span
                class="lbl">estimated cost</span
              >
            </div>
            <div>
              <span class="fig">{ov.usage ? ov.usage.activeDays : dash}</span><span class="lbl"
                >active days</span
              >
            </div>
            <div>
              <span class="fig"
                >{ov.storageBytes === null ? dash : formatBytes(ov.storageBytes)}</span
              ><span class="lbl">space used</span>
            </div>
          </div>
          {#if ov.usage}
            <div style="margin-top: var(--s6)">
              <Calendar from={ov.usage.from} to={ov.usage.to} daily={ov.usage.daily} />
            </div>
          {/if}
        </aside>
      </div>
    {/if}
  </main>
{/if}
