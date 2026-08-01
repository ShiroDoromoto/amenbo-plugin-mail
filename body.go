package main

import "strings"

// The body is plain text and nothing else — no HTML alternative beside it.
//
// What goes in it is a list of moments, and a list of moments has no headings, tables, images or
// links to gain anything from markup. Plain text reads the same in every mail client on every
// device, none of which this plugin gets to choose; an HTML body is likelier to be filed as spam,
// which is a poor trade for a notification; and one form to build is one form to check, where two
// is a pair that can drift until only one of them is right.

// messageBody writes the body of one message: the project it is about, then the events, oldest
// first.
//
// The heading is the project's name on its own. The subject carries it too, but the subject is
// cut to sixty characters and the project is what gives way there, so this is where a long name
// is read in full. It is written once rather than on every line, because everything held for a
// message belongs to one project — what waits to be sent is kept per project — so a name on each
// line would only repeat the first.
//
// A project whose name could not be read back gets no heading and the events start straight away.
// The lines already say everything that happened; not knowing what the project is called is no
// reason to hold a message back.
//
// The order the lines arrive in is the order they are written in. They are appended as events
// happen, and lines that failed to send are carried into the next message ahead of it, so oldest
// first is what that order already is — sorting it again here would only be this function
// deciding it knows better.
func messageBody(project string, lines []string) string {
	var written []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			written = append(written, line)
		}
	}

	var out strings.Builder
	if name := strings.TrimSpace(project); name != "" {
		out.WriteString(name)
		out.WriteString("\n")
		if len(written) > 0 {
			out.WriteString("\n")
		}
	}
	for _, line := range written {
		out.WriteString(line)
		out.WriteString("\n")
	}
	return out.String()
}
