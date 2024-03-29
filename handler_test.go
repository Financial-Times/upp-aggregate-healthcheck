package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/Financial-Times/go-logger"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
	"github.com/stretchr/testify/assert"
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

func init() {
	logger.InitLogger("upp-aggregate-healthcheck", "debug")
}

func (m *mockController) buildServicesHealthResult(_ context.Context, providedCategories []string, useCache bool) (fthealth.HealthResult, map[string]category, error) {
	if len(providedCategories) == 1 && providedCategories[0] == brokenCategoryName {
		return fthealth.HealthResult{}, map[string]category{}, errors.New("Broken category")
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

	return health, matchingCategories, nil
}

func (m *mockController) getMeasuredServices() map[string]measuredService {
	return map[string]measuredService{}
}

func (m *mockController) runServiceChecksByServiceNames(context.Context, map[string]service, map[string]category) ([]fthealth.CheckResult, error) {
	return []fthealth.CheckResult{}, nil
}

func (m *mockController) runServiceChecksFor(context.Context, map[string]category) ([]fthealth.CheckResult, error) {
	return []fthealth.CheckResult{}, nil
}

func (m *mockController) buildPodsHealthResult(_ context.Context, serviceName string) (fthealth.HealthResult, error) {
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

func (m *mockController) runPodChecksFor(context.Context, string) ([]fthealth.CheckResult, error) {
	return []fthealth.CheckResult{}, nil
}

func (m *mockController) collectChecksFromCachesFor(context.Context, map[string]category) ([]fthealth.CheckResult, error) {
	return []fthealth.CheckResult{}, nil
}

func (m *mockController) updateCachedHealth(context.Context, map[string]service, map[string]category) {

}

func (m *mockController) scheduleCheck(measuredService, time.Duration, *time.Timer) {

}

func (m *mockController) getIndividualPodHealth(_ context.Context, podName string) ([]byte, string, error) {
	if podName == brokenPodName {
		return []byte{}, "", errors.New("Broken pod")
	}
	return []byte("test pod health"), "", nil
}

func (m *mockController) addAck(_ context.Context, serviceName string, message string) error {
	if serviceName == brokenServiceName {
		return errors.New("Broken service")
	}

	return nil
}

func (m *mockController) updateStickyCategory(_ context.Context, categoryName string, isEnabled bool) error {
	if categoryName == brokenCategoryName {
		return errors.New("Broken category")
	}

	return nil
}

func (m *mockController) removeAck(_ context.Context, serviceName string) error {
	if serviceName == brokenServiceName {
		return errors.New("Broken service")
	}

	return nil
}

func (m *mockController) getEnvironment() string {
	return ""
}

func (m *mockController) getSeverityForService(context.Context, string, int32) uint8 {
	return 1
}

func (m *mockController) getSeverityForPod(context.Context, string, int32) uint8 {
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
