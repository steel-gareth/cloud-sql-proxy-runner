package secrets

import (
	"context"
	"fmt"
	"strings"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	smpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/googleapis/gax-go/v2"
)

type SecretClient interface {
	AccessSecretVersion(ctx context.Context, req *smpb.AccessSecretVersionRequest, opts ...gax.CallOption) (*smpb.AccessSecretVersionResponse, error)
}

// Verify that the real client satisfies the interface.
var _ SecretClient = (*secretmanager.Client)(nil)

func FetchSecret(ctx context.Context, client SecretClient, project, secretName string) (string, error) {
	name := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", project, secretName)
	resp, err := client.AccessSecretVersion(ctx, &smpb.AccessSecretVersionRequest{
		Name: name,
	})
	if err != nil {
		return "", fmt.Errorf("Failed to access secret %q in project %q.\n\nEnsure you have the Secret Manager Secret Accessor role.", secretName, project)
	}
	return strings.TrimSpace(string(resp.Payload.Data)), nil
}
