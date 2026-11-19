<script lang="ts">
  import { getJSON, postJSON, requireMe, sendJSON, type Me } from '../lib/api';
  import { NewToken, TokenState, type Host } from '../lib/hosts';
  import Icon from '../lib/Icon.svelte';
  import TopBar from '../lib/TopBar.svelte';

  let me: Me | undefined = $state();
  let token: NewToken | undefined = $state();
  let host: Host | undefined = $state();
  let expired = $state(false);
  let os: 'macos' | 'linux' | 'windows' = $state('linux');
  let profile = $state(false);
  let ephemeral = $state(false);
  let copied = $state(false);
  let name = $state('');
  const nameOk = $derived(/^[A-Za-z0-9._-]{1,64}$/.test(name));
  const shellSafeName = $derived(nameOk ? name : '');

  $effect(() => {
    requireMe().then((m) => m && (me = m));
  });

  // A new single-use token per choice: Ephemeral is fixed when the token is made.
  $effect(() => {
    const wantEphemeral = ephemeral;
    token = undefined;
    const made = wantEphemeral
      ? sendJSON('POST', '/api/hosts/tokens', { ephemeral: true }, NewToken).then((r) => r.body)
      : postJSON('/api/hosts/tokens', NewToken);
    let stale = false;
    made.then((t) => !stale && t && (token = t));
    return () => (stale = true);
  });

  $effect(() => {
    if (!token || host || expired) return;
    const id = token.id;
    const timer = setInterval(async () => {
      const s = await getJSON(`/api/hosts/tokens/${id}`, TokenState);
      if (s?.state === 'connected') host = s.host;
      else if (s?.state === 'expired') expired = true;
    }, 2000);
    return () => clearInterval(timer);
  });

  const command = $derived.by(() => {
    if (!token) return '';
    const { baseUrl, token: t } = token;
    if (os === 'windows')
      return `$env:AVT_TOKEN="${t}"; ${shellSafeName ? `$env:AV_HOST_NAME="${shellSafeName}"; ` : ''}irm ${baseUrl}/install/windows.ps1 | iex`;
    const env = `AVT_TOKEN=${t}${shellSafeName ? ` AV_HOST_NAME=${shellSafeName}` : ''}`;
    if (os === 'macos') return `curl -fsSL ${baseUrl}/install/macos.sh | ${env} bash`;
    return `curl -fsSL ${baseUrl}/install/linux.sh | ${env} bash${profile ? ' -s -- --profile' : ''}`;
  });

  async function copy() {
    await navigator.clipboard.writeText(command);
    copied = true;
    setTimeout(() => (copied = false), 2000);
  }
</script>

{#if me}
  <TopBar {me} tab="hosts" />
  <main class="wrap">
    <a class="back" href="/hosts"><Icon name="left" /> Hosts</a>
    <div class="ph">
      <div>
        <h1>Add Host</h1>
        <p class="mu">Install the agent on a machine to send its sessions to your Instance.</p>
      </div>
    </div>
    <ol class="steps">
      <li>
        <h2>Choose the operating system</h2>
        <div class="seg" role="radiogroup" aria-label="Operating system">
          <label
            ><input
              type="radio"
              name="os"
              checked={os === 'macos'}
              onchange={() => (os = 'macos')}
            /><span>macOS</span></label
          >
          <label
            ><input
              type="radio"
              name="os"
              checked={os === 'linux'}
              onchange={() => (os = 'linux')}
            /><span>Linux</span></label
          >
          <label
            ><input
              type="radio"
              name="os"
              checked={os === 'windows'}
              onchange={() => (os = 'windows')}
            /><span>Windows</span></label
          >
        </div>
        {#if os === 'linux'}
          <div style="margin-top: var(--s5)">
            <div
              style="display: grid; gap: var(--s3)"
              role="radiogroup"
              aria-label="How to install on Linux"
            >
              <label class="check"
                ><input
                  type="radio"
                  name="mode"
                  checked={!profile}
                  onchange={() => (profile = false)}
                /><span><b>Install as a system service</b></span></label
              >
              <label class="check"
                ><input
                  type="radio"
                  name="mode"
                  checked={profile}
                  onchange={() => (profile = true)}
                /><span><b>Add to shell profile</b></span></label
              >
            </div>
          </div>
        {/if}
        <div class="field" style="margin-top: var(--s5); max-width: 360px">
          <label class="f" for="hn">Host name <span class="mu">(optional)</span></label>
          <input
            class="input"
            id="hn"
            bind:value={name}
            maxlength="64"
            placeholder="The machine's name"
            aria-describedby="hn-hint"
          />
          <p class="hint" id="hn-hint" style={name && !nameOk ? 'color: var(--danger)' : ''}>
            {name && !nameOk
              ? 'Use 1 to 64 letters, digits, dots, dashes or underscores.'
              : 'Leave blank to use the machine’s name.'}
          </p>
        </div>
        <label class="check" style="margin-top: var(--s5)"
          ><input type="checkbox" bind:checked={ephemeral} /><span
            ><b>Ephemeral Host</b><span class="hint" style="display: block; margin-top: 0"
              >Host is removed automatically after it goes offline, for CI and cloud workspaces.</span
            ></span
          ></label
        >
      </li>
      <li>
        <h2>Run this command on the Host</h2>
        <div class="code">
          <code>{command || 'Preparing your command…'}</code>
          <button class="btn" type="button" onclick={copy} disabled={!token}>
            <Icon name={copied ? 'check' : 'copy'} />
            {copied ? 'Copied' : 'Copy'}
          </button>
        </div>
        <div class="notice info" style="margin-top: var(--s4)">
          <Icon name="info" />
          <div>This command includes a one-time Install Token valid for 1 hour.</div>
        </div>
      </li>
      <li>
        <h2>Wait for the Host to connect</h2>
        {#if host}
          <div style="display: flex; align-items: center; gap: var(--s4); flex-wrap: wrap">
            <span class="st"><b>{host.name} connected</b>&nbsp;Online</span>
            <a class="btn primary" href="/hosts/{host.id}">View Host</a>
          </div>
        {:else if expired}
          <div class="notice bad">
            <Icon name="alert" />
            <div>
              <b>This Install Token has expired.</b>
              <a href="/hosts/new">Add the Host again</a> for a new command.
            </div>
          </div>
        {:else}
          <div class="wait">
            <span class="spin"></span><span>Waiting for the Host to connect…</span>
          </div>
          <p class="hint">It usually takes under a minute.</p>
        {/if}
      </li>
    </ol>
  </main>
{/if}

<style>
  .steps {
    max-width: 720px;
  }
</style>
