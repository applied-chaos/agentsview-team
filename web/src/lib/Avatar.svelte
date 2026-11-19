<script lang="ts">
  import { gravatarUrl, initialsOf } from './avatar';

  let {
    name,
    email,
    size = 26,
  }: { name: string; email?: string | undefined; size?: number } = $props();

  let src: string | undefined = $state();
  let loaded = $state(false);

  $effect(() => {
    const e = email;
    src = undefined;
    loaded = false;
    if (e) void gravatarUrl(e, size * 2).then((u) => (src = u));
  });
</script>

<span class="av">
  {initialsOf(name)}
  {#if src}
    <img
      {src}
      alt=""
      hidden={!loaded}
      onload={() => (loaded = true)}
      onerror={() => (src = undefined)}
    />
  {/if}
</span>
