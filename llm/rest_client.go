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

type silentLogger struct{}

func (silentLogger) Errorf(string, ...any) {}
func (silentLogger) Warnf(string, ...any)  {}
func (silentLogger) Debugf(string, ...any) {}
