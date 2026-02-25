package preflight

import (
	"context"
	"errors"
	"strings"
	"testing"

	"golang.org/x/oauth2/google"
)

func TestCheckADC_CredentialsFound(t *testing.T) {
	finder := func(ctx context.Context, scopes ...string) (*google.Credentials, error) {
		return &google.Credentials{}, nil
	}
	err := CheckADC(context.Background(), finder)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckADC_CredentialsMissing(t *testing.T) {
	finder := func(ctx context.Context, scopes ...string) (*google.Credentials, error) {
		return nil, errors.New("could not find default credentials")
	}
	err := CheckADC(context.Background(), finder)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "gcloud auth application-default login") {
		t.Errorf("expected error to mention gcloud command, got: %v", err)
	}
	if !strings.Contains(err.Error(), "No Google Cloud credentials found") {
		t.Errorf("expected error to mention missing credentials, got: %v", err)
	}
}
