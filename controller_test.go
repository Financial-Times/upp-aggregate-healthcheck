package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Financial-Times/go-logger"

	"strconv"
	"strings"

	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
	"github.com/stretchr/testify/assert"
)

const (
	nonExistingServiceName  = "non-existing-service"
	serviceNameForAckErr    = "serviceNameWithAckError"
	invalidNameForService   = "invalidNameForService"
	nonExistingPodName      = "nonExistingPodName"
	podWithCriticalSeverity = "podWithCriticalSeverity"
	failingPod              = "failingPod"
	podWithBrokenService    = "podWithBrokenService"
	podWithBadAddress       = "podWithBadAddress"
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
	httpClient          *http.Client
	getServiceByNameErr error
	getDeploymentsErr   error
}

func (m *MockService) RLockServices() {}

func (m *MockService) RUnlockServices() {}

func (m *MockService) getCategories(_ context.Context) (map[string]category, error) {
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

func (m *MockService) updateCategory(_ context.Context, categoryName string, isEnabled bool) error {
	if categoryName == nonExistingCategoryName {
		return errors.New("Cannot find category")
	}

	return nil
}

func (m *MockService) getDeployments(_ context.Context) (map[string]deployment, error) {
	return map[string]deployment{
		"test-service-name": {
			desiredReplicas: 2,
		},
		"test-service-name-2": {
			desiredReplicas: 2,
		},
	}, m.getDeploymentsErr
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
	}, m.getServiceByNameErr
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

func (m *MockService) getPodsForService(_ context.Context, serviceName string) ([]pod, error) {
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

func (m *MockService) getPodByName(_ context.Context, podName string) (pod, error) {
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
	case podWithBadAddress:
		{
			return pod{
				name: "test-pod-name-8425234-9hdfg ",
				ip:   "[fe80::1%en0]",
			}, nil
		}
	default:
		return pod{
			name: "test-pod-name-8425234-9hdfg ",
			ip:   "10.2.51.2",
		}, nil
	}
}

func (m *MockService) checkServiceHealth(_ context.Context, service service, deployments map[string]deployment) (string, error) {
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

func (m *MockService) addAck(_ context.Context, serviceName string, ackMessage string) error {
	if serviceName == serviceNameForAckErr {
		return errors.New("Error")
	}
	return nil
}
func (m *MockService) removeAck(_ context.Context, serviceName string) error {
	if serviceName == serviceNameForAckErr {
		return errors.New("Cannot remove ack")
	}
	return nil
}
func (m *MockService) getHTTPClient() *http.Client {
	return m.httpClient
}

func initializeMockController(env string, httpClient *http.Client) (hcc *healthCheckController, service *MockService) {
	measuredServices := make(map[string]measuredService)
	service = new(MockService)
	service.httpClient = httpClient
	stickyCategoriesFailedServices := make(map[string]int)

	return &healthCheckController{
		healthCheckService:             service,
		environment:                    env,
		measuredServices:               measuredServices,
		stickyCategoriesFailedServices: stickyCategoriesFailedServices,
	}, service
}

func TestAddAckNilError(t *testing.T) {
	controller, _ := initializeMockController("test", nil)
	err := controller.addAck(context.TODO(), "abc", "abc")
	assert.Nil(t, err)
}

func TestAddAckInvalidServiceName(t *testing.T) {
	controller, _ := initializeMockController("test", nil)
	err := controller.addAck(context.TODO(), nonExistingServiceName, "abc")
	assert.NotNil(t, err)
}

func TestAddAckInvalidServiceNameWillAckingError(t *testing.T) {
	controller, _ := initializeMockController("test", nil)
	err := controller.addAck(context.TODO(), serviceNameForAckErr, "abc")
	assert.NotNil(t, err)
}

func TestRemoveAckNonExistingServiceErr(t *testing.T) {
	controller, _ := initializeMockController("test", nil)
	err := controller.removeAck(context.TODO(), nonExistingServiceName)
	assert.NotNil(t, err)
}

func TestRemoveAckServiceErr(t *testing.T) {
	controller, _ := initializeMockController("test", nil)
	err := controller.removeAck(context.TODO(), serviceNameForAckErr)
	assert.NotNil(t, err)
}

func TestRemoveAckHappyFlow(t *testing.T) {
	controller, _ := initializeMockController("test", nil)
	err := controller.removeAck(context.TODO(), validService)
	assert.Nil(t, err)
}

func TestBuildServicesHealthResult(t *testing.T) {
	controller, _ := initializeMockController("test", nil)
	_, _, err := controller.buildServicesHealthResult(context.TODO(), []string{"abc"}, false)
	assert.Nil(t, err)
}

func TestBuildServicesHealthResult_RunServiceChecksForFails(t *testing.T) {
	controller, m := initializeMockController("test", nil)
	m.getDeploymentsErr = errors.New("someerror")
	_, _, err := controller.buildServicesHealthResult(context.TODO(), []string{"abc"}, false)
	assert.Error(t, err)
}

func TestBuildServicesHealthResultFromCache(t *testing.T) {
	controller, _ := initializeMockController("test", nil)
	_, _, err := controller.buildServicesHealthResult(context.TODO(), []string{"abc"}, true)
	assert.Nil(t, err)
}

func TestGetIndividualPodHealthHappyFlowWithGetServiceByNameErr(t *testing.T) {
	httpClient := initializeMockHTTPClient(http.StatusOK, "")
	controller, m := initializeMockController("test", httpClient)
	m.getServiceByNameErr = errors.New("error is ignored")
	_, _, err := controller.getIndividualPodHealth(context.TODO(), "testPod")
	assert.NoError(t, err)
}

func TestGetIndividualPodHealthHappyFlow(t *testing.T) {
	httpClient := initializeMockHTTPClient(http.StatusOK, "")
	controller, _ := initializeMockController("test", httpClient)
	_, _, err := controller.getIndividualPodHealth(context.TODO(), "testPod")
	assert.Nil(t, err)
}

func TestGetIndividualPodHealthNonExistingPod(t *testing.T) {
	controller, _ := initializeMockController("test", nil)
	_, _, err := controller.getIndividualPodHealth(context.TODO(), nonExistingPodName)
	assert.NotNil(t, err)
}

func TestGetIndividualPodHealthBadUrl(t *testing.T) {
	controller, _ := initializeMockController("test", nil)
	_, _, err := controller.getIndividualPodHealth(context.TODO(), podWithBadAddress)
	assert.NotNil(t, err)
}

func TestGetIndividualPodHealthFailingService(t *testing.T) {
	httpClient := initializeMockHTTPClient(http.StatusOK, "")
	controller, _ := initializeMockController("test", httpClient)
	_, _, err := controller.getIndividualPodHealth(context.TODO(), podWithBrokenService)
	assert.Nil(t, err)
}

func TestBuildPodsHealthResultHappyFlow(t *testing.T) {
	controller, _ := initializeMockController("test", nil)
	_, err := controller.buildPodsHealthResult(context.TODO(), "testPod")
	assert.Nil(t, err)
}

func TestBuildPodsHealthResultInvalidPodName(t *testing.T) {
	controller, _ := initializeMockController("test", nil)
	_, err := controller.buildPodsHealthResult(context.TODO(), invalidNameForService)
	assert.NotNil(t, err)
}

func TestBuildPodsHealthResultInvalidServiceName(t *testing.T) {
	controller, _ := initializeMockController("test", nil)
	_, err := controller.buildPodsHealthResult(context.TODO(), nonExistingServiceName)
	assert.NotNil(t, err)
}

func TestGetSeverityForPodInvalidPodName(t *testing.T) {
	controller, _ := initializeMockController("test", nil)
	severity := controller.getSeverityForPod(context.TODO(), nonExistingPodName, 8080)
	assert.Equal(t, defaultSeverity, severity)
}

func TestComputeSeverityByPods(t *testing.T) {
	controller, _ := initializeMockController("test", nil)
	severity := controller.computeSeverityByPods([]pod{{name: nonExistingPodName}}, 8080)
	assert.Equal(t, defaultSeverity, severity)
}

func TestComputeSeverityForPodWithCriticalSeverity(t *testing.T) {
	controller, _ := initializeMockController("test", nil)
	severity := controller.computeSeverityByPods([]pod{{name: failingPod}, {name: podWithCriticalSeverity}}, 8080)
	assert.Equal(t, uint8(1), severity)
}

func TestGetSeverityForServiceInvalidServiceName(t *testing.T) {
	controller, _ := initializeMockController("test", nil)
	severity := controller.getSeverityForService(context.TODO(), invalidNameForService, 8080)
	assert.Equal(t, defaultSeverity, severity)
}

func TestGetSeverityForResilientService(t *testing.T) {
	controller, _ := initializeMockController("test", nil)

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
		actualSeverity := controller.getSeverityForService(context.TODO(), tc.serviceName, 8080)
		assert.Equal(t, tc.expectedSeverity, actualSeverity, tc.description)
	}

}

func TestGetSeverityForNonResilientService(t *testing.T) {
	controller, _ := initializeMockController("test", nil)
	severity := controller.getSeverityForService(context.TODO(), invalidNameForService, 8080)
	assert.Equal(t, defaultSeverity, severity)
}

func TestUpdateStickyCategoryInvalidCategoryName(t *testing.T) {
	controller, _ := initializeMockController("test", nil)
	err := controller.updateStickyCategory(context.TODO(), nonExistingCategoryName, false)
	assert.NotNil(t, err)
}

func TestUpdateStickyCategoryHappyFlow(t *testing.T) {
	controller, _ := initializeMockController("test", nil)
	err := controller.updateStickyCategory(context.TODO(), validCat, false)
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

	controller, _ := initializeMockController("test", nil)
	hc, categories, _ := controller.buildServicesHealthResult(context.TODO(), []string{"test", "publishing"}, false)

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

	controller, _ := initializeMockController("test", nil)
	hc, categories, _ := controller.buildServicesHealthResult(context.TODO(), []string{"test", "publishing", nonExistingCategoryName}, true)

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

	controller, _ := initializeMockController("test", nil)
	controller.disableStickyFailingCategories(context.TODO(), categories, healthchecks)
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

	controller, _ := initializeMockController("test", nil)
	controller.disableStickyFailingCategories(context.TODO(), categories, healthchecks)
	controller.disableStickyFailingCategories(context.TODO(), categories, healthchecks)
	controller.disableStickyFailingCategories(context.TODO(), categories, healthchecks)
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
