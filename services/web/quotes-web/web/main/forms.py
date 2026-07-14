from flask_wtf import FlaskForm, RecaptchaField, Recaptcha
from wtforms import StringField, SubmitField
from wtforms.validators import DataRequired, Email, ValidationError

class RequestTokenForm(FlaskForm):
    email = StringField("Email", validators=[DataRequired(), Email(check_deliverability=True)])
    submit = SubmitField("Send")
    recaptcha = RecaptchaField(validators=[Recaptcha(message="Please solve the captcha to proceed")])

    def invalid(self, message):
        self.email.errors += (ValidationError(message),)
