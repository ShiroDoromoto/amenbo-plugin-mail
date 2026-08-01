package main

import (
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// A message is handed to an SMTP server and nowhere else. Every mail provider speaks SMTP the
// same way, so an account the user already has is the whole setup, and no third party sits
// between amenbo and the mailbox it reports to. It is also what the standard library already
// speaks, which keeps this a single binary with nothing to install beside it.

// sendTimeout is how long one message gets, from the first packet to the last. A send is a
// conversation — connect, greet, upgrade, authenticate, name the recipients, hand over the
// body, close — so it is given far longer than a single round trip. The limit is there for the
// server that accepts a connection and then says nothing: without it that run never ends, and a
// plugin amenbo started never comes back.
const sendTimeout = 60 * time.Second

// implicitTLSPort is the port that is encrypted from the first byte. The other submission
// ports (587, 25) open in the clear and are upgraded in the conversation, and that split is old
// and settled among providers — so the port the user filled in already says which one they
// meant, and asking them a second time in a setting of its own would only let them contradict
// themselves.
const implicitTLSPort = "465"

// tlsFromTheStart reports whether the port is one that is encrypted before anything is said.
func tlsFromTheStart(port string) bool {
	return port == implicitTLSPort
}

// sendMessage is how a run hands a message over, indirected so a test of what a hook decides can
// keep the message instead of standing up a server to take it.
var sendMessage = send

// send hands one message to the server the settings name.
//
// The subject arrives ready to be a header — encoded, and cut to length — and the body ready to
// be read. What is added here is the envelope around them.
func send(s settings, subject, body string) error {
	c := conversation{
		settings:  s,
		tls:       &tls.Config{ServerName: s.host},
		fromStart: tlsFromTheStart(s.port),
		deadline:  time.Now().Add(sendTimeout),
	}
	return withoutSecret(c.send(compose(s, subject, body)), s.password)
}

// conversation is one send: who to talk to, how to encrypt it, and when to give up. It is
// separate from send so a test can hold the same conversation with a server of its own — one on
// a port it was given, holding a certificate it made, answering within a deadline it set.
type conversation struct {
	settings  settings
	tls       *tls.Config
	fromStart bool
	deadline  time.Time
}

// send carries out the conversation, from the connection to the closing QUIT.
//
// Nothing is retried. amenbo does not hand an event back for a second attempt, and a message
// this fails to deliver is one the caller still holds — so trying again is the caller's to
// decide with everything else it is holding, not this function's to do behind its back.
func (c conversation) send(msg string) error {
	conn, err := c.dial()
	if err != nil {
		return fmt.Errorf("no message was sent: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(c.deadline); err != nil {
		return err
	}

	client, err := smtp.NewClient(conn, c.settings.host)
	if err != nil {
		return fmt.Errorf("the server did not greet us: %w", err)
	}
	defer client.Close()

	if !c.fromStart {
		// A server that offers the upgrade gets it. One that does not is left in the
		// clear — and the standard library will then refuse to authenticate over it, so
		// a password never reaches a connection anyone could read it off.
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(c.tls); err != nil {
				return fmt.Errorf("the connection could not be encrypted: %w", err)
			}
		}
	}

	// An account is what says authentication is wanted. A relay that asks for none — one on
	// the same machine, one inside a network — is used by leaving smtp_user empty, and no
	// AUTH is sent at all.
	if c.settings.user != "" {
		auth := smtp.PlainAuth("", c.settings.user, c.settings.password, c.settings.host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("the server would not accept the account: %w", err)
		}
	}

	if err := client.Mail(c.settings.from); err != nil {
		return fmt.Errorf("the server would not take the message from %s: %w", c.settings.from, err)
	}
	for _, to := range c.settings.to {
		if err := client.Rcpt(to); err != nil {
			return fmt.Errorf("the server would not take the message for %s: %w", to, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("the server would not take the body: %w", err)
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("the body could not be written: %w", err)
	}
	// The close is what ends the body, so its error is the server's verdict on the message
	// and not a detail of shutting a writer.
	if err := w.Close(); err != nil {
		return fmt.Errorf("the server did not accept the message: %w", err)
	}
	return client.Quit()
}

// dial opens the connection, encrypted from the first byte or not.
func (c conversation) dial() (net.Conn, error) {
	d := net.Dialer{Deadline: c.deadline}
	addr := net.JoinHostPort(c.settings.host, c.settings.port)
	if c.fromStart {
		return tls.DialWithDialer(&d, "tcp", addr, c.tls)
	}
	return d.Dial("tcp", addr)
}

// compose writes the message: the headers a mail client needs to file it, then the body.
func compose(s settings, subject, body string) string {
	headers := []string{
		"From: " + headerValue(s.from),
		"To: " + headerValue(strings.Join(s.to, ", ")),
		"Subject: " + headerValue(subject),
		"Date: " + time.Now().Format(time.RFC1123Z),
		"Message-ID: " + messageID(s.from),
		"MIME-Version: 1.0",
		`Content-Type: text/plain; charset="utf-8"`,
	}
	return strings.Join(headers, "\r\n") + "\r\n\r\n" + crlf(body)
}

// headerValue makes one value safe to put on a header line. A line break inside it would end
// the header and start whatever came after it as a new one — an address the user never asked
// for, on a message assembled partly out of text read back from the store.
func headerValue(v string) string {
	return strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(strings.TrimSpace(v))
}

// crlf gives the body the line ending mail is written in.
func crlf(body string) string {
	return strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n")
}

// messageID is this message's own name, which is what a mail client files replies and repeats
// against. The domain is the sender's, since that is the one name in the message that belongs
// to whoever is sending it.
func messageID(from string) string {
	var b [16]byte
	rand.Read(b[:])
	domain := "localhost"
	if at := strings.LastIndex(from, "@"); at >= 0 && at+1 < len(from) {
		domain = from[at+1:]
	}
	return fmt.Sprintf("<%x@%s>", b, domain)
}

// withoutSecret keeps the password out of what is written down. A failure ends up in amenbo's
// execution log, and the log is part of what an export carries out of the store — so a server
// that quotes the credential it just rejected must not be repeated word for word.
func withoutSecret(err error, secret string) error {
	if err == nil || secret == "" {
		return err
	}
	if text := err.Error(); strings.Contains(text, secret) {
		return errors.New(strings.ReplaceAll(text, secret, "[the password]"))
	}
	return err
}
