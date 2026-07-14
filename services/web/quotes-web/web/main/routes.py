import logging
from concurrent.futures import ThreadPoolExecutor

import requests
from flask import jsonify
from flask import request
from flask import session
from flask import Blueprint
from flask import render_template
from flask import redirect, url_for, flash

from web.main.forms import RequestTokenForm
from web.main.notifications import send_notification, token_server_error
from web.main.notifications import token_request_response, token_request_failure
from web.main.operations import get_random_quote, search_for_keywords
from web.main.operations import get_quotes_by_author, exists

main = Blueprint('main', __name__)

logger = logging.getLogger(__name__)

# RESILIENCE (Audit C-7): if the Go API is unreachable, the landing page must
# degrade gracefully instead of throwing a 500 at every visitor. This fallback
# is served ONLY when the upstream call fails.
FALLBACK_AUTHOR = "SENECA"
FALLBACK_QUOTE = "Every new beginning comes from some other beginning's end."

# CONCURRENCY (Audit P-2): notification mail previously spawned one bare
# Thread per form submission — unbounded thread creation under burst load, and
# exceptions evaporated. A small module-level pool caps concurrency for the
# worker's lifetime and _notify() surfaces failures into the app log.
_notifier = ThreadPoolExecutor(max_workers=2, thread_name_prefix="mail")


def _notify(email_address: str):
    """Executor target: send the token-request notification, logging (never
    swallowing) any failure. Runs off the request thread by design."""
    try:
        send_notification(email_address)
    except Exception:
        logger.exception("token-request notification failed for %s", email_address)


@main.route('/', methods=['GET', 'POST'])
@main.route('/home', methods=['GET', 'POST'])
def home():
    form = RequestTokenForm()
    if request.method == "POST":
        if form.validate():
            email_address = request.form['email']
            requested = exists(email_address)
            if requested == 1:
                flash(token_request_failure, "error")
                return redirect(url_for('main.home'))

            if requested == 500:
                flash(token_server_error, "error")
                return redirect(url_for('main.home'))

            # Bounded, observable fire-and-forget (Audit P-2).
            _notifier.submit(_notify, email_address)
            flash(token_request_response, "success")
            return redirect(url_for('main.home'))

        # DEFENSE (Audit C-7): session.get with fallbacks — a POST arriving
        # without a prior GET (expired/cleared cookie, direct POST) previously
        # raised KeyError and 500'd the form re-render.
        return render_template(
            'index.html',
            attribution=session.get('author', FALLBACK_AUTHOR),
            text=session.get('quote', FALLBACK_QUOTE),
            form=form,
            expanded="collapse show",
        )

    # DEFENSE (Audit C-7): the landing page previously called the API with no
    # error handling — an upstream outage or empty payload crashed the home
    # route. Degrade to the fallback quote and log the fault instead.
    try:
        data = get_random_quote()
        author, quote = data[0]['attribution'], data[0]['quote']
    except (requests.RequestException, LookupError, TypeError, ValueError):
        logger.exception("quotes API unavailable; serving fallback quote")
        author, quote = FALLBACK_AUTHOR, FALLBACK_QUOTE

    session['author'], session['quote'] = author.upper(), quote
    display = "collapse show" if session.get('_flashes') else "collapse"
    return render_template('index.html', attribution=author.upper(), text=quote, form=form, expanded=display)


@main.route('/search', methods=['POST'])
def search():
    if request.headers.get('X-Requested-With') == 'XMLHttpRequest':
        terms = request.form['words']
        if terms:
            try:
                return search_for_keywords(terms)
            except (requests.RequestException, ValueError):
                # Upstream failure must not 500 the AJAX caller (Audit C-7).
                logger.exception("search upstream failure")
                return jsonify({"error": "search is temporarily unavailable"}), 503
        return jsonify({"error": "no data requested"}), 400
    return jsonify({"error": "forbidden"}), 403


@main.route('/author-quotes', methods=['POST'])
def get_author_quotes():
    if request.headers.get('X-Requested-With') == 'XMLHttpRequest':
        name = request.form['name'].strip()
        if name:
            try:
                return get_quotes_by_author(name)
            except (requests.RequestException, ValueError):
                logger.exception("author-quotes upstream failure")
                return jsonify({"error": "lookup is temporarily unavailable"}), 503
        return jsonify({"error": "no name provided"}), 400
    return jsonify({"error": "forbidden"}), 403
