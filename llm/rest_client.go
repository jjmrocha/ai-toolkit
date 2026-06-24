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
func newRestyClient(baseURL string) *resty.Client {
	return resty.New().
		SetBaseURL(baseURL).
		SetTimeout(defaultTimeout).
		SetRetryCount(retryCount).
		SetRetryWaitTime(retryWaitTime).
		SetRetryMaxWaitTime(retryMaxWaitTime).
		AddRetryCondition(func(r *resty.Response, _ error) bool {
			return r.StatusCode() == http.StatusTooManyRequests || r.StatusCode() >= http.StatusInternalServerError
		})
}
