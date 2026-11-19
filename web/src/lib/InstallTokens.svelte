<script lang="ts">
  import { clearCache, sendJSON, swr } from './api';
  import { ago, shortDate } from './format';
  import { CreatedToken, expiry, InstallTokens, uses, type InstallToken } from './hosts';
  import Icon from './Icon.svelte';

  const EXPIRIES = [
    ['1h', '1 hour'],
    ['1d', '1 day'],
    ['7d', '7 days'],
    ['30d', '30 days'],
    ['never', 'Never'],
  ] as const;
  type Expiry = (typeof EXPIRIES)[number][0];

  let tokens: InstallToken[] = $state([]);
  let form: HTMLDialogElement | undefined = $state();
  let shown: HTMLDialogElement | undefined = $state();
  let description = $state('');
  let expiresIn: Expiry = $state('7d');
  let reusable = $state(false);
  let ephemeral = $state(false);
  let failed = $state('');
  let secret = $state('');
  let validFor = $state('7 days');
  let copied = $state(false);

  const load = () => swr('/api/hosts/tokens', InstallTokens, (d) => (tokens = d.tokens));
  $effect(() => {
    load();
  });

  function openForm() {
    description = '';
    expiresIn = '7d';
    reusable = ephemeral = false;
    failed = '';
    form?.showModal();
  }

  async function create(e: SubmitEvent) {
    e.preventDefault();
    const res = await sendJSON(
      'POST',
      '/api/hosts/tokens',
      { description, reusable, ephemeral, expiresIn },
      CreatedToken,
    );
    if (res.status !== 201 || !res.body) {
      failed = 'Could not create the Install Token. Try again.';
      return;
    }
    secret = res.body.token;
    validFor = EXPIRIES.find(([k]) => k === expiresIn)?.[1] ?? '';
    copied = false;
    form?.close();
    shown?.showModal();
    load();
  }

  async function revoke(id: string) {
    await fetch(`/api/hosts/tokens/${id}/revoke`, { method: 'POST' });
    clearCache();
    load();
  }

  async function copy() {
    await navigator.clipboard.writeText(secret);
    copied = true;
  }
</script>

<div class="sec" style="margin-top:var(--s8)">
  <h2>Install Tokens</h2>
  <button class="btn" type="button" onclick={openForm}>New Install Token</button>
</div>
<p class="mu" style="margin:calc(-1 * var(--s2)) 0 var(--s4)">
  Tokens let a machine add itself as a Host. Revoke any you no longer need.
</p>
{#if tokens.length}
  <div class="list" style="--cols:minmax(0,1.6fr) 110px 110px 150px 100px 90px">
    <div class="r hd">
      <span>Description</span><span>Created</span><span>Expires</span><span>Uses</span><span
        >Ephemeral</span
      ><span></span>
    </div>
    {#each tokens as t (t.id)}
      <div class="r">
        <span class="nm"
          >{t.description || 'Add Host command'}<span class="sub om"
            >Expires {expiry(t.expiresAt)} · {uses(t)} uses</span
          >{#if t.lastUsedAt}<span class="sub">Last used {ago(t.lastUsedAt)}</span>{/if}</span
        >
        <span class="hm">{shortDate(t.createdAt)}</span>
        <span class="hm">{expiry(t.expiresAt)}</span>
        <span class="hm">{uses(t)}</span>
        <span class="hm">{t.ephemeral ? 'Yes' : 'No'}</span>
        <span class="end"
          ><button class="btn danger" type="button" onclick={() => revoke(t.id)}>Revoke</button
          ></span
        >
      </div>
    {/each}
  </div>
{:else}
  <p class="mu">No Install Tokens.</p>
{/if}

<dialog
  class="dlg"
  bind:this={form}
  aria-labelledby="t-h"
  onclick={(e) => e.target === form && form?.close()}
>
  <form class="box" onsubmit={create}>
    <div class="dh">
      <h2 id="t-h">New Install Token</h2>
      <button class="x" type="button" aria-label="Close" onclick={() => form?.close()}
        ><Icon name="x" /></button
      >
    </div>
    <div class="body">
      <div class="field">
        <label class="f" for="t-d">Description</label>
        <input
          class="input"
          id="t-d"
          maxlength="100"
          placeholder="e.g. CI runners"
          bind:value={description}
        />
      </div>
      <div class="field">
        <label class="f" for="t-e">Expires</label>
        <select class="input" id="t-e" bind:value={expiresIn}>
          {#each EXPIRIES as [k, label] (k)}<option value={k}>{label}</option>{/each}
        </select>
      </div>
      <label class="check"
        ><input type="checkbox" bind:checked={reusable} /><span
          ><b>Reusable</b><span class="hint" style="display:block;margin-top:0"
            >Lets more than one machine use this token.</span
          ></span
        ></label
      >
      <label class="check"
        ><input type="checkbox" bind:checked={ephemeral} /><span
          ><b>Ephemeral Hosts</b><span class="hint" style="display:block;margin-top:0"
            >Hosts are revoked automatically after 30 days unseen.</span
          ></span
        ></label
      >
      {#if failed}<p class="mu" role="alert">{failed}</p>{/if}
    </div>
    <div class="df">
      <button class="btn" type="button" onclick={() => form?.close()}>Cancel</button>
      <button class="btn primary" type="submit">Create Install Token</button>
    </div>
  </form>
</dialog>

<dialog
  class="dlg"
  bind:this={shown}
  aria-labelledby="c-h"
  onclick={(e) => e.target === shown && shown?.close()}
  onclose={() => (secret = '')}
>
  <div class="box">
    <div class="dh">
      <h2 id="c-h">Install Token created</h2>
      <button class="x" type="button" aria-label="Close" onclick={() => shown?.close()}
        ><Icon name="x" /></button
      >
    </div>
    <div class="body">
      <div class="notice">
        <Icon name="alert" />
        <div>
          <b>Copy this token now. You will not see it again.</b>{validFor === 'Never'
            ? 'It never expires.'
            : `It is valid for ${validFor}.`}
        </div>
      </div>
      <div class="code">
        <code>{secret}</code>
        <button class="btn" type="button" onclick={copy}
          ><Icon name={copied ? 'check' : 'copy'} /> {copied ? 'Copied' : 'Copy'}</button
        >
      </div>
    </div>
    <div class="df">
      <button class="btn primary" type="button" onclick={() => shown?.close()}>Done</button>
    </div>
  </div>
</dialog>
