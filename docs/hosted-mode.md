# Hosted (multi-user) mode

MileMinder's server runs in one of two modes from the **same binary**:

- **Single-user (default).** `mileminder serve` — no login, one user, data in
  `~/.mileminder`. This is the self-hosted / local tier and is unchanged from
  before Phase 2. The CLI is always single-user.
- **Hosted (multi-tenant).** `mileminder serve --hosted` — signup/login, per-user
  data isolation, session cookies. Auth gates web/sync access only; the offline
  app and CLI never require an account.

Auth is **optional and never a wall in front of the app** — it exists solely to
enable the hosted web dashboard and (later) sync.

## Running hosted mode

```bash
mileminder serve --hosted --data-dir /var/lib/mileminder
```

Flags (all also settable by env var, for containers):

| Flag | Env | Default | Purpose |
|---|---|---|---|
| `--hosted` | `MILEMINDER_HOSTED` | off | Enable multi-user mode |
| `--data-dir` | `MILEMINDER_DATA_DIR` | `~/.mileminder-hosted` | Hosted data root |
| `--secure-cookies` | — | `true` | `Secure` flag on session cookies |
| `--alerts-interval` | `MILEMINDER_ALERTS_INTERVAL` | `1h` | Background alert sweep cadence |
| `--no-alerts` | — | off | Disable the hosted alert scheduler |

**TLS is assumed to be terminated by the platform** (Fly.io / Render / a reverse
proxy). Session cookies are set `Secure` by default, so they are only sent over
HTTPS. For plain-HTTP **local testing** on `localhost`, disable that:

```bash
mileminder serve --hosted --data-dir /tmp/mm-hosted --secure-cookies=false --no-browser
```

## Security model

- **Passwords** are hashed with bcrypt (cost 12).
- **Sessions** are opaque 256-bit tokens; only their SHA-256 is stored. Logout
  deletes the session. Sessions slide to a 30-day expiry on use.
- **Transport**: the SPA uses an `HttpOnly`, `SameSite=Lax` cookie (no token in
  JS). Native/CLI clients use `Authorization: Bearer <token>` (returned in the
  login/signup response body).
- **CSRF**: cookie-authenticated state-changing requests are rejected when the
  browser's `Sec-Fetch-Site` header marks them cross-site. Bearer requests, which
  carry no ambient credential, skip this check.
- **Rate limiting**: `/api/v1/auth/*` is per-IP rate limited (429 on abuse).

## Data layout

A hosted user's directory is **byte-compatible with a single-user `~/.mileminder`**
(vehicle `<id>.yml` files plus a `current` pointer):

```
<data-dir>/users.yml              # accounts (bcrypt hashes)
<data-dir>/sessions.yml           # active sessions (token hashes)
<data-dir>/alert_prefs.yml        # per-user alert preferences
<data-dir>/alerts_state.yml       # per-user/vehicle alert dedup state
<data-dir>/users/<userID>/<vehicleID>.yml
<data-dir>/users/<userID>/current
```

> These file-backed user/session stores are the **Phase 2 interim**. Phase 3
> replaces them with managed Postgres behind the same `auth.UserStore` /
> `auth.SessionStore` / `storage.Tenants` interfaces — no handler changes.

## Allowance alerts

Hosted mode starts an in-process scheduler unless `--no-alerts` is set. Each
sweep recomputes every user's policy vehicles and sends an email only when a
vehicle crosses from OK to breached. Existing breached vehicles are silently
recorded on first observation, so turning on alerts for old data does not send a
burst.

Alert preferences are available in Settings and through
`GET/PUT /api/v1/alerts/prefs`. Defaults are `enabled=true` and
`threshold=100`.

SMTP is configured only through environment variables:

| Env | Purpose |
|---|---|
| `MILEMINDER_SMTP_HOST` | SMTP host; unset means alerts are logged locally |
| `MILEMINDER_SMTP_PORT` | SMTP port, default `587` |
| `MILEMINDER_SMTP_USER` | SMTP username |
| `MILEMINDER_SMTP_PASS` | SMTP password |
| `MILEMINDER_SMTP_FROM` | From address, required when SMTP host is set |

When SMTP is unset, MileMinder uses a log channel and prints a startup warning.
That keeps local hosted mode fully runnable without email credentials.

## Claiming your existing data (migration by copy)

Because a hosted user directory has the same layout as `~/.mileminder`, moving
your local data into your hosted account is a file copy. Sign up first to get
your `userID` (visible as the directory name, or in the signup response `id`),
then:

```bash
cp ~/.mileminder/*.yml ~/.mileminder/current \
   <data-dir>/users/<your-userID>/
```

Restart is not required — the next request reads the copied files. (Bulk history
import via CSV is tracked separately in #8; real device↔server sync is Phase 5.)

## Recovering a locked-out user (until password reset ships)

Self-service password reset is **deferred to Phase 4** (#33) because it needs the
email channel. Until then, an operator recovers a user who has forgotten their
password manually:

1. Remove the user's entry from `<data-dir>/users.yml` (the block with their
   `email`). Note their `id` first — it's their data directory name.
2. Remove any of their rows from `<data-dir>/sessions.yml` (match `user_id`), or
   simply clear the file; that only signs everyone out.
3. Have them sign up again with the same email. This creates a **new** account
   with a **new** `id` and an empty garage.
4. To preserve their existing vehicles, move the old data directory onto the new
   id:

   ```bash
   mv <data-dir>/users/<old-id>/* <data-dir>/users/<new-id>/
   ```

This is deliberately manual and operator-only. Do not expose it as an endpoint.
