package testutil

import "context"

// NoopSender is a no-op email sender for testing.
type NoopSender struct {
	LastTo      string
	LastSubject string
	LastBody    string
	SendCount   int
}

func (n *NoopSender) Send(_ context.Context, to, subject, body string) error {
	n.LastTo = to
	n.LastSubject = subject
	n.LastBody = body
	n.SendCount++
	return nil
}
