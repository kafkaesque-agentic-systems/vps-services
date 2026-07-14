"""HTTP operations against the Go quotes API.

RESILIENCE (Audit C-7): every outbound call in this module previously ran with
NO timeout. A stalled upstream (Go API, or anything between) pinned a gunicorn
sync worker indefinitely — with `-w 4`, four hung requests took the whole site
down. All calls now:

  * share one pooled `requests.Session` per worker (connection reuse),
  * carry an explicit (connect, read) TIMEOUT,
  * retry transient upstream failures (502/503/504) with backoff,
  * raise `requests.RequestException` on failure so callers decide the
    degradation policy — nothing here fails silently.

SECURITY (Audit C-9): the '@' -> '+' email transformation was removed. The
old scheme corrupted plus-addressed emails: 'user+tag@gmail.com' became
'user+tag+gmail.com' on the wire, which the API decoded (first '+') back to
'user@tag+gmail.com' — silently storing and mailing a WRONG address. The email
is now sent percent-encoded verbatim; the API accepts it as-is.
"""
import os
import re
import json
import requests
from requests.adapters import HTTPAdapter, Retry
from urllib.parse import quote as urlquote

from web.main.endpoints import endpoints as ep

SPACE = re.compile(r'\s+')
PUNCT = re.compile(r'[^\s\w]')

# (connect, read) deadlines — fail fast instead of starving workers (C-7).
TIMEOUT = (3.05, 10)

# One pooled session per worker process. Retries cover transient gateway
# errors only; connect errors get the default connect retries.
_session = requests.Session()
_retry = Retry(total=2, backoff_factor=0.2, status_forcelist=(502, 503, 504))
_session.mount("http://", HTTPAdapter(max_retries=_retry))
_session.mount("https://", HTTPAdapter(max_retries=_retry))


def get_random_quote():
    """Fetch one random quote. Raises requests.RequestException on failure
    (including non-2xx responses) — the route layer owns the fallback policy.
    """
    resp = _session.get(ep.quote.geturl(), timeout=TIMEOUT)
    resp.raise_for_status()
    return resp.json()


def exists(email: str) -> int:
    """Check/register a token request for `email` via the admin API.

    Returns the API's Result code (0 = newly registered, 1 = already
    requested) or 500 on ANY transport / server failure — the caller already
    treats 500 as "try again later" (fail closed, degrade loudly).

    C-9 fix: the email travels percent-encoded and untransformed. No more
    '@' <-> '+' round-trip, so plus-addressed emails survive intact.
    """
    url = ep.email.geturl()
    try:
        resp = _session.get(
            url.format(urlquote(email, safe='')),
            headers={"Authorization": os.environ.get("AUTHORIZED")},
            timeout=TIMEOUT,
        )
        if resp.ok:
            return resp.json()['Result']
        return 500
    except (requests.RequestException, ValueError, KeyError):
        # Transport error, malformed JSON, or missing key: never let an
        # upstream fault crash the request — surface the sentinel the
        # route layer already handles.
        return 500


def search_for_keywords(terms: str):
    def split_search_terms(query: str) -> tuple[list, list]:
        # splits the incoming search queries into words and phrases
        split = re.split(r'(?<!\\)\,\s*', terms)
        words = [n for n in split if ' ' not in n]
        phrases = [n.replace('\\', '') for n in split if ' ' in n]
        return words, phrases

    words, phrases = split_search_terms(terms)
    resp = _session.post(
        ep.search.geturl(),
        data=json.dumps({"terms": words, "phrases": phrases}),
        timeout=TIMEOUT,
    )

    return resp.json()


def get_quotes_by_author(name: str):
    url = ep.quotes_by_author.geturl()
    name = SPACE.sub(' ', name)         # remove extra whitespace
    name = PUNCT.sub('', name).lower()  # remove punctuation
    resp = _session.get(url.format(name.replace(' ', '-')), timeout=TIMEOUT)
    return resp.json()
