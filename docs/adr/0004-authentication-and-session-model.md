# ADR-0004: Authentication & session model

**Status:** Accepted (in implementation — Phase 2)
**Date:** 2026-07-02
**Deciders:** Jack Bishop

## Context

Hosted multi-user mode needs identity, but the local-first principle (ADR-0001)
requires that **auth is never a wall** — the CLI and the (future) iOS app work
offline with no account. Four sub-decisions: auth method, credential storage,
token strategy, and where users live before Postgres arrives (Phase 3).

## Decision

- **Two server modes, one binary:** single-user (default, unchanged behaviour,
  no login) vs **`--hosted`** (opt-in, multi-tenant).
- **Auth method:** email + password now; Sign in with Apple deferred to Phase 5
  (when the iOS client exists to exercise it).
- **Password hashing:** bcrypt cost 12.
- **Sessions:** opaque server-side tokens (32 random bytes), **SHA-256-hashed at
  rest**, 30-day sliding expiry; sent as `Authorization: Bearer` **and** an
  `HttpOnly`/`SameSite=Lax` cookie (`Secure` in hosted mode).
- **Per-user scoping:** `storage.Tenants{ ForUser(id) }` (ADR-0003 seam) returning
  a per-user `yamlstore` directory; store injected per-request via context.
- **User/session storage:** interim file-backed store (`internal/auth`), swapped
  for SQL in Phase 3. Keeps the Phase 2 → 3 order.
- **API:** clean break to `/api/v1` (only client is the co-updated SPA).

## Options Considered

| Decision | Chosen | Rejected & why |
|---|---|---|
| Auth method | email+password | SIWA-now — needs the iOS client + Apple plumbing with nothing to exercise it; not mandated unless other 3rd-party logins are offered |
| Token | opaque server sessions | JWT — no revocation without a denylist (reinvents sessions), plus key mgmt + refresh machinery, all to skip a lookup the server does anyway |
| Hashing | bcrypt cost 12 | argon2id — OWASP's pick but low-level in Go (manage salt/params or add a dep); bcrypt is quasi-stdlib, self-describing, upgradable, adequate at this threat model |
| Sequencing | keep 2→3, interim file store | merge 2+3 (big-bang, forces infra choices before the API contract settles); Postgres-first (build multi-tenant tables then retrofit `user_id` onto every row) |
| Auth gating | `--hosted` opt-in, default off | always-on auth — would force accounts onto self-hosters/CLI, violating ADR-0001 |

## Consequences

- Single-user mode stays **pixel-identical** to today (bar the `/api/v1` prefix);
  the CLI never touches the auth path.
- Handlers are mode-blind below the middleware line — they see a `storage.Store`
  and don't know if it's the whole store or one tenant directory.
- Users/sessions live in files until Phase 3 swaps in SQL behind the same
  interfaces; `sessions.yml` is the highest-write file (acceptable at small scale).
- **Security refinements from review** (folded into the build): constant-time
  login via a dummy-hash compare on unknown emails; signup input validation
  (min password length + email format).
- **Password reset is deferred to Phase 4** (needs the email channel) — tracked in
  #33; an interim manual user-deletion escape hatch is documented for testers.
- Not a pure refactor: adds endpoints and (in hosted mode) gates existing ones.

**Refs:** Phase 2, issue #15, follow-up #33; supersedes the original big-swing
framing of #15.
