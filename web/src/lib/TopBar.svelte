<script lang="ts">
  import { signOut, type Me } from './api';
  import Avatar from './Avatar.svelte';
  import Banner from './Banner.svelte';
  import Icon from './Icon.svelte';
  import Settings from './Settings.svelte';

  let { me, tab }: { me: Me | undefined; tab?: 'overview' | 'hosts' | 'team' } = $props();

  const TABS = [
    ['overview', 'Overview', '/'],
    ['hosts', 'Hosts', '/hosts'],
    ['team', 'Team', '/team'],
  ] as const;

  let menuOpen = $state(false);
  let navOpen = $state(false);
  let settings: HTMLDialogElement | undefined = $state();
  let acct: HTMLElement | undefined = $state();

  const who = $derived(me?.impersonatedBy ?? me);
  function openSettings() {
    menuOpen = false;
    settings?.showModal();
  }

  const openFromHash = () => {
    if (location.hash === '#settings' && !settings?.open) settings?.showModal();
  };
  $effect(openFromHash);
</script>

<svelte:window
  onhashchange={openFromHash}
  onclick={(e) => {
    if (acct && !acct.contains(e.target as Node)) menuOpen = false;
  }}
  onkeydown={(e) => e.key === 'Escape' && (menuOpen = false)}
/>

{#snippet items()}
  <button role="menuitem" onclick={openSettings}>Settings</button>
  {#if me?.isAdmin}<a role="menuitem" href="/admin/users">Admin</a>{/if}
  <button role="menuitem" onclick={signOut}>Sign out</button>
{/snippet}

{#if me?.impersonatedBy}<Banner kind="imp" name={me.name} userId={me.id} />{/if}
<header class="topbar" class:menu-open={navOpen}>
  <div class="wrap">
    <a class="brand" href="/"><span class="logo"></span>AgentsView Teams</a>
    <nav class="tabs" aria-label="Main">
      {#each TABS as [key, label, href] (key)}
        <a {href} aria-current={key === tab ? 'page' : undefined}>{label}</a>
      {/each}
      <a class="inst" href="/i/me/">My Instance <Icon name="ext" /></a>
    </nav>
    <span class="sp"></span>
    <div class="acct" class:open={menuOpen} bind:this={acct}>
      <button
        class="acct-btn"
        aria-haspopup="menu"
        aria-expanded={menuOpen}
        onclick={() => (menuOpen = !menuOpen)}
      >
        <Avatar name={who?.name ?? ''} email={who?.email} />{#if who}<span>{who.name}</span
          >{:else}<span class="sk" style="width:80px"></span>{/if}<Icon name="down" />
      </button>
      <div class="menu" role="menu">
        {#if who}<div class="who">{who.name}<span>{who.email}</span></div>{/if}
        {@render items()}
      </div>
    </div>
    <button
      class="burger"
      aria-label="Menu"
      aria-expanded={navOpen}
      onclick={() => (navOpen = !navOpen)}
    >
      <Icon name="menu" />
    </button>
  </div>
  <nav class="mnav" aria-label="Main">
    {#each TABS as [key, label, href] (key)}
      <a {href} aria-current={key === tab ? 'page' : undefined}>{label}</a>
    {/each}
    <a class="inst" href="/i/me/">My Instance <Icon name="ext" /></a>
    <hr />
    {#if who}<div class="who">{who.name} · {who.email}</div>{/if}
    {@render items()}
  </nav>
</header>
<Settings bind:dialog={settings} />
