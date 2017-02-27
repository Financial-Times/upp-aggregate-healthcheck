package main

import (
	"time"
	"testing"
	"net/http"
	"net/http/httptest"
	fthealth "github.com/Financial-Times/go-fthealth/v1a"
	"k8s.io/kubernetes/staging/src/k8s.io/client-go/_vendor/github.com/stretchr/testify/assert"
	"os"
	"fmt"
)

type mockController struct {

}

const (
	invalidCategoryName = "invalid"
	disabledCategoryName = "disabled"
	validPodName = "validPod"
)

func (m *mockController) buildServicesHealthResult(providedCategories []string, useCache bool) (fthealth.HealthResult, map[string]category, map[string]category, error) {
	matchingCategories := map[string]category{}

	if providedCategories[0] != invalidCategoryName {
		matchingCategories["default"] = category{
			name:"default",
			isEnabled:true,
		}
	}
	if providedCategories[0] == disabledCategoryName {
		matchingCategories["default"] = category{
			name:"default",
			isEnabled:false,
		}
	}

	health := fthealth.HealthResult{
		Checks:        []fthealth.CheckResult{},
		Description:   "test",
		Name:          "cluster health",
		SchemaVersion: 1,
		Ok:            true,
		Severity:      1,
	}

	return health, matchingCategories, map[string]category{}, nil
}

func (m *mockController)runServiceChecksByServiceNames([]service, map[string]category) []fthealth.CheckResult {
	return []fthealth.CheckResult{}
}

func (m *mockController)runServiceChecksFor(map[string]category) ([]fthealth.CheckResult, map[string][]fthealth.CheckResult) {
	return []fthealth.CheckResult{}, map[string][]fthealth.CheckResult{}
}

func (m *mockController)buildPodsHealthResult(string, bool) (fthealth.HealthResult, error) {
	return fthealth.HealthResult{}, nil
}

func (m *mockController)runPodChecksFor(string) ([]fthealth.CheckResult, error) {
	return []fthealth.CheckResult{}, nil
}

func (m *mockController)collectChecksFromCachesFor(map[string]category) ([]fthealth.CheckResult, map[string][]fthealth.CheckResult) {
	return []fthealth.CheckResult{}, map[string][]fthealth.CheckResult{}
}

func (m *mockController)updateCachedHealth([]service, map[string]category) {

}

func (m *mockController)scheduleCheck(measuredService, time.Duration, *time.Timer) {

}

func (m *mockController)getIndividualPodHealth(string) ([]byte, string, error) {
	return []byte("test pod health"), "", nil
}

func (m *mockController)addAck(string, string) error {
	return nil
}

func (m *mockController)updateStickyCategory(string, bool) error {
	return nil
}

func (m *mockController)removeAck(string) error {
	return nil
}

func (m *mockController)getEnvironment() string {
	return ""
}

func (m *mockController)getSeverityForService(string, int32) uint8 {
	return 1
}

func (m *mockController)getSeverityForPod(string, int32) uint8 {
	return 1
}

func initializeTestHandler() *httpHandler {
	mockController := new(mockController)
	return &httpHandler{
		pathPrefix: "",
		controller:mockController,
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

