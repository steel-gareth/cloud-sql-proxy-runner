package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

type mockDialer struct {
	dialFunc func(ctx context.Context, instance string) (net.Conn, error)
	closed   bool
}

func (m *mockDialer) Dial(ctx context.Context, instance string) (net.Conn, error) {
	return m.dialFunc(ctx, instance)
}

func (m *mockDialer) Close() error {
	m.closed = true
	return nil
}

func TestBidirectionalProxy(t *testing.T) {
	// Create a pipe to simulate the remote Cloud SQL connection
	remoteClient, remoteServer := net.Pipe()

	dialer := &mockDialer{
		dialFunc: func(ctx context.Context, instance string) (net.Conn, error) {
			return remoteServer, nil
		},
	}

	l := NewListener("proj:region:db", 0, dialer)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l.Port = 0
	if err := l.Start(ctx); err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer l.Close()

	addr := l.Addr().String()

	// Connect to the proxy
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}

	// Send data through the proxy (client -> remote)
	testData := []byte("hello from client")
	if _, err := conn.Write(testData); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	buf := make([]byte, len(testData))
	if _, err := io.ReadFull(remoteClient, buf); err != nil {
		t.Fatalf("failed to read from remote: %v", err)
	}
	if string(buf) != string(testData) {
		t.Errorf("expected %q, got %q", testData, buf)
	}

	// Send data back (remote -> client)
	responseData := []byte("hello from remote")
	if _, err := remoteClient.Write(responseData); err != nil {
		t.Fatalf("failed to write response: %v", err)
	}

	buf = make([]byte, len(responseData))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	if string(buf) != string(responseData) {
		t.Errorf("expected %q, got %q", responseData, buf)
	}

	// Close both ends to unblock the io.Copy goroutines
	conn.Close()
	remoteClient.Close()
}

func TestListenerClosesOnContextCancel(t *testing.T) {
	dialer := &mockDialer{
		dialFunc: func(ctx context.Context, instance string) (net.Conn, error) {
			return nil, errors.New("should not be called")
		},
	}

	l := NewListener("proj:region:db", 0, dialer)
	ctx, cancel := context.WithCancel(context.Background())

	l.Port = 0
	if err := l.Start(ctx); err != nil {
		t.Fatalf("failed to start: %v", err)
	}

	addr := l.Addr().String()

	cancel()
	l.Close()

	// Listener should no longer accept connections
	_, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected connection to fail after close")
	}
}

func TestDialerErrorHandled(t *testing.T) {
	dialer := &mockDialer{
		dialFunc: func(ctx context.Context, instance string) (net.Conn, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}

	l := NewListener("proj:region:db", 0, dialer)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l.Port = 0
	if err := l.Start(ctx); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer l.Close()

	// Connect - the proxy should accept then close the connection when dial fails
	conn, err := net.DialTimeout("tcp", l.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	// The connection should be closed by the proxy after dial failure
	buf := make([]byte, 1)
	conn.SetReadDeadline(time.Now().Add(time.Second))
	_, err = conn.Read(buf)
	if err == nil {
		t.Fatal("expected read to fail (connection should be closed)")
	}
}
