package main

import (
	"encoding/json"
	"fmt"
	fthealth "github.com/Financial-Times/go-fthealth/v1a"
	"html/template"
	"net/http"
	"net/url"
	"strings"
)

type httpHandler struct {
	controller controller
}

//IndividualHealthcheckParams struct used to populate HTML template with individual checks
type IndividualHealthcheckParams struct {
	Name          string
	Status        string
	AvailablePods string
	LastUpdated   string
	MoreInfoPath  string
	AckMessage    string
	Output        string
}

//AggregateHealthcheckParams struct used to populate HTML template with aggregate checks
type AggregateHealthcheckParams struct {
	PageTitle               string
	GeneralStatus           string
	RefreshFromCachePath    string
	RefreshWithoutCachePath string
	AckCount                int
	IndividualHealthChecks  []IndividualHealthcheckParams
}

//AddAckForm struct used to populate HTML template for add acknowledge form
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

func (h *httpHandler) updateStickyCategory(w http.ResponseWriter, r *http.Request, isEnabled bool) {
	categoryName := r.URL.Query().Get("category-name")
	if categoryName == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Provided category name is not valid."))
		return
	}

	err := h.controller.updateStickyCategory(categoryName, isEnabled)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Failed to enable category."))
		errorLogger.Printf("Failed to enable category with name %s. Error was: %s", categoryName, err.Error())
		return
	}
}

func (h *httpHandler) handleDisableCategory(w http.ResponseWriter, r *http.Request) {
	h.updateStickyCategory(w, r, false)
}

func (h *httpHandler) handleEnableCategory(w http.ResponseWriter, r *http.Request) {
	h.updateStickyCategory(w, r, true)
}

func (h *httpHandler) handleRemoveAck(w http.ResponseWriter, r *http.Request) {
	serviceName := getServiceNameFromURL(r.URL)
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
	serviceName := getServiceNameFromURL(r.URL)
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
	serviceName := getServiceNameFromURL(r.URL)

	if serviceName == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Provided service name is not valid."))
		return
	}

	w.Header().Add("Content-Type", "text/html")
	htmlTemplate := parseHTMLTemplate(w, "add-ack-message-form-template.html")
	if htmlTemplate == nil {
		return
	}

	addAckForm := AddAckForm{
		ServiceName: serviceName,
		AddAckPath:  fmt.Sprintf("add-ack?service-name=%s", serviceName),
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
		buildHealthcheckJSONResponse(w, healthResult)
	} else {
		env := h.controller.getEnvironment()
		buildServicesCheckHTMLResponse(w, healthResult, env, getCategoriesString(validCategories))
	}
}

func (h *httpHandler) handlePodsHealthCheck(w http.ResponseWriter, r *http.Request) {
	serviceName := getServiceNameFromURL(r.URL)

	if serviceName == "" {
		w.WriteHeader(http.StatusBadRequest)

		if r.Header.Get("Accept") != "application/json" {
			w.Write([]byte("Couldn't get service name from url."))
		}
		return
	}

	healthResult, err := h.controller.buildPodsHealthResult(serviceName, useCache(r.URL))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		errorLogger.Printf("Cannot perform checks for service with name %s, error was: %v", serviceName, err.Error())
		w.Write([]byte(fmt.Sprintf("Cannot perform checks for service with name %s", serviceName)))
		return
	}

	if r.Header.Get("Accept") == "application/json" {
		buildHealthcheckJSONResponse(w, healthResult)
	} else {
		env := h.controller.getEnvironment()
		buildPodsCheckHTMLResponse(w, healthResult, env, serviceName)
	}
}

func (h *httpHandler) handleIndividualPodHealthCheck(w http.ResponseWriter, r *http.Request) {
	podName := getPodNameFromURL(r.URL)

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

func getServiceNameFromURL(url *url.URL) string {
	return url.Query().Get("service-name")
}

func getPodNameFromURL(url *url.URL) string {
	return url.Query().Get("pod-name")
}

func useCache(theURL *url.URL) bool {
	//use cache by default
	return theURL.Query().Get("cache") != "false"
}

func buildHealthcheckJSONResponse(w http.ResponseWriter, healthResult fthealth.HealthResult) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	err := enc.Encode(healthResult)
	if err != nil {
		panic("Couldn't encode health results to ResponseWriter.")
	}
}

func buildServicesCheckHTMLResponse(w http.ResponseWriter, healthResult fthealth.HealthResult, environment string, categories string) {
	w.Header().Add("Content-Type", "text/html")
	htmlTemplate := parseHTMLTemplate(w, healthcheckTemplateName)
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

func buildPodsCheckHTMLResponse(w http.ResponseWriter, healthResult fthealth.HealthResult, environment string, serviceName string) {
	w.Header().Add("Content-Type", "text/html")
	htmlTemplate := parseHTMLTemplate(w, healthcheckTemplateName)
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

func parseHTMLTemplate(w http.ResponseWriter, templateName string) *template.Template {
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
		PageTitle:               buildPageTitle(environment, categories),
		GeneralStatus:           getGeneralStatus(healthResult),
		RefreshFromCachePath:    buildRefreshFromCachePath(categories),
		RefreshWithoutCachePath: buildRefreshWithoutCachePath(categories),
		AckCount:                ackCount,
		IndividualHealthChecks:  indiviualServiceChecks,
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
			Name:          individualCheck.Name,
			Status:        getServiceStatusFromCheck(individualCheck),
			AvailablePods: "3/3",
			LastUpdated:   individualCheck.LastUpdated.Format(timeLayout),
			MoreInfoPath:  fmt.Sprintf("/__pods-health?service-name=%s", individualCheck.Name),
			AckMessage:    individualCheck.Ack,
			Output:        individualCheck.Output,
		}

		indiviualServiceChecks = append(indiviualServiceChecks, hc)
	}

	return indiviualServiceChecks, ackCount
}

func populateIndividualPodChecks(checks []fthealth.CheckResult) ([]IndividualHealthcheckParams, int) {
	var indiviualServiceChecks []IndividualHealthcheckParams
	ackCount := 0
	for _, check := range checks {
		if check.Ack != "" {
			ackCount++
		}

		hc := IndividualHealthcheckParams{
			Name:         check.Name,
			Status:       getServiceStatusFromCheck(check),
			LastUpdated:  check.LastUpdated.Format(timeLayout),
			MoreInfoPath: fmt.Sprintf("/__pod-individual-health?pod-name=%s", check.Name),
			AckMessage:   check.Ack,
			Output:       check.Output,
		}

		indiviualServiceChecks = append(indiviualServiceChecks, hc)
	}

	return indiviualServiceChecks, ackCount
}

func populateAggregatePodChecks(healthResult fthealth.HealthResult, environment string, serviceName string) *AggregateHealthcheckParams {
	individualChecks, ackCount := populateIndividualPodChecks(healthResult.Checks)
	aggregateChecks := &AggregateHealthcheckParams{
		PageTitle:               fmt.Sprintf("CoCo %s cluster's pods of service %s", environment, serviceName),
		GeneralStatus:           getGeneralStatus(healthResult),
		RefreshFromCachePath:    fmt.Sprintf("/__pods-health?service-name=%s", serviceName),
		RefreshWithoutCachePath: fmt.Sprintf("/__pods-health?cache=false&service-name=%s", serviceName),
		IndividualHealthChecks:  individualChecks,
		AckCount:                ackCount,
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

func getCategoriesString(categories map[string]category) string {
	formattedCategoryNames := ""
	for categoryName := range categories {
		formattedCategoryNames += categoryName + ","
	}

	len := len(formattedCategoryNames)
	if len > 0 {
		formattedCategoryNames = formattedCategoryNames[:len - 1]
	}

	return formattedCategoryNames
}
