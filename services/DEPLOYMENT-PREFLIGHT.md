# Deployment Pre-Flight — 2026-07 Audit Remediation

Single-page checklist to ship the four-phase remediation (C-tier, P-tier,
O-tier) safely. Companion docs: [SECURITY-REMEDIATION.md](SECURITY-REMEDIATION.md)
(per-finding detail) and [ARCHITECTURE.md](ARCHITECTURE.md).

> **Scope of this deploy:** `api` (Go quotes-server) and `web` (Flask
> quotes-frontend) images are rebuilt; admin `scripts/` are updated; MongoDB
> data is unchanged but **indexes must be verified**. The `dbs`, `reverse-proxy`,
> and `go-mcp` services are not modified.

---

## 0. 🚫 BLOCKING — do these before anything else

The `web` app now **fails closed at boot**: it will refuse to start until the
three secrets below exist in the environment. Rotate first, then inject.

- [ ] **Rotate `SECRET_KEY`** (Flask session signing). Any value works; a new
      one invalidates existing session cookies (expected, harmless).
      `python -c "import secrets; print(secrets.token_hex(32))"`
- [ ] **Rotate the reCAPTCHA key pair** in the Google admin console →
      new `RECAPTCHA_PUBLIC_KEY` + `RECAPTCHA_PRIVATE_KEY`.
- [ ] **Rotate the DKIM key:**
      `openssl genrsa -out thirdeye.live.omail.pem 2048`
      then publish the new public half in the
      `omail._domainkey.thirdeye.live` DNS TXT record.
- [ ] **Purge the old DKIM key from git history** (`git filter-repo --path
      services/web/dkim/thirdeye.live.omail.pem --invert-paths`) — it remains
      compromised in history until then. The working-tree file is already a
      revoked placeholder.

---

## 1. Environment variables

Add to `/opt/micro-services.d/quotes-api/.environs` **and** the compose
`environment:` blocks.

### `web` service — NEW required vars (fail-closed)
| Var | Notes |
|-----|-------|
| `SECRET_KEY` | rotated above |
| `RECAPTCHA_PUBLIC_KEY` | rotated above |
| `RECAPTCHA_PRIVATE_KEY` | rotated above |

### `web` service — NEW optional var
| Var | Default | Notes |
|-----|---------|-------|
| `QUOTES_API_BASE` | `http://172.255.255.3:8080/` | Override Go API base URL (O-6). Leave unset in prod. |
| `FLASK_DEBUG` | unset → **off** | Only set to `1/true/yes/on` for local debugging (C-4). Never in prod. |

### Unchanged but still required
`MONGO_INITDB_ROOT_USERNAME`, `MONGO_INITDB_ROOT_PASSWORD`, `MONGO_DATABASE`,
`ADMIN_ID` (api); `MAILSERVER`, `MAILPASS`, `AUTHORIZED` (web).

- [ ] **Confirm `ADMIN_ID` is set** for the `api` service. Admin routes now
      **fail closed with 500 when it is empty** (C-1) — an unset value no
      longer silently grants access, but it also blocks all admin traffic.

---

## 2. Compose changes

- [ ] Add the three new `web` env vars to its `environment:` list.
- [ ] Add the **runtime DKIM mount** to the `web` service (key is no longer
      baked into the image — C-3):
      ```yaml
      web:
        environment:
          - SECRET_KEY
          - RECAPTCHA_PUBLIC_KEY
          - RECAPTCHA_PRIVATE_KEY
          # ...existing...
        volumes:
          - /etc/ssl:/etc/ssl:ro
          - /opt/micro-services.d/quotes-api/dkim:/etc/dkim:ro   # NEW
      ```
- [ ] Place the rotated `thirdeye.live.omail.pem` at the host path above
      (mode `600`, owned so the container's `appuser` can read it).

---

## 3. Database index prerequisites

The token dedup (C-9) and text search (P-1) **depend on these indexes**. There
is no migration tooling — verify manually against the `quotes-database`
container:

```javascript
db.qdata.createIndex({ quote: "text", attribution: "text" })
db.qdata.createIndex({ ueid: 1 }, { unique: true })
db.tokens.createIndex({ email: 1 }, { unique: true })   // required for EmailExists atomicity
```

- [ ] Confirm all three exist (`db.<coll>.getIndexes()`); create any missing.

---

## 4. Build

- [ ] `go build ./...` and `go vet ./...` in `services/api/src` (compile gate;
      O-4 also clears a vet duplicate-tag warning).
- [ ] Python syntax gate:
      `python -c "import ast,glob; [ast.parse(open(f).read()) for f in glob.glob('services/web/**/*.py', recursive=True)]"`
- [ ] `bash -n services/scripts/*.sh`
- [ ] Build images:
      ```bash
      docker build -t quotes-server:latest   services/api
      docker build -t quotes-frontend:latest services/web
      ```
      Expect a **much smaller api image** (distroless, ~15MB vs ~800MB) and a
      Bookworm-based web image (P-3).

---

## 5. Deploy

- [ ] `source /opt/micro-services.d/quotes-api/.environs`
- [ ] Rolling update, api first (it has no new required env, lowest risk), then web:
      ```bash
      docker compose up -d --build api
      docker compose up -d --build web
      ```
- [ ] Watch web startup logs. A `RuntimeError: FATAL: required environment
      variable ... is not set` means a Section 1 secret is missing — fix env,
      not code.

---

## 6. Smoke tests (post-deploy)

Run from a host that can reach the proxy. Replace `<host>` and `<token>`.

**C-1 — auth fails closed**
```bash
curl -s -o /dev/null -w '%{http_code}\n' -X POST https://<host>/quote           # 401
curl -s -X POST -H "Authorization: bogus" https://<host>/quote                  # 401 {"status":401,"error":"invalid credentials"}
curl -s -X POST -H "Authorization: <valid>" -H 'Content-Type: application/json' \
     -d '{"attribution":"Test","quote":"hello"}' https://<host>/quote           # 200
```

**C-5 — regex is escaped (no ReDoS / 500)**
```bash
curl -s -o /dev/null -w '%{http_code}\n' 'https://<host>/authors/(a+)+$'        # 404, fast (not a hang)
```

**C-6 — tarot bounds & no panic**
```bash
curl -s -o /dev/null -w '%{http_code}\n' https://<host>/tarot/card              # 200
curl -s -X POST -H 'Content-Type: application/json' \
     -d '{"name":"x","deck":"any","positions":[]}' https://<host>/tarot/spread  # 400 (empty)
# oversized positions array (>78) → 400, no worker storm
```

**C-7 — landing page degrades, not 500** (stop `api`, then):
```bash
curl -s -o /dev/null -w '%{http_code}\n' https://<host>/                        # 200 (fallback quote)
```

**C-11 — XSS sink neutralized:** submit a quote containing
`<img src=x onerror=alert(1)>`, load the demo page, confirm it renders as inert
text (no alert).

**O-2 — id validation**
```bash
curl -s -o /dev/null -w '%{http_code}\n' https://<host>/quote/not-an-id         # 400
curl -s -o /dev/null -w '%{http_code}\n' https://<host>/quote/0123456789abcdef01234567  # 404
```

**P-1 — search single-query, deduped, capped:** search a common word; confirm
one result set (≤200), no duplicates.

**Admin scripts (C-8) — injection rejected**
```bash
./services/scripts/add_user.sh "x'}); db.users.drop(); //"                       # rejected: not a valid email
./services/scripts/find_user_by_post_id.sh "notanobjectid"                       # rejected: not 24-hex
```

---

## 7. Behavior changes reviewers/clients must know

- Auth failures return a JSON `ErrorResponse` body (was bare 401 status).
- `PUT/DELETE /quote/:id` and `/admin/tokens/:id`: **400** on malformed id,
  **404** when nothing matched (was always ~200).
- `PUT /admin/tokens/:id` now returns a JSON body (was empty 200).
- `POST /quote/search` returns **400** on an empty query and results are now
  **deduped and capped at 200**.
- API container is **distroless → no shell**; debug via `docker logs`, not
  `docker exec sh`. (Admin `scripts/` target the Mongo container, unaffected.)
- Deploys now drain gracefully (SIGTERM) — allow up to ~10s for api shutdown.

---

## 8. Rollback

Images are tagged `:latest` and built locally (no registry). To roll back,
`docker compose up -d` the previously-built image, or `git revert` the
remediation commits and rebuild. **Note:** rolling back `web` code does *not*
un-rotate secrets — the old hard-coded `SECRET_KEY`/reCAPTCHA/DKIM values are
burned regardless, so a rollback must still supply the new secrets via env.

---

## 9. Outstanding after this deploy

- **None blocking.** The entire 2026-07 audit backlog is code-remediated.
- Deferred (non-blocking, needs coordination): C-9 `GET → POST` contract change
  for `/admin/tokens/:email`; Go language-version bump (pinned 1.19 to match
  `go.mod`).

*Prepared 2026-07-12.*
