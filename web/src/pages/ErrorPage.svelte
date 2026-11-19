<script lang="ts" module>
  type Copy = { title: string; body: string; action: [label: string, href: string] };
  const signIn: [string, string] = ['Sign in again', '/auth/login'];
  const overview: [string, string] = ['Back to Overview', '/'];

  const COPY = {
    'sign-in-expired': {
      title: 'Your sign-in expired',
      body: 'It took too long or was interrupted. Sign in again to carry on.',
      action: signIn,
    },
    'provider-refused': {
      title: "Your sign-in wasn't completed",
      body: "Your identity provider didn't let you sign in. Try again, or ask an Admin if it keeps happening.",
      action: signIn,
    },
    'provider-unavailable': {
      title: "We couldn't reach your identity provider",
      body: "We couldn't finish signing you in. Wait a moment, then try again.",
      action: signIn,
    },
    'bad-identity': {
      title: "We couldn't check your sign-in",
      body: "Your identity provider sent something we couldn't verify. Try again, or ask an Admin to check the provider's settings.",
      action: signIn,
    },
    'no-verified-email': {
      title: 'We need a verified email',
      body: "Your identity provider didn't confirm your email address. Verify it there, then sign in again.",
      action: signIn,
    },
    unauthorised: {
      title: 'Sign in to continue',
      body: 'You need to be signed in to see this page.',
      action: ['Sign in', '/auth/login'],
    },
    forbidden: {
      title: "You can't open this page",
      body: "Your account doesn't have access to it. Ask an Admin if you think it should.",
      action: overview,
    },
    'not-found': {
      title: 'Page not found',
      body: "This page doesn't exist or has moved.",
      action: overview,
    },
    unavailable: {
      title: 'AgentsView Teams is unavailable',
      body: "Something it depends on isn't responding. Wait a moment, then try again.",
      action: overview,
    },
    'server-error': {
      title: 'Something went wrong',
      body: "That wasn't your fault. Try again in a moment, or ask an Admin if it keeps happening.",
      action: overview,
    },
    'bad-request': {
      title: "That didn't work",
      body: "We couldn't make sense of that request. Go back to Overview and try again.",
      action: overview,
    },
  } satisfies Record<string, Copy>;

  export type ErrorReason = keyof typeof COPY;
  export const isReason = (v: string | null | undefined): v is ErrorReason =>
    v != null && Object.hasOwn(COPY, v);
  export const SERVER_REASON = 'server-error' satisfies ErrorReason;
  export const copyFor = (r: ErrorReason): Copy => COPY[r];
</script>

<script lang="ts">
  let { reason }: { reason: ErrorReason } = $props();
  const c = $derived(copyFor(reason));
</script>

<main class="center">
  <div class="box">
    <div class="logo" role="img" aria-label="AgentsView Teams"></div>
    <h1>{c.title}</h1>
    <p>{c.body}</p>
    <a class="btn big" href={c.action[1]}>{c.action[0]}</a>
    {#if reason !== 'not-found'}
      <p class="foot">If this keeps happening, ask an Admin.</p>
    {/if}
  </div>
</main>
