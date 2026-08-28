# Implementation Plan Archive

These files preserve design rationale and execution details for changes that have already landed. Their unchecked boxes describe the original execution sequence; they are not active project work. Use `TODO.md` for the current roadmap and current code, tests, and operator docs for shipped behavior.

| Plan | Lifecycle | Outcome |
| --- | --- | --- |
| [Homepage Performance Phase One](2026-07-22-homepage-performance-phase-one.md) | Historical, completed | Landed in `fcb16e3` (`perf(homepage): reduce route loading contention (#39)`) |
| [Public Site Guides](2026-08-12-site-guides.md) | Historical, completed | Landed in `b8e777f` (`feat(ui): add bilingual site guides (#72)`) |
| [Remove Runner Groups and Runner Policies](2026-08-13-remove-runner-groups-policies.md) | Historical, completed except separately authorized physical table cleanup | Release sequence completed through `f877cde`; remaining cleanup decision is tracked in `TODO.md` |
| [Admin Release Readiness](2026-08-20-admin-release-readiness.md) | Historical, completed then retired | Landed in `984bbdd`; the temporary readiness surface was removed by Release C |
| [Admin Readiness Drill-down](2026-08-21-admin-readiness-drilldown.md) | Historical, completed then retired | Landed in `33e8da4`; the temporary readiness surface was removed by Release C |
| [Release C And Unmatched Request Status](2026-08-25-release-c-and-unmatched-status.md) | Historical, completed | Landed in `f877cde` (`refactor(runner): complete Release C cleanup (#83)`) |
| [Runner Spec Path-Safe Name](2026-08-25-runner-spec-path-safe-name.md) | Historical, completed | Landed in `0bab1e0` (`fix(runner): reject path-unsafe Runner Spec names (#86)`) |
| [Runner Spec Template Validation](2026-08-26-template-validation.md) | Historical, completed | Landed in `5522d1a` (`fix(runner): validate templates before saving (#87)`) |

The implementation record for ordinary-user Runner management is [User-scoped Runner Configuration](../../user-scoped-runner-configuration.md). State, API, UI, and documentation changes have landed in staged commits; the plan records the remaining real-database, provider, GitHub, and deployment gates, which are tracked in `TODO.md`.
