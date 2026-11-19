<script lang="ts">
  import AdminGate from '../lib/AdminGate.svelte';
  import AdminHead from '../lib/AdminHead.svelte';
  import { AdminInstanceInfo, formatBytes } from '../lib/admin';
  import { swr } from '../lib/api';
  import { ago, num, tokenLabel } from '../lib/format';
  import ListSkel from '../lib/ListSkel.svelte';

  const now = new Date();
  let info: AdminInstanceInfo | null | undefined = $state();
  $effect(() => {
    swr('/api/admin/instance', AdminInstanceInfo, (i) => (info = i));
  });
  const usersBytes = $derived((info?.users ?? []).reduce((n, u) => n + u.storageBytes, 0));
  const money = (n: number) => `$${num(Math.round(n))}`;
</script>

<AdminGate>
  <main class="wrap">
    <AdminHead
      current="instance"
      lead="The Admin Instance holds every User's sessions, marked with their User."
    />
    <div>
      <p>
        Browse and search everyone's sessions in one agentsview. Each session shows which User it
        belongs to. Opening it is recorded in the Audit Log.
      </p>
      {#if info && info.state !== 'running'}
        <p class="mu" style="margin-top:var(--s3)">
          {info.state === 'failed'
            ? 'The Admin Instance has stopped.'
            : 'The Admin Instance is starting.'}
        </p>
      {/if}
    </div>
    {#if info}
      <div class="sec"><h2>Space used</h2></div>
      <div class="band" style="--n:4">
        <div>
          <span class="fig">{formatBytes(info.space.total)}</span><span class="lbl">total</span>
        </div>
        <div>
          <span class="fig">{formatBytes(usersBytes)}</span><span class="lbl"
            >Users, {num(info.users.length)} Instances</span
          >
        </div>
        <div>
          <span class="fig">{formatBytes(info.space.adminInstance)}</span><span class="lbl"
            >Admin Instance</span
          >
        </div>
        <div>
          <span class="fig">{formatBytes(info.space.spool)}</span><span class="lbl"
            >replay spool</span
          >
        </div>
      </div>
    {/if}
    <div class="sec"><h2>Usage by User, last 30 days</h2></div>
    {#if !info}
      <ListSkel
        cols="minmax(0,1.5fr) 100px 100px 130px 70px 100px 120px"
        heads={[
          'User',
          'Sessions',
          'Tokens',
          'Estimated cost',
          'Hosts',
          'Space used',
          'Last active',
        ]}
        rows={5}
      />
    {:else}
      <div class="list" style="--cols:minmax(0,1.5fr) 100px 100px 130px 70px 100px 120px">
        <div class="r hd">
          <span>User</span><span class="end">Sessions</span><span class="end">Tokens</span><span
            class="end">Estimated cost</span
          ><span class="end">Hosts</span><span class="end">Space used</span><span>Last active</span>
        </div>
        {#each info?.users ?? [] as u (u.id)}
          <a class="r" href={`/admin/users/${u.id}`}>
            <span class="nm"
              >{u.name}<span class="sub om"
                >{num(u.sessions)} sessions · {money(u.cost)} · {formatBytes(u.storageBytes)}</span
              ></span
            >
            <span class="hm end">{num(u.sessions)}</span>
            <span class="end">{tokenLabel(u.tokens)}</span>
            <span class="hm end">{money(u.cost)}</span>
            <span class="hm end">{num(u.hosts)}</span>
            <span class="hm end">{formatBytes(u.storageBytes)}</span>
            <span class="hm">{ago(u.lastActive, now, 'admin')}</span>
          </a>
        {/each}
      </div>
    {/if}
  </main>
</AdminGate>
