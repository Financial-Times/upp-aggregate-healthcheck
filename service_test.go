package main

import (
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"k8s.io/client-go/kubernetes/fake"
	"net/http"
	"strings"
	"testing"
)

type MockWebClient struct{}
type mockTransport struct {
	responseStatusCode int
	responseBody       string
}

const (
	validIP = "1.0.0.0"
	validK8sServiceName = "validServiceName"
	validK8sServiceNameWithAck = "validK8sServiceNameWithAck"
	nonExistingK8sServiceName ="vnonExistingServiceName"
	validSeverity = uint8(1)
	ackMsg = "ack-msg"
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

func initializeMockServiceWithK8sServices() *k8sHealthcheckService {
	services := make(map[string]service)
	services[validK8sServiceName] = service{
		name:validServiceName,
	}
	services[validK8sServiceNameWithAck] = service{
		name:validK8sServiceNameWithAck,
		ack:"test",
	}
	return &k8sHealthcheckService{
		services: servicesMap{
			m:services,
		},
	}
}

func initializeMockServiceWithDeployments() *k8sHealthcheckService {
	deployments := make(map[string]deployment)
	deployments[validK8sServiceName] = deployment{
		numberOfUnavailableReplicas:0,
		numberOfAvailableReplicas:2,
	}
	return &k8sHealthcheckService{
		deployments: deploymentsMap{
			m:deployments,
		},
	}
}

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
		responseStatusCode: responseStatusCode,
		responseBody:       responseBody,
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
	_, err := service.getHealthChecksForPod(pod{name: "test", ip: validIP}, 8080)
	assert.NotNil(t, err)
}

func TestGetHealthChecksForPodHealthAvailable(t *testing.T) {
	service := initializeMockService(initializeMockHTTPClient(http.StatusOK, validFailingHealthCheckResponseBody))
	healthCheckResponse, err := service.getHealthChecksForPod(pod{name: "test", ip: validIP}, 8080)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(healthCheckResponse.Checks))
}

func TestGetIndividualPodSeverityErrorWhilePodHealthCheck(t *testing.T) {
	service := initializeMockService(initializeMockHTTPClient(http.StatusInternalServerError, ""))
	severity, err := service.getIndividualPodSeverity(pod{name: "test", ip: validIP}, 8080)
	assert.NotNil(t, err)
	assert.Equal(t, defaultSeverity, severity)
}

func TestGetIndividualPodSeverityValidPodHealth(t *testing.T) {
	service := initializeMockService(initializeMockHTTPClient(http.StatusOK, validFailingHealthCheckResponseBody))
	severity, err := service.getIndividualPodSeverity(pod{name: "test", ip: validIP}, 8080)
	assert.Nil(t, err)
	assert.Equal(t, validSeverity, severity)
}

func TestCheckPodHealthFailingChecks(t *testing.T) {
	service := initializeMockService(initializeMockHTTPClient(http.StatusOK, validFailingHealthCheckResponseBody))
	err := service.checkPodHealth(pod{name: "test", ip: validIP}, 8080)
	assert.NotNil(t, err)
}

func TestCheckPodHealthWithInvalidUrl(t *testing.T) {
	service := initializeMockService(nil)
	err := service.checkPodHealth(pod{name: "test", ip: "%s"}, 8080)
	assert.NotNil(t, err)
}

func TestCheckPodHealthPassingChecks(t *testing.T) {
	service := initializeMockService(initializeMockHTTPClient(http.StatusOK, validPassingHealthCheckResponseBody))
	err := service.checkPodHealth(pod{name: "test", ip: validIP}, 8080)
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

func TestCheckServiceHealthByResiliencyNoPodsAvailable(t *testing.T) {
	_, err := checkServiceHealthByResiliency(service{}, 0, 3)
	assert.NotNil(t, err)
}

func TestCheckServiceHealthByResiliencyWithNonResilientServiceAndUnvavailablePods(t *testing.T) {
	s := service{
		isResilient: false,
	}
	_, err := checkServiceHealthByResiliency(s, 1, 3)
	assert.NotNil(t, err)
}

func TestCheckServiceHealthByResiliencyWithResilientServiceAndUnvavailablePods(t *testing.T) {
	s := service{
		isResilient: true,
	}
	msg, err := checkServiceHealthByResiliency(s, 1, 3)
	assert.Nil(t, err)
	assert.NotNil(t, msg)
}

func TestCheckServiceHealthByResiliencyHappyFlow(t *testing.T) {
	s := service{
		isResilient: false,
	}
	msg, err := checkServiceHealthByResiliency(s, 1, 0)
	assert.Nil(t, err)
	assert.Equal(t, "", msg)
}

func TestCheckServiceHealthWithDeploymentHappyFlow(t *testing.T) {
	k8sHcService := initializeMockServiceWithDeployments()
	s := service{
		name:validK8sServiceName,
		isResilient: false,
	}

	_,err := k8sHcService.checkServiceHealth(s)
	assert.Nil(t, err)
}

func TestCheckServiceHealthWithDeploymentNonExistingServiceName(t *testing.T) {
	k8sHcService := initializeMockServiceWithDeployments()
	s := service{
		name:nonExistingK8sServiceName,
		isResilient: false,
	}

	_,err := k8sHcService.checkServiceHealth(s)
	assert.NotNil(t, err)
}

func TestUpdateAcksForServicesEmptyAckList(t *testing.T) {
	hcService := initializeMockServiceWithK8sServices()
	acks := make(map[string]string)
	acks[validK8sServiceName] = ackMsg
	hcService.updateAcksForServices(acks)

	assert.Equal(t, hcService.services.m[validK8sServiceNameWithAck].ack,"")
	assert.Equal(t, hcService.services.m[validK8sServiceName].ack,ackMsg)
}

