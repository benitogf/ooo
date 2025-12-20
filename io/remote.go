package io

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/benitogf/ooo/client"
	"github.com/benitogf/ooo/key"
	"github.com/benitogf/ooo/meta"
	"github.com/goccy/go-json"
)

var (
	ErrClientRequired = errors.New("io: Client is required")
	ErrHostRequired   = errors.New("io: Host is required")
	ErrRequestFailed  = errors.New("io: request failed")
)

const (
	// DefaultMaxRetries is the default number of retry attempts
	DefaultMaxRetries = 3
	// DefaultRetryDelay is the initial delay between retries
	DefaultRetryDelay = 100 * time.Millisecond
	// DefaultMaxResponseSize is the maximum response body size (10MB)
	DefaultMaxResponseSize int64 = 10 * 1024 * 1024
)

// RetryConfig holds retry configuration for remote operations.
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts (0 = no retries)
	MaxRetries int
	// RetryDelay is the initial delay between retries (doubles each attempt)
	RetryDelay time.Duration
}

// isRetryable returns true if the error is a transient network error worth retrying
func isRetryable(err error, statusCode int) bool {
	if err != nil {
		return true // Network errors are retryable
	}
	// Retry on server errors (5xx) but not client errors (4xx)
	return statusCode >= 500 && statusCode < 600
}

// RemoteConfig holds connection configuration for remote operations.
// Required fields: Client, Host.
// Optional fields: SSL, Header, Retry, MaxResponseSize.
type RemoteConfig struct {
	Client          *http.Client
	SSL             bool
	Host            string
	Header          http.Header
	Retry           RetryConfig
	MaxResponseSize int64 // Maximum response body size in bytes (0 = use default)
}

// URL returns the full URL for a given path based on the config.
func (c *RemoteConfig) URL(path string) string {
	if c.SSL {
		return "https://" + c.Host + "/" + path
	}
	return "http://" + c.Host + "/" + path
}

// Validate checks that required fields are set and applies defaults.
func (c *RemoteConfig) Validate() error {
	if c.Client == nil {
		return ErrClientRequired
	}
	if c.Host == "" {
		return ErrHostRequired
	}
	// Apply defaults
	if c.MaxResponseSize == 0 {
		c.MaxResponseSize = DefaultMaxResponseSize
	}
	return nil
}

// readResponseBody reads the response body with size limit
func (c *RemoteConfig) readResponseBody(resp *http.Response) ([]byte, error) {
	return io.ReadAll(io.LimitReader(resp.Body, c.MaxResponseSize))
}

// retryResult holds the result of a retry operation
type retryResult struct {
	body       []byte
	statusCode int
}

// doWithRetry executes an HTTP request with retry logic.
// The buildRequest function is called for each attempt to create a fresh request.
// Returns the response body and any error.
func (c *RemoteConfig) doWithRetry(ctx context.Context, opName, path string, buildRequest func() (*http.Request, error)) (retryResult, error) {
	var lastErr error
	delay := c.Retry.RetryDelay
	if delay == 0 {
		delay = DefaultRetryDelay
	}

	for attempt := 0; attempt <= c.Retry.MaxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("%s[%s]: retrying, attempt %d", opName, path, attempt)
			select {
			case <-ctx.Done():
				return retryResult{}, ctx.Err()
			case <-time.After(delay):
			}
			delay *= 2 // Exponential backoff
		}

		req, err := buildRequest()
		if err != nil {
			log.Printf("%s[%s]: failed to create request: %v", opName, path, err)
			return retryResult{}, err // Not retryable
		}
		req = req.WithContext(ctx)

		resp, err := c.Client.Do(req)
		if err != nil {
			log.Printf("%s[%s]: request failed: %v", opName, path, err)
			lastErr = err
			if isRetryable(err, 0) {
				continue
			}
			return retryResult{}, err
		}

		body, err := c.readResponseBody(resp)
		resp.Body.Close()
		if err != nil {
			log.Printf("%s[%s]: failed to read response: %v", opName, path, err)
			lastErr = err
			continue
		}

		if resp.StatusCode >= 400 {
			log.Printf("%s[%s]: status %s: %s", opName, path, resp.Status, string(body))
			lastErr = ErrRequestFailed
			if isRetryable(nil, resp.StatusCode) {
				continue
			}
			return retryResult{body: body, statusCode: resp.StatusCode}, lastErr
		}

		return retryResult{body: body, statusCode: resp.StatusCode}, nil
	}
	return retryResult{}, lastErr
}

// RemoteSet stores an item at the given path (non-list path only).
// Use RemoteSetWithContext for cancellation support.
func RemoteSet[T any](cfg RemoteConfig, path string, item T) error {
	return RemoteSetWithContext(context.Background(), cfg, path, item)
}

// RemoteSetWithContext stores an item at the given path with context support.
func RemoteSetWithContext[T any](ctx context.Context, cfg RemoteConfig, path string, item T) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if key.LastIndex(path) == "*" {
		log.Println("RemoteSet["+path+"]: ", ErrPathGlobNotAllowed)
		return ErrPathGlobNotAllowed
	}

	data, err := json.Marshal(item)
	if err != nil {
		log.Printf("RemoteSet[%s]: failed to marshal data: %v", path, err)
		return err
	}

	_, err = cfg.doWithRetry(ctx, "RemoteSet", path, func() (*http.Request, error) {
		req, err := http.NewRequest("POST", cfg.URL(path), bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		for k, v := range cfg.Header {
			req.Header[k] = v
		}
		return req, nil
	})
	return err
}

// RemotePush adds an item to a list path (glob path only).
// Use RemotePushWithContext for cancellation support.
func RemotePush[T any](cfg RemoteConfig, path string, item T) error {
	return RemotePushWithContext(context.Background(), cfg, path, item)
}

// RemotePushWithContext adds an item to a list path with context support.
func RemotePushWithContext[T any](ctx context.Context, cfg RemoteConfig, path string, item T) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if key.LastIndex(path) != "*" {
		log.Println("RemotePush["+path+"]: ", ErrPathGlobRequired)
		return ErrPathGlobRequired
	}

	_path := key.Build(path)

	data, err := json.Marshal(item)
	if err != nil {
		log.Printf("RemotePush[%s]: failed to marshal data: %v", path, err)
		return err
	}

	_, err = cfg.doWithRetry(ctx, "RemotePush", path, func() (*http.Request, error) {
		req, err := http.NewRequest("POST", cfg.URL(_path), bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		for k, v := range cfg.Header {
			req.Header[k] = v
		}
		return req, nil
	})
	return err
}

// RemoteGet retrieves an item from the given path (non-list path only).
// Use RemoteGetWithContext for cancellation support.
func RemoteGet[T any](cfg RemoteConfig, path string) (client.Meta[T], error) {
	return RemoteGetWithContext[T](context.Background(), cfg, path)
}

// RemoteGetWithContext retrieves an item from the given path with context support.
func RemoteGetWithContext[T any](ctx context.Context, cfg RemoteConfig, path string) (client.Meta[T], error) {
	if err := cfg.Validate(); err != nil {
		return client.Meta[T]{}, err
	}
	if key.LastIndex(path) == "*" {
		log.Println("RemoteGet["+path+"]: ", ErrPathGlobNotAllowed)
		return client.Meta[T]{}, ErrPathGlobNotAllowed
	}

	result, err := cfg.doWithRetry(ctx, "RemoteGet", path, func() (*http.Request, error) {
		req, err := http.NewRequest("GET", cfg.URL(path), nil)
		if err != nil {
			return nil, err
		}
		for k, v := range cfg.Header {
			req.Header[k] = v
		}
		return req, nil
	})
	if err != nil {
		return client.Meta[T]{}, err
	}

	obj, err := meta.DecodePooled(result.body)
	if err != nil {
		log.Printf("RemoteGet[%s]: failed to decode data: %v", path, err)
		return client.Meta[T]{}, err
	}
	var item T
	if err := json.Unmarshal(obj.Data, &item); err != nil {
		meta.PutObject(obj)
		log.Printf("RemoteGet[%s]: failed to unmarshal data: %v", path, err)
		return client.Meta[T]{}, err
	}
	metaResult := client.Meta[T]{
		Created: obj.Created,
		Updated: obj.Updated,
		Index:   obj.Index,
		Data:    item,
	}
	meta.PutObject(obj)
	return metaResult, nil
}

// RemoteGetList retrieves a list of items from the given path (glob path only).
// Use RemoteGetListWithContext for cancellation support.
func RemoteGetList[T any](cfg RemoteConfig, path string) ([]client.Meta[T], error) {
	return RemoteGetListWithContext[T](context.Background(), cfg, path)
}

// RemoteGetListWithContext retrieves a list of items from the given path with context support.
func RemoteGetListWithContext[T any](ctx context.Context, cfg RemoteConfig, path string) ([]client.Meta[T], error) {
	if err := cfg.Validate(); err != nil {
		return []client.Meta[T]{}, err
	}
	if key.LastIndex(path) != "*" {
		log.Println("RemoteGetList["+path+"]: ", ErrPathGlobRequired)
		return []client.Meta[T]{}, ErrPathGlobRequired
	}

	result, err := cfg.doWithRetry(ctx, "RemoteGetList", path, func() (*http.Request, error) {
		req, err := http.NewRequest("GET", cfg.URL(path), nil)
		if err != nil {
			return nil, err
		}
		for k, v := range cfg.Header {
			req.Header[k] = v
		}
		return req, nil
	})
	if err != nil {
		return []client.Meta[T]{}, err
	}

	objs, err := meta.DecodeList(result.body)
	if err != nil {
		log.Printf("RemoteGetList[%s]: failed to decode data: %v", path, err)
		return []client.Meta[T]{}, err
	}

	items := make([]client.Meta[T], 0, len(objs))
	for _, obj := range objs {
		var item T
		if err := json.Unmarshal(obj.Data, &item); err != nil {
			log.Printf("RemoteGetList[%s]: failed to unmarshal item: %v", path, err)
			continue
		}
		items = append(items, client.Meta[T]{
			Created: obj.Created,
			Updated: obj.Updated,
			Index:   obj.Index,
			Data:    item,
		})
	}
	return items, nil
}

// RemoteDelete deletes an item at the given path.
// Use RemoteDeleteWithContext for cancellation support.
func RemoteDelete(cfg RemoteConfig, path string) error {
	return RemoteDeleteWithContext(context.Background(), cfg, path)
}

// RemoteDeleteWithContext deletes an item at the given path with context support.
func RemoteDeleteWithContext(ctx context.Context, cfg RemoteConfig, path string) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	_, err := cfg.doWithRetry(ctx, "RemoteDelete", path, func() (*http.Request, error) {
		req, err := http.NewRequest("DELETE", cfg.URL(path), nil)
		if err != nil {
			return nil, err
		}
		for k, v := range cfg.Header {
			req.Header[k] = v
		}
		return req, nil
	})
	return err
}
