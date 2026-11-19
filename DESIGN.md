---
name: AgentsView Teams
description: Calm light/dark admin console for a team's coding-agent Instances and Hosts.
colors:
  accent-blue: "#3b5bdb"
  on-accent: "#ffffff"
  paper: "#fafafa"
  surface: "#ffffff"
  ink: "#111111"
  muted-ink: "#636363"
  hairline: "#e8e8e8"
  calendar-empty: "#eef0f4"
  status-ok: "#16a34a"
  status-off: "#a3a3a3"
  status-warn: "#b45309"
  status-danger: "#b91c1c"
  impersonation-amber: "#b45309"
  impersonation-ink: "#7c2d12"
  night: "#0e1013"
  night-surface: "#15171b"
  night-ink: "#ececee"
  night-muted-ink: "#9b9ea6"
  night-hairline: "#262930"
  night-calendar-empty: "#1d2026"
  night-ok: "#22c55e"
  night-off: "#6b6e75"
  night-warn: "#f59e0b"
  night-danger: "#f87171"
typography:
  headline:
    fontFamily: "Source Sans 3, system-ui, sans-serif"
    fontSize: "27px"
    fontWeight: 600
    lineHeight: 1.2
    letterSpacing: "-0.01em"
  figure:
    fontFamily: "Source Sans 3, system-ui, sans-serif"
    fontSize: "34px"
    fontWeight: 600
    lineHeight: 1.15
    letterSpacing: "-0.02em"
    fontFeature: "tnum"
  title:
    fontFamily: "Source Sans 3, system-ui, sans-serif"
    fontSize: "18px"
    fontWeight: 600
    letterSpacing: "-0.01em"
  body:
    fontFamily: "Source Sans 3, system-ui, sans-serif"
    fontSize: "15px"
    fontWeight: 400
    lineHeight: 1.55
    fontFeature: "tnum"
  label:
    fontFamily: "Source Sans 3, system-ui, sans-serif"
    fontSize: "13px"
    fontWeight: 400
  mono:
    fontFamily: "JetBrains Mono, ui-monospace, monospace"
    fontSize: "13px"
    fontWeight: 400
    lineHeight: 1.6
rounded:
  cell: "2px"
  base: "5px"
  dialog: "8px"
  full: "9999px"
spacing:
  s1: "4px"
  s2: "8px"
  s3: "12px"
  s4: "16px"
  s5: "24px"
  s6: "32px"
  s7: "40px"
  s8: "56px"
components:
  button:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    rounded: "{rounded.base}"
    padding: "7px 14px"
    height: "36px"
  button-primary:
    backgroundColor: "{colors.accent-blue}"
    textColor: "{colors.on-accent}"
    rounded: "{rounded.base}"
    padding: "7px 14px"
    height: "36px"
  button-big:
    backgroundColor: "{colors.accent-blue}"
    textColor: "{colors.on-accent}"
    rounded: "{rounded.base}"
    padding: "12px 22px"
    height: "46px"
  button-danger-solid:
    backgroundColor: "{colors.status-danger}"
    textColor: "{colors.on-accent}"
    rounded: "{rounded.base}"
    padding: "7px 14px"
    height: "36px"
  input:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    rounded: "{rounded.base}"
    padding: "7px 11px"
    height: "38px"
  top-bar:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    height: "52px"
  platform-bar:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.muted-ink}"
    typography: "{typography.label}"
    height: "36px"
  list-row:
    textColor: "{colors.ink}"
    rounded: "{rounded.base}"
    padding: "11px 12px"
  code-block:
    backgroundColor: "{colors.calendar-empty}"
    textColor: "{colors.ink}"
    typography: "{typography.mono}"
    rounded: "{rounded.base}"
    padding: "12px 12px 12px 16px"
  banner-impersonation:
    backgroundColor: "{colors.impersonation-amber}"
    textColor: "{colors.on-accent}"
    padding: "8px 16px"
  dialog:
    backgroundColor: "{colors.surface}"
    rounded: "{rounded.dialog}"
    padding: "24px"
    width: "500px"
---

# Design System: AgentsView Teams

## Overview

**Creative North Star: "The Quiet Console"**

A standard light and dark SaaS admin console, built to sit beside agentsview without competing with it. The reference is the Tailscale admin console; the craft bar is Linear and Vercel. Content leads, chrome stays slim, and colour is kept for state and one accent the User chooses. It is not agentsview's look.

Density is calm rather than packed. Lists are striped rows on the page, not boxed cards; figures are large tabular numbers with a muted label; sections are separated by space and single hairlines, never by nested borders. Every screen has one clear job, one `h1`, and at most one primary action.

The same tokens drive a full light and a full dark theme. Dark is a first-class theme, not an inversion: it has its own neutrals, brighter status colours and a lightened accent for links.

**Key Characteristics:**
- Slim 52px top bar: brand, Overview / Hosts / Team, My Instance; everything else in the account menu.
- One accent (`accent-blue` by default), per-User, from a palette or HSL.
- Striped lists, hairline rules, generous spacing; no cards inside cards.
- Status always as dot plus word.
- Source Sans 3 for everything; JetBrains Mono only for commands, tokens and IDs.

## Colors

Near-neutral greys with one cool accent and a small, fixed set of status colours.

### Primary
- **Console Blue** (accent-blue): primary buttons, links, the My Instance tab, the logo tile, calendar intensity, chart bars, focus rings. The User can replace it; every accent use goes through the one `--accent` token so the swap is total. In dark theme, link text uses the accent mixed 62% with white for contrast (`--link`).

### Neutral
- **Paper** (paper) / **Night** (night): page background.
- **Surface** (surface) / **Night Surface** (night-surface): top bar, Platform bar, menus, dialogs, inputs, buttons.
- **Ink** (ink) / **Night Ink** (night-ink): body text and headings.
- **Muted Ink** (muted-ink) / **Night Muted Ink** (night-muted-ink): labels, secondary lines, column heads, inactive tabs. Passes AA on its background in both themes.
- **Hairline** (hairline) / **Night Hairline** (night-hairline): the only border colour. Row stripes are ink at 3.5% opacity.
- **Calendar Empty** (calendar-empty) / **Night Calendar Empty** (night-calendar-empty): empty calendar cells and the code-block well.

### Status
- **OK** (status-ok), **Off** (status-off), **Warn** (status-warn), **Danger** (status-danger), each with a brighter night variant. Used only as the status dot, the outdated-version ↑ badge, notice icons and destructive text.
- **Impersonation Amber** (impersonation-amber) with **Impersonation Ink** (impersonation-ink): the Impersonation banner only, fixed regardless of the User's accent.

### Named Rules
**The One Accent Rule.** The accent is the only hue that is not a status. If something is coloured and is not the accent, it must mean OK, Off, Warn, Danger or Impersonation.

**The Dot Plus Word Rule.** Status is never colour alone: a dot is always followed by its word (Online, Offline, Disabled, Failed).

**The Fixed Alarm Rule.** The Impersonation banner is amber whatever accent the User picks, so it can never be mistaken for the Instance View banner, which is accent-tinted.

## Typography

**Body Font:** Source Sans 3 (with system-ui)
**Mono Font:** JetBrains Mono (with ui-monospace)

**Character:** One humanist sans at few sizes, made to read calmly at 15px. Tabular numerals are on everywhere so figures and columns line up.

### Hierarchy
- **Headline** (600, 27px, 1.2): the page `h1`, one per page.
- **Figure** (600, 34px, 1.15, -0.02em): headline numbers in the Overview rail; 28px inside the Team figure band.
- **Title** (600, 18px): section `h2`s and dialog titles (dialogs use 20px).
- **Body** (400, 15px, 1.55): everything else, including buttons (600).
- **Label** (400, 13px): column heads, sub-lines under names, Platform bar, small print. Calendar weekday labels and legends drop to 12px.
- **Mono** (400, 13px, 1.6): install commands, Install Tokens, IDs. Inline `code` is 0.88em.

### Named Rules
**The Few Sizes Rule.** Six sizes, no more: 34, 27, 18, 15, 13, 12. Weight 600 and colour carry the rest of the hierarchy.

## Layout

Content sits in a centred 1200px column with 32px side padding. The spacing scale is 4, 8, 12, 16, 24, 32, 40, 56px; page headers sit 40px below the bar and 32px above content, sections are 40px apart.

Primary pages use a rail layout: the main list in a flexible column and a 320px right rail for figures and the activity calendar, 56px apart. The page header puts the title on the left and actions (such as the big Open AgentsView button) on the right.

My Instance replaces the top bar with a slim 36px full-width Platform bar so agentsview gets the screen. Banners (Instance View, Impersonation) are pinned to the top while scrolling.

- **Tablet (at or below 1000px):** the rail stacks under the main column; the Team figure band goes to three columns; two-column sections stack.
- **Mobile (at or below 640px):** tabs move into a menu behind a button; list rows restructure (secondary columns drop or move under the name) rather than shrinking; row buttons stay at least 36px tall.

## Elevation & Depth

Flat by default. Page surfaces are separated by tone (paper against surface) and single hairlines, never by shadow. One shadow exists, and only things that float above the page use it.

### Shadow Vocabulary
- **Float** (`0 1px 2px rgba(0,0,0,.06), 0 8px 24px rgba(0,0,0,.10)`; dark: `0 1px 2px rgba(0,0,0,.4), 0 12px 32px rgba(0,0,0,.5)`): account menu and dialogs.

### Named Rules
**The Only Floats Cast Rule.** If it scrolls with the page, it has no shadow. Shadow means "this is above the page and will go away".

## Shapes

Gently rounded and consistent: 5px on buttons, inputs, rows, notices and code wells; 8px on dialogs; 2px on calendar cells and chart bar tops, which stay square-looking; full circles for status dots, avatars, palette swatches and step numbers. Borders are one 1px hairline. Inputs and segmented controls use a slightly stronger border (ink at 18% over hairline) so they read as editable.

## Components

### Buttons
Plain and confident; one primary per view.
- **Default:** surface fill, hairline border, ink text, 600 weight, 36px tall. Hover darkens the border only.
- **Primary:** accent fill and border, white text. Hover mixes 12% black into the accent.
- **Big:** 46px tall, 18px text; reserved for the Open AgentsView call to action beside a page title.
- **Danger:** danger-coloured text on the default button; **Danger solid** (danger fill, white text) only as the confirm button inside a destructive dialog.
- **In list rows:** compact (13px text), but at least 36px tall on mobile.

### Segmented control
A bordered track with 2px inset; options are 6px by 14px; the selected option takes a 9% ink tint and ink text, others are muted. Used for OS choice on Add Host and filters on Admin Users.

### Inputs / Fields
Surface fill, stronger border, 5px radius, 38px tall. Focus is the global 2px accent outline at 2px offset. Fields stack 24px apart.

### Navigation
- **Top tabs:** muted text; the current page is ink with a 2px ink underline inset at the bar's bottom edge. My Instance sits beside the tabs as an accent-coloured link.
- **Sub-tabs:** same treatment, on a hairline, inside a page (Hosts and Install Tokens).
- **Account menu:** 250px floating panel with the User's name and email, then Settings, Admin, Dark mode toggle, Sign out. Admin lives here, never as a tab.
- **Platform bar:** 36px, 13px text, brand, the same destinations, account menu.

### Lists
Striped rows on the page with no outer box: 11px by 12px padding, every second row tinted, linked rows tint further on hover. Column heads are 13px muted, untinted. The name column is 600 with an optional 13px muted sub-line.

### Figures and the Team band
Rail figures are 34px over a muted label, separated by hairlines. The Team band is a ruled row of 4 to 6 figures divided by vertical hairlines, with an optional "you" line under each figure. The band never shows another person's figures.

### Activity calendar (signature)
A 7-column grid of square cells, 6px apart, no day numbers. Intensity encodes tokens per day in five steps: empty, then the accent mixed at 24%, 48% and 74% into the empty colour, then the full accent. Today has a 1.5px ink ring offset by the page background. A legend runs from less to more.

### Notices and banners
- **Notice:** 5px rounded panel tinted 12% with warn, danger or accent (info), with a matching icon. Used for one-time secrets ("shown once") and failures.
- **Instance View banner:** accent-tinted, ink text. **Impersonation banner:** solid amber, white text, white button. Both pinned.

### Dialogs
500px panel, 8px radius, Float shadow, 24px padding, over a 45% black scrim, placed 12vh from the top. Title left, close button right; actions right-aligned at the foot. In the build these are real modal dialogs that move and trap focus and return it on close.

## Do's and Don'ts

### Do:
- **Do** route every accent use through the one accent token so a User's choice changes the whole UI.
- **Do** pair every status dot with its word.
- **Do** show secrets (Install Tokens) once, in mono, with a copy button and a warning notice.
- **Do** keep banners pinned while scrolling.
- **Do** design both themes for every screen; check text at AA in each.
- **Do** restructure lists and rails at narrow widths rather than shrinking them.

### Don't:
- **Don't** copy agentsview's look; this is a separate, standard admin console.
- **Don't** put Admin in the top tabs; it belongs in the account menu.
- **Don't** box lists in cards or nest bordered containers.
- **Don't** add shadows to anything that scrolls with the page.
- **Don't** add type sizes beyond the six in the scale.
- **Don't** tint the Impersonation banner with the User's accent.
- **Don't** show another person's figures on the Team page.
