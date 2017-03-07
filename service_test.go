package main

import (
	"k8s.io/client-go/kubernetes/fake"
	"testing"
	"github.com/stretchr/testify/assert"
	"net/http"
	"strings"
	"io/ioutil"
)

type MockWebClient struct{}
type mockTransport struct {
	responseStatusCode int
	responseBody       string
}

const (
	validIP = "1.0.0.0"
	validSeverity = uint8(1)
	validFailingHealthCheckResponseBody = `{
  "schemaVersion": 1,
  "name": "CMSNotifierApplication",
  "description": "CMSNotifierApplication",
  "checks": [
    {
      "checkOutput": "",
      "panicGuide": "",
      "lastUpdated": "",
      "ok": false,
      "severity": 2,
      "businessImpact": "",
      "technicalSummary": "",
      "name": ""
    },
	{
      "checkOutput": "",
      "panicGuide": "",
      "lastUpdated": "",
      "ok": false,
      "severity": 1,
      "businessImpact": "",
      "technicalSummary": "",
      "name": ""
    }
  ]
}`
	validPassingHealthCheckResponseBody = `{
  "schemaVersion": 1,
  "name": "CMSNotifierApplication",
  "description": "CMSNotifierApplication",
  "checks": [
    {
      "checkOutput": "",
      "panicGuide": "",
      "lastUpdated": "",
      "ok": true,
      "severity": 2,
      "businessImpact": "",
      "technicalSummary": "",
      "name": ""
    }
  ]
}`
)

func initializeMockService(httpClient *http.Client) *k8sHealthcheckService {
	mockK8sClient := fake.NewSimpleClientset()

	return &k8sHealthcheckService{
		k8sClient:  mockK8sClient,
		httpClient: httpClient,
	}
}

func initializeMockHTTPClient(responseStatusCode int, responseBody string) *http.Client {
	client := http.DefaultClient
	client.Transport = &mockTransport{
		responseStatusCode:responseStatusCode,
		responseBody:responseBody,
	}

	return client
}

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	response := &http.Response{
		Header:     make(http.Header),
		Request:    req,
		StatusCode: t.responseStatusCode,
	}

	response.Header.Set("Content-Type", "application/json")
	response.Body = ioutil.NopCloser(strings.NewReader(t.responseBody))

	return response, nil
}

func TestGetHealthChecksForPodInternalServerErr(t *testing.T) {
	service := initializeMockService(initializeMockHTTPClient(http.StatusInternalServerError, ""))
	_, err := service.getHealthChecksForPod(pod{name:"test", ip:validIP, }, 8080)
	assert.NotNil(t, err)
}

func TestGetHealthChecksForPodHealthAvailable(t *testing.T) {
	service := initializeMockService(initializeMockHTTPClient(http.StatusOK, validFailingHealthCheckResponseBody))
	healthCheckResponse, err := service.getHealthChecksForPod(pod{name:"test", ip:validIP, }, 8080)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(healthCheckResponse.Checks))
}

func TestGetIndividualPodSeverityErrorWhilePodHealthCheck(t *testing.T) {
	service := initializeMockService(initializeMockHTTPClient(http.StatusInternalServerError, ""))
	severity, err := service.getIndividualPodSeverity(pod{name:"test", ip:validIP, }, 8080)
	assert.NotNil(t, err)
	assert.Equal(t, defaultSeverity, severity)
}

func TestGetIndividualPodSeverityValidPodHealth(t *testing.T) {
	service := initializeMockService(initializeMockHTTPClient(http.StatusOK, validFailingHealthCheckResponseBody))
	severity, err := service.getIndividualPodSeverity(pod{name:"test", ip:validIP, }, 8080)
	assert.Nil(t, err)
	assert.Equal(t, validSeverity, severity)
}

func TestCheckPodHealthFailingChecks(t *testing.T) {
	service := initializeMockService(initializeMockHTTPClient(http.StatusOK, validFailingHealthCheckResponseBody))
	err := service.checkPodHealth(pod{name:"test", ip:validIP, }, 8080)
	assert.NotNil(t, err)
}

func TestCheckPodHealthPassingChecks(t *testing.T) {
	service := initializeMockService(initializeMockHTTPClient(http.StatusOK, validPassingHealthCheckResponseBody))
	err := service.checkPodHealth(pod{name:"test", ip:validIP, }, 8080)
	assert.Nil(t, err)
}

func TestGetCategories(t *testing.T) {
	service := initializeMockService(nil)
	_, err := service.getCategories()
	assert.Nil(t, err)
}

func TestUpdateCategoryInvalidConfigMap(t *testing.T) {
	service := initializeMockService(nil)
	err := service.updateCategory("validCategoryName", true)
	assert.NotNil(t, err)
}

func TestAddAckConfigMapNotFound(t *testing.T) {
	service := initializeMockService(nil)
	err := service.addAck("invalidServiceName", "ack message")
	assert.NotNil(t, err)
}

func TestGetServicesByNamesInvalidServiceName(t *testing.T) {
	service := initializeMockService(nil)
	services := service.getServicesByNames([]string{"invalidServiceName"})
	assert.Zero(t, len(services))
}
