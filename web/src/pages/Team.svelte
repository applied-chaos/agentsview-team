<script lang="ts">
  import { showMe, swr, type Me } from '../lib/api';
  import { dateText, num, tokenLabel, placeholders } from '../lib/format';
  import ListSkel from '../lib/ListSkel.svelte';
  import TopBar from '../lib/TopBar.svelte';
  import { TeamData, type Ranked } from '../lib/team';
  import { cellSize, heatmap, STEPS } from '../lib/heatmap';

  let me: Me | undefined = $state();
  let data: TeamData | null | undefined = $state();
  $effect(() => {
    showMe((m) => (me = m));
    swr('/api/team', TeamData, (d) => (data = d));
  });

  const n = num;
  const money = (v: number) => `$${num(Math.round(v))}`;
  const fmt = (iso: string) =>
    dateText(`${iso}T00:00:00Z`, {
      weekday: 'short',
      day: 'numeric',
      month: 'short',
      timeZone: 'UTC',
    });
  const org = $derived(data?.org);
  const you = $derived(data?.you);
  const max = $derived(Math.max(1, ...(org?.daily.map((d) => d.tokens) ?? [])));
  const weekend = (iso: string) => [0, 6].includes(new Date(`${iso}T00:00:00Z`).getUTCDay());
  let chartH = $state(0);
  let hover: string | null = $state(null);
  const heat = $derived(heatmap(org?.daily ?? []));
  const blanks = $derived(Array.from({ length: heat.blanks }, (_, i) => i));
  const levels = Array.from({ length: STEPS }, (_, i) => i + 1);
  const cell = $derived(cellSize(chartH, heat.rows));
  const hoverDay = $derived(org?.daily.find((d) => d.date === hover));
  const over = (e: Event) => {
    hover = (e.target as HTMLElement).dataset.date ?? null;
  };
  const figs = $derived(
    org
      ? [
          [n(org.sessions), 'sessions', you && n(you.sessions)],
          [tokenLabel(org.tokensIn), 'tokens in', you && tokenLabel(you.tokensIn)],
          [tokenLabel(org.tokensOut), 'tokens out', you && tokenLabel(you.tokensOut)],
          [money(org.costUsd), 'estimated cost', you && money(you.costUsd)],
          [n(org.activeUsers), 'active Users', null],
          [n(org.activeHosts), 'active Hosts', you && n(you.activeHosts)],
        ]
      : [],
  );
</script>

{#snippet ranked(id: string, title: string, rows: Ranked[])}
  <section aria-labelledby={id}>
    <div class="sec"><h2 {id}>{title}</h2></div>
    <div class="list" style="--cols:minmax(0,1.6fr) 90px 90px 70px">
      <div class="r hd">
        <span>Name</span><span class="hm end">Sessions</span><span class="hm end">Tokens</span><span
          class="end">Share</span
        >
      </div>
      {#each rows as r (r.name)}
        <div class="r">
          <span class="nm">{r.name}</span><span class="hm end">{n(r.sessions)}</span><span
            class="hm end">{tokenLabel(r.tokens)}</span
          ><span class="end">{r.share}%</span>
        </div>
      {/each}
    </div>
  </section>
{/snippet}

<TopBar {me} tab="team" />
<main class="wrap" style="--gap:var(--s7)">
  <div class="ph">
    <div>
      <h1>Team</h1>
      <p class="mu">Everyone, last 30 days. Figures are never broken down by person.</p>
    </div>
  </div>
  {#if data === null}
    <p class="mu">The Team page is not available right now. Try again in a few minutes.</p>
  {:else if !org}
    <div class="band" style="--n:6" aria-busy="true" aria-label="Loading">
      {#each placeholders(6) as i (i)}
        <div><span class="sk fig"></span></div>
      {/each}
    </div>
    <div class="sec" style="margin-top:var(--s7)"><span class="sk" style="width:140px"></span></div>
    <span class="sk" style="height:180px"></span>
    <div class="cols2" style="margin-top:var(--s7)">
      {#each placeholders(2) as i (i)}
        <ListSkel cols="minmax(0,1.6fr) 90px 90px 70px" rows={5} />
      {/each}
    </div>
  {:else if org}
    <div class="band" style="--n:6">
      {#each figs as [fig, label, own] (label)}
        <div>
          <span class="fig">{fig}</span><span class="lbl">{label}</span>{#if own}<span class="you"
              >You: {own}</span
            >{/if}
        </div>
      {/each}
    </div>
    <section aria-labelledby="tpd" style="margin-top:var(--s7)">
      <div class="sec"><h2 id="tpd">Tokens per day</h2></div>
      <div class="heat">
        <div>
          <div
            class="chart"
            role="img"
            aria-label="Tokens per day, last 30 days"
            bind:offsetHeight={chartH}
            onpointerover={over}
            onmouseleave={() => (hover = null)}
          >
            {#each org.daily as d (d.date)}
              <i
                class:we={weekend(d.date)}
                class:on={hover === d.date}
                data-date={d.date}
                style="height:{((d.tokens / max) * 100).toFixed(1)}%"
                title="{fmt(d.date)}: {tokenLabel(d.tokens)} tokens, {num(
                  d.sessions,
                )} sessions, ${d.cost.toFixed(2)}, {num(d.activeUsers)} active Users, {num(
                  d.activeHosts,
                )} active Hosts"
              ></i>
            {/each}
          </div>
          <div class="chart-x">
            <span>{fmt(org.daily[0]?.date ?? '')}</span><span>{fmt(org.daily[15]?.date ?? '')}</span
            ><span>Today</span>
          </div>
        </div>
        <div>
          <div
            class="hmap"
            role="img"
            aria-label="Tokens per day calendar, last 30 days"
            style="--cell:{cell}px"
            onpointerover={over}
            onmouseleave={() => (hover = null)}
          >
            {#each ['M', 'T', 'W', 'T', 'F', 'S', 'S'] as w, i (i)}<div
                class="h"
                aria-hidden="true"
              >
                {w}
              </div>{/each}
            {#each blanks as i (i)}<div class="c out"></div>{/each}
            {#each heat.cells as c (c.date)}
              <div
                class="c h{c.level}"
                class:on={hover === c.date}
                data-date={c.date}
                title="{fmt(c.date)}: {tokenLabel(c.tokens)} tokens"
              ></div>
            {/each}
          </div>
          <div class="hmap-leg" aria-hidden="true">
            Less <i></i>{#each levels as k (k)}<i class="h{k}"></i>{/each} More
          </div>
        </div>
      </div>
      <div class="heat-tip" aria-live="polite">
        {#if hoverDay}{fmt(hoverDay.date)}: {tokenLabel(hoverDay.tokens)} tokens{/if}
      </div>
    </section>
    <div class="cols2" style="margin-top:var(--s7)">
      {@render ranked('tm', 'Top models', org.models)}
      {@render ranked('ta', 'Top agents', org.agents)}
    </div>
  {/if}
</main>

<style>
  .cols2 {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--s8);
  }
  @media (max-width: 1000px) {
    .cols2 {
      grid-template-columns: 1fr;
      gap: var(--s7);
    }
  }
</style>
