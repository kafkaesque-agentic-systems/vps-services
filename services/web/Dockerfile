# PERFORMANCE / SECURITY (Audit P-3): moved off EOL Debian Buster.
# python:3.10.9-slim-buster receives NO further security patches. This pins a
# supported Bookworm base (still Python 3.10, so the vendored-package path
# below is unchanged), reorders layers so dependency installs are cached
# independently of source, and runs the app as a non-root user.
FROM python:3.10-slim-bookworm
LABEL MAINTAINER="Thomas Romanov <admin@thepromethean.net>"

WORKDIR /app

# Install dependencies FIRST (cached unless requirements change) — before the
# rest of the source is copied.
COPY quotes-web/requirements.txt ./
RUN pip install --no-cache-dir --root-user-action=ignore -r requirements.txt \
    && pip install --no-cache-dir --root-user-action=ignore gunicorn

COPY quotes-web /app

# SECURITY (Audit C-3): the DKIM private key is NO LONGER baked into the image.
# Baked-in secrets persist in every pushed/pulled image layer forever.
# Mount the (rotated) key material at runtime instead, e.g. in docker-compose:
#
#   web:
#     volumes:
#       - /opt/micro-services.d/quotes-api/dkim:/etc/dkim:ro
#
# mail.generate_dkim() reads /etc/dkim/<domain>.<key>.pem at send time, so a
# missing mount degrades ONLY outbound-mail signing — and loudly, via syslog.

COPY flask_recaptcha.py /usr/local/lib/python3.10/site-packages

# Run as an unprivileged user (defense in depth).
RUN useradd --create-home --uid 10001 appuser
USER appuser

# RESILIENCE (Audit P-3): --timeout kills a worker stuck on a hung request
# (belt-and-suspenders alongside the C-7 request timeouts); --graceful-timeout
# bounds drain time on redeploy.
CMD [ "gunicorn", "-w", "4", "--timeout", "30", "--graceful-timeout", "30", "--bind", "0.0.0.0:5000", "wsgi" ]
