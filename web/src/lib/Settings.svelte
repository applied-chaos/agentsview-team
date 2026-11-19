<script lang="ts">
  import { clearHash } from './api';
  import Icon from './Icon.svelte';
  import {
    currentAccent,
    currentTheme,
    DEFAULT_ACCENT,
    hsl,
    PALETTE,
    setAccent,
    setTheme,
  } from './theme';

  let { dialog = $bindable() }: { dialog: HTMLDialogElement | undefined } = $props();

  let accent = $state(currentAccent());
  let theme = $state(currentTheme());
  let h = $state(226);
  let s = $state(69);
  let l = $state(55);

  function pick(v: string) {
    accent = v;
    setAccent(v);
  }
  const custom = () => pick(hsl(h, s, l));
  const lightness = [25, 65] as const;
  const stops = (f: (v: number) => string, n: number) =>
    `linear-gradient(to right, ${Array.from({ length: n + 1 }, (_, i) => f(i / n)).join(', ')})`;
  const hueTrack = $derived(stops((t) => hsl(Math.round(t * 360), s, l), 12));
  const satTrack = $derived(stops((t) => hsl(h, Math.round(t * 100), l), 2));
  const lightTrack = $derived(
    stops((t) => hsl(h, s, Math.round(lightness[0] + t * (lightness[1] - lightness[0]))), 2),
  );
  const close = () => dialog?.close();
</script>

<dialog
  class="dlg"
  bind:this={dialog}
  aria-labelledby="settings-h"
  onclick={(e) => e.target === dialog && close()}
  onclose={clearHash}
>
  <div class="box" tabindex="-1" autofocus>
    <div class="dh">
      <h2 id="settings-h">Settings</h2>
      <button class="x" aria-label="Close" onclick={close}><Icon name="x" /></button>
    </div>
    <div class="body">
      <div><h3>Theme</h3></div>
      <div class="seg" role="radiogroup" aria-label="Theme">
        {#each ['light', 'dark'] as const as t (t)}
          <label>
            <input
              type="radio"
              name="theme"
              value={t}
              checked={theme === t}
              onchange={() => {
                theme = t;
                setTheme(t);
              }}
            /><span>{t === 'light' ? 'Light' : 'Dark'}</span>
          </label>
        {/each}
      </div>
      <div>
        <h3>Accent colour</h3>
        <p class="mu">Used for links, buttons and activity charts.</p>
      </div>
      <div class="pal" role="group" aria-label="Accent colour">
        {#each Object.entries(PALETTE) as [value, name] (value)}
          <button
            style="--c:{value}"
            aria-label={name}
            title={name}
            aria-pressed={value === accent}
            onclick={() => pick(value)}
          ></button>
        {/each}
      </div>
      <div><h3>Custom colour</h3></div>
      <div class="hsl" style="--th:{hueTrack};--ts:{satTrack};--tl:{lightTrack}">
        <label for="hsl0">Hue</label>
        <input
          id="hsl0"
          style="--track:var(--th)"
          type="range"
          min="0"
          max="360"
          bind:value={h}
          oninput={custom}
        />
        <output for="hsl0">{h}°</output>
        <label for="hsl1">Saturation</label>
        <input
          id="hsl1"
          style="--track:var(--ts)"
          type="range"
          min="0"
          max="100"
          bind:value={s}
          oninput={custom}
        />
        <output for="hsl1">{s}%</output>
        <label for="hsl2">Lightness</label>
        <input
          id="hsl2"
          style="--track:var(--tl)"
          type="range"
          min="25"
          max="65"
          bind:value={l}
          oninput={custom}
        />
        <output for="hsl2">{l}%</output>
      </div>
      <div class="swatch"><i></i><span>{accent}</span></div>
    </div>
    <div class="df">
      <button class="btn" onclick={() => pick(DEFAULT_ACCENT)}>Reset to default</button>
      <button class="btn primary" onclick={close}>Done</button>
    </div>
  </div>
</dialog>
