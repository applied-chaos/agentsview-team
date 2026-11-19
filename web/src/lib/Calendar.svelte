<script lang="ts">
  import { dateText, tokenLabel } from './format';

  let { from, to, daily }: { from: string; to: string; daily: { date: string; tokens: number }[] } =
    $props();

  const DAY = 86400000;
  const fmt = (d: Date) =>
    dateText(d, {
      weekday: 'short',
      day: 'numeric',
      month: 'short',
      timeZone: 'UTC',
    });

  const days = $derived.by(() => {
    const byDate = new Map(daily.map((d) => [d.date, d.tokens]));
    const start = Date.parse(`${from}T00:00:00Z`);
    const n = Math.round((Date.parse(`${to}T00:00:00Z`) - start) / DAY) + 1;
    return Array.from({ length: n }, (_, i) => {
      const d = new Date(start + i * DAY);
      return { d, tokens: byDate.get(d.toISOString().slice(0, 10)) ?? 0 };
    });
  });
  const max = $derived(Math.max(1, ...days.map((x) => x.tokens)));
  const pad = $derived(((days[0]?.d.getUTCDay() ?? 1) + 6) % 7);
  const level = (t: number) => (t ? Math.min(4, Math.ceil((t / max) * 4)) : 0);
  const label = (t: number) => (t ? `${tokenLabel(t)} tokens` : 'no activity');
</script>

<div class="cal-wd" aria-hidden="true">
  {#each ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'] as w (w)}<span>{w}</span>{/each}
</div>
<div class="cal" role="img" aria-label="Tokens per day, last 30 days">
  {#each Array.from({ length: pad }, (_, i) => i) as p (p)}<i class="pad"></i>{/each}
  {#each days as x, i (x.d.getTime())}
    <i
      class="l{level(x.tokens)}"
      class:today={i === days.length - 1}
      title="{fmt(x.d)}: {label(x.tokens)}"
    ></i>
  {/each}
</div>
<div class="legend">
  <span>Tokens per day</span>Less <i></i><i class="l1"></i><i class="l2"></i><i class="l3"></i><i
    class="l4"
  ></i> More
</div>
