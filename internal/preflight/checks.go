package preflight

import (
	"context"
	"fmt"

	"golang.org/x/oauth2/google"
)

type CredentialFinder func(ctx context.Context, scopes ...string) (*google.Credentials, error)

func CheckADC(ctx context.Context, finder CredentialFinder) error {
	_, err := finder(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return fmt.Errorf("No Google Cloud credentials found.\n\nRun: gcloud auth application-default login")
	}
	return nil
}

var DefaultCredentialFinder CredentialFinder = google.FindDefaultCredentials
