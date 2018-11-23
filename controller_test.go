package main

import (
	"errors"
	"fmt"
	"github.com/Financial-Times/go-logger"
	"net/http"
	"testing"
	"time"

	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
	"github.com/stretchr/testify/assert"
	"strconv"
	"strings"
)

const (
	nonExistingServiceName  = "non-existing-service"
	serviceNameForAckErr    = "serviceNameWithAckError"
	invalidNameForService   = "invalidNameForService"
	nonExistingPodName      = "nonExistingPodName"
	podWithCriticalSeverity = "podWithCriticalSeverity"
	failingPod              = "failingPod"
	podWithBrokenService    = "podWithBrokenService"
	nonExistingCategoryName = "nonExistingCategoryName"
	validCat                = "validCat"
	validService            = "validService"
	validEnvName            = "valid-env-name"
	ip                      = "10.2.51.2"
	severity1               = uint8(1)
)

func init() {
	logger.InitLogger("upp-aggregate-healthcheck", "debug")
}

var defaultPods = []pod{
	{
		name: "test-pod-name2-8425234-9hdfg ",
		ip:   "10.2.51.2",
	},
	{
		name: "test-pod-name1-8425234-9hdfg ",
		ip:   "10.2.51.2",
	},
}

type MockService struct {
	httpClient *http.Client
}

func (m *MockService) getCategories() (map[string]category, error) {
	categories := make(map[string]category)

	categories["default"] = category{
		name: "default",
	}

	categories["content-read"] = category{
		name:     "content-read",
		isSticky: true,
	}
	return categories, nil
}

func (m *MockService) updateCategory(categoryName string, isEnabled bool) error {
	if categoryName == nonExistingCategoryName {
		return errors.New("Cannot find category")
	}

	return nil
}

func (m *MockService) getDeployments() (map[string]deployment, error) {
	return map[string]deployment{
		"test-service-name": {
			desiredReplicas: 2,
		},
		"test-service-name-2": {
			desiredReplicas: 2,
		},
	}, nil
}

func (m *MockService) isServicePresent(serviceName string) bool {
	return serviceName != nonExistingServiceName
}

func (m *MockService) getServiceByName(serviceName string) (service, error) {
	if serviceName == nonExistingServiceName {
		return service{}, fmt.Errorf("Cannot find service with name %s", serviceName)
	}
	return service{
		name:        "test-service-name",
		ack:         "test ack",
		isResilient: strings.HasPrefix(serviceName, "resilient"),
	}, nil
}

func (m *MockService) getServicesMapByNames(serviceNames []string) map[string]service {
	if len(serviceNames) != 0 && serviceNames[0] == nonExistingServiceName {
		return map[string]service{}
	}
	services := make(map[string]service)
	services["test-service-name"] = service{
		name: "test-service-name",
		ack:  "test ack",
	}
	services["test-service-name-2"] = service{
		name: "test-service-name-2",
	}
	return services
}

func createPods(goodCount int, notOkSeverities []int) []pod {
	var pods []pod
	for i := 0; i < goodCount; i++ {
		pods = append(pods, pod{name: "ok-pod-" + strconv.Itoa(i), ip: ip})
	}
	for _, sev := range notOkSeverities {
		pods = append(pods, pod{name: "notok-pod-" + strconv.Itoa(sev), ip: ip})
	}
	return pods
}

func (m *MockService) getPodsForService(serviceName string) ([]pod, error) {
	switch serviceName {
	case "invalidNameForService":
		return []pod{}, errors.New("invalid pod name")
	case "resilient-notok-sev1":
		return createPods(0, []int{2, 1}), nil
	case "resilient-notok-sev2":
		return createPods(0, []int{2, 2}), nil
	case "resilient-halfok-sev1":
		return createPods(1, []int{1}), nil
	case "resilient-halfok-sev2":
		return createPods(1, []int{2}), nil
	case "non-resilient-halfok-sev1":
		return createPods(1, []int{1}), nil
	case "non-resilient-halfok-sev2":
		return createPods(1, []int{2}), nil
	default:
		return defaultPods, nil
	}
}

func (m *MockService) getPodByName(podName string) (pod, error) {
	switch podName {
	case nonExistingPodName:
		{
			return pod{}, errors.New("Pod not found")
		}
	case podWithBrokenService:
		{
			return pod{
				name:        "test-pod-name-8425234-9hdfg ",
				ip:          "10.2.51.2",
				serviceName: nonExistingServiceName,
			}, nil
		}
	default:
		return pod{
			name: "test-pod-name-8425234-9hdfg ",
			ip:   "10.2.51.2",
		}, nil
	}
}

func (m *MockService) checkServiceHealth(service service, deployments map[string]deployment) (string, error) {
	return "", errors.New("Error reading healthcheck response: ")
}

func (m *MockService) checkPodHealth(pod, int32) error {
	return errors.New("Error reading healthcheck response: ")
}

func (m *MockService) getIndividualPodSeverity(pod pod, port int32) (uint8, bool, error) {
	switch pod.name {
	case failingPod:
		return 1, false, errors.New("Test")
	case podWithCriticalSeverity:
		return 1, true, nil
	case "notok-pod-1":
		return 1, true, nil
	case "notok-pod-2":
		return 2, true, nil
	default:
		return defaultSeverity, false, nil
	}
}

func (m *MockService) getHealthChecksForPod(pod pod, port int32) (healthcheckResponse, error) {
	if pod.name == nonExistingPodName {
		return healthcheckResponse{}, errors.New("Cannot find pod")
	}
	return healthcheckResponse{}, nil
}

func (m *MockService) addAck(serviceName string, ackMessage string) error {
	if serviceName == serviceNameForAckErr {
		return errors.New("Error")
	}
	return nil
}
func (m *MockService) removeAck(serviceName string) error {
	if serviceName == serviceNameForAckErr {
		return errors.New("Cannot remove ack")
	}
	return nil
}
func (m *MockService) getHTTPClient() *http.Client {
	return m.httpClient
}

func initializeMockController(env string, httpClient *http.Client) *healthCheckController {
	measuredServices := make(map[string]measuredService)
	service := new(MockService)
	service.httpClient = httpClient
	stickyCategoriesFailedServices := make(map[string]int)

	return &healthCheckController{
		healthCheckService:             service,
		environment:                    env,
		measuredServices:               measuredServices,
		stickyCategoriesFailedServices: stickyCategoriesFailedServices,
	}
}

func TestAddAckNilError(t *testing.T) {
	controller := initializeMockController("test", nil)
	err := controller.addAck("abc", "abc")
	assert.Nil(t, err)
}

func TestAddAckInvalidServiceName(t *testing.T) {
	controller := initializeMockController("test", nil)
	err := controller.addAck(nonExistingServiceName, "abc")
	assert.NotNil(t, err)
}

func TestAddAckInvalidServiceNameWillAckingError(t *testing.T) {
	controller := initializeMockController("test", nil)
	err := controller.addAck(serviceNameForAckErr, "abc")
	assert.NotNil(t, err)
}

func TestRemoveAckNonExistingServiceErr(t *testing.T) {
	controller := initializeMockController("test", nil)
	err := controller.removeAck(nonExistingServiceName)
	assert.NotNil(t, err)
}

func TestRemoveAckServiceErr(t *testing.T) {
	controller := initializeMockController("test", nil)
	err := controller.removeAck(serviceNameForAckErr)
	assert.NotNil(t, err)
}

func TestRemoveAckHappyFlow(t *testing.T) {
	controller := initializeMockController("test", nil)
	err := controller.removeAck(validService)
	assert.Nil(t, err)
}

func TestBuildServicesHealthResult(t *testing.T) {
	controller := initializeMockController("test", nil)
	_, _, _, err := controller.buildServicesHealthResult([]string{"abc"}, false)
	assert.Nil(t, err)
}

func TestBuildServicesHealthResultFromCache(t *testing.T) {
	controller := initializeMockController("test", nil)
	_, _, _, err := controller.buildServicesHealthResult([]string{"abc"}, true)
	assert.Nil(t, err)
}

func TestGetIndividualPodHealthHappyFlow(t *testing.T) {
	httpClient := initializeMockHTTPClient(http.StatusOK, "")
	controller := initializeMockController("test", httpClient)
	_, _, err := controller.getIndividualPodHealth("testPod")
	assert.Nil(t, err)
}

func TestGetIndividualPodHealthNonExistingPod(t *testing.T) {
	controller := initializeMockController("test", nil)
	_, _, err := controller.getIndividualPodHealth(nonExistingPodName)
	assert.NotNil(t, err)
}

func TestGetIndividualPodHealthFailingService(t *testing.T) {
	httpClient := initializeMockHTTPClient(http.StatusOK, "")
	controller := initializeMockController("test", httpClient)
	_, _, err := controller.getIndividualPodHealth(podWithBrokenService)
	assert.Nil(t, err)
}

func TestBuildPodsHealthResultHappyFlow(t *testing.T) {
	controller := initializeMockController("test", nil)
	_, err := controller.buildPodsHealthResult("testPod")
	assert.Nil(t, err)
}

func TestBuildPodsHealthResultInvalidPodName(t *testing.T) {
	controller := initializeMockController("test", nil)
	_, err := controller.buildPodsHealthResult(invalidNameForService)
	assert.NotNil(t, err)
}

func TestBuildPodsHealthResultInvalidServiceName(t *testing.T) {
	controller := initializeMockController("test", nil)
	_, err := controller.buildPodsHealthResult(nonExistingServiceName)
	assert.NotNil(t, err)
}

func TestGetSeverityForPodInvalidPodName(t *testing.T) {
	controller := initializeMockController("test", nil)
	severity := controller.getSeverityForPod(nonExistingPodName, 8080)
	assert.Equal(t, defaultSeverity, severity)
}

func TestComputeSeverityByPods(t *testing.T) {
	controller := initializeMockController("test", nil)
	severity := controller.computeSeverityByPods([]pod{{name: nonExistingPodName}}, 8080)
	assert.Equal(t, defaultSeverity, severity)
}

func TestComputeSeverityForPodWithCriticalSeverity(t *testing.T) {
	controller := initializeMockController("test", nil)
	severity := controller.computeSeverityByPods([]pod{{name: failingPod}, {name: podWithCriticalSeverity}}, 8080)
	assert.Equal(t, uint8(1), severity)
}

func TestGetSeverityForServiceInvalidServiceName(t *testing.T) {
	controller := initializeMockController("test", nil)
	severity := controller.getSeverityForService(invalidNameForService, 8080)
	assert.Equal(t, defaultSeverity, severity)
}

func TestGetSeverityForResilientService(t *testing.T) {
	controller := initializeMockController("test", nil)

	var testCases = []struct {
		serviceName      string
		expectedSeverity uint8
		description      string
	}{
		{
			serviceName:      "resilient-notok-sev1",
			expectedSeverity: severity1,
			description:      "resilient service with all pods failing a severity 1 check",
		},
		{
			serviceName:      "resilient-notok-sev2",
			expectedSeverity: defaultSeverity,
			description:      "resilient service with all pods failing a severity 2 check",
		},
		{
			serviceName:      "resilient-halfok-sev1",
			expectedSeverity: defaultSeverity,
			description:      "resilient service with one of two pods failing a severity 1 check",
		},
		{
			serviceName:      "resilient-halfok-sev2",
			expectedSeverity: defaultSeverity,
			description:      "resilient service with one of two pods failing a severity 2 check",
		},
		{
			serviceName:      "non-resilient-halfok-sev1",
			expectedSeverity: severity1,
			description:      "non-resilient service with one of two pods failing a severity 1 check",
		},
		{
			serviceName:      "non-resilient-halfok-sev2",
			expectedSeverity: defaultSeverity,
			description:      "non-resilient service with one of two pods failing a severity 2 check",
		},
	}
	for _, tc := range testCases {
		actualSeverity := controller.getSeverityForService(tc.serviceName, 8080)
		assert.Equal(t, tc.expectedSeverity, actualSeverity, tc.description)
	}

}

func TestGetSeverityForNonResilientService(t *testing.T) {
	controller := initializeMockController("test", nil)
	severity := controller.getSeverityForService(invalidNameForService, 8080)
	assert.Equal(t, defaultSeverity, severity)
}

func TestUpdateStickyCategoryInvalidCategoryName(t *testing.T) {
	controller := initializeMockController("test", nil)
	err := controller.updateStickyCategory(nonExistingCategoryName, false)
	assert.NotNil(t, err)
}

func TestUpdateStickyCategoryHappyFlow(t *testing.T) {
	controller := initializeMockController("test", nil)
	err := controller.updateStickyCategory(validCat, false)
	assert.Nil(t, err)
}

func TestFundShortestPeriodWithValidCategories(t *testing.T) {
	minRefreshPeriod := 15 * time.Second
	categories := make(map[string]category)
	categories["default"] = category{
		refreshPeriod: 60 * time.Second,
	}
	categories["image-publish"] = category{
		refreshPeriod: minRefreshPeriod,
	}

	refreshPeriod := findShortestPeriod(categories)

	assert.Equal(t, minRefreshPeriod, refreshPeriod)
}

func TestGetServiceNamesFromCategoriesDefaultCategory(t *testing.T) {
	categories := make(map[string]category)
	categories["default"] = category{}
	serviceNames := getServiceNamesFromCategories(categories)
	assert.Zero(t, len(serviceNames))
}

func TestGetServiceNamesFromCategoriesTwoategory(t *testing.T) {
	categories := make(map[string]category)
	categories["publishing"] = category{
		services: []string{"service1", "service2"},
	}
	categories["image-publish"] = category{
		services: []string{"service2", "service3"},
	}
	serviceNames := getServiceNamesFromCategories(categories)
	assert.Equal(t, 3, len(serviceNames))
}

func TestRunServiceChecksForStickyCategory(t *testing.T) {
	categories := make(map[string]category)
	categories["publishing"] = category{
		services: []string{"test-service-name"},
	}
	categories["test"] = category{
		services:  []string{"test-service-name"},
		isSticky:  true,
		isEnabled: true,
	}

	controller := initializeMockController("test", nil)
	hc, categories, _, _ := controller.buildServicesHealthResult([]string{"test", "publishing"}, false)

	assert.NotNil(t, hc)
	assert.False(t, categories["test"].isEnabled)
}

func TestRunServiceChecksForStickyCategoryUpdateError(t *testing.T) {
	categories := make(map[string]category)
	categories["publishing"] = category{
		services: []string{"test-service-name"},
	}

	categories[nonExistingCategoryName] = category{
		services:  []string{"test-service-name"},
		isSticky:  true,
		isEnabled: true,
	}
	categories["test"] = category{
		services:  []string{"test-service-name"},
		isSticky:  true,
		isEnabled: true,
	}

	controller := initializeMockController("test", nil)
	hc, categories, _, _ := controller.buildServicesHealthResult([]string{"test", "publishing", nonExistingCategoryName}, true)

	assert.NotNil(t, hc)
	assert.False(t, hc.Ok)
	assert.False(t, categories["test"].isEnabled)
}

func TestDisableStickyFailingCategoriesThresholdNotReached(t *testing.T) {
	categories := make(map[string]category)
	categories["publishing"] = category{
		services: []string{"service1", "service2"},
	}
	categories["test"] = category{
		services:  []string{"test-service-name"},
		isSticky:  true,
		isEnabled: true,
	}
	healthchecks := []fthealth.CheckResult{
		{
			ID: "service1",
			Ok: true,
		},
		{
			ID: "service2",
			Ok: true,
		},
		{
			ID: "test-service-name",
			Ok: false,
		},
	}

	controller := initializeMockController("test", nil)
	controller.disableStickyFailingCategories(categories, healthchecks)
	assert.True(t, categories["test"].isEnabled)
}

func TestDisableStickyFailingCategoriesThresholdReached(t *testing.T) {
	categories := make(map[string]category)
	categories["publishing"] = category{
		services:         []string{"service1", "service2"},
		name:             "service",
		isEnabled:        true,
		failureThreshold: 2,
	}
	categories["test"] = category{
		services:         []string{"test-service-name"},
		name:             "test-service-name",
		isSticky:         true,
		isEnabled:        true,
		failureThreshold: 2,
	}
	healthchecks := []fthealth.CheckResult{
		{
			ID:   "service1",
			Name: "service1",
			Ok:   true,
		},
		{
			ID:   "service2",
			Name: "service2",
			Ok:   true,
		},
		{
			ID:   "test-service-name",
			Name: "test-service-name",
			Ok:   false,
		},
	}

	controller := initializeMockController("test", nil)
	controller.disableStickyFailingCategories(categories, healthchecks)
	controller.disableStickyFailingCategories(categories, healthchecks)
	controller.disableStickyFailingCategories(categories, healthchecks)
	assert.False(t, categories["test"].isEnabled)
}

func TestGetMatchingCategoriesHappyFlow(t *testing.T) {
	categories := make(map[string]category)
	categories["publishing"] = category{
		name: "publishing",
	}

	matchingCategories := getMatchingCategories([]string{"publishing"}, categories)
	assert.NotNil(t, matchingCategories["publishing"])
}

func TestGetFinalResultCategoryDisabled(t *testing.T) {
	categories := make(map[string]category)
	categories["publishing"] = category{
		services: []string{"test-service-name"},
	}
	categories["test"] = category{
		services:  []string{"test-service-name"},
		isSticky:  true,
		isEnabled: false,
	}

	checkResults := []fthealth.CheckResult{
		{
			Ok:       false,
			Severity: 1,
		},
	}

	finalOk, _ := getFinalResult(checkResults, categories)

	assert.False(t, finalOk)
}

func TestGetFinalResultEmptyCheckResultsList(t *testing.T) {
	finalOk, finalSeverity := getFinalResult([]fthealth.CheckResult{}, map[string]category{})
	assert.False(t, finalOk)
	assert.Equal(t, defaultSeverity, finalSeverity)
}

func TestGetEnvironment(t *testing.T) {
	healthCheckController := &healthCheckController{
		environment: validEnvName,
	}

	env := healthCheckController.getEnvironment()

	assert.Equal(t, validEnvName, env)
}
