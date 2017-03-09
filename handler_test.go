package main

import (
	"fmt"
	fthealth "github.com/Financial-Times/go-fthealth/v1a"
	"github.com/golang/go/src/pkg/errors"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

type mockController struct {
}

const (
	invalidCategoryName  = "invalid"
	disabledCategoryName = "disabled"
	categoryWithChecks   = "catWithChecks"
	brokenCategoryName   = "brokencat"
	brokenServiceName    = "brokenServiceName"
	validPodName         = "validPod"
	validServiceName     = "validServiceName"
	brokenPodName        = "brokenPod"
)

func (m *mockController) buildServicesHealthResult(providedCategories []string, useCache bool) (fthealth.HealthResult, map[string]category, map[string]category, error) {
	if len(providedCategories) == 1 && providedCategories[0] == brokenCategoryName {
		return fthealth.HealthResult{}, map[string]category{}, map[string]category{}, errors.New("Broken category")
	}

	matchingCategories := map[string]category{}

	if providedCategories[0] != invalidCategoryName {
		matchingCategories["default"] = category{
			name:      "default",
			isEnabled: true,
		}
	}
	if providedCategories[0] == disabledCategoryName {
		matchingCategories["default"] = category{
			name:      "default",
			isEnabled: false,
		}
	}

	var checks []fthealth.CheckResult
	finalOk := true
	if len(providedCategories) == 1 && providedCategories[0] == categoryWithChecks {
		checks = []fthealth.CheckResult{
			{
				Ok: true,
			},
			{
				Ok: false,
			},
		}

		finalOk = false
	}

	health := fthealth.HealthResult{
		Checks:        checks,
		Description:   "test",
		Name:          "cluster health",
		SchemaVersion: 1,
		Ok:            finalOk,
		Severity:      1,
	}

	return health, matchingCategories, map[string]category{}, nil
}

func (m *mockController) runServiceChecksByServiceNames([]service, map[string]category) []fthealth.CheckResult {
	return []fthealth.CheckResult{}
}

func (m *mockController) runServiceChecksFor(map[string]category) ([]fthealth.CheckResult, map[string][]fthealth.CheckResult) {
	return []fthealth.CheckResult{}, map[string][]fthealth.CheckResult{}
}

func (m *mockController) buildPodsHealthResult(serviceName string) (fthealth.HealthResult, error) {
	if serviceName == brokenServiceName {
		return fthealth.HealthResult{}, errors.New("Broken pod")
	}

	if serviceName == validServiceName {
		checks := []fthealth.CheckResult{
			{
				Ok: true,
			},
			{
				Ok: false,
			},
		}

		return fthealth.HealthResult{
			Checks:        checks,
			Description:   "test",
			Name:          "cluster health",
			SchemaVersion: 1,
			Ok:            true,
			Severity:      1,
		}, nil
	}

	return fthealth.HealthResult{}, nil
}

func (m *mockController) runPodChecksFor(string) ([]fthealth.CheckResult, error) {
	return []fthealth.CheckResult{}, nil
}

func (m *mockController) collectChecksFromCachesFor(map[string]category) ([]fthealth.CheckResult, map[string][]fthealth.CheckResult) {
	return []fthealth.CheckResult{}, map[string][]fthealth.CheckResult{}
}

func (m *mockController) updateCachedHealth([]service, map[string]category) {

}

func (m *mockController) scheduleCheck(measuredService, time.Duration, *time.Timer) {

}

func (m *mockController) getIndividualPodHealth(podName string) ([]byte, string, error) {
	if podName == brokenPodName {
		return []byte{}, "", errors.New("Broken pod")
	}
	return []byte("test pod health"), "", nil
}

func (m *mockController) addAck(serviceName string, message string) error {
	if serviceName == brokenServiceName {
		return errors.New("Broken service")
	}

	return nil
}

func (m *mockController) updateStickyCategory(categoryName string, isEnabled bool) error {
	if categoryName == brokenCategoryName {
		return errors.New("Broken category")
	}

	return nil
}

func (m *mockController) removeAck(serviceName string) error {
	if serviceName == brokenServiceName {
		return errors.New("Broken service")
	}

	return nil
}

func (m *mockController) getEnvironment() string {
	return ""
}

func (m *mockController) getSeverityForService(string, int32) uint8 {
	return 1
}

func (m *mockController) getSeverityForPod(string, int32) uint8 {
	return 1
}

func initializeTestHandler() *httpHandler {
	mockController := new(mockController)
	return &httpHandler{
		pathPrefix: "",
		controller: mockController,
	}
}

func TestRemoveAckWithEmptyServiceName(t *testing.T) {
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handleRemoveAck)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusBadRequest, respRecorder.Code)
}

func TestRemoveAckWithNonEmptyServiceName(t *testing.T) {
	initLogs(os.Stdout, os.Stdout, os.Stderr)
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", "/rem-ack?service-name=testservice", nil)
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handleRemoveAck)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusMovedPermanently, respRecorder.Code)
}

func TestRemoveAckWithInternalError(t *testing.T) {
	initLogs(os.Stdout, os.Stdout, os.Stderr)
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", fmt.Sprintf("/rem-ack?service-name=%s", brokenServiceName), nil)
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handleRemoveAck)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusInternalServerError, respRecorder.Code)
}

func TestAddAckWithEmptyServiceName(t *testing.T) {
	initLogs(os.Stdout, os.Stdout, os.Stderr)
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handleAddAck)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusBadRequest, respRecorder.Code)
}

func TestAddAckWithNonEmptyServiceName(t *testing.T) {
	initLogs(os.Stdout, os.Stdout, os.Stderr)
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", "/add-ack?service-name=testservice", nil)
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handleAddAck)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusMovedPermanently, respRecorder.Code)
}

func TestAddAckWithBrokenService(t *testing.T) {
	initLogs(os.Stdout, os.Stdout, os.Stderr)
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", fmt.Sprintf("/add-ack?service-name=%s", brokenServiceName), nil)
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handleAddAck)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusInternalServerError, respRecorder.Code)
}

func TestAddAckFromWithNonEmptyServiceName(t *testing.T) {
	initLogs(os.Stdout, os.Stdout, os.Stderr)
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", "/add-ack?service-name=testservice", nil)
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handleAddAckForm)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusOK, respRecorder.Code)
}

func TestAddAckFromWithEmptyServiceName(t *testing.T) {
	initLogs(os.Stdout, os.Stdout, os.Stderr)
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handleAddAckForm)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusBadRequest, respRecorder.Code)
}

func TestDisableCategoryWithEmptyCategoryName(t *testing.T) {
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handleDisableCategory)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusBadRequest, respRecorder.Code)
}

func TestDisableCategoryWithIntrnalError(t *testing.T) {
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", fmt.Sprintf("disable-category?category-name=%s", brokenCategoryName), nil)
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handleDisableCategory)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusInternalServerError, respRecorder.Code)
}

func TestDisableCategoryWithValidCategoryName(t *testing.T) {
	initLogs(os.Stdout, os.Stdout, os.Stderr)
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", "disable-category?category-name=testcat", nil)
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handleDisableCategory)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusOK, respRecorder.Code)
}

func TestEnableCategoryWithValidCategoryName(t *testing.T) {
	initLogs(os.Stdout, os.Stdout, os.Stderr)
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", "enable-category?category-name=testcat", nil)
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handleEnableCategory)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusOK, respRecorder.Code)
}

func TestGoodToGoInvalidCategory(t *testing.T) {
	initLogs(os.Stdout, os.Stdout, os.Stderr)
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", fmt.Sprintf("health?categories=%s", invalidCategoryName), nil)
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handleGoodToGo)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusBadRequest, respRecorder.Code)
}

func TestGoodToGoDefaultCategory(t *testing.T) {
	initLogs(os.Stdout, os.Stdout, os.Stderr)
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", "health", nil)
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handleGoodToGo)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusOK, respRecorder.Code)
}

func TestGoodToGoDisabledCategory(t *testing.T) {
	initLogs(os.Stdout, os.Stdout, os.Stderr)
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", fmt.Sprintf("health?categories=%s", disabledCategoryName), nil)
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handleGoodToGo)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusServiceUnavailable, respRecorder.Code)
}

func TestGoodToGoBrokenCategory(t *testing.T) {
	initLogs(os.Stdout, os.Stdout, os.Stderr)
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", fmt.Sprintf("health?categories=%s", brokenCategoryName), nil)
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handleGoodToGo)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusServiceUnavailable, respRecorder.Code)
}

func TestGoodToGoWithFailingCheck(t *testing.T) {
	initLogs(os.Stdout, os.Stdout, os.Stderr)
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", fmt.Sprintf("health?categories=%s", categoryWithChecks), nil)
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handleGoodToGo)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusServiceUnavailable, respRecorder.Code)
}

func TestIndividualPodCheckEmptyPodName(t *testing.T) {
	initLogs(os.Stdout, os.Stdout, os.Stderr)
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handleIndividualPodHealthCheck)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusBadRequest, respRecorder.Code)
}

func TestIndividualPodCheckValidPodName(t *testing.T) {
	initLogs(os.Stdout, os.Stdout, os.Stderr)
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", fmt.Sprintf("pod?pod-name=%s", validPodName), nil)
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handleIndividualPodHealthCheck)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusOK, respRecorder.Code)
}

func TestIndividualPodCheckBrokenPod(t *testing.T) {
	initLogs(os.Stdout, os.Stdout, os.Stderr)
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", fmt.Sprintf("pod?pod-name=%s", brokenPodName), nil)
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handleIndividualPodHealthCheck)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusInternalServerError, respRecorder.Code)
}

func TestServiceHealthCheckInvalidCategory(t *testing.T) {
	initLogs(os.Stdout, os.Stdout, os.Stderr)
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", fmt.Sprintf("health?categories=%s", invalidCategoryName), nil)
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handleServicesHealthCheck)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusBadRequest, respRecorder.Code)
}

func TestServiceHealthCheckInternalError(t *testing.T) {
	initLogs(os.Stdout, os.Stdout, os.Stderr)
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", fmt.Sprintf("health?categories=%s", brokenCategoryName), nil)
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handleServicesHealthCheck)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusInternalServerError, respRecorder.Code)
}

func TestServiceHealthCheckDefaultCategory(t *testing.T) {
	initLogs(os.Stdout, os.Stdout, os.Stderr)
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", "", nil)
	req.Header.Add("Accept", "application/json")
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handleServicesHealthCheck)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusOK, respRecorder.Code)
}

func TestServiceHealthCheckDefaultCategoryHtmlResponse(t *testing.T) {
	initLogs(os.Stdout, os.Stdout, os.Stderr)
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handleServicesHealthCheck)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusOK, respRecorder.Code)
}

func TestPodsHealthCheckEmptyServiceName(t *testing.T) {
	initLogs(os.Stdout, os.Stdout, os.Stderr)
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handlePodsHealthCheck)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusBadRequest, respRecorder.Code)
}

func TestPodsHealthCheckBrokenService(t *testing.T) {
	initLogs(os.Stdout, os.Stdout, os.Stderr)
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", fmt.Sprintf("pods-health?service-name=%s", brokenServiceName), nil)
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handlePodsHealthCheck)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusInternalServerError, respRecorder.Code)
}

func TestPodsHealthCheckHappyFlow(t *testing.T) {
	initLogs(os.Stdout, os.Stdout, os.Stderr)
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", fmt.Sprintf("pods-health?service-name=%s", validServiceName), nil)
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handlePodsHealthCheck)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusOK, respRecorder.Code)
}

func TestPodsHealthCheckHappyFlowJson(t *testing.T) {
	initLogs(os.Stdout, os.Stdout, os.Stderr)
	aggHealthCheckcHandler := initializeTestHandler()
	req, err := http.NewRequest("GET", fmt.Sprintf("pods-health?service-name=%s", validServiceName), nil)
	req.Header.Add("Accept", "application/json")
	if err != nil {
		t.Fatal(err)
	}
	respRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(aggHealthCheckcHandler.handlePodsHealthCheck)
	handler.ServeHTTP(respRecorder, req)
	assert.Equal(t, http.StatusOK, respRecorder.Code)
}
