import os
from web import app as application

if __name__ == "__main__":
    port = int(os.environ.get('PORT', 5000))

    # SECURITY (Audit C-4): the previous logic was INVERTED —
    #     prod = False if bool(os.environ.get('FLASK_DEBUG')) else True
    #     application.run(debug=prod, ...)
    # meant an UNSET FLASK_DEBUG (i.e. production) enabled debug=True, arming
    # the interactive Werkzeug debugger (remote code execution) by default.
    #
    # FAIL CLOSED: debug is now OFF unless FLASK_DEBUG is explicitly set to a
    # truthy value. Note bool("0") is True in Python, so we compare against an
    # explicit allow-list rather than truthiness of the raw string.
    debug = os.environ.get('FLASK_DEBUG', '').strip().lower() in ('1', 'true', 'yes', 'on')
    application.run(debug=debug, host='0.0.0.0', port=port)
