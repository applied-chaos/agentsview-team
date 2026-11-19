<script lang="ts">
  import { HttpError, showMe, sendJSON, swr, type Me } from '../lib/api';
  import Calendar from '../lib/Calendar.svelte';
  import { ago, num, tokenLabel, placeholders } from '../lib/format';
  import { day, HostDetail, INSTALL_MODE } from '../lib/hosts';
  import Icon from '../lib/Icon.svelte';
  import { STATUS_LABEL } from '../lib/status';
  import TopBar from '../lib/TopBar.svelte';

  const id = location.pathname.split('/').filter(Boolean)[1];
  let me: Me | undefined = $state();
  let d: HostDetail | undefined = $state();
  let missing = $state(false);
  let renameDlg: HTMLDialogElement | undefined = $state();
  let revokeDlg: HTMLDialogElement | undefined = $state();
  let deleteDlg: HTMLDialogElement | undefined = $state();
  let newName = $state('');
  let error = $state('');
  let busy = $state(false);
  let copied = $state('');

  async function load() {
    try {
      await swr(
        `/api/hosts/${id}`,
        HostDetail,
        (v) => (d = v),
        (v) => v.usageStale,
      );
    } catch (e) {
      if (e instanceof HttpError && e.status === 404) missing = true;
      else throw e;
    }
  }
  $effect(() => {
    showMe((m) => (me = m));
    load();
  });

  function openRename() {
    newName = d!.host.name;
    error = '';
    renameDlg?.showModal();
  }

  async function rename(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    const res = await sendJSON('PATCH', `/api/hosts/${id}`, { name: newName.trim() });
    busy = false;
    if (res.status === 204) {
      renameDlg?.close();
      await load();
    } else if (res.status === 409) {
      error = 'Another Host already has that name.';
    } else if (res.status === 400) {
      error = 'Use 1 to 64 letters, digits, dots, dashes or underscores.';
    } else {
      error = 'Could not rename the Host. Try again in a minute.';
    }
  }

  async function revoke() {
    busy = true;
    const res = await sendJSON('POST', `/api/hosts/${id}/revoke`, {});
    busy = false;
    if (res.status === 204) {
      revokeDlg?.close();
      await load();
    } else {
      error = 'Could not revoke the Host. Try again in a minute.';
    }
  }

  async function remove() {
    busy = true;
    const res = await sendJSON('DELETE', `/api/hosts/${id}`, {});
    busy = false;
    if (res.status === 204 || res.status === 404) location.assign('/hosts');
    else error = 'Could not delete the Host. Try again in a minute.';
  }

  async function copy(key: string, text: string) {
    await navigator.clipboard.writeText(text);
    copied = key;
    setTimeout(() => (copied = ''), 2000);
  }

  const closeOnBackdrop = (dlg: HTMLDialogElement | undefined) => (e: MouseEvent) =>
    e.target === dlg && dlg?.close();
</script>

<TopBar {me} tab="hosts" />
{#if missing}
  <main class="wrap">
    <a class="back" href="/hosts"><Icon name="left" /> Hosts</a>
    <div class="ph"><h1>Host not found</h1></div>
  </main>
{:else if !d}
  <main class="wrap" aria-busy="true" aria-label="Loading">
    <a class="back" href="/hosts"><Icon name="left" /> Hosts</a>
    <div class="ph">
      <div>
        <span class="sk h1"></span>
        <p style="margin-top: var(--s2)"><span class="sk" style="width: 80px"></span></p>
      </div>
    </div>
    <div class="rail">
      <dl class="kv">
        {#each placeholders(8) as i (i)}
          <dt><span class="sk" style="width: 70px"></span></dt>
          <dd><span class="sk" style="width: 120px"></span></dd>
        {/each}
      </dl>
      <aside aria-label="Host usage">
        <div class="figs">
          {#each placeholders(2) as i (i)}
            <div><span class="sk fig"></span></div>
          {/each}
        </div>
      </aside>
    </div>
  </main>
{:else}
  {@const h = d.host}
  {@const revoked = h.status === 'revoked'}
  <main class="wrap">
    <a class="back" href="/hosts"><Icon name="left" /> Hosts</a>
    <div class="ph">
      <div>
        <h1>{h.name}</h1>
        <p style="margin-top: var(--s2)">
          <span class="st {STATUS_LABEL[h.status][0]}">{STATUS_LABEL[h.status][1]}</span>
        </p>
      </div>
      {#if !revoked}
        <div class="acts">
          <button class="btn" type="button" onclick={openRename}>Rename</button>
        </div>
      {/if}
    </div>
    {#if h.outdated && !revoked}
      <div class="notice" style="margin-bottom: var(--s7)">
        <Icon name="alert" />
        <div style="flex: 1; min-width: 0">
          <b>Update available: {d.pinnedVersion}</b>Run the install command again on this Host to
          update it.
          <div class="code" style="margin-top: var(--s3); background: var(--surface)">
            <code>{d.updateCommand}</code>
            <button class="btn" type="button" onclick={() => copy('update', d!.updateCommand)}>
              <Icon name={copied === 'update' ? 'check' : 'copy'} />
              {copied === 'update' ? 'Copied' : 'Copy'}
            </button>
          </div>
        </div>
      </div>
    {/if}
    <div class="rail">
      <dl class="kv">
        <dt>Status</dt>
        <dd>
          <span class="st {STATUS_LABEL[h.status][0]}">{STATUS_LABEL[h.status][1]}</span>
        </dd>
        <dt>OS</dt>
        <dd>{h.os}</dd>
        <dt>Version</dt>
        <dd>
          {h.version}{#if h.outdated && !revoked}<span
              class="upd"
              role="img"
              aria-label="Update available"
              title="Update available: {d.pinnedVersion}"><Icon name="up" /></span
            >{/if}
        </dd>
        <dt>Install mode</dt>
        <dd>{INSTALL_MODE[h.installMode].label}</dd>
        <dt>Ephemeral</dt>
        <dd>{h.ephemeral ? 'Yes' : 'No'}</dd>
        <dt>Last seen</dt>
        <dd>{ago(h.lastSeenAt)}</dd>
        <dt>Added</dt>
        <dd>{day(h.createdAt)}</dd>
        {#if h.revokedAt}
          <dt>Revoked</dt>
          <dd>{day(h.revokedAt)}</dd>
        {/if}
        <dt>Last upload</dt>
        <dd>{ago(h.lastUploadAt)}</dd>
      </dl>
      <aside aria-label="Host usage">
        <div class="figs">
          <div>
            <span class="fig">{d.usage ? num(d.usage.sessions) : '–'}</span><span class="lbl"
              >sessions, last 30 days</span
            >
          </div>
          <div>
            <span class="fig">{d.usage ? tokenLabel(d.usage.tokens) : '–'}</span><span class="lbl"
              >tokens, last 30 days</span
            >
          </div>
        </div>
        {#if d.usage}
          <div style="margin-top: var(--s6)">
            <Calendar from={d.usage.from} to={d.usage.to} daily={d.usage.daily} />
          </div>
        {/if}
      </aside>
    </div>
    {#if !revoked}
      <section style="margin-top: var(--s8)" aria-labelledby="re-h">
        <div class="sec" style="margin-top: 0"><h2 id="re-h">Reinstall or update</h2></div>
        <p class="mu" style="margin-bottom: var(--s4)">
          Run this on the Host to reinstall or update it. It keeps the Host's name, Credential and
          history.
        </p>
        <div class="code">
          <code>{d.updateCommand}</code>
          <button class="btn" type="button" onclick={() => copy('update', d!.updateCommand)}>
            <Icon name={copied === 'update' ? 'check' : 'copy'} />
            {copied === 'update' ? 'Copied' : 'Copy'}
          </button>
        </div>
      </section>
    {/if}
    <section style="margin-top: var(--s8)" aria-labelledby="un-h">
      <div class="sec" style="margin-top: 0"><h2 id="un-h">Uninstall</h2></div>
      <p class="mu" style="margin-bottom: var(--s4)">
        Run this on the Host to stop uploading and remove agentsview from it.
        {#if INSTALL_MODE[d.host.installMode].purge}Add <code>-s -- --purge</code> to delete its local
          data too.{/if}
        This does not revoke the Host.
      </p>
      <div class="code">
        <code>{d.uninstallCommand}</code>
        <button class="btn" type="button" onclick={() => copy('uninstall', d!.uninstallCommand)}>
          <Icon name={copied === 'uninstall' ? 'check' : 'copy'} />
          {copied === 'uninstall' ? 'Copied' : 'Copy'}
        </button>
      </div>
    </section>
    <section class="danger-zone" aria-labelledby="dz-h" style="margin-top: var(--s8)">
      <div class="sec" style="margin-top: 0"><h2 id="dz-h">Danger zone</h2></div>
      {#if !revoked}
        <div class="r">
          <div>
            <b>Revoke Host</b>
            <p class="mu">
              Stops this Host uploading and invalidates its credential. Existing sessions stay in
              your Instance.
            </p>
          </div>
          <button
            class="btn danger"
            type="button"
            onclick={() => {
              error = '';
              revokeDlg?.showModal();
            }}>Revoke Host</button
          >
        </div>
      {/if}
      <div class="r">
        <div>
          <b>Delete Host</b>
          <p class="mu">
            Revokes this Host and removes it from your Hosts. Existing sessions stay in your
            Instance.
          </p>
        </div>
        <button
          class="btn danger"
          type="button"
          onclick={() => {
            error = '';
            deleteDlg?.showModal();
          }}>Delete Host</button
        >
      </div>
    </section>

    <dialog
      class="dlg"
      bind:this={renameDlg}
      aria-labelledby="rn-h"
      onclick={closeOnBackdrop(renameDlg)}
    >
      <form class="box" onsubmit={rename}>
        <div class="dh">
          <h2 id="rn-h">Rename {h.name}</h2>
          <button class="x" type="button" aria-label="Close" onclick={() => renameDlg?.close()}
            ><Icon name="x" /></button
          >
        </div>
        <div class="body">
          <div class="field">
            <label class="f" for="rn">Name</label>
            <input class="input" id="rn" bind:value={newName} maxlength="64" required />
            <p class="hint">Names are unique among your active Hosts.</p>
            {#if error}<p class="hint" role="alert" style="color: var(--danger)">{error}</p>{/if}
          </div>
        </div>
        <div class="df">
          <button class="btn" type="button" onclick={() => renameDlg?.close()}>Cancel</button>
          <button class="btn primary" type="submit" disabled={busy}>Rename</button>
        </div>
      </form>
    </dialog>

    <dialog
      class="dlg"
      bind:this={revokeDlg}
      aria-labelledby="r-h"
      onclick={closeOnBackdrop(revokeDlg)}
    >
      <div class="box">
        <div class="dh">
          <h2 id="r-h">Revoke {h.name}?</h2>
          <button class="x" type="button" aria-label="Close" onclick={() => revokeDlg?.close()}
            ><Icon name="x" /></button
          >
        </div>
        <div class="body">
          <p>
            {h.name} will stop uploading and its credential will stop working. Sessions it already uploaded
            stay in your Instance.
          </p>
          <p class="mu">To use this machine again, add it as a new Host.</p>
          {#if error}<p role="alert" style="color: var(--danger)">{error}</p>{/if}
        </div>
        <div class="df">
          <button class="btn" type="button" onclick={() => revokeDlg?.close()}>Cancel</button>
          <button class="btn danger solid" type="button" onclick={revoke} disabled={busy}
            >Revoke Host</button
          >
        </div>
      </div>
    </dialog>

    <dialog
      class="dlg"
      bind:this={deleteDlg}
      aria-labelledby="d-h"
      onclick={closeOnBackdrop(deleteDlg)}
    >
      <div class="box">
        <div class="dh">
          <h2 id="d-h">Delete {h.name}?</h2>
          <button class="x" type="button" aria-label="Close" onclick={() => deleteDlg?.close()}
            ><Icon name="x" /></button
          >
        </div>
        <div class="body">
          <p>
            {h.name} will stop uploading and disappear from your Hosts. Sessions it already uploaded stay
            in your Instance.
          </p>
          <p class="mu">This cannot be undone. To use this machine again, add it as a new Host.</p>
          {#if error}<p role="alert" style="color: var(--danger)">{error}</p>{/if}
        </div>
        <div class="df">
          <button class="btn" type="button" onclick={() => deleteDlg?.close()}>Cancel</button>
          <button class="btn danger solid" type="button" onclick={remove} disabled={busy}
            >Delete Host</button
          >
        </div>
      </div>
    </dialog>
  </main>
{/if}
