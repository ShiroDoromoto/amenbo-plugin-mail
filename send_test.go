package main

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// sink is an SMTP server the tests talk to over the loopback interface. Nothing here leaves the
// machine: the point is to hold the whole conversation — greeting, upgrade, authentication,
// body — and read back what the sender actually said.
type sink struct {
	// tlsCert, when set, is what the server encrypts with: offered as STARTTLS, or spoken
	// from the first byte when the sender opens the connection encrypted.
	tlsCert *tls.Certificate
	// fromStart makes the server expect an already-encrypted connection.
	fromStart bool
	// replies replaces the answer to one verb, for the failures a working server never gives.
	replies map[string]string
	// silent makes the server accept the connection and then say nothing at all.
	silent bool

	mu    sync.Mutex
	heard []string
	body  string
}

// start puts the server on a port of the operating system's choosing and returns host and port.
func (s *sink) start(t *testing.T) (host, port string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	host, port, err = net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	return host, port
}

func (s *sink) serve(conn net.Conn) {
	defer conn.Close()
	if s.silent {
		// Held open, and answered never: this is the connection the deadline is for.
		<-time.After(time.Minute)
		return
	}
	if s.fromStart {
		tc := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{*s.tlsCert}})
		if err := tc.Handshake(); err != nil {
			return
		}
		conn = tc
	}
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	say := func(line string) {
		w.WriteString(line + "\r\n")
		w.Flush()
	}

	say("220 sink")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		s.record(line)
		verb := strings.ToUpper(strings.Fields(line + " x")[0])

		if reply, ok := s.replies[verb]; ok {
			say(reply)
			continue
		}
		switch verb {
		case "EHLO", "HELO":
			say("250-sink")
			if s.tlsCert != nil && !s.fromStart {
				say("250-STARTTLS")
			}
			say("250 AUTH PLAIN")
		case "STARTTLS":
			say("220 go ahead")
			tc := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{*s.tlsCert}})
			if err := tc.Handshake(); err != nil {
				return
			}
			conn = tc
			r, w = bufio.NewReader(conn), bufio.NewWriter(conn)
		case "AUTH":
			say("235 ok")
		case "DATA":
			say("354 go ahead")
			var body strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimRight(l, "\r\n") == "." {
					break
				}
				body.WriteString(l)
			}
			s.setBody(body.String())
			say("250 queued")
		case "QUIT":
			say("221 bye")
			return
		default:
			say("250 ok")
		}
	}
}

func (s *sink) record(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.heard = append(s.heard, line)
}

func (s *sink) setBody(v string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.body = v
}

// transcript is everything the sender said, as one string.
func (s *sink) transcript() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.heard, "\n")
}

func (s *sink) delivered() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.body
}

// selfSigned makes a certificate for 127.0.0.1 and the pool that trusts it, so a test can check
// an encrypted conversation without turning verification off in the code being tested.
func selfSigned(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "sink"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}

// talkTo builds the conversation a test holds with its own server.
func talkTo(s settings, host, port string, roots *x509.CertPool, fromStart bool) conversation {
	s.host, s.port = host, port
	return conversation{
		settings:  s,
		tls:       &tls.Config{ServerName: host, RootCAs: roots},
		fromStart: fromStart,
		deadline:  time.Now().Add(10 * time.Second),
	}
}

// beginning is the thread a project's first message makes: a name of its own, and nothing yet to
// answer. What a message says about a thread it joins is thread_test.go's.
func beginning() thread {
	return thread{id: messageID(sending().from)}
}

// sending is a configuration with everything a send needs, for the tests that vary one thing.
func sending() settings {
	return settings{
		user:     "",
		password: "app-password",
		from:     "amenbo@example.test",
		to:       []string{"you@example.test"},
	}
}

func TestSendHandsTheMessageOver(t *testing.T) {
	server := &sink{}
	host, port := server.start(t)

	if err := talkTo(sending(), host, port, nil, false).send(compose(sending(), "Subject line", "the body", beginning())); err != nil {
		t.Fatalf("send: %v", err)
	}

	heard := server.transcript()
	for _, want := range []string{"MAIL FROM:<amenbo@example.test>", "RCPT TO:<you@example.test>", "DATA", "QUIT"} {
		if !strings.Contains(heard, want) {
			t.Errorf("the server never heard %q, only:\n%s", want, heard)
		}
	}
	body := server.delivered()
	for _, want := range []string{
		"From: amenbo@example.test", "To: you@example.test", "Subject: Subject line",
		"Date: ", "Message-ID: <", "MIME-Version: 1.0", `Content-Type: text/plain; charset="utf-8"`,
		"the body",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the message is missing %q:\n%s", want, body)
		}
	}
}

func TestSendGivesEveryRecipientTheMessage(t *testing.T) {
	server := &sink{}
	host, port := server.start(t)
	s := sending()
	s.to = []string{"you@example.test", "them@example.test"}

	if err := talkTo(s, host, port, nil, false).send(compose(s, "Subject line", "the body", beginning())); err != nil {
		t.Fatalf("send: %v", err)
	}

	heard := server.transcript()
	for _, want := range []string{"RCPT TO:<you@example.test>", "RCPT TO:<them@example.test>"} {
		if !strings.Contains(heard, want) {
			t.Errorf("the server never heard %q, only:\n%s", want, heard)
		}
	}
	if want := "To: you@example.test, them@example.test"; !strings.Contains(server.delivered(), want) {
		t.Errorf("the message is missing %q:\n%s", want, server.delivered())
	}
}

func TestSendTakesTheUpgradeAServerOffers(t *testing.T) {
	cert, roots := selfSigned(t)
	server := &sink{tlsCert: &cert}
	host, port := server.start(t)

	if err := talkTo(sending(), host, port, roots, false).send(compose(sending(), "s", "b", beginning())); err != nil {
		t.Fatalf("send: %v", err)
	}

	if heard := server.transcript(); !strings.Contains(heard, "STARTTLS") {
		t.Errorf("the connection was never upgraded:\n%s", heard)
	}
	if server.delivered() == "" {
		t.Error("nothing arrived over the upgraded connection")
	}
}

func TestSendTalksEncryptedFromTheFirstByte(t *testing.T) {
	cert, roots := selfSigned(t)
	server := &sink{tlsCert: &cert, fromStart: true}
	host, port := server.start(t)

	if err := talkTo(sending(), host, port, roots, true).send(compose(sending(), "s", "b", beginning())); err != nil {
		t.Fatalf("send: %v", err)
	}

	if heard := server.transcript(); strings.Contains(heard, "STARTTLS") {
		t.Errorf("a connection that was already encrypted was upgraded again:\n%s", heard)
	}
	if server.delivered() == "" {
		t.Error("nothing arrived over the encrypted connection")
	}
}

func TestTLSFromTheStartIsDecidedByThePort(t *testing.T) {
	for port, want := range map[string]bool{"465": true, "587": false, "25": false, "": false} {
		if got := tlsFromTheStart(port); got != want {
			t.Errorf("tlsFromTheStart(%q) = %v, want %v", port, got, want)
		}
	}
}

func TestSendAuthenticatesOnlyForAnAccount(t *testing.T) {
	for _, tc := range []struct {
		name string
		user string
		want bool
	}{
		{"an account is authenticated for", "you@example.test", true},
		{"a relay that asks for none is not", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := &sink{}
			host, port := server.start(t)
			s := sending()
			s.user = tc.user

			if err := talkTo(s, host, port, nil, false).send(compose(s, "s", "b", beginning())); err != nil {
				t.Fatalf("send: %v", err)
			}
			if got := strings.Contains(server.transcript(), "AUTH PLAIN"); got != tc.want {
				t.Errorf("AUTH sent = %v, want %v:\n%s", got, tc.want, server.transcript())
			}
		})
	}
}

func TestSendReportsAServerThatRefuses(t *testing.T) {
	server := &sink{replies: map[string]string{"DATA": "451 not now"}}
	host, port := server.start(t)

	err := talkTo(sending(), host, port, nil, false).send(compose(sending(), "s", "b", beginning()))
	if err == nil {
		t.Fatal("a refused message was reported as sent")
	}
	if !strings.Contains(err.Error(), "451") {
		t.Errorf("err = %v, want what the server said", err)
	}
}

func TestSendReportsAServerThatIsNotThere(t *testing.T) {
	// A port nothing is listening on: the listener is opened and closed to be given one.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	ln.Close()

	if err := talkTo(sending(), host, port, nil, false).send("msg"); err == nil {
		t.Fatal("a message sent to nobody was reported as sent")
	}
}

func TestSendGivesUpOnAServerThatSaysNothing(t *testing.T) {
	server := &sink{silent: true}
	host, port := server.start(t)
	c := talkTo(sending(), host, port, nil, false)
	c.deadline = time.Now().Add(200 * time.Millisecond)

	done := make(chan error, 1)
	go func() { done <- c.send("msg") }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("a server that never answered was reported as having taken the message")
		}
	case <-time.After(5 * time.Second):
		t.Error("the run never gave up, so a plugin amenbo started would never come back")
	}
}

func TestSendKeepsThePasswordOutOfWhatIsWrittenDown(t *testing.T) {
	s := sending()
	s.user = "you@example.test"
	// A server naive enough to quote the credential it just rejected.
	server := &sink{replies: map[string]string{"AUTH": "535 rejected: " + s.password}}
	host, port := server.start(t)

	err := talkTo(s, host, port, nil, false).send(compose(s, "s", "b", beginning()))
	if err == nil {
		t.Fatal("a rejected account was reported as sent")
	}
	if got := withoutSecret(err, s.password); strings.Contains(got.Error(), s.password) {
		t.Errorf("err = %v, which carries the password", got)
	}
}

func TestComposeKeepsAHeaderOnItsOwnLine(t *testing.T) {
	s := sending()

	msg := compose(s, "done\r\nBcc: elsewhere@example.test", "the body", beginning())

	if strings.Contains(msg, "\r\nBcc:") {
		t.Errorf("a line break in the subject started a header of its own:\n%s", msg)
	}
	if !strings.Contains(msg, "Subject: done Bcc: elsewhere@example.test") {
		t.Errorf("the subject was not kept on one line:\n%s", msg)
	}
}

func TestComposeWritesTheBodyInTheLineEndingMailUses(t *testing.T) {
	msg := compose(sending(), "s", "one\ntwo", beginning())

	if !strings.Contains(msg, "one\r\ntwo") {
		t.Errorf("the body kept a bare newline:\n%q", msg)
	}
	if strings.Contains(msg, "\r\r\n") {
		t.Errorf("a line ending was doubled:\n%q", msg)
	}
}

func TestComposeWritesTheMessageIntoItsThread(t *testing.T) {
	msg := compose(sending(), "s", "b", thread{id: "<two@example.test>", root: "<one@example.test>"})

	for _, want := range []string{
		"Message-ID: <two@example.test>\r\n",
		"In-Reply-To: <one@example.test>\r\n",
		"References: <one@example.test>\r\n",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("the message is missing %q:\n%s", want, msg)
		}
	}
	if !strings.Contains(msg, "MIME-Version: 1.0") {
		t.Errorf("the headers that follow the thread's were lost:\n%s", msg)
	}
}
