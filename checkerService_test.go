package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type mockHTTPClient struct {
	doFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.doFunc(req)
}

type mockHTTPClientWithChangingResponses struct {
	attempt     bool
	doFuncFirst func(req *http.Request) (*http.Response, error)
	doFuncOther func(req *http.Request) (*http.Response, error)
}

func (m *mockHTTPClientWithChangingResponses) Do(req *http.Request) (*http.Response, error) {
	if !m.attempt {
		m.attempt = true
		return m.doFuncFirst(req)
	}
	return m.doFuncOther(req)
}

func Test_getHealthChecksForPodWithRetry(t *testing.T) {
	testRequest := httptest.NewRequest(http.MethodGet, "/health", nil)
	testHealthcheckResponseBody := `{"schemaVersion":1,"systemCode":"content-unroller","name":"Content Unroller","description":"Content Unroller - unroll images and dynamic content for a given content","checks":[{"id":"check-connect-content-public-read","name":"Check connectivity to content-public-read","ok":true,"severity":1,"businessImpact":"Unrolled images and dynamic content won't be available","technicalSummary":"Cannot connect to content-public-read.","panicGuide":"https://dewey.in.ft.com/runbooks/contentreadapi","checkOutput":"Ok","lastUpdated":"2025-08-26T09:09:44.953777115Z"}],"ok":true}`
	type args struct {
		httpClient        httpClient
		remainingAttempts int
		cooldown          time.Duration
	}
	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "Healthcheck is OK",
			args: args{
				httpClient: &mockHTTPClient{
					doFunc: func(_ *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: 200,
							Body:       io.NopCloser(bytes.NewBufferString(testHealthcheckResponseBody)),
						}, nil
					},
				},
				remainingAttempts: 1,
				cooldown:          0,
			},
			wantErr: assert.NoError,
		},
		{
			name: "Healthcheck times out",
			args: args{
				httpClient: &mockHTTPClient{
					doFunc: func(_ *http.Request) (*http.Response, error) {
						return nil, fmt.Errorf("timed out")
					},
				},
				remainingAttempts: 1,
				cooldown:          0,
			},
			wantErr: assert.Error,
		},
		{
			name: "Healthcheck returns non 200 status code",
			args: args{
				httpClient: &mockHTTPClient{
					doFunc: func(_ *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: 500,
							Body:       io.NopCloser(bytes.NewBufferString(testHealthcheckResponseBody)),
						}, nil
					},
				},
				remainingAttempts: 1,
				cooldown:          0,
			},
			wantErr: assert.Error,
		},
		{
			name: "Healthcheck returns non 200 status code",
			args: args{
				httpClient: &mockHTTPClient{
					doFunc: func(_ *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: 500,
							Body:       io.NopCloser(bytes.NewBufferString(testHealthcheckResponseBody)),
						}, nil
					},
				},
				remainingAttempts: 1,
				cooldown:          0,
			},
			wantErr: assert.Error,
		},
		{
			name: "Healthcheck returns error on first attempt, no error after",
			args: args{
				httpClient: &mockHTTPClientWithChangingResponses{
					doFuncFirst: func(_ *http.Request) (*http.Response, error) {
						return nil, fmt.Errorf("timed out")
					},
					doFuncOther: func(_ *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: 200,
							Body:       io.NopCloser(bytes.NewBufferString(testHealthcheckResponseBody)),
						}, nil
					},
				},
				remainingAttempts: 2,
				cooldown:          0,
			},
			wantErr: assert.NoError,
		},
		{
			name: "Healthcheck returns non 200 status code on first attempt, 200 after",
			args: args{
				httpClient: &mockHTTPClientWithChangingResponses{
					doFuncFirst: func(_ *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: 500,
							Body:       io.NopCloser(bytes.NewBufferString(testHealthcheckResponseBody)),
						}, nil
					},
					doFuncOther: func(_ *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: 200,
							Body:       io.NopCloser(bytes.NewBufferString(testHealthcheckResponseBody)),
						}, nil
					},
				},
				remainingAttempts: 2,
				cooldown:          0,
			},
			wantErr: assert.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := getHealthChecksForPodWithRetry(testRequest, tt.args.httpClient, tt.args.remainingAttempts, tt.args.cooldown)
			assert.True(t, tt.wantErr(t, err))
		})
	}
}
