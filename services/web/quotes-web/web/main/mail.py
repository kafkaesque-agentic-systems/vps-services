# Mail server IMAP, SMTP protocol utilities
import os
import dkim
import email
import syslog
import imaplib
import certifi
import smtplib, ssl
from email import encoders
from email.utils import formatdate, make_msgid
from email.mime.base import MIMEBase
from email.mime.text import MIMEText
from email.mime.multipart import MIMEMultipart

class MailAttachment(object):
    # Create an email attachment object.
    def __new__(cls, filepath):
        if not os.path.exists(filepath):
            raise FileNotFoundError('File Does Exist In The Specified Directory!')
        return object.__new__(cls)

    def __init__(self, filepath):
        self.path = filepath
        self.name = self.path.split('/')[-1]

    def __repr__(self):
        return f"<MailAttachment: file='{self.path}'>"

    def attach(self):
        # O-7: MIMEBase and encoders are now imported at module top (they were
        # referenced but never imported, so the first attachment ever built
        # crashed with NameError). Also fixed the 'octate-stream' typo.
        with open(self.path, 'rb') as attachment:
            payload = MIMEBase('application', 'octet-stream')
            payload.set_payload(attachment.read())
            encoders.encode_base64(payload)
            payload.add_header('Content-Disposition', 'attachment', filename=self.name)
        return payload


def generate_dkim(key: str, domain: str, dkim_selector: str, msg: str):
    # O-7: 'key' is the selector label embedded in the key filename
    # (e.g. 'omail' -> /etc/dkim/thirdeye.live.omail.pem). Previously the file
    # contents were read back into the same 'key' variable, shadowing the
    # parameter. Read into a distinct 'privkey' for clarity.
    with open(f'/etc/dkim/{domain}.{key}.pem') as f:
        privkey = f.read()

    signature = dkim.sign(
        message=msg.as_bytes(),
        selector=str(dkim_selector).encode(),
        domain=domain.encode(),
        privkey=privkey.encode(),
        include_headers=[b"To", b"From", b"Subject"]
    )

    return signature[len("DKIM-Signature: "):].decode()


def create_message(rcpt=None, sender=None, subject=None, body=True, html=None, text=None, attachment=None, signed=True):
    """format and return MIME Multipart email message

       ARGUMENTS:
          'recipient':     str:  the recipient's email address
             'sender':     str:  the message header FROM attribute
               'body':    bool:  determines if the email contains text or html content. Set to false if you only
                                 need to send an attachment without body content. defaults to True.
            'subject':     str:  the email subject field
               'html':     str:  the html content of the message
              'plain':     str:  the plain text content of the message
         'attachment':   bytes:  An encoded email.MIMEBase() object (use MailAttachment class from this module)
    """
    msg = MIMEMultipart('alternative')
    msg['Subject'] = subject
    msg['To'] = rcpt
    msg['From'] = sender
    msg['Date'] = formatdate()
    msg['Message-ID'] = make_msgid()

    if body:
        if html:
            msg.attach(MIMEText(html, 'html'))

        if text:
            msg.attach(MIMEText(text, 'plain'))

    if attachment:
        msg.attach(attachment)

    if signed:
        msg['DKIM-Signature'] = generate_dkim('omail', 'thirdeye.live', 'omail._domainkey', msg)

    return msg


def sendmail(destination=None, origin=None, password=None, message=None):
    """send an email from the specified mail server

       ARGUMENTS:
        'destination':   str:  the recipient's email address
             'origin':   str:  the sender's email address
           'password':   str:  the appropiate password for the mail server that will be used to send this email
            message':    str:  MIME Multipart email message: use mail.create_message()
    """
    host = 'thepromethean.net'
    context = ssl.create_default_context(purpose=ssl.Purpose.SERVER_AUTH, cafile=certifi.where())
    syslog.openlog(ident='[PYTHON.MAIL.SENDMAIL]', logoption=syslog.LOG_PID)

    try:
        with smtplib.SMTP_SSL(host, 465, context=context) as server:
            syslog.syslog(syslog.LOG_INFO, f'LOGGING INTO mail.{host}')
            server.login(origin, password)
            syslog.syslog(syslog.LOG_INFO, 'LOGIN SUCCESS')
            syslog.syslog(syslog.LOG_INFO, f'MAILTO: {destination}')
            server.sendmail(origin, destination, message.as_string())
            syslog.syslog(syslog.LOG_INFO, f'MESSAGE SENT TO {destination}')

    except Exception as error:
        syslog.syslog(syslog.LOG_ERR, f'MAIL FAILED: {error}')

    syslog.closelog()

