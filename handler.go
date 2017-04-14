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
	pathPrefix string
}

//IndividualHealthcheckParams struct used to populate HTML template with individual checks
type IndividualHealthcheckParams struct {
	Name                   string
	Status                 string
	LastUpdated            string
	MoreInfoPath           string
	AddOrRemoveAckPath     string
	AddOrRemoveAckPathName string
	AckMessage             string
	Output                 string
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
	timeLayout              = "15:04:05 MST"
	healthcheckTemplateName = "html-templates/healthcheck-template.html"
	addAckMsgTemplatePath   = "html-templates/add-ack-message-form-template.html"
	healthcheckPath         = "/__health"
	jsonContentType         = "application/json"
)

func handleResponseWriterErr(err error) {
	if err != nil {
		errorLogger.Printf("Cannot write the http response body. Error was: %s", err.Error())
	}
}

func (h *httpHandler) updateStickyCategory(w http.ResponseWriter, r *http.Request, isEnabled bool) {
	categoryName := r.URL.Query().Get("category-name")
	if categoryName == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, err := w.Write([]byte("Provided category name is not valid."))
		handleResponseWriterErr(err)
		return
	}

	infoLogger.Printf("Updating category [%s] with isEnabled flag value of [%t]", categoryName, isEnabled)
	err := h.controller.updateStickyCategory(categoryName, isEnabled)

	if err != nil {
		errorLogger.Printf("Failed to update category with name %s. Error was: %s", categoryName, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		_, err := w.Write([]byte("Failed to enable category."))
		handleResponseWriterErr(err)
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
		_, err := w.Write([]byte("Provided service name is not valid."))
		handleResponseWriterErr(err)
		return
	}

	infoLogger.Printf("Removing ack for service with name %s", serviceName)
	err := h.controller.removeAck(serviceName)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		errorLogger.Printf("Cannot remove ack for service with name %s. Error was: %s", serviceName, err.Error())
		return
	}

	http.Redirect(w, r, fmt.Sprintf("%s?cache=false", h.pathPrefix), http.StatusMovedPermanently)
}

func (h *httpHandler) handleAddAck(w http.ResponseWriter, r *http.Request) {
	serviceName := getServiceNameFromURL(r.URL)
	ackMessage := r.PostFormValue("ack-msg")
	if serviceName == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, err := w.Write([]byte("Provided service name is not valid."))
		handleResponseWriterErr(err)
		return
	}

	infoLogger.Printf("Acking service with name %s", serviceName)
	err := h.controller.addAck(serviceName, ackMessage)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		errorLogger.Printf("Cannot add acknowledge for service with name %s. Error was: %s", serviceName, err.Error())
	}

	http.Redirect(w, r, fmt.Sprintf("%s?cache=false", h.pathPrefix), http.StatusMovedPermanently)
}

func (h *httpHandler) handleAddAckForm(w http.ResponseWriter, r *http.Request) {
	serviceName := getServiceNameFromURL(r.URL)

	if serviceName == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, err := w.Write([]byte("Provided service name is not valid."))
		handleResponseWriterErr(err)
		return
	}

	w.Header().Add("Content-Type", "text/html")
	htmlTemplate := parseHTMLTemplate(w, addAckMsgTemplatePath)
	if htmlTemplate == nil {
		return
	}

	addAckForm := AddAckForm{
		ServiceName: serviceName,
		AddAckPath:  fmt.Sprintf("%s/add-ack?service-name=%s", h.pathPrefix, serviceName),
	}

	if err := htmlTemplate.Execute(w, addAckForm); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		errorLogger.Printf("Cannot apply params to html template, error was: %v", err.Error())
		_, err := w.Write([]byte("Couldn't render template file for html response"))
		handleResponseWriterErr(err)
		return
	}
}

func (h *httpHandler) handleServicesHealthCheck(w http.ResponseWriter, r *http.Request) {
	categories := parseCategories(r.URL)
	useCache := useCache(r.URL)
	healthResult, validCategories, _, err := h.controller.buildServicesHealthResult(categories, useCache)

	if len(validCategories) == 0 && err == nil {
		w.WriteHeader(http.StatusBadRequest)

		if r.Header.Get("Accept") != "application/json" {
			_, error := w.Write([]byte("Provided categories are not valid."))
			handleResponseWriterErr(error)
		}
		return
	}

	infoLogger.Printf("Checking services health for categories %s, useCache: %t", getCategoriesString(validCategories), useCache)

	if err != nil {
		errorLogger.Printf("Cannot build services health result, error was: %v", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if r.Header.Get("Accept") == jsonContentType {
		buildHealthcheckJSONResponse(w, healthResult)
	} else {
		env := h.controller.getEnvironment()
		buildServicesCheckHTMLResponse(w, healthResult, env, getCategoriesString(validCategories), h.pathPrefix)
	}
}

func (h *httpHandler) handlePodsHealthCheck(w http.ResponseWriter, r *http.Request) {
	serviceName := getServiceNameFromURL(r.URL)

	if serviceName == "" {
		w.WriteHeader(http.StatusBadRequest)

		if r.Header.Get("Accept") != jsonContentType {
			_, err := w.Write([]byte("Couldn't get service name from url."))
			handleResponseWriterErr(err)
		}
		return
	}

	healthResult, err := h.controller.buildPodsHealthResult(serviceName)

	infoLogger.Printf("Checking pods health for service [%s]", serviceName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		errorLogger.Printf("Cannot perform checks for service with name %s, error was: %v", serviceName, err.Error())
		_, err := w.Write([]byte(fmt.Sprintf("Cannot perform checks for service with name %s", serviceName)))
		handleResponseWriterErr(err)
		return
	}

	if r.Header.Get("Accept") == jsonContentType {
		buildHealthcheckJSONResponse(w, healthResult)
	} else {
		env := h.controller.getEnvironment()
		buildPodsCheckHTMLResponse(w, healthResult, env, serviceName, h.pathPrefix)
	}
}

func (h *httpHandler) handleIndividualPodHealthCheck(w http.ResponseWriter, r *http.Request) {
	podName := getPodNameFromURL(r.URL)

	if podName == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, err := w.Write([]byte("Cannot parse pod name from url."))
		handleResponseWriterErr(err)
		return
	}

	infoLogger.Printf("Retrieving individual pod health check for pod with name %s", podName)
	podHealth, contentTypeHeader, err := h.controller.getIndividualPodHealth(podName)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		errorLogger.Printf("Cannot get individual healthcheck for pod %s, error was: %v", podName, err.Error())
		_, err := w.Write([]byte(fmt.Sprintf("Cannot get individual healthcheck for pod %s", podName)))
		handleResponseWriterErr(err)
		return
	}

	w.Header().Add("Content-Type", contentTypeHeader)
	_, error := w.Write(podHealth)
	handleResponseWriterErr(error)
}

func (h *httpHandler) handleGoodToGo(w http.ResponseWriter, r *http.Request) {
	categories := parseCategories(r.URL)
	useCache := useCache(r.URL)
	healthResults, validCategories, _, err := h.controller.buildServicesHealthResult(categories, useCache)

	if len(validCategories) == 0 && err == nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	infoLogger.Printf("Handling gtg for categories %s, useCache: %t", getCategoriesString(validCategories), useCache)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	for _, validCategory := range validCategories {
		if !validCategory.isEnabled {
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

func buildServicesCheckHTMLResponse(w http.ResponseWriter, healthResult fthealth.HealthResult, environment string, categories string, pathPrefix string) {
	w.Header().Add("Content-Type", "text/html")
	htmlTemplate := parseHTMLTemplate(w, healthcheckTemplateName)
	if htmlTemplate == nil {
		return
	}

	aggregateHealthcheckParams := populateAggregateServiceChecks(healthResult, environment, categories, pathPrefix)

	if err := htmlTemplate.Execute(w, aggregateHealthcheckParams); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		errorLogger.Printf("Cannot apply params to html template, error was: %v", err.Error())
		_, err := w.Write([]byte("Couldn't render template file for html response"))
		handleResponseWriterErr(err)
		return
	}
}

func buildPodsCheckHTMLResponse(w http.ResponseWriter, healthResult fthealth.HealthResult, environment string, serviceName string, pathPrefix string) {
	w.Header().Add("Content-Type", "text/html")
	htmlTemplate := parseHTMLTemplate(w, healthcheckTemplateName)
	if htmlTemplate == nil {
		return
	}

	aggregateHealthcheckParams := populateAggregatePodChecks(healthResult, environment, serviceName, pathPrefix)

	if err := htmlTemplate.Execute(w, aggregateHealthcheckParams); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		errorLogger.Printf("Cannot apply params to html template, error was: %v", err.Error())
		_, err := w.Write([]byte("Couldn't render template file for html response"))
		handleResponseWriterErr(err)
		return
	}
}

func parseHTMLTemplate(w http.ResponseWriter, templateName string) *template.Template {
	htmlTemplate, err := template.ParseFiles(templateName)
	if err != nil {
		errorLogger.Printf("Could not parse html template with name %s, error was: %v", templateName, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		_, err := w.Write([]byte("Couldn't open template file for html response"))
		handleResponseWriterErr(err)
		return nil
	}

	return htmlTemplate
}

func populateAggregateServiceChecks(healthResult fthealth.HealthResult, environment string, categories string, pathPrefix string) *AggregateHealthcheckParams {
	indiviualServiceChecks, ackCount := populateIndividualServiceChecks(healthResult.Checks, pathPrefix)
	aggregateChecks := &AggregateHealthcheckParams{
		PageTitle:               buildPageTitle(environment, categories),
		GeneralStatus:           getGeneralStatus(healthResult),
		RefreshFromCachePath:    buildRefreshFromCachePath(categories, pathPrefix),
		RefreshWithoutCachePath: buildRefreshWithoutCachePath(categories, pathPrefix),
		AckCount:                ackCount,
		IndividualHealthChecks:  indiviualServiceChecks,
	}

	return aggregateChecks
}

func buildRefreshFromCachePath(categories string, pathPrefix string) string {
	if categories != "" {
		return fmt.Sprintf("%s?categories=%s", pathPrefix, categories)
	}

	return healthcheckPath
}

func buildRefreshWithoutCachePath(categories string, pathPrefix string) string {
	refreshWithoutCachePath := fmt.Sprintf("%s?cache=false", pathPrefix)
	if categories != "" {
		return fmt.Sprintf("%s&categories=%s", refreshWithoutCachePath, categories)
	}

	return refreshWithoutCachePath
}

func populateIndividualServiceChecks(checks []fthealth.CheckResult, pathPrefix string) ([]IndividualHealthcheckParams, int) {
	var indiviualServiceChecks []IndividualHealthcheckParams
	ackCount := 0
	for _, individualCheck := range checks {
		if individualCheck.Ack != "" {
			ackCount++
		}

		addOrRemoveAckPath, addOrRemoveAckPathName := buildAddOrRemoveAckPath(individualCheck.Name, pathPrefix, individualCheck.Ack)
		hc := IndividualHealthcheckParams{
			Name:                   individualCheck.Name,
			Status:                 getServiceStatusFromCheck(individualCheck),
			LastUpdated:            individualCheck.LastUpdated.Format(timeLayout),
			MoreInfoPath:           fmt.Sprintf("%s/__pods-health?service-name=%s", pathPrefix, individualCheck.Name),
			AddOrRemoveAckPath:     addOrRemoveAckPath,
			AddOrRemoveAckPathName: addOrRemoveAckPathName,
			AckMessage:             individualCheck.Ack,
			Output:                 individualCheck.Output,
		}

		indiviualServiceChecks = append(indiviualServiceChecks, hc)
	}

	return indiviualServiceChecks, ackCount
}

func buildAddOrRemoveAckPath(serviceName string, pathPrefix string, ackMessage string) (string, string) {
	if ackMessage == "" {
		return fmt.Sprintf("%s/add-ack-form?service-name=%s", pathPrefix, serviceName), "Ack service"
	}

	return fmt.Sprintf("%s/rem-ack?service-name=%s", pathPrefix, serviceName), "Remove ack"
}

func populateIndividualPodChecks(checks []fthealth.CheckResult, pathPrefix string) ([]IndividualHealthcheckParams, int) {
	var indiviualServiceChecks []IndividualHealthcheckParams
	ackCount := 0
	for _, check := range checks {
		if check.Ack != "" {
			ackCount++
		}
		podName := extractPodName(check.Name)
		hc := IndividualHealthcheckParams{
			Name:         check.Name,
			Status:       getServiceStatusFromCheck(check),
			LastUpdated:  check.LastUpdated.Format(timeLayout),
			MoreInfoPath: fmt.Sprintf("%s/__pod-individual-health?pod-name=%s", pathPrefix, podName),
			AckMessage:   check.Ack,
			Output:       check.Output,
		}

		indiviualServiceChecks = append(indiviualServiceChecks, hc)
	}

	return indiviualServiceChecks, ackCount
}

func extractPodName(checkName string) string {
	s := strings.Split(checkName, " ")

	if len(s) >= 1 {
		return s[0]
	}

	return ""
}

func populateAggregatePodChecks(healthResult fthealth.HealthResult, environment string, serviceName string, pathPrefix string) *AggregateHealthcheckParams {
	individualChecks, ackCount := populateIndividualPodChecks(healthResult.Checks, pathPrefix)
	aggregateChecks := &AggregateHealthcheckParams{
		PageTitle:               fmt.Sprintf("UPP %s cluster's pods of service %s", environment, serviceName),
		GeneralStatus:           getGeneralStatus(healthResult),
		RefreshFromCachePath:    fmt.Sprintf("%s/__pods-health?service-name=%s", pathPrefix, serviceName),
		RefreshWithoutCachePath: fmt.Sprintf("%s/__pods-health?cache=false&service-name=%s", pathPrefix, serviceName),
		IndividualHealthChecks:  individualChecks,
		AckCount:                ackCount,
	}

	return aggregateChecks
}

func buildPageTitle(environment string, categories string) string {
	return fmt.Sprintf("UPP %s cluster's services from categories %s", environment, categories)
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
		formattedCategoryNames = formattedCategoryNames[:len-1]
	}

	return formattedCategoryNames
}
