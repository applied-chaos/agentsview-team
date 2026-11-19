<script lang="ts">
  import AdminGate from '../lib/AdminGate.svelte';
  import AdminHead from '../lib/AdminHead.svelte';
  import { AdminUsers, formatBytes, type AdminUser } from '../lib/admin';
  import { clearHash, getJSON, sendJSON, swr } from '../lib/api';
  import { ago, num } from '../lib/format';
  import Icon from '../lib/Icon.svelte';
  import ListSkel from '../lib/ListSkel.svelte';
  import { PublicConfig } from '../lib/rule';
  import { STATUS_LABEL } from '../lib/status';

  type Filter = 'all' | 'admins' | 'disabled';
  const FILTERS = [
    ['all', 'All'],
    ['admins', 'Admins'],
    ['disabled', 'Disabled'],
  ] as const;

  let users: AdminUser[] = $state([]);
  let loaded = $state(false);
  let cfg: PublicConfig | null | undefined = $state();
  let query = $state('');
  let filter: Filter = $state('all');
  let dialog: HTMLDialogElement | undefined = $state();
  let email = $state('');
  let error = $state('');
  let busy = $state(false);
  const now = new Date();

  const load = () =>
    swr('/api/admin/users', AdminUsers, (r) => {
      users = r.users;
      loaded = true;
    });
  $effect(() => {
    load();
    getJSON('/api/public', PublicConfig).then((c) => (cfg = c));
  });

  const shown = $derived(
    users.filter((u) => {
      const q = query.trim().toLowerCase();
      if (q && !`${u.name} ${u.email}`.toLowerCase().includes(q)) return false;
      if (filter === 'admins') return u.role === 'admin' || u.pendingAdmin;
      if (filter === 'disabled') return u.status === 'disabled';
      return true;
    }),
  );

  const openFromHash = () => {
    if (location.hash === '#dlg-grant' && !dialog?.open) dialog?.showModal();
  };
  $effect(openFromHash);

  function open() {
    error = '';
    email = '';
    dialog?.showModal();
  }
  async function grant(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    error = '';
    const res = await sendJSON('POST', '/api/admin/admins', { email: email.trim() });
    busy = false;
    if (res.status === 200) {
      dialog?.close();
      await load();
    } else {
      error = res.status === 400 ? 'Enter an email address.' : 'Could not grant Admin. Try again.';
    }
  }
</script>

<svelte:window onhashchange={openFromHash} />

<AdminGate>
  <main class="wrap">
    <AdminHead
      current="users"
      lead={`Everyone with a sign-in${cfg ? ` to ${cfg.providerName}` : ''}.`}
    />
    <div class="toolbar">
      <div class="search">
        <label class="vh" for="q">Search Users</label>
        <Icon name="search" />
        <input id="q" class="input" type="search" placeholder="Search Users" bind:value={query} />
      </div>
      <div class="seg" role="radiogroup" aria-label="Filter Users">
        {#each FILTERS as [key, label] (key)}
          <label>
            <input type="radio" name="f" checked={filter === key} onchange={() => (filter = key)} />
            <span>{label}</span>
          </label>
        {/each}
      </div>
      <button class="btn primary" style="margin-left:auto" onclick={open}>Grant Admin</button>
    </div>
    {#if !loaded}
      <ListSkel
        cols="minmax(0,1.6fr) 140px 150px 70px 90px 110px"
        heads={['User', 'Role', 'Status', 'Hosts', 'Storage', 'Last active']}
        rows={5}
      />
    {:else}
      <div class="list" style="--cols:minmax(0,1.6fr) 140px 150px 70px 90px 110px">
        <div class="r hd">
          <span>User</span><span>Role</span><span>Status</span>
          <span class="end">Hosts</span><span class="end">Storage</span><span>Last active</span>
        </div>
        {#each shown as u (u.id || u.email)}
          {@const [cls, label] = STATUS_LABEL[u.status]}
          <svelte:element
            this={u.id ? 'a' : 'div'}
            class="r"
            href={u.id ? `/admin/users/${u.id}` : undefined}
          >
            <span class="nm">{u.name}<span class="sub">{u.email}</span></span>
            <span class="hm">
              {u.role === 'admin' || u.pendingAdmin ? 'Admin' : 'User'}
            </span>
            <span class="st stc {cls}">{label}</span>
            <span class="hm end">{num(u.hosts)}</span>
            <span class="hm end">{formatBytes(u.storageBytes)}</span>
            <span class="hm">{ago(u.lastActive, now, 'admin')}</span>
          </svelte:element>
        {/each}
      </div>
    {/if}
  </main>

  <dialog
    class="dlg"
    id="dlg-grant"
    bind:this={dialog}
    aria-labelledby="grant-h"
    onclick={(e) => e.target === dialog && dialog?.close()}
    onclose={clearHash}
  >
    <form class="box" tabindex="-1" autofocus onsubmit={grant}>
      <div class="dh">
        <h2 id="grant-h">Grant Admin</h2>
        <button type="button" class="x" aria-label="Close" onclick={() => dialog?.close()}
          ><Icon name="x" /></button
        >
      </div>
      <div class="body">
        <div>
          <label class="f" for="ge">Email</label>
          <input
            id="ge"
            class="input"
            type="email"
            placeholder={`name@${cfg?.domains[0] ?? 'example.com'}`}
            required
            bind:value={email}
          />
          <p class="hint">If they haven't signed in yet, the grant stays pending until they do.</p>
          {#if error}<p class="hint" role="alert" style="color:var(--danger)">{error}</p>{/if}
        </div>
      </div>
      <div class="df">
        <button type="button" class="btn" onclick={() => dialog?.close()}>Cancel</button>
        <button type="submit" class="btn primary" disabled={busy}>Grant Admin</button>
      </div>
    </form>
  </dialog>
</AdminGate>
