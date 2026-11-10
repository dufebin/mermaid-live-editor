# Go Backend + Registration/Login + Per-User Data Isolation Design

Date: 2026-11-10
Status: Approved (pending implementation)

## Goal

Replace the Node storage API with a behavior-identical Go service, then add
registration/login so every user has their own private diagrams and folders.

Deployment context: LAN / small team (a few to a few dozen trusted people).
Email + password, HttpOnly cookie sessions, no email verification, no
anti-abuse machinery. Registration is open to anyone who can reach the
instance.

## Phases (one spec, two independently verifiable phases)

- **Phase A — Behavior-identical Go port.** Rewrite the existing storage API
  (folders/diagrams CRUD, import/export, positions) in Go, listening on the
  same port 18790. Acceptance: the entire existing e2e suite passes unchanged.
  After acceptance, delete `server/*.mjs` and point the systemd unit's
  ExecStart at the Go binary.
- **Phase B — Auth + per-user data.** The feature work, implemented in Go on
  top of Phase A.

Rationale: land the port first with the full e2e suite as the parity net,
then build auth on a verified foundation.

## Go Project Layout

```
server-go/
  go.mod            # modernc.org/sqlite + golang.org/x/crypto/scrypt (the only deps)
  main.go           # http server, routing, JWT middleware, config from env
  db.go             # connection, idempotent schema migrations (reuses existing DB file)
  auth.go           # register/login/logout/me, scrypt hashing, JWT sign/verify
  handlers.go       # folders/diagrams/import/export handlers
  Makefile          # build -> bin/mermaid-storage
```

- Environment contract preserved: `HOST` (default 127.0.0.1), `PORT`
  (default 18790), `DATA_DIR` (default `server-data`), plus new
  `MERMAID_JWT_SECRET` (auto-generated and persisted to
  `server-data/jwt-secret` when absent).
- The `pnpm server` npm script is replaced by running the built binary
  (`make -C server-go build` + `bin/mermaid-storage`, or `make run`). The
  systemd unit `ExecStart` points at the binary — this also removes the
  known `pnpm server` built-in-command pitfall.
- Same SQLite file (`server-data/diagrams.db`), same schema, same JSON API
  shapes — the frontend does not change for Phase A.

## Auth Flow (Phase B)

| Endpoint                  | Behavior                                                                                                                                                                                                                                 |
| ------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `POST /api/auth/register` | email + password; hashes with scrypt; issues JWT and Set-Cookie. **The first registered user atomically claims every existing folder/diagram row with NULL user_id** (single transaction; later registrants start with an empty library) |
| `POST /api/auth/login`    | verifies scrypt hash; issues JWT and Set-Cookie                                                                                                                                                                                          |
| `POST /api/auth/logout`   | clears the cookie (client-side; stateless server has nothing to revoke)                                                                                                                                                                  |
| `GET /api/auth/me`        | returns `{id, email}` or 401                                                                                                                                                                                                             |

**JWT spec**: HS256, hand-rolled on `crypto/hmac` + `crypto/sha256` (~40
lines); claims `sub` (userId), `iat`, `exp` (7 days). Verify pins alg=HS256
(alg-confusion guard) and compares signatures with a constant-time compare.

**Cookie**: `HttpOnly; SameSite=Lax; Path=/; Max-Age=7d` — no `Secure`
(HTTP on LAN). Logout does not revoke; old tokens die at expiry (accepted
tradeoff of stateless JWT). Password change does not kill old tokens either.

Password storage: scrypt (work params recorded as constants in `auth.go`),
random per-user salt.

**Input validation (server-side, mirrored client-side):** email must match a
basic `user@host.tld` shape (lowercased before storing/lookup); password must
be 8–128 characters. Violations return 400 with a clear message; no other
complexity rules.

## Data Isolation (SQL-enforced)

- New `users` table: `id TEXT PRIMARY KEY, email TEXT UNIQUE NOT NULL,
password_hash TEXT NOT NULL, created_at TEXT NOT NULL`.
- `folders` and `diagrams` gain `user_id TEXT REFERENCES users(id)` columns
  (existing rows NULL until claimed).
- Every query and write takes a required `userId` and filters by
  `user_id = ?` in SQL. Isolation is enforced at the SQL layer; the frontend
  only handles UX.
- `main.go` middleware: every `/api/*` route except `/api/auth/*` requires a
  valid JWT; invalid → 401 with the userId extracted and passed to handlers
  otherwise.

## Frontend (Phase B)

- New routes `/login` and `/register` sharing one form component
  (email, password, confirm-password on register).
- Navbar: "Login" button when anonymous; user menu (email + logout) when
  authenticated.
- `/edit` and dashboard pages: on startup, `GET /api/auth/me`; 401 → redirect
  to `/login?redirect=<original path>`.
- **`/view` and `/embed` stay public** — share links carry full state in the
  URL hash and need no API; gating them would break sharing.
- `src/lib/util/api.ts` centralizes 401 handling (one redirect trigger);
  individual call sites do not each handle it.
- localStorage `codeStore` is untouched — it is per-browser editor state,
  unrelated to accounts.

## Migration & Compatibility

- Schema changes use the existing idempotent `ALTER TABLE ... catch`
  pattern (precedent: the `positions` column).
- Existing rows keep NULL `user_id` until the first registration claims them.
- Concurrent first registrations: claiming runs in one transaction;
  first-writer wins, the second user simply registers with an empty library.
- API contract is unchanged for Phase A; Phase B adds the four `/api/auth/*`
  endpoints and the 401 behavior.

## Testing

- **Go unit tests** (`go test ./...` in `server-go`): JWT sign/verify (valid,
  tampered, expired, wrong secret, non-HS256 alg), scrypt hash/verify, NULL
  claiming migration, per-user query isolation at the db layer.
- **Phase A acceptance**: the existing Playwright suite runs unchanged
  against the Go server; row counts in the DB file match the Node server's
  after the same operations.
- **Phase B e2e** (Playwright + `TID`): register → first user sees existing
  data claimed; user A saves a diagram → logout → register user B → B does
  not see A's diagrams; logout then visiting `/edit` redirects to `/login`;
  `/view` share link works while anonymous.
- Existing tests that call the API anonymously gain a login fixture.
