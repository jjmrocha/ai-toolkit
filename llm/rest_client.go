package llm

import (
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	defaultTimeout   = 60 * time.Second
	retryCount       = 5
	retryWaitTime    = 100 * time.Millisecond
	retryMaxWaitTime = 2 * time.Second
)

// newRestyClient builds a resty client with the shared timeout and retry policy
// (retrying rate limits and transient server errors). Callers add per-provider
// settings such as authentication.
//
// Its logger is silenced: resty otherwise writes retry warnings and errors to
// stderr, which corrupts a full-screen terminal UI driving this client. Errors
// are already returned to the caller, so nothing is lost.
func newRestyClient(baseURL string) *resty.Client {
	return resty.New().
		SetBaseURL(baseURL).
		SetTimeout(defaultTimeout).
		SetLogger(silentLogger{}).
		SetDisableWarn(true).
		SetRetryCount(retryCount).
		SetRetryWaitTime(retryWaitTime).
		SetRetryMaxWaitTime(retryMaxWaitTime).
		AddRetryCondition(func(r *resty.Response, _ error) bool {
			return r.StatusCode() == http.StatusTooManyRequests || r.StatusCode() >= http.StatusInternalServerError
		})
}

// silentLogger is a resty.Logger that discards everything, keeping the library
// from writing to stderr underneath a TUI.
type silentLogger struct{}

func (silentLogger) Errorf(string, ...any) {}
func (silentLogger) Warnf(string, ...any)  {}
func (silentLogger) Debugf(string, ...any) {}
