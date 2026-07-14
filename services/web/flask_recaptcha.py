"""
The new Google ReCaptcha implementation for Flask without Flask-WTF
Can be used as standalone
"""

__NAME__ = "Flask-ReCaptcha"
__version__ = "0.4.2"
__license__ = "MIT"
__author__ = "Mardix"
__copyright__ = "(c) 2015 Mardix"

# O-9: the previous version caught ImportError, printed a message, and
# CONTINUED — guaranteeing a NameError the moment any of these names was used.
# A missing hard dependency is fatal; re-raise so the failure is loud and
# immediate rather than deferred and cryptic.
from flask import request
from jinja2.utils import markupsafe
import requests


class DEFAULTS(object):
    IS_ENABLED = True
    THEME = "light"
    TYPE = "image"
    SIZE = "normal"
    TABINDEX = 0


class ReCaptcha(object):

    VERIFY_URL = "https://www.google.com/recaptcha/api/siteverify"
    site_key = None
    secret_key = None
    is_enabled = False

    def __init__(self, app=None, site_key=None, secret_key=None, is_enabled=True, **kwargs):
        if site_key:
            self.site_key = site_key
            self.secret_key = secret_key
            self.is_enabled = is_enabled
            self.theme = kwargs.get('theme', DEFAULTS.THEME)
            self.type = kwargs.get('type', DEFAULTS.TYPE)
            self.size = kwargs.get('size', DEFAULTS.SIZE)
            self.tabindex = kwargs.get('tabindex', DEFAULTS.TABINDEX)

        elif app:
            self.init_app(app=app)

    def init_app(self, app=None):
        # O-9: accept both the library's original key names AND the
        # RECAPTCHA_PUBLIC_KEY/RECAPTCHA_PRIVATE_KEY names the app actually
        # sets (and that Flask-WTF uses). Previously this read only
        # RECAPTCHA_SITE_KEY/RECAPTCHA_SECRET_KEY, which the app never defines,
        # so site_key resolved to None and the instance SILENTLY disabled
        # itself — making verify() fail open. Aligning the names removes that
        # trap.
        self.__init__(site_key=app.config.get("RECAPTCHA_SITE_KEY") or app.config.get("RECAPTCHA_PUBLIC_KEY"),
                      secret_key=app.config.get("RECAPTCHA_SECRET_KEY") or app.config.get("RECAPTCHA_PRIVATE_KEY"),
                      is_enabled=app.config.get("RECAPTCHA_ENABLED", DEFAULTS.IS_ENABLED),
                      theme=app.config.get("RECAPTCHA_THEME", DEFAULTS.THEME),
                      type=app.config.get("RECAPTCHA_TYPE", DEFAULTS.TYPE),
                      size=app.config.get("RECAPTCHA_SIZE", DEFAULTS.SIZE),
                      tabindex=app.config.get("RECAPTCHA_TABINDEX", DEFAULTS.TABINDEX))

        @app.context_processor
        def get_code():
            return dict(recaptcha=markupsafe.Markup(self.get_code()))

    def get_code(self):
        """
        Returns the new ReCaptcha code
        :return:
        """
        return "" if not self.is_enabled else ("""
        <script src='//www.google.com/recaptcha/api.js'></script>
        <div class="g-recaptcha" data-sitekey="{SITE_KEY}" data-theme="{THEME}" data-type="{TYPE}" data-size="{SIZE}"\
         data-tabindex="{TABINDEX}"></div>
        """.format(SITE_KEY=self.site_key, THEME=self.theme, TYPE=self.type, SIZE=self.size, TABINDEX=self.tabindex))

    def verify(self, response=None, remote_ip=None):
        if self.is_enabled:
            data = {
                "secret": self.secret_key,
                "response": response or request.form.get('g-recaptcha-response'),
                "remoteip": remote_ip or request.environ.get('REMOTE_ADDR')
            }

            # RESILIENCE (Audit C-7): the verification call previously had no
            # timeout (a hung Google endpoint pinned a gunicorn worker) and
            # r.json() could raise on a malformed body. FAIL CLOSED: any
            # transport error, timeout, or unparsable response verifies False.
            try:
                r = requests.get(self.VERIFY_URL, params=data, timeout=(3.05, 10))
                return r.json().get("success", False) if r.status_code == 200 else False
            except (requests.RequestException, ValueError):
                return False
        return True
