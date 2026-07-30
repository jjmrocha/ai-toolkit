package llm

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	defaultTimeout   = 60 * time.Second
	retryCount       = 5
	retryWaitTime    = 100 * time.Millisecond
	retryMaxWaitTime = 30 * time.Second
)

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
