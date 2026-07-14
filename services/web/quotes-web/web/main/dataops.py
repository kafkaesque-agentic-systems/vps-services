"""Direct MongoDB data operations for the quotes-web service.

SECURITY (Audit C-2) — this module was patched for two critical defects:

1. Credential typo: ``dbpass`` previously read ``MONGO_INITDB_ROOT_USERNAME``
   (a copy-paste bug), so every connection authenticated as username:username.
   It now reads ``MONGO_INITDB_ROOT_PASSWORD``.

2. TOCTOU race in ``exists()``: the old code did ``count_documents`` (check)
   followed by ``insert_one`` (use). Two concurrent requests for the same
   email could both observe count == 0 and both insert. The check-then-act
   pattern has been replaced with a single ATOMIC ``insert_one`` arbitrated by
   a unique index on ``email``; the duplicate case is caught organically via
   ``DuplicateKeyError`` instead of being pre-checked.

Also folded in: one module-level ``MongoClient`` per process (pymongo pools
connections internally and is thread-safe) instead of constructing and tearing
down a full client — connection pool, topology monitor and all — on every call.
"""
import os
import threading

from pymongo import ASCENDING, MongoClient
from pymongo.errors import DuplicateKeyError

# C-2 fix: password now comes from the PASSWORD variable (was *_USERNAME).
# Both values are injected via env config — never hardcoded.
dbuser = os.environ.get("MONGO_INITDB_ROOT_USERNAME")
dbpass = os.environ.get("MONGO_INITDB_ROOT_PASSWORD")
uri = f"mongodb://{dbuser}:{dbpass}@172.255.255.2:27017/test?authSource=admin"

# One client per worker process. Defensive timeouts ensure a dead/unreachable
# Mongo fails fast instead of hanging a gunicorn worker indefinitely.
_client = MongoClient(
    uri,
    serverSelectionTimeoutMS=3000,  # fail fast if no server is reachable
    connectTimeoutMS=3000,          # TCP connect deadline
    socketTimeoutMS=5000,           # per-operation socket deadline
)
_tokens = _client.qdb["tokens"]

# The atomicity of exists() depends entirely on the unique index below.
# It is ensured lazily (first call) so importing this module never requires a
# live database, and the success flag is cached so the hot path pays nothing.
_index_lock = threading.Lock()
_index_ready = False


def _ensure_unique_email_index():
    """Guarantee the unique index on ``email`` exists before any insert.

    FAIL CLOSED: if the index cannot be confirmed, this raises and the caller
    aborts. We must never fall back to a non-atomic insert path, because that
    silently reintroduces the TOCTOU race this patch removes.
    """
    global _index_ready
    if _index_ready:  # fast path: already confirmed for this process
        return
    with _index_lock:
        if _index_ready:  # re-check under the lock (double-checked locking)
            return
        # create_index is idempotent: a no-op if the index already exists.
        _tokens.create_index([("email", ASCENDING)], unique=True)
        _index_ready = True


def fetch_token_requests():
    # fetch all the token request records
    return list(_tokens.find({}))


def exists(email: str) -> bool:
    """Atomically register ``email`` if new; report whether it already existed.

    Returns True when the email was already present, False when it was just
    inserted — same contract as before the patch.

    C-2 fix: no pre-check. We attempt the insert directly and let the unique
    index arbitrate concurrency. Exactly one concurrent caller wins the
    insert; every other caller receives DuplicateKeyError, which we translate
    to ``True``. There is no window in which two requests can both insert.
    """
    _ensure_unique_email_index()  # raises (fail closed) if not guaranteed

    try:
        _tokens.insert_one({"email": email, "granted": "false"})
        return False
    except DuplicateKeyError:
        # Organic failure path: the unique index rejected the duplicate.
        return True
