# Security Remediation Ledger — ThirdEye Quotes Platform

This is the standard ledger for tracking findings from the 2026-07 robustness
audit and their remediation status. Every patch that closes an audit item MUST
be recorded here with the finding ID, the files touched, and the verification
notes. Companion document: [ARCHITECTURE.md](ARCHITECTURE.md) (§ "Known
implementation notes for operators").

**Remediation goal:** bring the legacy quotes stack up to the strict
*fail-closed* posture already demonstrated by the MCP server
(`mcp-server/internal/auth/middleware.go`).

---

## Remediated

### C-1 — Auth middleware did not halt the handler chain (auth bypass)
| | |
|---|---|
| **Status** | ✅ Remediated — 2026-07-12 (Phase 1) |
| **Severity** | Critical |
| **Files** | `api/src/handlers/auth.go` |

**Defect.** In Gin, returning from a middleware without `c.Abort()` does not
stop the chain. `AuthMiddleware` (a) kept executing after `AbortWithStatus(401)`,
double-writing a 500 over the 401, and (b) on decode errors wrote a 500 and
returned *without* aborting, allowing the protected handler to run with an
empty user. `AdminAuthMiddleware` compared against `os.Getenv("ADMIN_ID")` per
request — an unset `ADMIN_ID` (`""`) matched any empty uid (privilege
escalation under misconfiguration), used panic-prone `c.MustGet`, and called
`c.Next()` after aborting.

**Fix.**
- Every rejection path now terminates via `c.AbortWithStatusJSON(...)` with a
  standardized `models.ErrorResponse` envelope, followed by `return`.
- Missing header → 401; unknown key (`errors.Is(err, mongo.ErrNoDocuments)`)
  → 401; any other lookup/decode failure → 500. **Fail closed** in all cases.
- `AdminAuthMiddleware` reads `ADMIN_ID` once at construction and refuses all
  admin traffic with 500 when it is unset; `c.MustGet` → `c.Get` (no panic).
- Auth DB lookup now runs under `c.Request.Context()` (per-request
  cancellation) instead of the boot-time context.
- Removed `fmt.Println("ERROR:", key)` — presented credentials are never
  logged.

**Verification.** Manual trace of all four paths (no header / unknown key /
DB error / valid key) confirms `c.Next()` is reachable only after a successful
credential decode. Recommended smoke test:
`curl -X POST https://<host>/quote` (expect 401 envelope, no handler side
effects) and `curl -H "Authorization: bogus" -X POST .../quote` (expect 401).

---

### C-2 — `dataops.py` password env-var typo + TOCTOU race
| | |
|---|---|
| **Status** | ✅ Remediated — 2026-07-12 (Phase 1) |
| **Severity** | Critical |
| **Files** | `web/quotes-web/web/main/dataops.py` |

**Defect.** (1) `dbpass` read `MONGO_INITDB_ROOT_USERNAME` — copy-paste bug;
every connection authenticated as `username:username`. (2) `exists()` used
check-then-insert (`count_documents` → `insert_one`): two concurrent requests
for the same email could both pass the check and both insert. (3) A full
`MongoClient` was constructed and torn down per call.

**Fix.**
- `dbpass` now reads `MONGO_INITDB_ROOT_PASSWORD` (env-injected, per config
  directive — nothing hardcoded).
- TOCTOU removed: `exists()` performs a single **atomic** `insert_one`
  arbitrated by a **unique index on `email`**; the duplicate case is caught
  organically via `except DuplicateKeyError`. No pre-check remains.
- The unique index is ensured lazily (double-checked locking) and
  **fail-closed**: if it cannot be confirmed, `exists()` raises rather than
  running a non-atomic insert.
- One module-level `MongoClient` per worker with defensive timeouts
  (`serverSelectionTimeoutMS=3000`, `connectTimeoutMS=3000`,
  `socketTimeoutMS=5000`).

**Notes.** This module is currently dormant (active routes use the Go API for
the token flow — see ARCHITECTURE.md §2.2), so no live traffic is affected.
Public contract of `exists(email) -> bool` is unchanged (True = already
present). Dependency `pymongo==4.3.3` already declared in `requirements.txt`.

---

### C-3 — Committed secrets moved to env-injected config; secrets flagged for rotation
| | |
|---|---|
| **Status** | ✅ Code remediated — 2026-07-12 (Phase 2) · ⚠️ **rotation pending human action before re-deploy** |
| **Severity** | Critical |
| **Files** | `web/quotes-web/web/__init__.py`, `web/Dockerfile`, `web/dkim/thirdeye.live.omail.pem` |

**Defect.** Flask `SECRET_KEY` (session-cookie signing) and the reCAPTCHA key
pair were hard-coded in `__init__.py`; the DKIM **private** key was committed to
`web/dkim/` and baked into the image via `COPY dkim /etc/dkim`.

**Fix.**
- `SECRET_KEY`, `RECAPTCHA_PUBLIC_KEY`, `RECAPTCHA_PRIVATE_KEY` now loaded via
  `_require_env()`, which **fails closed** (raises at boot) if any is
  absent/empty.
- Added session-cookie hardening: `SESSION_COOKIE_SECURE/HTTPONLY/SAMESITE`.
- Dockerfile no longer copies `dkim/` into the image; the key is mounted
  read-only at runtime (`/etc/dkim:ro`).
- The committed `.pem` was replaced with a REVOKED placeholder + rotation
  instructions.

**⚠️ Required human action before re-deploy:**
1. Rotate `SECRET_KEY` (invalidates existing sessions — expected).
2. Rotate the reCAPTCHA key pair in the Google admin console.
3. Generate a new DKIM key, publish the public half in the
   `omail._domainkey.thirdeye.live` TXT record, mount the private half.
4. Purge the old DKIM key from git history (`git filter-repo`) — it remains
   compromised in history until then.
5. Add `SECRET_KEY`, `RECAPTCHA_PUBLIC_KEY`, `RECAPTCHA_PRIVATE_KEY` to
   `.environs` and the `web` service `environment:` block; add the
   `- /opt/.../dkim:/etc/dkim:ro` volume.

---

### C-5 — Unescaped user regex into Mongo `$regex` (ReDoS / injection)
| | |
|---|---|
| **Status** | ✅ Remediated — 2026-07-12 (Phase 2) |
| **Severity** | Critical |
| **Files** | `api/src/dbs/operations.go` |

**Fix.** `CreateRegexQueryString` now `regexp.QuoteMeta`-escapes every
user-supplied segment before splicing, so only our own anchors/separators carry
regex semantics — catastrophic-backtracking and wildcard-match inputs are
neutralized. The `default` branch also replaces the O(N²) concat loop with
`strings.Join`.

---

### C-6 — Tarot handler panics + unbounded-spread DoS
| | |
|---|---|
| **Status** | ✅ Remediated — 2026-07-12 (Phase 2) |
| **Severity** | Critical |
| **Files** | `api/src/handlers/tarot.go` |

**Fix.** Added `maxSpreadPositions` (78) bound rejecting empty/oversized
spreads before any DB work; guarded every slice index (`card[0]`,
`rand.Intn(len)`) against empty results; fixed the `RandomCardHandler`
off-by-one (`Intn(len-1)` → `Intn(len)`); removed per-request `rand.Seed`
(auto-seeded since Go 1.20); replaced magic `70-1` with `deckCount`; routed all
queries through `c.Request.Context()`.

---

### C-7 — Missing outbound timeouts + unguarded landing page (worker starvation)
| | |
|---|---|
| **Status** | ✅ Remediated — 2026-07-12 (Phase 2) |
| **Severity** | Critical |
| **Files** | `web/quotes-web/web/main/operations.py`, `routes.py`, `web/flask_recaptcha.py` |

**Fix.** All outbound `requests` calls now use a pooled `Session` with an
explicit `(3.05, 10)` timeout and retry/backoff on 502/503/504. `home()` wraps
the upstream quote fetch and serves a fallback quote on failure; POST re-render
uses `session.get(...)` defaults. reCAPTCHA `verify()` gained a timeout and
**fails closed** (verify False) on transport/parse error. AJAX routes now
return real 400/403/503 status codes (was P-tier O-8, folded in here).
Also folded **P-2**: token-notification mail moved from an unbounded
thread-per-request to a bounded `ThreadPoolExecutor(max_workers=2)` with logged
failures.

---

### C-4 — Inverted `FLASK_DEBUG` → debug-on-by-default (latent Werkzeug RCE)
| | |
|---|---|
| **Status** | ✅ Remediated — 2026-07-12 (Phase 2) |
| **Severity** | Critical |
| **Files** | `web/quotes-web/wsgi.py` |

**Fix.** Debug is now **off** unless `FLASK_DEBUG` is explicitly truthy
(allow-list `1/true/yes/on`, avoiding the `bool("0") == True` trap). Fails
closed to production posture.

---

### C-8 — Injection into `mongo --eval` via script arguments
| | |
|---|---|
| **Status** | ✅ Remediated — 2026-07-12 (Phase 2) |
| **Severity** | Critical |
| **Files** | `scripts/add_user.sh`, `scripts/remove_user.sh`, `scripts/find_user_by_post_id.sh` |

**Fix.** Each script validates its argument against a strict allow-list regex
(email pattern / 24-char hex ObjectId) **before** interpolation, and aborts
otherwise. The permitted character classes contain no quotes/braces/backslashes,
so a passing value cannot break out of the JS literal. Also fixed a `$OBJ_Id`
typo in the find script.

---

### C-9 — Email decode corruption of plus-addressed addresses
| | |
|---|---|
| **Status** | ✅ Remediated — 2026-07-12 (Phase 2) |
| **Severity** | Critical |
| **Files** | `api/src/handlers/token.go`, `web/quotes-web/web/main/operations.py` |

**Fix.** Web tier sends the email percent-encoded and untransformed (no more
`@`→`+`). API `decodeEmailParam()` uses the address as-is when it contains `@`,
else restores `@` at the **last** `+` (legacy-compat). The non-idempotent-GET
concern is documented as an accepted deviation pending a coordinated POST
contract change (see below).

---

### C-10 — Boot-time DB error handling, client shadowing, server timeouts, graceful shutdown
| | |
|---|---|
| **Status** | ✅ Remediated — 2026-07-12 (Phase 2) |
| **Severity** | Critical |
| **Files** | `api/src/main.go` |

**Fix.** `init()` now assigns the package-level `client` (no shadowing) and
checks the `Connect` error independently of `Ping`, both under a 10s timeout;
`ListDatabaseNames` failure is non-fatal. `main()` replaces `router.Run` with
an explicit `http.Server` carrying Read/Write/Idle/ReadHeader timeouts
(Slowloris defense) and a SIGINT/SIGTERM graceful-shutdown path that drains
in-flight requests and disconnects Mongo. Normalized the `DELETE
/admin/tokens/:id` route (was missing its leading `/`).

---

### C-11 — Stored XSS via `.html(JSON.stringify(...))`
| | |
|---|---|
| **Status** | ✅ Remediated — 2026-07-12 (Phase 2) |
| **Severity** | Critical |
| **Files** | `web/quotes-web/web/static/js/main.js` |

**Fix.** All seven AJAX success sinks changed from `.html()` to `.text()`, so
attacker-controlled quote content is rendered as inert text, never parsed as
HTML.

---

### P-1 — Search N+1 queries, no dedupe/cap, swallowed decode errors
| | |
|---|---|
| **Status** | ✅ Remediated — 2026-07-12 (Phase 3) |
| **Severity** | Performance |
| **Files** | `api/src/handlers/quotes.go` |

**Fix.** `SearchQuotesHandler` now builds ONE `$text` `$search` string mixing
quoted phrases (`%q`) and bare terms, issuing a single bounded query
(`SetLimit(200)`) instead of N+1 sequential round-trips. Dedupe is implicit (one
query can't return a document twice); `curs.All` replaces the manual loop and
surfaces decode errors (previously discarded); empty query → 400; runs under
`c.Request.Context()`.

---

### P-4 — `RandomQuoteHandler` `$sample size:2` nested-loop
| | |
|---|---|
| **Status** | ✅ Remediated — 2026-07-12 (Phase 3) |
| **Severity** | Performance (+ correctness) |
| **Files** | `api/src/handlers/quotes.go` |

**Fix.** Samples exactly one document (`$sample size:1`) via `mongo.Pipeline`
and decodes with `curs.All` — halves fetched data, removes the accidental
nested-`Next` loop, and surfaces decode errors. Wire shape unchanged (array of
one quote).

---

### P-3 — Docker images: unpinned single-stage Go + EOL Debian base
| | |
|---|---|
| **Status** | ✅ Remediated — 2026-07-12 (Phase 3) |
| **Severity** | Performance / Security |
| **Files** | `api/Dockerfile`, `web/Dockerfile` |

**Fix (api).** Multi-stage build: pinned `golang:1.19` builder with cached
`go mod download`, static stripped binary, shipped on
`gcr.io/distroless/static-debian12:nonroot` (~15MB, no shell, non-root).
**Fix (web).** Moved off EOL `slim-buster` → supported `python:3.10-slim-bookworm`
(same Python minor, so the vendored-package path is unchanged); dependency layer
cached before source; runs as non-root `appuser`; gunicorn gains
`--timeout 30 --graceful-timeout 30`.

---

### O-1 — Fragile error-string matching → driver sentinels
| | |
|---|---|
| **Status** | ✅ Remediated — 2026-07-12 (Phase 4) |
| **Files** | `api/src/handlers/quotes.go`, `api/src/handlers/tarot.go` |

**Fix.** `GetQuoteHandler` and `TarotDeckHandler` now branch on
`errors.Is(err, mongo.ErrNoDocuments)` instead of comparing the human-readable
`"mongo: no documents in result"` string (which can change between driver
releases).

---

### O-2 — Ignored `ObjectIDFromHex` errors + unchecked write counts
| | |
|---|---|
| **Status** | ✅ Remediated — 2026-07-12 (Phase 4) |
| **Files** | `api/src/handlers/quotes.go`, `api/src/handlers/token.go` |

**Fix.** Every `:id` handler (`GetQuote`, `UpdateQuote`, `DeleteQuote`,
`UpdateTokenRequest`, `DeleteTokenRequest`) now returns **400** on a malformed
ObjectId instead of silently querying the zero value, and Update/Delete return
**404** when `MatchedCount`/`DeletedCount == 0` instead of reporting a
misleading success. List handlers switched to `curs.All`, surfacing decode
errors previously discarded. The redundant, schema-polluting `{"id", id}`
`$set` field was dropped from both update handlers.

---

### O-3 — `UpdateTokenRequest` returned an empty 200
| | |
|---|---|
| **Status** | ✅ Remediated — 2026-07-12 (Phase 4) |
| **Files** | `api/src/handlers/token.go` |

**Fix.** The handler now emits an explicit `TokenRequestSuccessResponse`
(mirroring the other mutating token handlers) instead of falling off the end
with no body.

---

### O-4 — Struct-tag typo could leak API keys
| | |
|---|---|
| **Status** | ✅ Remediated — 2026-07-12 (Phase 4) |
| **Files** | `api/src/models/users.go` |

**Fix.** `Authorization` tag `json:"authorization" json:"authorization"`
(duplicate json key, no bson tag) → `json:"-" bson:"authorization"`: correctly
mapped from Mongo and never serialized outward in JSON responses.

---

### O-5 — No-op `Distinct` copy loops
| | |
|---|---|
| **Status** | ✅ Remediated — 2026-07-12 (Phase 4) |
| **Files** | `api/src/handlers/quotes.go`, `api/src/handlers/tarot.go` |

**Fix.** `GetAuthorsHandler`/`ListDecksHandler` assign the `Distinct` result
(already `[]interface{}`) directly to `Names` instead of copying element by
element.

---

### O-6 — `endpoints.py` misused `NamedTuple` as a namespace
| | |
|---|---|
| **Status** | ✅ Remediated — 2026-07-12 (Phase 4) |
| **Files** | `web/quotes-web/web/main/endpoints.py` |

**Fix.** Replaced the fieldless `NamedTuple` subclass with a plain namespace
class; base URL is now read from `QUOTES_API_BASE` (env-overridable, default
unchanged). Values remain `ParseResult` objects so `operations.py` call sites
are untouched.

---

### O-7 — Latent `NameError` in `mail.py`
| | |
|---|---|
| **Status** | ✅ Remediated — 2026-07-12 (Phase 4) |
| **Files** | `web/quotes-web/web/main/mail.py` |

**Fix.** Imported `MIMEBase` and `encoders` (referenced but never imported —
the first attachment ever built would crash); fixed the `octate-stream` typo;
renamed the shadowed `key` variable to `privkey` in `generate_dkim`.

---

### O-9 — Vendored reCAPTCHA fails open + swallowed import errors
| | |
|---|---|
| **Status** | ✅ Remediated — 2026-07-12 (Phase 4) |
| **Files** | `web/flask_recaptcha.py` |

**Fix.** The `except ImportError: print(...)` (which then guaranteed a
`NameError`) now lets the import fail loudly. `init_app` reads
`RECAPTCHA_PUBLIC_KEY`/`RECAPTCHA_PRIVATE_KEY` in addition to the original
`*_SITE_KEY`/`*_SECRET_KEY`, so the instance can no longer silently disable
itself via a config-name mismatch (the fail-open trap). Transport-level
fail-closed behavior was already added under C-7.

**Context.** Actual captcha enforcement is via Flask-WTF's `RecaptchaField`
(`forms.py` + `index.html`), which was already functional; this vendored module
is otherwise inert (no template uses bare `{{ recaptcha }}`; its `verify()` is
not called).

---

## Open

*None.* All CRITICAL, PERFORMANCE, and OPTIMIZATION findings from the 2026-07
audit are code-remediated.

**Notes:**
- P-2 and O-8 were folded into the C-7 patch.
- **C-3 still requires human credential rotation before re-deploy** (see the
  C-3 entry above) — this is the only outstanding action.
- Future/deferred (not audit-blocking): C-9's GET→POST contract change for
  `/admin/tokens/:email` requires coordinated client updates; consider a
  Go language-version bump (currently pinned to 1.19 to match `go.mod`).

---

## Ledger conventions

- One entry per finding ID; move rows from *Open* to *Remediated* with date,
  files touched, fix summary, and verification notes.
- Never record secret values here — reference env var names only.
- Update `ARCHITECTURE.md` "Known implementation notes for operators" when a
  patch retires a documented behavior.

*Last updated: 2026-07-12 — Phase 1 (C-1, C-2) + Phase 2 (C-3…C-11, incl. P-2/O-8) + Phase 3 (P-1, P-3, P-4) + Phase 4 (O-1…O-7, O-9) complete. Entire 2026-07 audit backlog code-remediated. Sole outstanding action: C-3 credential rotation before re-deploy.*
