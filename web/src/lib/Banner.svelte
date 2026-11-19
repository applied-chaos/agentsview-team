<script lang="ts">
  import { stopImpersonating } from './api';
  import Icon from './Icon.svelte';

  /** `userId` is the User being viewed or impersonated; exiting returns to their detail page. */
  let { kind, name, userId }: { kind: 'view' | 'imp'; name: string; userId: string } = $props();
</script>

<div class="banner {kind}" role="status">
  {#if kind === 'view'}
    <Icon name="eye" />
    <span>Instance View: you are looking at {name}’s Instance as yourself.</span>
    <a class="btn" href="/admin/users/{userId}">Exit Instance View</a>
  {:else}
    <Icon name="shield" />
    <span>Impersonating {name}. Everything you do is recorded in the Audit Log.</span>
    <button class="btn" onclick={() => stopImpersonating(userId)}>Stop impersonating</button>
  {/if}
</div>
