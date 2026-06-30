---
name: runnerd-dev-smoke
description: Use when verifying qiniu-ci-runner local startup, task dev, Vite proxy behavior, smee webhook forwarding, or development workflow fixes.
---

# Runnerd Dev Smoke

Start by saying: "I am using the runnerd-dev-smoke skill to verify the local runnerd development stack."

## Goal

Validate the real `task dev` path instead of only checking helper scripts.

## When To Use

- `task dev` fails or was changed.
- `scripts/smee.sh` changes.
- Development UI proxy behavior changes.
- The user asks to verify local startup.
- Docs around local dev, webhook forwarding, or Vite ports changed and need a real smoke.

## Preconditions

- Do not commit `.smee-url`, `runnerd.local.yaml`, sqlite DBs, private keys, or cookie jars.
- If default ports are occupied, use temporary ports and a temporary local config instead of stopping unrelated processes.
- Keep `SMEE_TARGET` aligned with the runnerd HTTP port being tested.

## Workflow

1. Inspect current startup definitions:

```bash
sed -n '1,260p' Taskfile.yaml
sed -n '1,220p' scripts/smee.sh
```

2. Check prerequisites:

```bash
command -v task
command -v reflex
test -x ui/node_modules/.bin/vite
command -v curl
```

3. Use a local config. Prefer `runnerd.local.yaml` if it is already valid. For smoke tests that must avoid collisions, create a temporary ignored config from `runnerd.yaml.example` and change only local-safe values such as `server.http_addr` and sqlite path.

4. Start the real dev entrypoint:

```bash
RUNNERD_CONFIG=<local-config> RUNNERD_VITE_PORT=<free-vite-port> SMEE_TARGET=http://127.0.0.1:<runnerd-port>/webhooks/github task dev
```

5. Verify service and admin UI routing:

```bash
curl -fsS http://127.0.0.1:<runnerd-port>/healthz
curl -I http://127.0.0.1:<runnerd-port>/admin/
```

If Vite is part of the issue, also check the Vite dev server URL selected by `task dev`.

6. Stop only the dev process you started. Do not kill unrelated listeners.

7. Sync docs if startup behavior changed:

- `README.md`
- `docs/testing.md`
- `TODO.md`
- `AGENTS.md`
- `.agents/rules/development-workflow.md`
- `.agents/rules/testing-and-verification.md`

## Output

Report:

- config and ports used;
- whether smee was skipped or forwarded;
- `healthz` result;
- `/admin/` result;
- any prerequisites that were missing;
- any docs updated to match the actual startup path.
