<script lang="ts">
  import AdminGate from '../lib/AdminGate.svelte';
  import {
    AdminUserDetail,
    calendar,
    formatBytes,
    formatFullDateTime,
    formatJoined,
  } from '../lib/admin';
  import { clearHash, getJSON, sendJSON } from '../lib/api';
  import { ago, num, shortDate, tokenLabel } from '../lib/format';
  import { expiry, uses } from '../lib/hosts';
  import Icon from '../lib/Icon.svelte';
  import { object, optional, string } from '../lib/parse';
  import { STATUS_LABEL } from '../lib/status';

  const RoleReply = object({ error: optional(string) });

  const id = location.pathname.split('/').pop() ?? '';
  const now = new Date();

  let user: AdminUserDetail | null | undefined = $state();
  let error = $state('');
  const load = () => getJSON(`/api/admin/users/${id}`, AdminUserDetail).then((u) => (user = u));
  $effect(() => {
    load().catch(() => (user = null));
  });

  async function setAdmin(admin: boolean) {
    error = '';
    const res = await sendJSON('PUT', `/api/admin/users/${id}/role`, { admin }, RoleReply);
    if (res.status === 204) await load();
    else if (res.body?.error === 'last_admin')
      error = 'This is the last Admin. Make someone else an Admin first.';
    else error = 'Could not change the role. Try again.';
  }

  async function impersonate() {
    error = '';
    const res = await sendJSON('POST', `/auth/impersonate/${id}`, {});
    if (res.status === 204) location.assign('/');
    else error = 'Could not start Impersonation. Try again.';
  }

  let disableDlg: HTMLDialogElement | undefined = $state();
  let deleteDlg: HTMLDialogElement | undefined = $state();
  let confirmText = $state('');
  let busy = $state(false);
  const firstName = $derived(user?.name.split(' ')[0] ?? '');
  const matches = $derived(confirmText.trim().toLowerCase() === user?.email.toLowerCase());

  const openFromHash = () => {
    if (location.hash === '#dlg-disable' && !disableDlg?.open) disableDlg?.showModal();
    if (location.hash === '#dlg-delete' && !deleteDlg?.open) deleteDlg?.showModal();
  };
  $effect(() => {
    if (user) openFromHash();
  });

  async function act(path: string, body: unknown, failure: string) {
    busy = true;
    error = '';
    const res = await sendJSON('POST', `/api/admin/users/${id}/${path}`, body);
    busy = false;
    disableDlg?.close();
    deleteDlg?.close();
    if (res.status !== 204) {
      error = failure;
      return false;
    }
    return true;
  }
  const disable = async () => {
    if (await act('disable', {}, 'Could not disable the User. Try again.')) await load();
  };
  const enable = async () => {
    if (await act('enable', {}, 'Could not enable the User. Try again.')) await load();
  };
  const remove = async () => {
    if (await act('delete', { confirm: confirmText }, 'Could not delete the User. Try again.'))
      location.assign('/admin/users');
  };

  const online = $derived(user?.hostList.filter((h) => h.online).length ?? 0);
  const cal = $derived(calendar(user?.usage.daily ?? [], now));
</script>

<svelte:window onhashchange={openFromHash} />

<AdminGate>
  <main class="wrap">
    <a class="back" href="/admin/users"><Icon name="left" /> Users</a>
    {#if user}
      {@const [cls, label] = STATUS_LABEL[user.status]}
      <div class="ph">
        <div>
          <h1>
            {user.name}
            <span class="st {cls}" style="font-size:var(--fs);font-weight:400;margin-left:var(--s3)"
              >{label}</span
            >
          </h1>
          <p class="mu">{user.email}</p>
        </div>
        <div class="acts">
          <a class="btn" href="/i/{user.id}/"><Icon name="eye" /> Open Instance</a>
          <button class="btn" disabled={user.status === 'disabled'} onclick={impersonate}
            ><Icon name="shield" /> Impersonate</button
          >
        </div>
      </div>
      <div class="rail">
        <div>
          <dl class="kv">
            <dt>Role</dt>
            <dd>
              {user.role === 'admin' ? 'Admin' : 'User'}
              <button
                class="btn"
                style="margin-left:var(--s3);min-height:28px;padding:2px 10px;font-size:var(--fs-sm)"
                onclick={() => setAdmin(user?.role !== 'admin')}
              >
                {user.role === 'admin' ? 'Remove Admin' : 'Make Admin'}
              </button>
              {#if error}<span
                  role="alert"
                  class="mu"
                  style="margin-left:var(--s3);color:var(--danger)">{error}</span
                >{/if}
            </dd>
            <dt>Joined</dt>
            <dd>{formatJoined(user.joined)}</dd>
            <dt>Last sign-in</dt>
            <dd>{user.lastActive ? formatFullDateTime(user.lastActive) : 'Never'}</dd>
            <dt>Instance</dt>
            <dd>
              {#if user.instance}
                <span class="st {user.instance.status === 'failed' ? 'bad' : ''}"
                  >{user.instance.status.charAt(0).toUpperCase() +
                    user.instance.status.slice(1)}</span
                >
                <span class="mu">· agentsview {user.instance.version}</span>
              {:else}
                <span class="mu">{user.status === 'disabled' ? 'Stopped' : 'Not started'}</span>
              {/if}
            </dd>
            <dt>Storage</dt>
            <dd>{formatBytes(user.storageBytes)}</dd>
          </dl>
          <section aria-labelledby="h-h" style="margin-top:var(--s7)">
            <div class="sec">
              <h2 id="h-h">
                Hosts
                {#if user.hostList.length}<span class="mu"
                    >{online} of {user.hostList.length} online</span
                  >{/if}
              </h2>
            </div>
            {#if user.hostList.length}
              <div class="list" style="--cols:100px minmax(0,1.3fr) minmax(0,1.1fr) 120px">
                <div class="r hd">
                  <span>Status</span><span>Name</span><span>OS</span><span>Last seen</span>
                </div>
                {#each user.hostList as h (h.id)}
                  <div class="r">
                    <span class="st stc {h.online ? '' : 'off'}"
                      >{h.online ? 'Online' : 'Offline'}</span
                    >
                    <span class="nm"
                      >{h.name}<span class="sub om">{h.os} · {ago(h.lastSeen, now, 'admin')}</span
                      ></span
                    >
                    <span class="mu hm">{h.os}</span>
                    <span class="hm">{ago(h.lastSeen, now, 'admin')}</span>
                  </div>
                {/each}
              </div>
            {:else}
              <p class="mu">No Hosts yet.</p>
            {/if}
          </section>
          <section aria-labelledby="t-h" style="margin-top:var(--s7)">
            <div class="sec"><h2 id="t-h">Install Tokens</h2></div>
            {#if user.installTokens.length}
              <div class="list" style="--cols:minmax(0,1.6fr) 110px 110px 150px 100px">
                <div class="r hd">
                  <span>Description</span><span>Created</span><span>Expires</span><span>Uses</span
                  ><span>Ephemeral</span>
                </div>
                {#each user.installTokens as t (t.id)}
                  <div class="r">
                    <span class="nm"
                      >{t.description || 'Add Host command'}<span class="sub om"
                        >Expires {expiry(t.expiresAt)} · {uses(t)} uses</span
                      >{#if t.lastUsedAt}<span class="sub">Last used {ago(t.lastUsedAt)}</span
                        >{/if}</span
                    >
                    <span class="hm">{shortDate(t.createdAt)}</span>
                    <span class="hm">{expiry(t.expiresAt)}</span>
                    <span class="hm">{uses(t)}</span>
                    <span class="hm">{t.ephemeral ? 'Yes' : 'No'}</span>
                  </div>
                {/each}
              </div>
            {:else}
              <p class="mu">No Install Tokens.</p>
            {/if}
          </section>
        </div>
        <aside aria-label="Usage, last 30 days">
          <div class="figs">
            <div>
              <span class="fig">{num(user.usage.sessions)}</span><span class="lbl"
                >sessions, last 30 days</span
              >
            </div>
            <div>
              <span class="fig">{tokenLabel(user.usage.tokens)}</span><span class="lbl">tokens</span
              >
            </div>
            <div>
              <span class="fig">${Math.round(user.usage.costUsd)}</span><span class="lbl"
                >estimated cost</span
              >
            </div>
          </div>
          <div style="margin-top:var(--s6)">
            <div class="cal-wd" aria-hidden="true">
              {#each ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'] as w (w)}<span>{w}</span
                >{/each}
            </div>
            <div class="cal" role="img" aria-label="Tokens per day, last 30 days">
              {#each cal.pad as i (i)}<i class="pad"></i>{/each}
              {#each cal.cells as c, i (i)}
                <i class="l{c.level}" class:today={c.today} title={c.title}></i>
              {/each}
            </div>
            <div class="legend">
              <span>Tokens per day</span>Less <i></i><i class="l1"></i><i class="l2"></i><i
                class="l3"
              ></i><i class="l4"></i> More
            </div>
          </div>
        </aside>
      </div>
      <section class="danger-zone" aria-labelledby="dz-h" style="margin-top:var(--s8)">
        <div class="sec" style="margin-top:0"><h2 id="dz-h">Danger zone</h2></div>
        <div class="r">
          {#if user.status === 'disabled'}
            <div>
              <b>Enable User</b>
              <p class="mu">
                Restores sign-in and starts their Instance. Revoked Hosts stay revoked.
              </p>
            </div>
            <button class="btn" disabled={busy} onclick={enable}>Enable User</button>
          {:else}
            <div>
              <b>Disable User</b>
              <p class="mu">Blocks sign-in and uploads. Their data is kept.</p>
            </div>
            <button class="btn danger" onclick={() => disableDlg?.showModal()}>Disable User</button>
          {/if}
        </div>
        <div class="r">
          <div>
            <b>Delete User</b>
            <p class="mu">Deletes their Instance and all sessions.</p>
          </div>
          <button
            class="btn danger"
            onclick={() => {
              confirmText = '';
              deleteDlg?.showModal();
            }}>Delete User</button
          >
        </div>
      </section>
      <dialog
        class="dlg"
        id="dlg-disable"
        bind:this={disableDlg}
        aria-labelledby="disable-h"
        onclick={(e) => e.target === disableDlg && disableDlg?.close()}
        onclose={clearHash}
      >
        <div class="box" tabindex="-1" autofocus>
          <div class="dh">
            <h2 id="disable-h">Disable {user.name}?</h2>
            <button class="x" aria-label="Close" onclick={() => disableDlg?.close()}
              ><Icon name="x" /></button
            >
          </div>
          <div class="body">
            <p>
              {firstName} will be signed out and can't sign in or upload until you enable the account
              again. Their data is kept.
            </p>
          </div>
          <div class="df">
            <button class="btn" onclick={() => disableDlg?.close()}>Cancel</button>
            <button class="btn danger solid" disabled={busy} onclick={disable}>Disable User</button>
          </div>
        </div>
      </dialog>
      <dialog
        class="dlg"
        id="dlg-delete"
        bind:this={deleteDlg}
        aria-labelledby="delete-h"
        onclick={(e) => e.target === deleteDlg && deleteDlg?.close()}
        onclose={clearHash}
      >
        <div class="box" tabindex="-1" autofocus>
          <div class="dh">
            <h2 id="delete-h">Delete {user.name}?</h2>
            <button class="x" aria-label="Close" onclick={() => deleteDlg?.close()}
              ><Icon name="x" /></button
            >
          </div>
          <div class="body">
            <div class="notice bad">
              <Icon name="alert" />
              <div>
                <b>This can't be undone</b>{firstName}'s Instance and all of its sessions are
                deleted.
              </div>
            </div>
            <div>
              <label class="f" for="dc">Type {user.email} to confirm</label>
              <input
                id="dc"
                class="input"
                type="text"
                autocomplete="off"
                bind:value={confirmText}
              />
            </div>
          </div>
          <div class="df">
            <button class="btn" onclick={() => deleteDlg?.close()}>Cancel</button>
            <button class="btn danger solid" disabled={!matches || busy} onclick={remove}
              >Delete User</button
            >
          </div>
        </div>
      </dialog>
    {:else if user === null}
      <div class="empty"><h1>User not found</h1></div>
    {/if}
  </main>
</AdminGate>
