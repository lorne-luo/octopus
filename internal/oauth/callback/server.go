package callback

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// Server is a local OAuth callback server
type Server struct {
	port       int
	server     *http.Server
	resultChan chan *CallbackResult
	mu         sync.Mutex
	started    bool
}

// CallbackResult contains the callback result
type CallbackResult struct {
	Code  string
	State string
	Error string
}

var (
	ErrServerNotStarted = errors.New("server not started")
	ErrTimeout          = errors.New("callback timeout")
	ErrPortInUse        = errors.New("port already in use")
)

// NewServer creates a new callback server
func NewServer(port int) *Server {
	return &Server{
		port:       port,
		resultChan: make(chan *CallbackResult, 1),
	}
}

// Start starts the server and returns the callback URL
func (s *Server) Start(ctx context.Context) (callbackURL string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return "", errors.New("server already started")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", s.handleCallback)
	mux.HandleFunc("/", s.handleCallback) // Also handle root for flexibility

	addr := fmt.Sprintf(":%d", s.port)
	s.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Start server in background
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Server error
		}
	}()

	s.started = true
	return fmt.Sprintf("http://localhost:%d/auth/callback", s.port), nil
}

// handleCallback handles the OAuth callback request
func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	result := &CallbackResult{
		Code:  query.Get("code"),
		State: query.Get("state"),
		Error: query.Get("error"),
	}

	// Send result to channel (non-blocking)
	select {
	case s.resultChan <- result:
	default:
	}

	// Return success page
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if result.Error != "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>OAuth Error</title></head>
<body style="font-family: system-ui; text-align: center; padding: 50px;">
<h1 style="color: #dc2626;">❌ Authentication Failed</h1>
<p style="color: #666;">Error: %s</p>
<p>You can close this window now.</p>
</body>
</html>`, result.Error)
	} else {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>OAuth Success</title></head>
<body style="font-family: system-ui; text-align: center; padding: 50px;">
<h1 style="color: #16a34a;">✅ Authentication Successful</h1>
<p>You can close this window and return to the application.</p>
<script>setTimeout(() => window.close(), 3000);</script>
</body>
</html>`)
	}
}

// WaitForCallback waits for the callback with timeout
func (s *Server) WaitForCallback(ctx context.Context, timeout time.Duration) (*CallbackResult, error) {
	if !s.started {
		return nil, ErrServerNotStarted
	}

	select {
	case result := <-s.resultChan:
		return result, nil
	case <-time.After(timeout):
		return nil, ErrTimeout
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Stop stops the server
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started || s.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := s.server.Shutdown(ctx)
	s.started = false
	return err
}

// GetPort returns the server port
func (s *Server) GetPort() int {
	return s.port
}

// FindAvailablePort finds an available port in the given range
func FindAvailablePort(startPort, endPort int) (int, error) {
	for port := startPort; port < endPort; port++ {
		addr := fmt.Sprintf(":%d", port)
		listener, err := net.Listen("tcp", addr)
		if err == nil {
			listener.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available port in range [%d, %d)", startPort, endPort)
}