package main

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/mutationpayload"
)

func TestPromptPasswordVisibilityDefaultsToHidden(t *testing.T) {
	var output bytes.Buffer
	visible, err := promptPasswordVisibility(bufio.NewReader(strings.NewReader("\n")), &output)
	if err != nil {
		t.Fatalf("promptPasswordVisibility returned an error: %v", err)
	}
	if visible {
		t.Fatal("empty answer should select hidden password input")
	}
}

func TestPromptAdminUsernameAndEmailRetryInvalidInput(t *testing.T) {
	var output bytes.Buffer
	reader := bufio.NewReader(strings.NewReader("x\nvalid-admin\n\nadmin@example.com\n"))

	username, err := promptAdminUsername(reader, &output)
	if err != nil {
		t.Fatalf("promptAdminUsername returned an error: %v", err)
	}
	if username != "valid-admin" {
		t.Fatalf("username = %q, want valid-admin", username)
	}

	email, err := promptAdminEmail(reader, &output)
	if err != nil {
		t.Fatalf("promptAdminEmail returned an error: %v", err)
	}
	if email != "admin@example.com" {
		t.Fatalf("email = %q, want admin@example.com", email)
	}
	if !strings.Contains(output.String(), "Invalid username") {
		t.Fatalf("invalid username did not produce guidance: %q", output.String())
	}
	if !strings.Contains(output.String(), "Email must not be empty") {
		t.Fatalf("empty email did not produce guidance: %q", output.String())
	}
}

func TestPromptPasswordVisibilityAcceptsTurkishAndRetriesInvalidChoice(t *testing.T) {
	var output bytes.Buffer
	visible, err := promptPasswordVisibility(bufio.NewReader(strings.NewReader("maybe\ne\n")), &output)
	if err != nil {
		t.Fatalf("promptPasswordVisibility returned an error: %v", err)
	}
	if !visible {
		t.Fatal("e should select visible password input")
	}
	if !strings.Contains(output.String(), "Please enter y/e") {
		t.Fatalf("invalid choice did not produce guidance: %q", output.String())
	}
}

func TestReadAndConfirmPasswordRetriesAfterMismatch(t *testing.T) {
	answers := []string{"first-password", "different-password", "second-password", "second-password"}
	read := func(string) (string, error) {
		answer := answers[0]
		answers = answers[1:]
		return answer, nil
	}

	var output bytes.Buffer
	password, err := readAndConfirmPassword(read, &output)
	if err != nil {
		t.Fatalf("readAndConfirmPassword returned an error: %v", err)
	}
	if password != "second-password" {
		t.Fatalf("password = %q, want second-password", password)
	}
	if !strings.Contains(output.String(), "Passwords do not match") {
		t.Fatalf("mismatch did not produce guidance: %q", output.String())
	}
}

func TestReadAndConfirmPasswordRetriesShortPassword(t *testing.T) {
	answers := []string{"short", "long-enough", "long-enough"}
	read := func(string) (string, error) {
		answer := answers[0]
		answers = answers[1:]
		return answer, nil
	}

	var output bytes.Buffer
	password, err := readAndConfirmPassword(read, &output)
	if err != nil {
		t.Fatalf("readAndConfirmPassword returned an error: %v", err)
	}
	if password != "long-enough" {
		t.Fatalf("password = %q, want long-enough", password)
	}
	if !strings.Contains(output.String(), "at least 8 characters") {
		t.Fatalf("short password did not produce guidance: %q", output.String())
	}
}

func TestReadAndConfirmPasswordStopsOnInputError(t *testing.T) {
	wantErr := errors.New("terminal closed")
	_, err := readAndConfirmPassword(func(string) (string, error) {
		return "", wantErr
	}, &bytes.Buffer{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want wrapped %v", err, wantErr)
	}
}

func TestRequireInteractiveAdminTerminal(t *testing.T) {
	if err := requireInteractiveAdminTerminal(true); err != nil {
		t.Fatalf("terminal input was rejected: %v", err)
	}
	if err := requireInteractiveAdminTerminal(false); err == nil ||
		!strings.Contains(err.Error(), "requires a terminal") {
		t.Fatalf("non-terminal error = %v, want explicit refusal", err)
	}
}

// The interactive prompt takes exactly what the certificate request takes
// (owner decision of 2026-10-23): what the validator refuses is asked again,
// what it accepts is returned in its canonical form.
func TestPromptAdminEmailUsesTheCertificateContactValidator(t *testing.T) {
	const fallback = "owner@example.com"
	for _, typed := range []string{
		"ali", "ali@", "@x.com", "a b@x.com", "ali@localhost", "ali@192.0.2.1",
		"Name <ali@x.com>", "ali@x.com", "  Ali@Example.COM  ", "a.b+tag@mail.example.tr",
	} {
		t.Run(typed, func(t *testing.T) {
			var output bytes.Buffer
			reader := bufio.NewReader(strings.NewReader(typed + "\n" + fallback + "\n"))
			email, err := promptAdminEmail(reader, &output)
			if err != nil {
				t.Fatalf("promptAdminEmail returned an error: %v", err)
			}
			canonical, invalid := mutationpayload.CanonicalContactEmail(typed)
			if invalid != nil {
				if email != fallback || !strings.Contains(output.String(), "That is not a valid email address") ||
					!strings.Contains(output.String(), "Bu geçerli bir e-posta adresi değil") {
					t.Fatalf("refused address: email=%q output=%q", email, output.String())
				}
				return
			}
			if email != canonical || strings.Contains(output.String(), "not a valid email") {
				t.Fatalf("accepted address: email=%q want %q output=%q", email, canonical, output.String())
			}
		})
	}
	// The typos the owner named are among the refused ones.
	for _, typo := range []string{"ali", "ali@", "@x.com", "a b@x.com", "ali@localhost"} {
		if _, err := mutationpayload.CanonicalContactEmail(typo); err == nil {
			t.Fatalf("validator accepted %q", typo)
		}
	}
}
