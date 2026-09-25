// backend/internal/adapters/awsses/templates.go
package awsses

import "strings"

// emailCopy holds the per-language text for one kind of transactional
// email. body never has the code interpolated into it -- both renderHTML
// and renderText take the code as a separate argument and place it
// themselves, so this struct only ever holds static, hardcoded copy.
type emailCopy struct {
	subject string
	heading string
	body    string
}

var verificationCopy = map[string]emailCopy{
	"en": {
		subject: "Your Memória Carioca verification code",
		heading: "Verify your email",
		body:    "Enter this code in the app to verify your email address. It expires in 15 minutes.",
	},
	"fr": {
		subject: "Votre code de vérification Memória Carioca",
		heading: "Vérifie ton e-mail",
		body:    "Entre ce code dans l'application pour vérifier ton adresse e-mail. Il expire dans 15 minutes.",
	},
	"pt": {
		subject: "Seu código de verificação Memória Carioca",
		heading: "Verifique seu e-mail",
		body:    "Digite este código no aplicativo para verificar seu e-mail. Ele expira em 15 minutos.",
	},
	"es": {
		subject: "Tu código de verificación de Memória Carioca",
		heading: "Verifica tu correo",
		body:    "Ingresa este código en la aplicación para verificar tu correo electrónico. Caduca en 15 minutos.",
	},
}

var resetCopy = map[string]emailCopy{
	"en": {
		subject: "Your Memória Carioca password reset code",
		heading: "Reset your password",
		body:    "Enter this code in the app to reset your password. It expires in 15 minutes. If you didn't request this, you can ignore this email.",
	},
	"fr": {
		subject: "Votre code de réinitialisation Memória Carioca",
		heading: "Réinitialise ton mot de passe",
		body:    "Entre ce code dans l'application pour réinitialiser ton mot de passe. Il expire dans 15 minutes. Si tu n'es pas à l'origine de cette demande, ignore cet e-mail.",
	},
	"pt": {
		subject: "Seu código de redefinição de senha Memória Carioca",
		heading: "Redefina sua senha",
		body:    "Digite este código no aplicativo para redefinir sua senha. Ele expira em 15 minutos. Se você não fez essa solicitação, pode ignorar este e-mail.",
	},
	"es": {
		subject: "Tu código de restablecimiento de Memória Carioca",
		heading: "Restablece tu contraseña",
		body:    "Ingresa este código en la aplicación para restablecer tu contraseña. Caduca en 15 minutos. Si no solicitaste esto, puedes ignorar este correo.",
	},
}

// verificationCopyFor/resetCopyFor fall back to English for an empty or
// unrecognized language -- this is the one place in the whole feature that
// decides the fallback; every caller upstream (application layer, HTTP
// layer) passes the language through unvalidated.
func verificationCopyFor(language string) emailCopy {
	if c, ok := verificationCopy[language]; ok {
		return c
	}
	return verificationCopy["en"]
}

func resetCopyFor(language string) emailCopy {
	if c, ok := resetCopy[language]; ok {
		return c
	}
	return resetCopy["en"]
}

// htmlShell is a table-based, inline-CSS layout -- the only reliable way to
// get consistent rendering across email clients, Outlook desktop in
// particular, which ignores most modern CSS and any <style> block. A single
// column capped at 480px is what makes this "responsive" without needing
// media queries: it just naturally reflows on a narrow (mobile) viewport.
// Brand colors match mobile/src/theme/tokens.ts. The app's custom fonts
// (Playfair Display, Inter) aren't usable in email clients reliably, so
// this uses web-safe stacks that approximate them instead (a serif stack
// for the heading, a system sans-serif stack for body text).
const htmlShell = `<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  </head>
  <body style="margin:0;padding:0;background-color:#FAF5EE;">
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color:#FAF5EE;">
      <tr>
        <td align="center" style="padding:32px 16px;">
          <table role="presentation" width="100%" style="max-width:480px;background-color:#FFFFFF;border-radius:12px;border:1px solid rgba(43,33,27,0.14);" cellpadding="0" cellspacing="0">
            <tr>
              <td style="padding:32px;">
                <h1 style="margin:0 0 16px 0;font-family:Georgia,'Times New Roman',serif;font-size:24px;color:#2B211B;">{{HEADING}}</h1>
                <table role="presentation" cellpadding="0" cellspacing="0" style="margin:0 0 20px 0;">
                  <tr>
                    <td style="background-color:#FAF5EE;border-radius:8px;padding:16px 24px;">
                      <span style="font-family:-apple-system,'Segoe UI',Helvetica,Arial,sans-serif;font-size:28px;letter-spacing:6px;color:#C1592E;font-weight:bold;">{{CODE}}</span>
                    </td>
                  </tr>
                </table>
                <p style="margin:0;font-family:-apple-system,'Segoe UI',Helvetica,Arial,sans-serif;font-size:14px;line-height:21px;color:#6B5D4F;">{{BODY}}</p>
              </td>
            </tr>
          </table>
        </td>
      </tr>
    </table>
  </body>
</html>`

// renderHTML/renderText use strings.NewReplacer (not fmt.Sprintf) --
// htmlShell is full of literal "%" characters (e.g. width="100%"), which
// fmt.Sprintf would try to parse as format verbs; token replacement sidesteps
// that entirely.
func renderHTML(heading, body, code string) string {
	r := strings.NewReplacer("{{HEADING}}", heading, "{{CODE}}", code, "{{BODY}}", body)
	return r.Replace(htmlShell)
}

func renderText(body, code string) string {
	return body + "\n\nCode: " + code
}
