package rest

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	headerTimeout    = 5 * time.Minute
	retryCount       = 5
	retryWaitTime    = 100 * time.Millisecond
	retryMaxWaitTime = 30 * time.Second
)

// NewClient returns a client bound to baseURL, with a retry policy that backs
// off on 429 and 5xx responses, honoring Retry-After.
//
// A request fails when its response headers take longer than 5 minutes to
// arrive. Reading the body has no deadline of its own: a caller streaming the
// body bounds it with [WithIdleTimeout], and any caller can bound the whole
// request through its context.
func NewClient(baseURL string) *resty.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = headerTimeout

	return resty.New().
		SetBaseURL(baseURL).
		SetTransport(transport).
		SetLogger(silentLogger{}).
		SetDisableWarn(true).
		SetRetryCount(retryCount).
		SetRetryWaitTime(retryWaitTime).
		SetRetryMaxWaitTime(retryMaxWaitTime).
		AddRetryCondition(func(r *resty.Response, _ error) bool {
			return r.StatusCode() == http.StatusTooManyRequests || r.StatusCode() >= http.StatusInternalServerError
		}).
		SetRetryAfter(func(_ *resty.Client, r *resty.Response) (time.Duration, error) {
			if r == nil {
				return 0, nil
			}

			return retryAfterWait(r.Header().Get("Retry-After")), nil
		})
}

func retryAfterWait(header string) time.Duration {
	seconds, err := strconv.Atoi(header)
	if err != nil || seconds <= 0 {
		return 0
	}

	return min(time.Duration(seconds)*time.Second, retryMaxWaitTime)
}

type silentLogger struct{}

func (silentLogger) Errorf(string, ...any) {}
func (silentLogger) Warnf(string, ...any)  {}
func (silentLogger) Debugf(string, ...any) {}
