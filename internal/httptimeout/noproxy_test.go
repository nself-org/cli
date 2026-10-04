package httptimeout

import (
	"net/http"
	"testing"
	"time"
)

func TestNoProxy(t *testing.T) {
	c := NoProxy(7 * time.Second)
	if c.Timeout != 7*time.Second {
		t.Errorf("Timeout = %v, want 7s", c.Timeout)
	}
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport is %T, want *http.Transport", c.Transport)
	}
	if tr.Proxy != nil {
		t.Error("Transport.Proxy must be nil")
	}
	if tr == http.DefaultTransport.(*http.Transport) {
		t.Error("Transport must be a clone, not the shared default")
	}
	if c.CheckRedirect == nil {
		t.Fatal("CheckRedirect must be set")
	}
	if err := c.CheckRedirect(&http.Request{}, nil); err != http.ErrUseLastResponse {
		t.Errorf("CheckRedirect = %v, want http.ErrUseLastResponse", err)
	}
}
