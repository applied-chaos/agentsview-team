<script lang="ts">
  import { prefetch } from './api';
  import { ago, shortDate } from './format';
  import type { Host } from './hosts';
  import { STATUS_LABEL } from './status';
  import Icon from './Icon.svelte';

  let {
    hosts,
    pinned,
    cols,
    ephemeralTag = true,
  }: { hosts: Host[]; pinned: string; cols: string; ephemeralTag?: boolean } = $props();
</script>

<div class="list" style="--cols:{cols}">
  <div class="r hd">
    <span>Status</span><span>Name</span><span>OS</span><span>Last seen</span><span>Version</span>
  </div>
  {#each hosts as h (h.id)}
    {@const gone = h.revokedAt ? shortDate(h.revokedAt) : ''}
    <a
      class="r"
      href="/hosts/{h.id}"
      onpointerenter={() => prefetch(`/api/hosts/${h.id}`)}
      onfocus={() => prefetch(`/api/hosts/${h.id}`)}
    >
      <span class="st {STATUS_LABEL[h.status][0]} stc">{STATUS_LABEL[h.status][1]}</span>
      <span class="nm" class:mu={h.status === 'revoked'}
        >{h.name}{#if ephemeralTag && h.ephemeral}<span class="tag">Ephemeral</span>{/if}<span
          class="sub om"
          >{h.os} · {gone ? `revoked ${gone}` : ago(h.lastSeenAt)}{h.outdated && !gone
            ? ' · update available'
            : ''}</span
        ></span
      >
      <span class="mu hm">{h.os}</span>
      <span class="hm" class:mu={gone}>{gone ? `Revoked ${gone}` : ago(h.lastSeenAt)}</span>
      <span class="mu hm"
        >{h.version}{#if h.outdated && !gone}<span
            class="upd"
            role="img"
            aria-label="Update available"
            title="Update available: {pinned}"><Icon name="up" /></span
          >{/if}</span
      >
    </a>
  {/each}
</div>
