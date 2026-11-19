<script lang="ts">
  import type { Snippet } from 'svelte';
  import { showMe, type Me } from './api';
  import TopBar from './TopBar.svelte';

  let { children }: { children: Snippet } = $props();

  let me: Me | undefined = $state();
  $effect(() => {
    showMe((m) => {
      if (!m.isAdmin) location.replace('/');
      else me = m;
    });
  });
</script>

<TopBar {me} />
{#if me}
  {@render children()}
{:else}
  <main class="wrap">
    <div class="ph"><span class="sk h1"></span></div>
  </main>
{/if}
