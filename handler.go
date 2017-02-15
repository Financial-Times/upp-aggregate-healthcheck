package main

import (
	"net/http"
	"encoding/json"
	"net/url"
	"strings"
	fthealth "github.com/Financial-Times/go-fthealth/v1a"
	"html/template"
	"fmt"
)

type httpHandler struct {
	controller controller
}

type IndividualHealthcheckParams struct {
	Name          string
	Status        string
	AvailablePods string
	LastUpdated   string
	MoreInfoPath  string
	AckMessage    string
}

type AggregateHealthcheckParams struct {
	PageTitle               string
	GeneralStatus           string
	RefreshFromCachePath    string
	RefreshWithoutCachePath string
	AckCount                int
	IndividualHealthChecks  []IndividualHealthcheckParams
}

type AddAckForm struct {
	ServiceName string
	AddAckPath  string
}

var defaultCategories = []string{"default"}

const (
	timeLayout = "15:04:05 MST"
	healthcheckTemplateName = "healthcheck-template.html"
	healthcheckPath = "/__health"
)

func (h *httpHandler) handleEnableCategory(w http.ResponseWriter, r *http.Request) {
	categoryName := r.URL.Query().Get("category-name")
	if categoryName == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Provided category name is not valid."))
		return
	}

	err := h.controller.enableStickyCategory(categoryName)

	if categoryName == "" {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Failed to enable category."))
		errorLogger.Printf("Failed to enable category with name %s. Error was: %s", categoryName, err.Error())
		return
	}
}

func (h *httpHandler) handleRemoveAck(w http.ResponseWriter, r *http.Request) {
	serviceName := getServiceNameFromUrl(r.URL)
	if serviceName == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Provided service name is not valid."))
		return
	}

	err := h.controller.removeAck(serviceName)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		errorLogger.Printf("Cannot remove acknowledge for service with name %s. Error was: %s", serviceName, err.Error())
		return
	}

	http.Redirect(w, r, "__health?cache=false", http.StatusMovedPermanently)
}

func (h *httpHandler) handleAddAck(w http.ResponseWriter, r *http.Request) {
	serviceName := getServiceNameFromUrl(r.URL)
	ackMessage := r.PostFormValue("ack-msg")
	if serviceName == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Provided service name is not valid."))
		return
	}

	err := h.controller.addAck(serviceName, ackMessage)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		errorLogger.Printf("Cannot add acknowledge for service with name %s. Error was: %s", serviceName, err.Error())
	}

	http.Redirect(w, r, "__health?cache=false", http.StatusMovedPermanently)
}

func (h *httpHandler) handleAddAckForm(w http.ResponseWriter, r *http.Request) {
	serviceName := getServiceNameFromUrl(r.URL)

	if serviceName == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Provided service name is not valid."))
		return
	}

	w.Header().Add("Content-Type", "text/html")
	htmlTemplate := parseHtmlTemplate(w, "add-ack-message-form-template.html")
	if htmlTemplate == nil {
		return
	}

	addAckForm := AddAckForm{
		ServiceName:serviceName,
		AddAckPath:fmt.Sprintf("add-ack?service-name=%s", serviceName),
	}

	if err := htmlTemplate.Execute(w, addAckForm); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		errorLogger.Printf("Cannot apply params to html template, error was: %v", err.Error())
		w.Write([]byte("Couldn't render template file for html response"))
		return
	}
}

func (h *httpHandler) handleServicesHealthCheck(w http.ResponseWriter, r *http.Request) {
	categories := parseCategories(r.URL)
	healthResult, validCategories, _, err := h.controller.buildServicesHealthResult(categories, useCache(r.URL))

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		errorLogger.Printf("Cannot build services health result, error was: %v", err.Error())
		return
	}

	if len(validCategories) == 0 {
		w.WriteHeader(http.StatusBadRequest)

		if r.Header.Get("Accept") != "application/json" {
			w.Write([]byte("Provided categories are not valid."))
		}
		return
	}

	if r.Header.Get("Accept") == "application/json" {
		buildHealthcheckJsonResponse(w, healthResult)
	} else {
		env := h.controller.getEnvironment()
		buildServicesCheckHtmlResponse(w, healthResult, env, getCategoriesString(validCategories))
	}
}

func (h *httpHandler) handlePodsHealthCheck(w http.ResponseWriter, r *http.Request) {
	serviceName := getServiceNameFromUrl(r.URL)

	if (serviceName == "") {
		w.WriteHeader(http.StatusBadRequest)

		if r.Header.Get("Accept") != "application/json" {
			w.Write([]byte("Couldn't get service name from url."))
		}
		return
	}

	healthResult := h.controller.buildPodsHealthResult(serviceName, useCache(r.URL))

	if r.Header.Get("Accept") == "application/json" {
		buildHealthcheckJsonResponse(w, healthResult)
	} else {
		env := h.controller.getEnvironment()
		buildPodsCheckHtmlResponse(w, healthResult, env, serviceName)
	}
}

func (h *httpHandler) handleIndividualPodHealthCheck(w http.ResponseWriter, r *http.Request) {
	podName := getPodNameFromUrl(r.URL)

	if podName == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Cannot parse pod name from url."))
		return
	}

	podHealth, err := h.controller.getIndividualPodHealth(podName)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		errorLogger.Printf("Cannot get individual healthcheck for pod %s, error was: %v", podName, err.Error())
		w.Write([]byte(fmt.Sprintf("Cannot get individual healthcheck for pod %s", podName)))
		return
	}

	w.Write(podHealth)
}

func (h *httpHandler) handleGoodToGo(w http.ResponseWriter, r *http.Request) {
	categories := parseCategories(r.URL)
	healthResults, validCategories, _, err := h.controller.buildServicesHealthResult(categories, useCache(r.URL))

	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	if len(validCategories) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	for _, validCategory := range validCategories {
		if validCategory.isEnabled == false {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
	}

	if !healthResults.Ok {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
}

func parseCategories(theURL *url.URL) []string {
	queriedCategories := theURL.Query().Get("categories")
	if queriedCategories == "" {
		return defaultCategories
	}

	return strings.Split(queriedCategories, ",")
}

func getServiceNameFromUrl(url *url.URL) string {
	return url.Query().Get("service-name")
}

func getPodNameFromUrl(url *url.URL) string {
	return url.Query().Get("pod-name")
}

func useCache(theURL *url.URL) bool {
	//use cache by default
	return theURL.Query().Get("cache") != "false"
}

func buildHealthcheckJsonResponse(w http.ResponseWriter, healthResult fthealth.HealthResult) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	err := enc.Encode(healthResult)
	if err != nil {
		panic("Couldn't encode health results to ResponseWriter.")
	}
}

func buildServicesCheckHtmlResponse(w http.ResponseWriter, healthResult fthealth.HealthResult, environment string, categories string) {
	w.Header().Add("Content-Type", "text/html")
	htmlTemplate := parseHtmlTemplate(w, healthcheckTemplateName)
	if htmlTemplate == nil {
		return
	}

	aggregateHealthcheckParams := populateAggregateServiceChecks(healthResult, environment, categories)

	if err := htmlTemplate.Execute(w, aggregateHealthcheckParams); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		errorLogger.Printf("Cannot apply params to html template, error was: %v", err.Error())
		w.Write([]byte("Couldn't render template file for html response"))
		return
	}
}

func buildPodsCheckHtmlResponse(w http.ResponseWriter, healthResult fthealth.HealthResult, environment string, serviceName string) {
	w.Header().Add("Content-Type", "text/html")
	htmlTemplate := parseHtmlTemplate(w, healthcheckTemplateName)
	if htmlTemplate == nil {
		return
	}

	aggregateHealthcheckParams := populateAggregatePodChecks(healthResult, environment, serviceName)

	if err := htmlTemplate.Execute(w, aggregateHealthcheckParams); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		errorLogger.Printf("Cannot apply params to html template, error was: %v", err.Error())
		w.Write([]byte("Couldn't render template file for html response"))
		return
	}
}

func parseHtmlTemplate(w http.ResponseWriter, templateName string) *template.Template {
	htmlTemplate, err := template.ParseFiles(templateName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Couldn't open template file for html response"))
		errorLogger.Printf("Could not parse html template with name %s, error was: %v", templateName, err.Error())

		return nil
	}

	return htmlTemplate
}

func populateAggregateServiceChecks(healthResult fthealth.HealthResult, environment string, categories string) *AggregateHealthcheckParams {
	indiviualServiceChecks, ackCount := populateIndividualServiceChecks(healthResult.Checks)
	aggregateChecks := &AggregateHealthcheckParams{
		PageTitle: buildPageTitle(environment, categories),
		GeneralStatus: getGeneralStatus(healthResult),
		RefreshFromCachePath: buildRefreshFromCachePath(categories),
		RefreshWithoutCachePath: buildRefreshWithoutCachePath(categories),
		AckCount:ackCount,
		IndividualHealthChecks: indiviualServiceChecks,
	}

	return aggregateChecks
}

func buildRefreshFromCachePath(categories string) string {
	if categories != "" {
		return fmt.Sprintf("%s?categories=%s", healthcheckPath, categories)
	}

	return healthcheckPath
}

func buildRefreshWithoutCachePath(categories string) string {
	refreshWithoutCachePath := fmt.Sprintf("%s?cache=false", healthcheckPath)
	if categories != "" {
		return fmt.Sprintf("%s&categories=%s", refreshWithoutCachePath, categories)
	}

	return refreshWithoutCachePath
}

func populateIndividualServiceChecks(checks []fthealth.CheckResult) ([]IndividualHealthcheckParams, int) {
	var indiviualServiceChecks []IndividualHealthcheckParams
	ackCount := 0
	for _, individualCheck := range checks {
		if individualCheck.Ack != "" {
			ackCount++
		}

		hc := IndividualHealthcheckParams{
			Name: individualCheck.Name,
			Status: getServiceStatusFromCheck(individualCheck),
			AvailablePods: "3/3",
			LastUpdated: individualCheck.LastUpdated.Format(timeLayout),
			MoreInfoPath: fmt.Sprintf("/__pods-health?service-name=%s", individualCheck.Name),
			AckMessage: individualCheck.Ack,
		}

		indiviualServiceChecks = append(indiviualServiceChecks, hc)
	}

	return indiviualServiceChecks, ackCount
}

func populateIndividualPodChecks(checks []fthealth.CheckResult) []IndividualHealthcheckParams {
	var indiviualServiceChecks []IndividualHealthcheckParams

	for _, check := range checks {
		hc := IndividualHealthcheckParams{
			Name: check.Name,
			Status: getStatusFromCheck(check),
			LastUpdated: check.LastUpdated.Format(timeLayout),
			MoreInfoPath: fmt.Sprintf("/__pod-individual-health?pod-name=%s", check.Name),
			AckMessage: check.Ack,
		}

		indiviualServiceChecks = append(indiviualServiceChecks, hc)
	}

	return indiviualServiceChecks
}

func populateAggregatePodChecks(healthResult  fthealth.HealthResult, environment string, serviceName string) *AggregateHealthcheckParams {
	aggregateChecks := &AggregateHealthcheckParams{
		PageTitle: fmt.Sprintf("CoCo %s pods of service %s", environment, serviceName),
		GeneralStatus: getGeneralStatus(healthResult),
		RefreshFromCachePath: fmt.Sprintf("/__pods-health?service-name=%s", serviceName),
		RefreshWithoutCachePath:  fmt.Sprintf("/__pods-health?cache=false&service-name=%s", serviceName),
		IndividualHealthChecks: populateIndividualPodChecks(healthResult.Checks),
	}

	return aggregateChecks
}

func buildPageTitle(environment string, categories string) string {
	return fmt.Sprintf("CoCo %s cluster's services from categories %s", environment, categories)
}

func getServiceStatusFromCheck(check fthealth.CheckResult) string {
	status := getStatusFromCheck(check)
	if check.Ack != "" {
		return status + " acked"
	}

	return status
}

func getStatusFromCheck(check fthealth.CheckResult) string {
	if check.Ok {
		return "ok"
	}

	if check.Severity == 2 {
		return "warning"
	}

	return "critical"
}

func getGeneralStatus(healthResult fthealth.HealthResult) string {
	if healthResult.Ok {
		return "healthy"
	}

	if healthResult.Severity == 2 {
		return "unhealthy"
	}

	return "critical"
}

func getCategoriesString(categories  map[string]category) string {
	formattedCategoryNames := ""
	for categoryName := range categories {
		formattedCategoryNames += categoryName + ","
	}

	if len(formattedCategoryNames) > 0 {
		formattedCategoryNames = formattedCategoryNames[:len(formattedCategoryNames) - 1]
	}

	return formattedCategoryNames
}
