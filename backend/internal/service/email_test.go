package service

import (
	"context"
	"testing"
)

func TestNoopSender_RecordsCalls(t *testing.T) {
	sender := &NoopSender{}

	if sender.SendCount != 0 {
		t.Fatalf("expected SendCount=0, got %d", sender.SendCount)
	}

	err := sender.Send(context.Background(), "user@example.com", "Test Subject", "<p>Hello</p>")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sender.SendCount != 1 {
		t.Errorf("expected SendCount=1, got %d", sender.SendCount)
	}
	if sender.LastTo != "user@example.com" {
		t.Errorf("expected LastTo=user@example.com, got %s", sender.LastTo)
	}
	if sender.LastSubject != "Test Subject" {
		t.Errorf("expected LastSubject=Test Subject, got %s", sender.LastSubject)
	}
	if sender.LastBody != "<p>Hello</p>" {
		t.Errorf("expected LastBody=<p>Hello</p>, got %s", sender.LastBody)
	}

	// Second call overwrites the previous values
	_ = sender.Send(context.Background(), "other@example.com", "Second", "<p>World</p>")
	if sender.SendCount != 2 {
		t.Errorf("expected SendCount=2, got %d", sender.SendCount)
	}
	if sender.LastTo != "other@example.com" {
		t.Errorf("expected LastTo=other@example.com, got %s", sender.LastTo)
	}
}

func TestSMTPSender_ConstructsCorrectly(t *testing.T) {
	sender := NewSMTPSender("mail.example.com", 587, "noreply@example.com")

	if sender.Host != "mail.example.com" {
		t.Errorf("expected Host=mail.example.com, got %s", sender.Host)
	}
	if sender.Port != 587 {
		t.Errorf("expected Port=587, got %d", sender.Port)
	}
	if sender.From != "noreply@example.com" {
		t.Errorf("expected From=noreply@example.com, got %s", sender.From)
	}
}

func TestForgotPassword_SendsEmail(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewMemorySessionStore()
	sender := &NoopSender{}
	svc := NewAuthService(repo, sessions, NewMemoryPasswordResetStore(), sender, "http://localhost:8080")

	// Register a user
	_, _, _ = svc.Register(context.Background(), RegisterRequest{
		Username: "emailuser",
		Email:    "emailuser@example.com",
		Password: "password123",
	}, "127.0.0.1")

	err := svc.ForgotPassword(context.Background(), ForgotPasswordRequest{
		Email: "emailuser@example.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sender.SendCount != 1 {
		t.Fatalf("expected 1 email sent, got %d", sender.SendCount)
	}
	if sender.LastTo != "emailuser@example.com" {
		t.Errorf("expected email to emailuser@example.com, got %s", sender.LastTo)
	}
	if sender.LastSubject == "" {
		t.Error("expected non-empty subject")
	}
	if sender.LastBody == "" {
		t.Error("expected non-empty body")
	}
}

func TestForgotPassword_NoEmailForNonexistentUser(t *testing.T) {
	repo := newMockUserRepo()
	sessions := NewMemorySessionStore()
	sender := &NoopSender{}
	svc := NewAuthService(repo, sessions, NewMemoryPasswordResetStore(), sender, "http://localhost:8080")

	_ = svc.ForgotPassword(context.Background(), ForgotPasswordRequest{
		Email: "nobody@example.com",
	})

	if sender.SendCount != 0 {
		t.Errorf("expected no emails sent for nonexistent user, got %d", sender.SendCount)
	}
}
