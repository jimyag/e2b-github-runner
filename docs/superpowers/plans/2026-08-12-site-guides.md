# Public Site Guides Implementation Plan

> **Lifecycle:** Historical plan, completed in `b8e777f` (`feat(ui): add bilingual site guides (#72)`). The checklist below is retained as execution history, not active work; see [`README.md`](README.md) and the repository `TODO.md` for current status.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give Qiniu CI Runner users public, bilingual, same-origin paths from the landing page to their first successful managed GitHub Actions job, a self-hosted runnerd deployment, or a verified custom runner template and workflow.

**Architecture:** Add a small public documentation surface to the existing React/Vite application. English and Chinese Markdown files remain the source of truth, a typed catalog maps only known `/docs` routes to those files, and a source-controlled Markdown renderer presents them inside a responsive Qiniu-branded documentation shell. Existing landing, authentication, and account-menu entry points link to the relevant guide instead of sending every user to the GitHub README.

**Tech Stack:** React 19, TypeScript, Vite raw Markdown imports, `react-markdown`, `remark-gfm`, i18next, Tailwind CSS, Bun tests, Playwright production smoke.

**Status:** Executed on `feat/site-guides`. All implementation steps, including the custom-template extension in Task 7 and review corrections, are complete; verification results are recorded below. Commit and push were subsequently authorized; PR creation and deployment remain out of scope.

## Global Constraints

- `/` remains the public product landing page and `/jobs` remains the protected ordinary-user Jobs homepage.
- Only exact documented `/docs` routes are public; unknown routes continue to render 404.
- Hosted onboarding uses the managed label pair `runs-on: [qiniu, ubuntu-24.04]`.
- Account or organization Preferences remain the only ordinary-user Sandbox credential editor; the guides link to readiness and Preferences without embedding credential forms.
- English and Chinese content must remain structurally aligned and switch in place using the existing language preference.
- Edit `ui/` source only; never hand-edit generated files in `internal/server/ui/`.
- Existing unrelated `.gitignore` worktree changes are preserved and excluded from this feature's review or staging scope.
- This request authorizes a feature branch and implementation, but not a push, pull request, or publication.

---

**Execution order:** Complete Task 2 before Task 1 because the typed catalog imports the Markdown sources. Continue with Tasks 3-7 in numeric order.

### Task 1: Typed Public Documentation Catalog And Routes

**Files:**
- Create: `ui/src/site-docs.ts`
- Create: `ui/src/site-doc-routes.ts`
- Create: `ui/src/site-docs.test.js`
- Modify: `ui/src/app-load-policy.ts:98-103`
- Modify: `ui/src/app-load-policy.test.js:87-103`

**Interfaces:**
- Produces: `siteDocumentRoutes`, `isSiteDocumentPath(path)`, `siteDocuments(language)`, and `siteDocumentForPath(path, language)`.
- Consumes: raw Markdown modules from Task 2 through `?raw` imports.

- [x] **Step 1: Write failing route and catalog tests**

```javascript
test("publishes only the known documentation routes", () => {
  expect(appRouteAccess("/docs")).toBe("public")
  expect(appRouteAccess("/docs/getting-started/hosted")).toBe("public")
  expect(appRouteAccess("/docs/not-a-guide")).toBe("not-found")
})

test("keeps the selected article while changing language", () => {
  expect(siteDocumentForPath("/docs/guides/workflow", "en").title).toBe("Run your first workflow")
  expect(siteDocumentForPath("/docs/guides/workflow", "zh").title).toBe("运行第一个工作流")
})
```

- [x] **Step 2: Run tests and verify RED**

```bash
cd ui && bun test src/site-docs.test.js src/app-load-policy.test.js
```

Expected: FAIL because `/docs` is classified as `not-found` and the catalog exports do not exist.

- [x] **Step 3: Implement exact-route classification and Markdown metadata parsing**

```typescript
export function isSiteDocumentPath(path: string): boolean {
  return siteDocumentRoutes.includes(normalizeSiteDocumentPath(path))
}

export function siteDocumentForPath(path: string, language: string): SiteDocument | null {
  const locale = language.toLowerCase().startsWith("zh") ? "zh" : "en"
  return siteDocuments(locale).find((document) => document.path === normalizeSiteDocumentPath(path)) ?? null
}
```

- [x] **Step 4: Run focused tests and verify GREEN**

```bash
cd ui && bun test src/site-docs.test.js src/app-load-policy.test.js
```

Expected: PASS with exact public docs routes and unknown-doc 404 coverage.

### Task 2: Bilingual Markdown Source Of Truth

**Files:**
- Create: `ui/src/content/site-docs/en/index.md`
- Create: `ui/src/content/site-docs/en/getting-started-hosted.md`
- Create: `ui/src/content/site-docs/en/getting-started-deploy.md`
- Create: `ui/src/content/site-docs/en/workflow.md`
- Create: `ui/src/content/site-docs/en/troubleshooting.md`
- Create: `ui/src/content/site-docs/en/runner-labels.md`
- Create: `ui/src/content/site-docs/zh/index.md`
- Create: `ui/src/content/site-docs/zh/getting-started-hosted.md`
- Create: `ui/src/content/site-docs/zh/getting-started-deploy.md`
- Create: `ui/src/content/site-docs/zh/workflow.md`
- Create: `ui/src/content/site-docs/zh/troubleshooting.md`
- Create: `ui/src/content/site-docs/zh/runner-labels.md`

**Interfaces:**
- Produces: source-controlled Markdown strings consumed by `site-docs.ts`.
- Consumes: current route, managed-template, readiness, and authorization contracts from code and repository documentation.

- [x] **Step 1: Write the English golden path**

```markdown
# Get started with the hosted service

Run a GitHub Actions job on a clean Qiniu Sandbox without deploying runnerd yourself.

## Before you start
## 1. Sign in with GitHub
## 2. Connect a repository
## 3. Check Sandbox readiness
## 4. Add the managed runner label
## 5. Run and verify the job
## What success looks like
```

- [x] **Step 2: Write the Chinese golden path with the same stage order**

```markdown
# 开始使用托管服务

无需自行部署 runnerd，即可让 GitHub Actions 任务运行在干净的 Qiniu Sandbox 中。

## 开始前
## 1. 使用 GitHub 登录
## 2. 连接仓库
## 3. 检查 Sandbox 就绪状态
## 4. 添加托管 Runner 标签
## 5. 运行并验证任务
## 成功标准
```

- [x] **Step 3: Add deployment, workflow, troubleshooting, and label-reference documents**

```yaml
name: Qiniu CI Runner smoke
on:
  workflow_dispatch:
jobs:
  smoke:
    runs-on: [qiniu, ubuntu-24.04]
    steps:
      - run: uname -a
```

The troubleshooting documents must lead with user-visible symptoms: job stays queued, repository is missing, Sandbox creation fails, Runner registration returns 404, and OAuth sign-in fails.

- [x] **Step 4: Verify language-pair structure and stale-label absence**

```bash
for locale in en zh; do find "ui/src/content/site-docs/$locale" -maxdepth 1 -name '*.md' -print | sort; done
rg -n 'self-hosted, qiniu-sandbox|qiniu-sandbox' ui/src/content/site-docs && exit 1 || true
```

Expected after Task 7: seven files per locale and no obsolete custom-label example.

### Task 3: Responsive Documentation Shell And Markdown Rendering

**Files:**
- Modify: `ui/package.json`
- Modify: `ui/bun.lock`
- Create: `ui/src/components/docs-page.tsx`
- Create: `ui/src/components/docs-page.test.js`
- Create: `ui/src/markdown.d.ts`
- Modify: `ui/src/App.tsx:947-958`
- Modify: `ui/src/index.css`

**Interfaces:**
- Consumes: `siteDocumentForPath()` and `siteDocuments()` from Task 1.
- Produces: `DocsPage({ path }: { path: string })`, semantic navigation landmarks, in-page heading links, and source-controlled Markdown rendering.

- [x] **Step 1: Add renderer dependencies**

```bash
cd ui && bun add react-markdown remark-gfm
```

- [x] **Step 2: Write a failing real-render test**

```javascript
test("renders the hosted guide in Chinese with public navigation", () => {
  const html = renderToStaticMarkup(createElement(DocsPage, { path: "/docs/getting-started/hosted" }))
  expect(html).toContain("开始使用托管服务")
  expect(html).toContain('href="/docs/guides/workflow"')
  expect(html).toContain('href="/jobs"')
  expect(html).toContain("qiniu, ubuntu-24.04")
})
```

- [x] **Step 3: Run the test and verify RED**

```bash
cd ui && bun test src/components/docs-page.test.js
```

Expected: FAIL because `DocsPage` does not exist.

- [x] **Step 4: Implement the documentation shell**

```tsx
<div className="site-docs min-h-screen bg-[#f7fbfd] text-[#10242f] dark:bg-[#061119] dark:text-[#edf8fc]">
  <header>{/* product home, documentation identity, language, theme, Jobs */}</header>
  <div className="mx-auto grid max-w-[1480px] lg:grid-cols-[17rem_minmax(0,1fr)_13rem]">
    <nav aria-label={t("docs.navigation")}>{/* grouped guide links */}</nav>
    <main id="docs-content"><ReactMarkdown remarkPlugins={[remarkGfm]}>{document.markdown}</ReactMarkdown></main>
    <aside aria-label={t("docs.onThisPage")}>{/* extracted H2 links */}</aside>
  </div>
</div>
```

Use source-controlled Markdown only, block unsafe protocols in links, render tables/code responsively, preserve visible focus states, and collapse both sidebars into compact mobile navigation.

- [x] **Step 5: Wire public docs rendering in App and verify GREEN**

```tsx
if (routeAccess === "public") {
  return locationPath === "/" ? <LandingPage /> : <DocsPage path={locationPath} />
}
```

```bash
cd ui && bun test src/components/docs-page.test.js src/app-load-policy.test.js
```

Expected: PASS.

### Task 4: Contextual Entry Points

**Files:**
- Modify: `ui/src/components/landing-page.tsx`
- Modify: `ui/src/components/landing-page.test.js`
- Modify: `ui/src/components/auth-pages.tsx`
- Modify: `ui/src/components/auth-pages.test.js`
- Modify: `ui/src/components/site-header.tsx`
- Modify: `ui/src/components/site-header.test.js`
- Modify: `ui/src/locales/en.ts`
- Modify: `ui/src/locales/zh.ts`

**Interfaces:**
- Consumes: known public docs paths.
- Produces: same-origin links from discovery, pre-authentication, and authenticated surfaces.

- [x] **Step 1: Write failing link-behavior tests**

```javascript
expect(landingHTML).toContain('href="/docs"')
expect(landingHTML).toContain('href="/docs/getting-started/hosted"')
expect(landingHTML).toContain('href="/docs/getting-started/deploy"')
expect(signInHTML).toContain('href="/docs/getting-started/hosted"')
expect(siteHeaderHTML).toContain('href="/docs"')
```

- [x] **Step 2: Run tests and verify RED**

```bash
cd ui && bun test src/components/landing-page.test.js src/components/auth-pages.test.js src/components/site-header.test.js
```

Expected: FAIL because current documentation links point to GitHub and auth/account surfaces have no guide link.

- [x] **Step 3: Replace generic links with contextual same-origin destinations**

```typescript
const documentationURL = "/docs"
const hostedGuideURL = "/docs/getting-started/hosted"
const deploymentGuideURL = "/docs/getting-started/deploy"
```

Add translated `docs`, `readGettingStarted`, and authentication helper copy to both locale trees. Keep GitHub source and license links external.

- [x] **Step 4: Run focused tests and i18n verification**

```bash
cd ui && bun test src/components/landing-page.test.js src/components/auth-pages.test.js src/components/site-header.test.js
task ui-i18n-check
```

Expected: PASS with matching locale structure and no fixed user-visible literals.

### Task 5: Synchronize Repository Documentation And Durable Rules

**Files:**
- Modify: `README.md`
- Modify: `README.zh.md`
- Modify: `docs/README.md`
- Modify: `docs/zh/README.md`
- Modify: `TODO.md`
- Modify: `AGENTS.md`
- Modify: `.agents/rules/frontend-internationalization.md`
- Modify: `.agents/rules/testing-and-verification.md`

**Interfaces:**
- Consumes: shipped docs routes and verification commands from Tasks 1-4.
- Produces: trustworthy operator, contributor, and agent guidance.

- [x] **Step 1: Update public-route and documentation contracts**

```markdown
- `/docs` and its known guide routes are public and bilingual.
- Site guide Markdown source lives under `ui/src/content/site-docs/{en,zh}/`.
- Add or remove guide routes through the typed catalog and keep both locale files aligned.
```

- [x] **Step 2: Separate hosted-user and operator quick starts**

```markdown
Use the hosted guide for `runner.qiniuinc.com`. The root README quick start remains the operator path for deploying runnerd.
```

Remove Issue #59 as the primary guide destination. Keep it only as historical context if still useful, and point current users to the site guide.

- [x] **Step 3: Record only unfinished measurement work in TODO**

```markdown
- Measure the public guide funnel from guide entry through Sandbox readiness to the first successful Job before choosing a site analytics implementation.
```

- [x] **Step 4: Verify docs structure, links, and whitespace**

```bash
test -f AGENTS.md
test -d .agents/rules
test -d ui/src/content/site-docs/en
test -d ui/src/content/site-docs/zh
git diff --check
```

Expected: PASS with paired indexes and no whitespace errors.

### Task 6: Production Verification And Browser Acceptance

**Files:**
- Modify: `ui/e2e/production-smoke.pw.ts`
- Modify if required by a reproduced failure: `ui/src/components/docs-page.tsx`

**Interfaces:**
- Consumes: complete feature implementation.
- Produces: fresh unit, localization, lint, build, embedded-asset, and browser evidence.

- [x] **Step 1: Run the focused and complete UI gates**

```bash
cd ui && bun run test
task ui-i18n-check
cd ui && bun run lint
```

- [x] **Step 2: Build the production embedded UI**

```bash
task build
```

- [x] **Step 3: Run the production browser smoke**

```bash
task ui-production-smoke
```

Expected: public landing and docs load without browser/console/resource errors, existing fixture-backed Jobs scrolling remains correct, and narrow docs layouts do not clip content.

- [x] **Step 4: Inspect real pages at desktop and mobile widths**

```text
Desktop: /docs, /docs/getting-started/hosted, /docs/troubleshooting
Mobile: 390x844 for the hosted guide and workflow code block
Language: switch English to Chinese without changing the current path
Navigation: landing -> hosted guide -> Jobs -> signed-out page -> hosted guide
```

- [x] **Step 5: Review the exact feature diff**

```bash
git status --short
git diff --stat
git diff --check
git diff -- ui docs README.md README.zh.md TODO.md AGENTS.md .agents/rules
```

Exclude the unrelated pre-existing `.gitignore` change from feature scope. Stop before commit, push, PR creation, or deployment unless separately authorized.

### Task 7: Custom Runner Template Build-To-Use Guide

**Files:**
- Create: `ui/src/content/site-docs/en/custom-templates.md`
- Create: `ui/src/content/site-docs/zh/custom-templates.md`
- Modify: `ui/src/site-doc-routes.ts`
- Modify: `ui/src/site-docs.ts`
- Modify: `ui/src/site-docs.test.js`
- Modify: `ui/src/content/site-docs/{en,zh}/index.md`
- Modify: `ui/src/content/site-docs/{en,zh}/runner-labels.md`
- Modify: `ui/e2e/production-smoke.pw.ts`
- Modify: root and agent documentation indexes and verification rules

- [x] **Step 1: Verify the custom template contract against qshell, runnerd, template scripts, and Runner Spec matching**
- [x] **Step 2: Add a failing exact-route, catalog, navigation, and content-contract test**
- [x] **Step 3: Write aligned English and Chinese guides from compatible image through remote build, remote verification, Runner Spec, Repository Policy, workflow labels, and rollout**
- [x] **Step 4: Add the guide to public navigation, indexes, label reference, and production browser smoke**
- [x] **Step 5: Run localization, lint, full test, production build, and responsive browser verification**
- [x] **Step 6: Close review findings for injected-script prerequisites and per-guide document metadata**

### Verification Evidence

- `task ui-i18n-check`: passed, including TypeScript build and 9 checker tests.
- `cd ui && bun run lint`: passed.
- `task test`: passed on the final implementation tree; 166 Bun tests passed with 528 expectations, followed by all Go race/coverage packages with exit code 0.
- `task build`: passed; the lazy-loaded docs chunk is 214.22 KB minified and 66.57 KB gzip, separate from the landing-page entry chunk.
- `RUNNERD_UI_SMOKE_PORT=4176 task ui-production-smoke`: 4/4 Chromium tests passed for the landing page, responsive hosted guide, responsive custom-template guide, and existing Jobs scroll behavior.
- Desktop (1440 px) and mobile (390 px) production-preview screenshots were inspected; navigation, typography, code blocks, responsive flow, and page width were acceptable. The inspection found and fixed the mobile Jobs link's accessible name.
- `git diff --check`: passed. The pre-existing `.gitignore` modification remains unrelated and unstaged.
