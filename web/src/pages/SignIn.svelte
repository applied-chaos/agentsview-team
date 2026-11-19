<script lang="ts">
  import { clearCache, getJSON } from '../lib/api';
  import { describeRule, PublicConfig } from '../lib/rule';

  let cfg: PublicConfig | null | undefined = $state();
  $effect(() => {
    clearCache();
    getJSON('/api/public', PublicConfig).then((c) => (cfg = c));
  });
</script>

<main class="center">
  <div class="box">
    <div class="logo" role="img" aria-label="AgentsView Teams"></div>
    <h1>Sign in to AgentsView Teams</h1>
    {#if cfg}
      <p>Use your {cfg.providerName} account.</p>
      <a class="btn primary big" href="/auth/login">Continue with {cfg.providerName} SSO</a>
      <p class="foot">
        {describeRule(cfg)
          ? `Access is limited to ${describeRule(cfg)}.`
          : `Any ${cfg.providerName} account can sign in.`}
      </p>
    {/if}
  </div>
</main>
