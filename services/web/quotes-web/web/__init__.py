"""Flask application factory wiring for quotes-web.

SECURITY (Audit C-3): this module previously hard-coded the Flask SECRET_KEY
and the reCAPTCHA key pair directly in source. The session-signing key and the
captcha private key are live credentials — committing them means anyone with
repository access can forge session cookies or bypass the captcha.

All secrets are now injected via environment variables (matching the
convention already used by the API and MCP services) and the app FAILS CLOSED
at boot when a required secret is absent: a loud crash at startup is far safer
than silently running with a missing or predictable key.

Required environment variables (add to .environs / compose `environment:`):
    SECRET_KEY               Flask session-signing key (rotate the old one!)
    RECAPTCHA_PUBLIC_KEY     reCAPTCHA site key
    RECAPTCHA_PRIVATE_KEY    reCAPTCHA secret key (rotate the old one!)
"""
import os

from flask import Flask
from flask_cors import CORS
from flask_recaptcha import ReCaptcha

from web.main.routes import main


def _require_env(name: str) -> str:
    """Fetch a mandatory secret from the environment, failing closed.

    Raising at import time aborts the gunicorn worker before it can serve a
    single request with a missing/empty secret. os.environ[name] alone would
    raise KeyError, but an explicit check also rejects empty-string values
    and produces an actionable, operator-friendly message.
    """
    value = os.environ.get(name, "")
    if not value:
        raise RuntimeError(
            f"FATAL: required environment variable {name} is not set. "
            "Refusing to start with missing secrets (fail-closed). "
            "See services/SECURITY-REMEDIATION.md (C-3)."
        )
    return value


app = Flask(__name__)
recaptcha = ReCaptcha(app)
cors = CORS(app, resources={r'/*': {'origins': 'http://localhost'}})

app.register_blueprint(main)

app.config['JSONIFY_PRETTYPRINT_REGULAR'] = True

# --- Secrets: env-injected only, never hard-coded (Audit C-3) ---------------
app.config['SECRET_KEY'] = _require_env("SECRET_KEY")
app.config['RECAPTCHA_PUBLIC_KEY'] = _require_env("RECAPTCHA_PUBLIC_KEY")
app.config['RECAPTCHA_PRIVATE_KEY'] = _require_env("RECAPTCHA_PRIVATE_KEY")
app.config['RECAPTCHA_USE_SSL'] = False

# --- Session-cookie hardening (folded into C-3) ------------------------------
# The site is served exclusively over HTTPS via the NGINX gateway, so the
# session cookie must never travel in cleartext, must be invisible to
# document.cookie, and should not ride along on cross-site requests.
app.config.update(
    SESSION_COOKIE_SECURE=True,     # HTTPS only
    SESSION_COOKIE_HTTPONLY=True,   # no JavaScript access
    SESSION_COOKIE_SAMESITE="Lax",  # CSRF hardening for state-changing GETs
)
