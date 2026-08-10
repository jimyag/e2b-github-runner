# UI Internationalization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add complete English and Simplified Chinese UI support with `i18next` and `react-i18next`.

**Architecture:** A single `ui/src/i18n.ts` module owns resources, language detection, persistence, and `<html lang>` synchronization. React components consume fixed UI copy through `useTranslation`; a shared icon-based language switcher changes the global language from public and authenticated headers. Runtime data such as repository names, GitHub identities, logs, IDs, and API error details remains untranslated.

**Tech Stack:** React 19, TypeScript, i18next, react-i18next, Bun tests, Vite

## Global Constraints

- Supported languages are English (`en`) and Simplified Chinese (`zh`).
- Persist the explicit selection under `runnerd:language`.
- Prefer a saved language, then a Chinese browser locale, then English.
- Keep `document.documentElement.lang` synchronized with the active language.
- Do not edit generated files under `internal/server/ui/` by hand.
- Do not commit or push unless the user requests it.

---

### Task 1: Language Runtime Contract

**Files:**
- Create: `ui/src/i18n.test.js`
- Create: `ui/src/i18n.ts`
- Modify: `ui/package.json`
- Modify: `ui/bun.lock`
- Modify: `ui/src/main.tsx`

**Interfaces:**
- Produces: `supportedLanguages`, `normalizeLanguage(language)`, `detectLanguage(storage, navigatorLanguage)`, `setLanguage(language)`, and the initialized default i18next instance.

- [x] **Step 1: Write failing tests for language normalization, saved/browser fallback, translation resolution, persistence, and `<html lang>` synchronization.**

```ts
expect(normalizeLanguage("zh-CN")).toBe("zh")
expect(detectLanguage(storage, "zh-CN")).toBe("zh")
expect(i18n.t("common.language", { lng: "zh" })).toBe("语言")
```

- [x] **Step 2: Run `cd ui && bun test src/i18n.test.js` and confirm the module is missing.**
- [x] **Step 3: Add `i18next` and `react-i18next`, initialize synchronously, and import the module before rendering `App`.**
- [x] **Step 4: Rerun `cd ui && bun test src/i18n.test.js` and confirm the contract passes.**

### Task 2: Shared Language Control

**Files:**
- Create: `ui/src/components/language-switcher.tsx`
- Create: `ui/src/components/language-switcher.test.js`
- Modify: `ui/src/components/landing-page.tsx`
- Modify: `ui/src/components/account-menu.tsx`
- Modify: `ui/src/components/site-header.tsx`

**Interfaces:**
- Consumes: `supportedLanguages` and `setLanguage(language)` from `ui/src/i18n.ts`.
- Produces: `LanguageSwitcher`, an accessible globe control with English and 中文 options.

- [x] **Step 1: Write a failing server-render test that asserts the language control exposes both choices and the active choice.**
- [x] **Step 2: Run `cd ui && bun test src/components/language-switcher.test.js` and confirm the component is missing.**
- [x] **Step 3: Implement the shared control with the existing Radix dropdown primitives and Lucide `Languages` icon.**
- [x] **Step 4: Place it in the public header and authenticated account/header controls, then rerun the focused tests.**

### Task 3: Full Fixed-Copy Migration

**Files:**
- Modify: `ui/src/i18n.ts`
- Modify: `ui/src/App.tsx`
- Modify: `ui/src/components/*.tsx` excluding generic primitives under `ui/src/components/ui/`
- Modify: affected `ui/src/**/*.test.{js,ts,tsx}` files

**Interfaces:**
- Consumes: `useTranslation()` and namespaced translation keys.
- Produces: bilingual public, authentication, ordinary-user, repository-readiness, settings, job/log/terminal, and administrator surfaces.

- [x] **Step 1: Update one page family at a time to call `t()` for headings, descriptions, labels, buttons, placeholders, tooltips, empty/error states, and toast messages.**
- [x] **Step 2: Use interpolation and pluralization for dynamic counts instead of concatenating translated fragments.**
- [x] **Step 3: Keep statuses and helper outputs translated at their final rendering boundary while leaving API values unchanged.**
- [x] **Step 4: Update behavior tests to initialize explicit English or Chinese language where copy is asserted, and add Chinese rendering assertions for representative public, user, and admin surfaces.**

### Task 4: Verification And Coverage Audit

**Files:**
- Verify: all files changed above

**Interfaces:**
- Consumes: the completed bilingual UI.
- Produces: test, lint, production-build, browser-smoke, and hard-coded-copy audit evidence.

- [x] **Step 1: Run focused tests after each page family, then `cd ui && bun run test`.**
- [x] **Step 2: Run `task ui-lint`, `task build`, and `task ui-production-smoke`.**
- [x] **Step 3: Search component areas for remaining fixed English UI strings and inspect every hit in context; preserve only brands, protocols, technical tokens, and runtime data.**
- [x] **Step 4: Run `git diff --check`, inspect `git diff --stat` and the complete diff, and leave the verified changes uncommitted.**
