<script lang="ts">
  import AddHost from './pages/AddHost.svelte';
  import AdminInstance from './pages/AdminInstance.svelte';
  import AdminUser from './pages/AdminUser.svelte';
  import AdminUsers from './pages/AdminUsers.svelte';
  import Audit from './pages/Audit.svelte';
  import Home from './pages/Home.svelte';
  import HostDetail from './pages/HostDetail.svelte';
  import Hosts from './pages/Hosts.svelte';
  import Instance from './pages/Instance.svelte';
  import NotPermitted from './pages/NotPermitted.svelte';
  import SignIn from './pages/SignIn.svelte';
  import Team from './pages/Team.svelte';
  import Footer from './lib/Footer.svelte';
  import ErrorPage, { isReason, SERVER_REASON } from './pages/ErrorPage.svelte';

  const path = location.pathname.replace(/\/+$/, '') || '/';
  const serverErrorReason = document
    .querySelector('meta[name="av-error"]')
    ?.getAttribute('content');
</script>

{#if serverErrorReason != null}
  <ErrorPage reason={isReason(serverErrorReason) ? serverErrorReason : SERVER_REASON} />
{:else if path === '/sign-in'}
  <SignIn />
{:else if path === '/not-permitted'}
  <NotPermitted />
{:else if path.startsWith('/i/')}
  <Instance />
{:else if path === '/'}
  <Home />
{:else if path === '/hosts'}
  <Hosts />
{:else if path === '/team'}
  <Team />
{:else if path === '/hosts/new'}
  <AddHost />
{:else if /^\/hosts\/[^/]+$/.test(path)}
  <HostDetail />
{:else if path === '/admin' || path === '/admin/users'}
  <AdminUsers />
{:else if path.startsWith('/admin/users/')}
  <AdminUser />
{:else if path === '/admin/instance'}
  <AdminInstance />
{:else if path === '/admin/audit'}
  <Audit />
{:else}
  <ErrorPage reason="not-found" />
{/if}
<Footer />
