<script lang="ts">
  import { ICONS, type IconName } from './icons';

  let { name }: { name: IconName } = $props();
</script>

<svg class="i" viewBox="0 0 24 24" aria-hidden="true">
  {#each ICONS[name] as d (d)}<path {d} />{/each}
</svg>
