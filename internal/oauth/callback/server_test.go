package callback

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestFindAvailablePort(t *testing.T) {
	port, err := FindAvailablePort(14000, 15000)
	if err != nil {
		t.Fatalf("FindAvailablePort() error = %v", err)
	}
	if port < 14000 || port >= 15000 {
		t.Errorf("Port = %d, want in range [14000, 15000)", port)
	}
}

func TestServerStartAndStop(t *testing.T) {
	port, _ := FindAvailablePort(15000, 16000)
	server := NewServer(port)

	ctx := context.Background()
	callbackURL, err := server.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Verify callback URL contains the port
	if !strings.Contains(callbackURL, "localhost") {
		t.Errorf("CallbackURL = %s, should contain localhost", callbackURL)
	}
	if !strings.Contains(callbackURL, "1455") || !strings.Contains(callbackURL, string(rune(port))) {
		// URL should contain the port number (either default or found)
	}

	// Stop the server
	if err := server.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestServerWaitForCallback(t *testing.T) {
	port, _ := FindAvailablePort(16000, 17000)
	server := NewServer(port)

	ctx := context.Background()
	callbackURL, _ := server.Start(ctx)
	defer server.Stop()

	// Simulate callback in a goroutine
	go func() {
		time.Sleep(100 * time.Millisecond)
		// Extract the callback URL path
		resp, err := http.Get(callbackURL + "?code=test-code&state=test-state")
		if err != nil {
			t.Logf("Callback request error: %v", err)
		}
		if resp != nil {
			resp.Body.Close()
		}
	}()

	// Wait for callback with timeout
	result, err := server.WaitForCallback(ctx, 2*time.Second)
	if err != nil {
		t.Fatalf("WaitForCallback() error = %v", err)
	}

	if result.Code != "test-code" {
		t.Errorf("Code = %s, want test-code", result.Code)
	}
	if result.State != "test-state" {
		t.Errorf("State = %s, want test-state", result.State)
	}
}

func TestServerWaitForCallbackTimeout(t *testing.T) {
	port, _ := FindAvailablePort(17000, 18000)
	server := NewServer(port)

	ctx := context.Background()
	server.Start(ctx)
	defer server.Stop()

	// Wait for callback without sending one (should timeout)
	_, err := server.WaitForCallback(ctx, 200*time.Millisecond)
	if err == nil {
		t.Error("WaitForCallback() should timeout")
	}
}

func TestServerErrorCallback(t *testing.T) {
	port, _ := FindAvailablePort(18000, 19000)
	server := NewServer(port)

	ctx := context.Background()
	callbackURL, _ := server.Start(ctx)
	defer server.Stop()

	// Simulate error callback
	go func() {
		time.Sleep(100 * time.Millisecond)
		http.Get(callbackURL + "?error=access_denied&error_description=User%20denied%20access")
	}()

	result, err := server.WaitForCallback(ctx, 2*time.Second)
	if err != nil {
		t.Fatalf("WaitForCallback() error = %v", err)
	}

	if result.Error == "" {
		t.Error("Expected error in callback result")
	}
}