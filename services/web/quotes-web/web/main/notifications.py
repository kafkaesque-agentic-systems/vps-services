import os 
from web.main.mail import sendmail, create_message

token_request_response = "We have received your request. It will be reviewed by a team member and a response will be sent out soon. Thank you for your patience"

token_request_failure = "This email has already requested a token. Please be patient while we review your request"

token_server_error = "There was an internal error processing your request. Please try again later"

credentials = {
	"origin": os.environ.get("MAILSERVER"),
	"password": os.environ.get("MAILPASS")
}

# Mail Headers
TO =   "admin@thepromethean.net"
FROM = "Quotes API Notifications <theoracle@thirdeye.live>"
SUBJ = "API Token Request Alert"
TEXT = "Notification of token request. Please review and respond to {0}" 

def send_notification(email: str):
	MSG = create_message(rcpt=TO, sender=FROM, subject=SUBJ, text=TEXT.format(email))
	sendmail(destination=TO, message=MSG, **credentials)
