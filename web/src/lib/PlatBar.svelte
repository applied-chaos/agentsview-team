<script lang="ts">
  import { signOut, type Me } from './api';
  import Avatar from './Avatar.svelte';
  import Banner from './Banner.svelte';

  /** `viewing` is the User whose Instance an Admin is looking at in Instance View. */
  let { me, viewing }: { me: Me; viewing?: { name: string; id: string } } = $props();

  let open = $state(false);
  let acct: HTMLElement | undefined = $state();

  const who = $derived(me.impersonatedBy ?? me);
</script>

<svelte:window
  onclick={(e) => {
    if (acct && !acct.contains(e.target as Node)) open = false;
  }}
  onkeydown={(e) => e.key === 'Escape' && (open = false)}
/>

{#if viewing}
  <Banner kind="view" name={viewing.name} userId={viewing.id} />
{:else if me.impersonatedBy}
  <Banner kind="imp" name={me.name} userId={me.id} />
{/if}
<div class="platbar">
  <div class="wrap">
    <a class="brand" href="/"><span class="logo"></span>AgentsView Teams</a>
    <nav aria-label="Platform">
      <a href="/">Overview</a><a href="/hosts">Hosts</a><a href="/team">Team</a>
    </nav>
    <span class="sp"></span>
    <span class="here">My Instance</span>
    <div class="acct" class:open bind:this={acct}>
      <button
        class="acct-btn"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label="Account"
        onclick={() => (open = !open)}
      >
        <Avatar name={who.name} email={who.email} size={22} />
      </button>
      <div class="menu" role="menu">
        <div class="who">{who.name}<span>{who.email}</span></div>
        <a role="menuitem" href="/#settings">Settings</a>
        {#if me.isAdmin}<a role="menuitem" href="/admin/users">Admin</a>{/if}
        <button role="menuitem" onclick={signOut}>Sign out</button>
      </div>
    </div>
  </div>
</div>
