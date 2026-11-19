<script lang="ts">
  import { Denied, getJSON } from '../lib/api';
  import { describeRule, PublicConfig } from '../lib/rule';

  let cfg: PublicConfig | null | undefined = $state();
  let who: Denied | null | undefined = $state();
  $effect(() => {
    getJSON('/api/public', PublicConfig).then((c) => (cfg = c));
    getJSON('/api/denied', Denied).then((d) => (who = d));
  });
</script>

<main class="center">
  <div class="box">
    <div class="logo" role="img" aria-label="AgentsView Teams"></div>
    <h1>This account can't use AgentsView Teams</h1>
    {#if cfg}
      <p>{who ? `You signed in as ${who.email}. ` : ''}Only {describeRule(cfg)} are allowed.</p>
    {/if}
    <a class="btn big" href="/sign-in">Sign in with a different account</a>
    <p class="foot">If you think this is wrong, ask an Admin.</p>
  </div>
</main>
