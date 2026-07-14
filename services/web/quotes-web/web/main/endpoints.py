import os
from urllib.parse import urlparse

# Base URL of the Go quotes API. O-6/config directive: overridable via env,
# defaulting to the compose static IP so local/staging can retarget without a
# code change.
BASE = urlparse(os.environ.get("QUOTES_API_BASE", "http://172.255.255.3:8080/"))


class endpoints:
    """Namespace of API endpoint URL components.

    O-6: this previously subclassed typing.NamedTuple purely as a namespace,
    which is a misuse — the assignments below are plain class attributes, not
    NamedTuple fields (they carry values, not annotations), so none of
    NamedTuple's machinery ever applied. A plain class states the intent
    honestly. Values remain urllib ParseResult objects; callers use
    `.geturl()` (and `.format(...)` for the templated `{0}` paths), so the
    existing call sites in operations.py are unchanged.
    """
    quote = BASE._replace(path='quote')
    search = BASE._replace(path='quote/search')
    quote_id = BASE._replace(path='quote/{0}')
    authors = BASE._replace(path='authors')
    quotes_by_author = BASE._replace(path='authors/{0}')
    email = BASE._replace(path='admin/tokens/{0}')
