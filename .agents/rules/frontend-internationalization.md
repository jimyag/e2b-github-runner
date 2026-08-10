# Frontend Internationalization

## Current Contract

- The UI supports English (`en`) and Simplified Chinese (`zh`) through
  `i18next` and `react-i18next`.
- `ui/src/i18n.ts` is the single language-runtime entrypoint. It owns language
  detection, persistence under `runnerd:language`, supported resources, and
  `document.documentElement.lang` synchronization.
- Fixed product copy belongs in `ui/src/locales/en.ts` and
  `ui/src/locales/zh.ts`. Keep both resource trees aligned when adding a key.
- `ui/src/i18next.d.ts` derives valid translation keys from the English
  resource. Keep dynamic keys as literal unions or explicit lookup maps so
  TypeScript can reject missing keys.
- Use `useTranslation()` at rendering boundaries. Use the initialized `i18n`
  instance only in non-React helpers or generic primitives that cannot use a
  hook.

## Translation Boundaries

- Translate headings, labels, buttons, placeholders, accessible names, empty
  states, errors, statuses, toast messages, and dynamic count text.
- Keep runtime data untranslated: repository names, GitHub identities, branch
  names, workflow/job names, IDs, logs, commit SHAs, API error details, and
  server-provided reasons.
- Never use translated text in stable identifiers, route keys, API query
  parameters, cache keys, React keys, or persistence values. For example, the
  detached branch sentinel is always the literal `detached`; only its displayed
  copy may be translated.
- Pass `i18n.resolvedLanguage` explicitly to `formatTime()` and
  `formatOptionalTime()` so dates follow the selected application language
  instead of the browser locale.
- Use interpolation and pluralization for dynamic translated text. Do not
  concatenate translated fragments with runtime values.

## Placement And Components

- The public landing page keeps its standalone `LanguageSwitcher` button.
- Authenticated top navigation places `LanguageSwitcher variant="menu"` inside
  `AccountMenu`; do not add a second standalone language button to
  `SiteHeader` or `UserDashboard`.
- Reuse the existing Radix dropdown primitives and Lucide icons. Keep language
  choices keyboard-accessible and expose the active language through the
  control's accessible label or radio state.
- Keep generic UI primitives' accessible copy localized without coupling them
  to a page-specific translation namespace.

## Change Checklist

- Add or update the key in both locale files.
- Add a focused behavior test for language selection, locale formatting, or
  stable identifiers when that behavior changes.
- Confirm runtime identifiers and raw server data remain unchanged.
- Search the changed surface for remaining fixed user-facing English copy.
- Run `task ui-i18n-check`. Exact product names, technical identifiers, and
  command examples may be added to the checker's narrow allowlist only after
  confirming they should remain identical in both languages.
- Edit only `ui/`; rebuild generated assets instead of editing
  `internal/server/ui/` by hand.

## Verification

For focused UI work, run:

```bash
task ui-i18n-check
cd ui && bun run test
cd ui && bun run lint
cd ui && bun run build
```

The i18n check rejects mismatched locale trees, missing or empty values, array
length differences, interpolation-variable differences, invalid typed keys,
and fixed literals in JSX text, selected visible attributes, and direct Sonner
toast calls. It intentionally does not translate runtime data, logs, IDs,
repository names, or raw server errors. The GitHub Actions `i18n` job runs the
same gate, but it becomes merge-blocking only when repository branch protection
or a ruleset requires that check.

For changes to shared i18n resources, headers, routing, or production asset
loading, also run:

```bash
task test
task ui-production-smoke
```

Before handoff, run `git diff --check` and verify that no generated assets,
local configuration, secrets, or sqlite databases are staged.
