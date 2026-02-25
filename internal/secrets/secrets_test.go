package secrets

import (
	"context"
	"errors"
	"strings"
	"testing"

	smpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/googleapis/gax-go/v2"
)

type mockSecretClient struct {
	response *smpb.AccessSecretVersionResponse
	err      error
}

func (m *mockSecretClient) AccessSecretVersion(ctx context.Context, req *smpb.AccessSecretVersionRequest, opts ...gax.CallOption) (*smpb.AccessSecretVersionResponse, error) {
	return m.response, m.err
}

func TestFetchSecret_Success(t *testing.T) {
	client := &mockSecretClient{
		response: &smpb.AccessSecretVersionResponse{
			Payload: &smpb.SecretPayload{
				Data: []byte("  s3cretPa$$  "),
			},
		},
	}
	val, err := FetchSecret(context.Background(), client, "my-project", "my-secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "s3cretPa$$" {
		t.Errorf("expected trimmed value 's3cretPa$$', got %q", val)
	}
}

func TestFetchSecret_NotFound(t *testing.T) {
	client := &mockSecretClient{
		err: errors.New("rpc error: code = NotFound"),
	}
	_, err := FetchSecret(context.Background(), client, "my-project", "missing-secret")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing-secret") {
		t.Errorf("expected error to contain secret name, got: %v", err)
	}
	if !strings.Contains(err.Error(), "my-project") {
		t.Errorf("expected error to contain project name, got: %v", err)
	}
}

func TestFetchSecret_PermissionDenied(t *testing.T) {
	client := &mockSecretClient{
		err: errors.New("rpc error: code = PermissionDenied"),
	}
	_, err := FetchSecret(context.Background(), client, "my-project", "restricted-secret")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Secret Manager Secret Accessor role") {
		t.Errorf("expected error to mention role, got: %v", err)
	}
}
