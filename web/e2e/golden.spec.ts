import { expect, test } from '@playwright/test';
import { setIdentity, signIn } from './helpers';

test('sign in to the empty home, then sign out', async ({ page, context }) => {
  await setIdentity(context, 'golden@halcyon.example', 'Golden Path');
  await signIn(page);

  await expect(page).toHaveURL('/');
  await expect(page.getByRole('heading', { name: 'Add your first Host' })).toBeVisible();

  const burger = page.getByRole('button', { name: 'Menu' });
  if (await burger.isVisible()) await burger.click();
  else await page.getByRole('button', { name: /Golden Path/ }).click();
  await page.getByRole('menuitem', { name: 'Sign out' }).click();

  await expect(page).toHaveURL('/sign-in');
  await page.goto('/');
  await expect(page).toHaveURL('/sign-in');
});

test('a person outside the rule lands on the not-permitted page', async ({ page, context }) => {
  await setIdentity(context, 'mallory@gmail.example', 'Mallory');
  await signIn(page);
  await expect(page).toHaveURL('/not-permitted');
  await expect(page.getByText('You signed in as mallory@gmail.example.')).toBeVisible();
});

test('add a Host: the command follows the install choice and the page sees the Host connect', async ({
  page,
  context,
  request,
}, info) => {
  await setIdentity(context, `hosts-golden-${info.project.name}@halcyon.example`, 'Hosts Golden');
  await signIn(page);
  await page.getByRole('link', { name: 'Add Host' }).click();
  await expect(page).toHaveURL('/hosts/new');

  const command = page.locator('.code code');
  await expect(command).toContainText('/install/linux.sh | AVT_TOKEN=avt_tk_');
  await expect(command).toHaveText(/ bash$/);
  await page.getByText('Add to shell profile').click();
  await expect(command).toHaveText(/ bash -s -- --profile$/);
  await expect(page.getByText('Waiting for the Host to connect')).toBeVisible();

  await page.getByRole('radio', { name: 'macOS' }).check({ force: true });
  await expect(command).toContainText('/install/macos.sh | AVT_TOKEN=avt_tk_');
  await expect(page.getByText('Add to shell profile')).toBeHidden();
  await page.getByRole('radio', { name: 'Windows' }).check({ force: true });
  await expect(command).toHaveText(
    /^\$env:AVT_TOKEN="avt_tk_\S+"; irm \S+\/install\/windows\.ps1 \| iex$/,
  );
  await page.getByRole('radio', { name: 'Linux' }).check({ force: true });
  await page.getByRole('textbox', { name: 'Host name' }).fill('bad name');
  await expect(page.getByText('Use 1 to 64 letters')).toBeVisible();
  await expect(command).not.toContainText('AV_HOST_NAME');
  await page.getByRole('textbox', { name: 'Host name' }).fill('golden-box');
  await expect(command).toHaveText(
    / AVT_TOKEN=avt_tk_\S+ AV_HOST_NAME=golden-box bash -s -- --profile$/,
  );
  await page.getByRole('radio', { name: 'Windows' }).check({ force: true });
  await expect(command).toContainText('$env:AV_HOST_NAME="golden-box"; irm ');
  await page.getByRole('radio', { name: 'Linux' }).check({ force: true });

  const token = (await command.innerText()).match(/AVT_TOKEN=(\S+)/)![1];
  await expect
    .poll(
      async () => {
        const res = await request.post('/api/hosts/enrol', {
          data: { token, name: 'golden-box', os: 'Debian 12', mode: 'profile', version: '0.47.0' },
        });
        return res.status();
      },
      { timeout: 90_000 },
    )
    .toBe(200);
  await expect(page.getByText('golden-box connected')).toBeVisible({ timeout: 15_000 });
  await page.getByRole('link', { name: 'View Host' }).click();
  await expect(page).toHaveURL(/\/hosts\/[0-9a-f-]{36}$/);

  await page.goto('/hosts');
  await expect(page.getByText('golden-box').first()).toBeVisible();
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Your month' })).toBeVisible();
});

test('rename, revoke and delete a Host; revoked Hosts stay listed until deleted', async ({
  page,
  context,
}, info) => {
  await setIdentity(context, `host-detail-${info.project.name}@halcyon.example`, 'Host Detail');
  await signIn(page);
  const api = context.request;
  await expect
    .poll(
      async () => {
        const tok = await api.post('/api/hosts/tokens');
        if (tok.status() !== 201) return tok.status();
        const { token } = await tok.json();
        const res = await api.post('/api/hosts/enrol', {
          data: { token, name: 'old-box', os: 'Debian 12', mode: 'profile', version: '0.40.0' },
        });
        return res.status();
      },
      { timeout: 90_000 },
    )
    .toBe(200);

  await page.goto('/hosts');
  await page
    .getByRole('link', { name: /old-box/ })
    .first()
    .click();
  await expect(page.getByRole('heading', { name: 'old-box' })).toBeVisible();
  await expect(page.getByText('Update available: ')).toBeVisible();
  await expect(page.locator('.code code').last()).toContainText('/uninstall/linux.sh | bash');

  await page.getByRole('button', { name: 'Rename' }).click();
  await page.getByRole('textbox', { name: 'Name' }).fill('new-box');
  await page.getByRole('dialog').getByRole('button', { name: 'Rename' }).click();
  await expect(page.getByRole('heading', { name: 'new-box' })).toBeVisible();

  await page.getByRole('button', { name: 'Revoke Host' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Revoke Host' }).click();
  await expect(page.locator('.kv').getByText('Revoked', { exact: true }).first()).toBeVisible();
  await expect(page.getByRole('button', { name: 'Revoke Host' })).toHaveCount(0);

  await page.goto('/hosts');
  await expect(page.getByText('new-box').first()).toBeVisible();

  await page
    .getByRole('link', { name: /new-box/ })
    .first()
    .click();
  await page.getByRole('button', { name: 'Delete Host' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Delete Host' }).click();
  await expect(page).toHaveURL('/hosts');
  await expect(page.getByText('new-box')).toHaveCount(0);
});

test('an Admin grants Admin to someone yet to sign in and the Audit Log records it', async ({
  page,
  context,
}) => {
  await setIdentity(context, 'jess@halcyon.example', 'Jess Okafor');
  await signIn(page);
  await expect(page).toHaveURL('/');

  const burger = page.getByRole('button', { name: 'Menu' });
  if (await burger.isVisible()) await burger.click();
  else await page.getByRole('button', { name: /Jess Okafor/ }).click();
  await page.getByRole('menuitem', { name: 'Admin' }).click();
  await expect(page).toHaveURL('/admin/users');

  const email = `newadmin-${Date.now()}@halcyon.example`;
  await page.getByRole('button', { name: 'Grant Admin' }).click();
  await page.getByLabel('Email').fill(email);
  await page.getByRole('dialog').getByRole('button', { name: 'Grant Admin' }).click();
  await expect(page.getByText(email)).toBeVisible();

  await page.goto('/admin/audit');
  await page.getByLabel('Search the Audit Log').fill(email);
  await expect(page.getByText('Admin granted')).toHaveCount(1);
});

test('a User who is not an Admin cannot open Admin pages', async ({ page, context }) => {
  await setIdentity(context, 'plain@halcyon.example', 'Plain Person');
  await signIn(page);
  await page.goto('/admin/audit');
  await expect(page).toHaveURL('/');
});

test("an Admin opens a User's Instance, Impersonates them and exits", async ({
  page,
  context,
  browser,
  baseURL,
}) => {
  const email = `imp-${Date.now()}@halcyon.example`;
  const who = `Imogen ${Date.now()}`;
  const other = await browser.newContext({ baseURL });
  await setIdentity(other, email, who);
  await signIn(await other.newPage());
  await other.close();

  await setIdentity(context, 'jess@halcyon.example', 'Jess Okafor');
  await signIn(page);
  await page.goto('/admin/users');
  await page
    .getByRole('link', { name: new RegExp(who) })
    .first()
    .click();
  await expect(page.getByRole('heading', { name: new RegExp(who) })).toBeVisible();

  await page.getByRole('link', { name: 'Open Instance' }).click();
  await expect(page.locator('#av-platbar .banner.view')).toContainText(who, {
    timeout: 60_000,
  });
  await page
    .locator('#av-platbar .banner.view')
    .getByRole('link', { name: 'Exit Instance View' })
    .click();
  await expect(page).toHaveURL(/\/admin\/users\/[0-9a-f-]{36}$/);

  await page.getByRole('button', { name: 'Impersonate' }).click();
  await expect(page).toHaveURL('/');
  await expect(page.getByRole('status')).toContainText(`Impersonating ${who}`);
  await page.getByRole('button', { name: 'Stop impersonating' }).click();
  await expect(page).toHaveURL(/\/admin\/users\/[0-9a-f-]{36}$/);
  await expect(page.getByRole('status')).toHaveCount(0);

  await test.step("stopping from the Instance bar clears the impersonated User's cached data", async () => {
    await page.getByRole('button', { name: 'Impersonate' }).click();
    await expect(page).toHaveURL('/');
    await page.goto('/i/me/');
    let release!: () => void;
    const held = new Promise<void>((r) => (release = r));
    await page.route('/api/me', async (route) => {
      await held;
      await route.continue();
    });
    const asked = page.waitForRequest((r) => new URL(r.url()).pathname === '/api/me');
    await page.locator('#av-platbar').getByRole('button', { name: 'Stop impersonating' }).click();
    await expect(page).toHaveURL(/\/admin\/users\/[0-9a-f-]{36}$/);
    await asked;
    await expect(page.getByText(`Impersonating ${who}`)).toHaveCount(0);
    await expect(page).toHaveURL(/\/admin\/users\/[0-9a-f-]{36}$/);
    release();
  });

  await page.goto('/admin/audit');
  await page.getByLabel('Search the Audit Log').fill(who);
  await expect(page.getByText('Impersonation started').first()).toBeVisible();
  await expect(page.getByText('Impersonation ended').first()).toBeVisible();
});

test('every page body is as wide as the top bar', async ({ page, context }) => {
  await setIdentity(context, 'widths@halcyon.example', 'Width Check');
  await signIn(page);
  for (const path of ['/', '/hosts', '/hosts/new', '/team']) {
    await page.goto(path);
    const bar = await page.locator('.topbar .wrap').boundingBox();
    const main = await page.locator('main').boundingBox();
    expect(main?.width, path).toBe(bar?.width);
  }
});
