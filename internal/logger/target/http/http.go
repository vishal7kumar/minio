// Copyright (c) 2015-2023 MinIO, Inc.
//
// This file is part of MinIO Object Storage stack
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	xhttp "github.com/minio/minio/internal/http"
	"github.com/minio/minio/internal/logger/target/types"
	"github.com/minio/minio/internal/once"
	"github.com/minio/minio/internal/store"
	xnet "github.com/minio/pkg/net"
)

const (
	// Timeout for the webhook http call
	webhookCallTimeout = 5 * time.Second

	// maxWorkers is the maximum number of concurrent operations.
	maxWorkers = 16

	// the suffix for the configured queue dir where the logs will be persisted.
	httpLoggerExtension = ".http.log"
)

const (
	statusOffline = iota
	statusOnline
	statusClosed
)

// Config http logger target
type Config struct {
	Enabled    bool              `json:"enabled"`
	Name       string            `json:"name"`
	UserAgent  string            `json:"userAgent"`
	Endpoint   string            `json:"endpoint"`
	AuthToken  string            `json:"authToken"`
	ClientCert string            `json:"clientCert"`
	ClientKey  string            `json:"clientKey"`
	QueueSize  int               `json:"queueSize"`
	QueueDir   string            `json:"queueDir"`
	Proxy      string            `json:"string"`
	Transport  http.RoundTripper `json:"-"`

	// Custom logger
	LogOnce func(ctx context.Context, err error, id string, errKind ...interface{}) `json:"-"`
}

// Target implements logger.Target and sends the json
// format of a log entry to the configured http endpoint.
// An internal buffer of logs is maintained but when the
// buffer is full, new logs are just ignored and an error
// is returned to the caller.
type Target struct {
	totalMessages  int64
	failedMessages int64
	status         int32

	// Worker control
	workers       int64
	workerStartMu sync.Mutex
	lastStarted   time.Time

	wg sync.WaitGroup

	// Channel of log entries.
	// Reading logCh must hold read lock on logChMu (to avoid read race)
	// Sending a value on logCh must hold read lock on logChMu (to avoid closing)
	logCh   chan interface{}
	logChMu sync.RWMutex

	// If the first init fails, this starts a goroutine that
	// will attempt to establish the connection.
	revive sync.Once

	// store to persist and replay the logs to the target
	// to avoid missing events when the target is down.
	store          store.Store[interface{}]
	storeCtxCancel context.CancelFunc

	initQueueStoreOnce once.Init

	config Config
	client *http.Client
}

// Name returns the name of the target
func (h *Target) Name() string {
	return "minio-http-" + h.config.Name
}

// Endpoint returns the backend endpoint
func (h *Target) Endpoint() string {
	return h.config.Endpoint
}

func (h *Target) String() string {
	return h.config.Name
}

// IsOnline returns true if the target is reachable.
func (h *Target) IsOnline(ctx context.Context) bool {
	if err := h.checkAlive(ctx); err != nil {
		return !xnet.IsNetworkOrHostDown(err, false)
	}
	return true
}

// Stats returns the target statistics.
func (h *Target) Stats() types.TargetStats {
	h.logChMu.RLock()
	queueLength := len(h.logCh)
	h.logChMu.RUnlock()
	stats := types.TargetStats{
		TotalMessages:  atomic.LoadInt64(&h.totalMessages),
		FailedMessages: atomic.LoadInt64(&h.failedMessages),
		QueueLength:    queueLength,
	}

	return stats
}

// This will check if we can reach the remote.
func (h *Target) checkAlive(ctx context.Context) (err error) {
	return h.send(ctx, []byte(`{}`), webhookCallTimeout)
}

// Init validate and initialize the http target
func (h *Target) Init(ctx context.Context) (err error) {
	if h.config.QueueDir != "" {
		return h.initQueueStoreOnce.DoWithContext(ctx, h.initQueueStore)
	}
	return h.initLogChannel(ctx)
}

func (h *Target) initQueueStore(ctx context.Context) (err error) {
	var queueStore store.Store[interface{}]
	queueDir := filepath.Join(h.config.QueueDir, h.Name())
	queueStore = store.NewQueueStore[interface{}](queueDir, uint64(h.config.QueueSize), httpLoggerExtension)
	if err = queueStore.Open(); err != nil {
		return fmt.Errorf("unable to initialize the queue store of %s webhook: %w", h.Name(), err)
	}
	ctx, cancel := context.WithCancel(ctx)
	h.store = queueStore
	h.storeCtxCancel = cancel
	store.StreamItems(h.store, h, ctx.Done(), h.config.LogOnce)
	return
}

func (h *Target) initLogChannel(ctx context.Context) (err error) {
	switch atomic.LoadInt32(&h.status) {
	case statusOnline:
		return nil
	case statusClosed:
		return errors.New("target is closed")
	}

	if !h.IsOnline(ctx) {
		// Start a goroutine that will continue to check if we can reach
		h.revive.Do(func() {
			go func() {
				t := time.NewTicker(time.Second)
				defer t.Stop()
				for range t.C {
					if atomic.LoadInt32(&h.status) != statusOffline {
						return
					}
					if h.IsOnline(ctx) {
						// We are online.
						if atomic.CompareAndSwapInt32(&h.status, statusOffline, statusOnline) {
							h.workerStartMu.Lock()
							h.lastStarted = time.Now()
							h.workerStartMu.Unlock()
							atomic.AddInt64(&h.workers, 1)
							go h.startHTTPLogger(ctx)
						}
						return
					}
				}
			}()
		})
		return err
	}

	if atomic.CompareAndSwapInt32(&h.status, statusOffline, statusOnline) {
		h.workerStartMu.Lock()
		h.lastStarted = time.Now()
		h.workerStartMu.Unlock()
		atomic.AddInt64(&h.workers, 1)
		go h.startHTTPLogger(ctx)
	}
	return nil
}

func (h *Target) send(ctx context.Context, payload []byte, timeout time.Duration) (err error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		h.config.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("invalid configuration for '%s'; %v", h.config.Endpoint, err)
	}
	req.Header.Set(xhttp.ContentType, "application/json")
	req.Header.Set(xhttp.MinIOVersion, xhttp.GlobalMinIOVersion)
	req.Header.Set(xhttp.MinioDeploymentID, xhttp.GlobalDeploymentID)

	// Set user-agent to indicate MinIO release
	// version to the configured log endpoint
	req.Header.Set("User-Agent", h.config.UserAgent)

	if h.config.AuthToken != "" {
		req.Header.Set("Authorization", h.config.AuthToken)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("%s returned '%w', please check your endpoint configuration", h.config.Endpoint, err)
	}

	// Drain any response.
	xhttp.DrainBody(resp.Body)

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted, http.StatusNoContent:
		// accepted HTTP status codes.
		return nil
	case http.StatusForbidden:
		return fmt.Errorf("%s returned '%s', please check if your auth token is correctly set", h.config.Endpoint, resp.Status)
	default:
		return fmt.Errorf("%s returned '%s', please check your endpoint configuration", h.config.Endpoint, resp.Status)
	}
}

func (h *Target) logEntry(ctx context.Context, entry interface{}) {
	logJSON, err := json.Marshal(&entry)
	if err != nil {
		atomic.AddInt64(&h.failedMessages, 1)
		return
	}

	tries := 0
	for {
		if tries > 0 {
			if tries >= 10 || atomic.LoadInt32(&h.status) == statusClosed {
				// Don't retry when closing...
				return
			}
			// sleep = (tries+2) ^ 2 milliseconds.
			sleep := time.Duration(math.Pow(float64(tries+2), 2)) * time.Millisecond
			if sleep > time.Second {
				sleep = time.Second
			}
			time.Sleep(sleep)
		}
		tries++
		if err := h.send(ctx, logJSON, webhookCallTimeout); err != nil {
			h.config.LogOnce(ctx, err, h.config.Endpoint)
			atomic.AddInt64(&h.failedMessages, 1)
		} else {
			return
		}
	}
}

func (h *Target) startHTTPLogger(ctx context.Context) {
	h.logChMu.RLock()
	logCh := h.logCh
	if logCh != nil {
		// We are not allowed to add when logCh is nil
		h.wg.Add(1)
		defer h.wg.Done()
	}
	h.logChMu.RUnlock()

	defer atomic.AddInt64(&h.workers, -1)

	if logCh == nil {
		return
	}
	// Send messages until channel is closed.
	for entry := range logCh {
		atomic.AddInt64(&h.totalMessages, 1)
		h.logEntry(ctx, entry)
	}
}

// New initializes a new logger target which
// sends log over http to the specified endpoint
func New(config Config) *Target {
	h := &Target{
		logCh:  make(chan interface{}, config.QueueSize),
		config: config,
		status: statusOffline,
	}

	// If proxy available, set the same
	if h.config.Proxy != "" {
		proxyURL, _ := url.Parse(h.config.Proxy)
		transport := h.config.Transport
		ctransport := transport.(*http.Transport).Clone()
		ctransport.Proxy = http.ProxyURL(proxyURL)
		h.config.Transport = ctransport
	}
	h.client = &http.Client{Transport: h.config.Transport}

	return h
}

// SendFromStore - reads the log from store and sends it to webhook.
func (h *Target) SendFromStore(key string) (err error) {
	var eventData interface{}
	eventData, err = h.store.Get(key)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	atomic.AddInt64(&h.totalMessages, 1)
	logJSON, err := json.Marshal(&eventData)
	if err != nil {
		atomic.AddInt64(&h.failedMessages, 1)
		return
	}
	if err := h.send(context.Background(), logJSON, webhookCallTimeout); err != nil {
		atomic.AddInt64(&h.failedMessages, 1)
		if xnet.IsNetworkOrHostDown(err, true) {
			return store.ErrNotConnected
		}
		return err
	}
	// Delete the event from store.
	return h.store.Del(key)
}

// Send log message 'e' to http target.
// If servers are offline messages are queued until queue is full.
// If Cancel has been called the message is ignored.
func (h *Target) Send(ctx context.Context, entry interface{}) error {
	if h.store != nil {
		// save the entry to the queue store which will be replayed to the target.
		return h.store.Put(entry)
	}
	if atomic.LoadInt32(&h.status) == statusClosed {
		return nil
	}
	h.logChMu.RLock()
	defer h.logChMu.RUnlock()
	if h.logCh == nil {
		// We are closing...
		return nil
	}
	select {
	case h.logCh <- entry:
	default:
		// Drop messages until we are online.
		if !h.IsOnline(ctx) {
			atomic.AddInt64(&h.totalMessages, 1)
			atomic.AddInt64(&h.failedMessages, 1)
			return errors.New("log buffer full and remote offline")
		}
		nWorkers := atomic.LoadInt64(&h.workers)
		if nWorkers < maxWorkers {
			// Only have one try to start at the same time.
			h.workerStartMu.Lock()
			defer h.workerStartMu.Unlock()
			// Start one max every second.
			if time.Since(h.lastStarted) > time.Second {
				if atomic.CompareAndSwapInt64(&h.workers, nWorkers, nWorkers+1) {
					// Start another logger.
					h.lastStarted = time.Now()
					go h.startHTTPLogger(ctx)
				}
			}
			h.logCh <- entry
			return nil
		}
		// log channel is full, do not wait and return
		// an error immediately to the caller
		atomic.AddInt64(&h.totalMessages, 1)
		atomic.AddInt64(&h.failedMessages, 1)
		return errors.New("log buffer full, remote endpoint is not able to keep up")
	}

	return nil
}

// Cancel - cancels the target.
// All queued messages are flushed and the function returns afterwards.
// All messages sent to the target after this function has been called will be dropped.
func (h *Target) Cancel() {
	atomic.StoreInt32(&h.status, statusClosed)

	// If queuestore is configured, cancel it's context to
	// stop the replay go-routine.
	if h.store != nil {
		h.storeCtxCancel()
	}

	// Set logch to nil and close it.
	// This will block all Send operations,
	// and finish the existing ones.
	// All future ones will be discarded.
	h.logChMu.Lock()
	close(h.logCh)
	h.logCh = nil
	h.logChMu.Unlock()

	// Wait for messages to be sent...
	h.wg.Wait()
}

// Type - returns type of the target
func (h *Target) Type() types.TargetType {
	return types.TargetHTTP
}
