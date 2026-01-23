package email

type EmailMessage struct {
	To      string
	Subject string
	HTML    string
	Text    string

	replace map[string]any
}
