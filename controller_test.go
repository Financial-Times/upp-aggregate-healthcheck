package main

import (
	"errors"
	"github.com/stretchr/testify/assert"
	"net/http"
	"testing"
)

const (
	nonExistingServiceName     = "non-existing-service"
	serviceNameForRemoveAckErr = "serviceNameWithRemoveAckError"
)

type MockService struct {
}

func (m *MockService) getCategories() (map[string]category, error) {
	categories := make(map[string]category)

	categories["default"] = category{
		name: "default",
	}

	categories["content-read"] = category{
		name: "content-read",
	}
	return categories, nil
}

func (m *MockService) updateCategory(string, bool) error {
	return nil
}
func (m *MockService) getServicesByNames(serviceNames []string) []service {
	if len(serviceNames) != 0 && serviceNames[0] == nonExistingServiceName {
		return []service{}
	}

	services := []service{
		{
			name:     "test-service-name",
			severity: 1,
			ack:      "test ack",
		},
		{
			name:     "test-service-name-2",
			severity: 2,
		},
	}

	return services
}

func (m *MockService) getPodsForService(string) ([]pod, error) {
	return []pod{
		{
			name: "test-pod-name1-8425234-9hdfg ",
			ip:   "10.2.51.2",
		},
		{
			name: "test-pod-name2-8425234-9hdfg ",
			ip:   "10.2.51.2",
		},
	}, nil
}

func (m *MockService) getPodByName(string) (pod, error) {
	return pod{
		name: "test-pod-name-8425234-9hdfg ",
		ip:   "10.2.51.2",
	}, nil
}

func (m *MockService) checkServiceHealth(string) (string, error) {
	return "", errors.New("Error reading healthcheck response: ")
}

func (m *MockService) checkPodHealth(pod) error {
	return errors.New("Error reading healthcheck response: ")
}

func (m *MockService) getIndividualPodSeverity(pod) (uint8, error) {
	return 1, nil
}

func (m *MockService) getHealthChecksForPod(pod) (healthcheckResponse, error) {
	return healthcheckResponse{}, nil
}

func (m *MockService) addAck(string, string) error {
	return nil
}
func (m *MockService) removeAck(serviceName string) error {
	if serviceName == serviceNameForRemoveAckErr {
		return errors.New("Cannot remove ack")
	}

	return nil
}
func (m *MockService) getHTTPClient() *http.Client {
	return &http.Client{}
}

func initializeMockController(env string, service healthcheckService) *healthCheckController {
	measuredServices := make(map[string]measuredService)

	return &healthCheckController{
		healthCheckService: service,
		environment:        env,
		measuredServices:   measuredServices,
	}
}

func TestAddAckNilErr(t *testing.T) {
	service := new(MockService)
	controller := initializeMockController("test", service)
	err := controller.addAck("abc", "abc")
	assert.Nil(t, err)
}

func TestRemoveAckNonExistingServiceErr(t *testing.T) {
	service := new(MockService)
	controller := initializeMockController("test", service)
	err := controller.removeAck(nonExistingServiceName)
	assert.NotNil(t, err)
}

func TestRemoveAckServiceErr(t *testing.T) {
	service := new(MockService)
	controller := initializeMockController("test", service)
	err := controller.removeAck(serviceNameForRemoveAckErr)
	assert.NotNil(t, err)
}

func TestBuildServicesHealthResult(t *testing.T) {
	service := new(MockService)
	controller := initializeMockController("test", service)
	_, _, _, err := controller.buildServicesHealthResult([]string{"abc"}, false)
	assert.Nil(t, err)
}
