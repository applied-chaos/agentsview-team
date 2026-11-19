<script lang="ts">
  import { placeholders } from './format';
  let { cols, heads = [], rows = 4 }: { cols: string; heads?: string[]; rows?: number } = $props();
  const cells = $derived(heads.length || cols.split(' ').length);
</script>

<div class="list" style="--cols:{cols}" aria-busy="true" aria-label="Loading">
  {#if heads.length}
    <div class="r hd">
      {#each heads as h (h)}<span>{h}</span>{/each}
    </div>
  {/if}
  {#each placeholders(rows) as i (i)}
    <div class="r">
      {#each placeholders(cells) as j (j)}<span
          class="sk"
          class:hm={j > 1}
          style="width:{j === 1 ? 70 : 55}%"
        ></span>{/each}
    </div>
  {/each}
</div>
