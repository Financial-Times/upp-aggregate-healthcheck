package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
	log "github.com/Financial-Times/go-logger"
)

type httpHandler struct {
	controller controller
	pathPrefix string
	clusterURL string
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
	timeLayout              = "2006-01-02 15:04:05 MST"
	healthcheckTemplateName = "html-templates/healthcheck-template.html"
	addAckMsgTemplatePath   = "html-templates/add-ack-message-form-template.html"
	healthcheckPath         = "/__health"
	jsonContentType         = "application/json"
)

func handleResponseWriterErr(err error) {
	if err != nil {
		log.WithError(err).Error("Cannot write the http response body.")
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
	log.Infof("Updating category [%s] with isEnabled flag value of [%t]", categoryName, isEnabled)
	err := h.controller.updateStickyCategory(r.Context(), categoryName, isEnabled)

	if err != nil {
		log.WithError(err).Errorf("Failed to update category with name %s.", categoryName)
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
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	serviceName := getServiceNameFromURL(r.URL)
	if serviceName == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, err := w.Write([]byte("Provided service name is not valid."))
		handleResponseWriterErr(err)
		return
	}

	log.Infof("Removing ack for service with name %s", serviceName)
	err := h.controller.removeAck(r.Context(), serviceName)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.WithError(err).Errorf("Cannot remove ack for service with name %s.", serviceName)
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

	log.Infof("Acking service with name %s", serviceName)
	err := h.controller.addAck(r.Context(), serviceName, ackMessage)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.WithError(err).Errorf("Cannot add acknowledge for service with name %s.", serviceName)
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
		log.WithError(err).Error("Cannot apply params to html template")
		_, err := w.Write([]byte("Couldn't render template file for html response"))
		handleResponseWriterErr(err)
		return
	}
}

func (h *httpHandler) handleServicesHealthCheck(w http.ResponseWriter, r *http.Request) {
	categories := parseCategories(r.URL)
	useCache := useCache(r.URL)
	healthResult, validCategories, err := h.controller.buildServicesHealthResult(r.Context(), categories, useCache)

	if len(validCategories) == 0 && err == nil {
		w.WriteHeader(http.StatusBadRequest)

		if r.Header.Get("Accept") != "application/json" {
			_, err := w.Write([]byte("Provided categories are not valid."))
			handleResponseWriterErr(err)
		}
		return
	}

	log.Infof("Checking services health for categories %s, useCache: %t", getCategoriesString(validCategories), useCache)

	if err != nil {
		log.WithError(err).Error("Cannot build services health result")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if r.Header.Get("Accept") == jsonContentType {
		for i, serviceCheck := range healthResult.Checks {
			serviceHealthcheckURL := getServiceHealthcheckURL(h.clusterURL, h.pathPrefix, serviceCheck.Name)
			healthResult.Checks[i].TechnicalSummary = fmt.Sprintf("%s Service healthcheck: %s", serviceCheck.TechnicalSummary, serviceHealthcheckURL)
		}

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

	healthResult, err := h.controller.buildPodsHealthResult(r.Context(), serviceName)

	log.Infof("Checking pods health for service [%s]", serviceName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.WithError(err).Errorf("Cannot perform checks for service with name %s", serviceName)
		_, err := w.Write([]byte(fmt.Sprintf("Cannot perform checks for service with name %s", serviceName)))
		handleResponseWriterErr(err)
		return
	}

	if r.Header.Get("Accept") == jsonContentType {
		for i, podCheck := range healthResult.Checks {
			serviceHealthcheckURL := getIndividualPodHealthcheckURL(h.clusterURL, h.pathPrefix, podCheck.Name)
			healthResult.Checks[i].TechnicalSummary = fmt.Sprintf("%s Pod healthcheck: %s", podCheck.TechnicalSummary, serviceHealthcheckURL)
		}

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

	log.Infof("Retrieving individual pod health check for pod with name %s", podName)
	podHealth, contentTypeHeader, err := h.controller.getIndividualPodHealth(r.Context(), podName)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

		log.WithError(err).Errorf("Cannot get individual healthcheck for pod %s", podName)
		_, err := w.Write([]byte(fmt.Sprintf("Cannot get individual healthcheck for pod %s", podName)))
		handleResponseWriterErr(err)
		return
	}

	w.Header().Add("Content-Type", contentTypeHeader)
	_, err = w.Write(podHealth)
	handleResponseWriterErr(err)
}

func (h *httpHandler) handleGoodToGo(w http.ResponseWriter, r *http.Request) {
	categories := parseCategories(r.URL)
	useCache := useCache(r.URL)
	healthResults, validCategories, err := h.controller.buildServicesHealthResult(r.Context(), categories, useCache)

	if len(validCategories) == 0 && err == nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	log.Infof("Handling gtg for categories %s, useCache: %t", getCategoriesString(validCategories), useCache)
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

	type CheckResultWithHeimdalAck struct {
		fthealth.CheckResult
		HeimdalAck string `json:"_acknowledged,omitempty"`
	}

	type HealthResult struct {
		SchemaVersion float64                     `json:"schemaVersion"`
		SystemCode    string                      `json:"systemCode"`
		Name          string                      `json:"name"`
		Description   string                      `json:"description"`
		Checks        []CheckResultWithHeimdalAck `json:"checks"`
		Ok            bool                        `json:"ok"`
		Severity      uint8                       `json:"severity,omitempty"`
	}

	var newChecks []CheckResultWithHeimdalAck
	for _, check := range healthResult.Checks {
		newCheck := CheckResultWithHeimdalAck{
			CheckResult: check,
			HeimdalAck:  check.Ack,
		}
		newChecks = append(newChecks, newCheck)
	}

	newHealthResult := &HealthResult{
		SchemaVersion: healthResult.SchemaVersion,
		SystemCode:    healthResult.SystemCode,
		Name:          healthResult.Name,
		Description:   healthResult.Description,
		Ok:            healthResult.Ok,
		Severity:      healthResult.Severity,
		Checks:        newChecks,
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	err := enc.Encode(newHealthResult)
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
		log.WithError(err).Error("Cannot apply params to html template")
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
		log.WithError(err).Error("Cannot apply params to html template")
		_, err := w.Write([]byte("Couldn't render template file for html response"))
		handleResponseWriterErr(err)
		return
	}
}

func parseHTMLTemplate(w http.ResponseWriter, templateName string) *template.Template {
	htmlTemplate, err := template.ParseFiles(templateName)
	if err != nil {
		log.WithError(err).Errorf("Could not parse html template with name %s", templateName)
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
	indiviualServiceChecks := make([]IndividualHealthcheckParams, len(checks))
	ackCount := 0
	for i, individualCheck := range checks {
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
			Output:                 individualCheck.CheckOutput,
		}

		indiviualServiceChecks[i] = hc
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
	indiviualServiceChecks := make([]IndividualHealthcheckParams, len(checks))
	ackCount := 0
	for i, check := range checks {
		if check.Ack != "" {
			ackCount++
		}
		podName := extractPodName(check.Name)
		hc := IndividualHealthcheckParams{
			Name:         check.Name,
			Status:       getServiceStatusFromCheck(check),
			LastUpdated:  check.LastUpdated.Format(timeLayout),
			MoreInfoPath: getIndividualPodHealthcheckURL("", pathPrefix, podName),
			AckMessage:   check.Ack,
			Output:       check.CheckOutput,
		}

		indiviualServiceChecks[i] = hc
	}

	return indiviualServiceChecks, ackCount
}

func getIndividualPodHealthcheckURL(clusterURL, pathPrefix, podName string) string {
	return fmt.Sprintf("%s%s/__pod-individual-health?pod-name=%s", clusterURL, pathPrefix, podName)
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
		RefreshFromCachePath:    getServiceHealthcheckURL("", pathPrefix, serviceName),
		RefreshWithoutCachePath: fmt.Sprintf("%s/__pods-health?cache=false&service-name=%s", pathPrefix, serviceName),
		IndividualHealthChecks:  individualChecks,
		AckCount:                ackCount,
	}

	return aggregateChecks
}

func getServiceHealthcheckURL(hostURL, pathPrefix, serviceName string) string {
	return fmt.Sprintf("%s%s/__pods-health?service-name=%s", hostURL, pathPrefix, serviceName)
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

	l := len(formattedCategoryNames)
	if l > 0 {
		formattedCategoryNames = formattedCategoryNames[:l-1]
	}

	return formattedCategoryNames
}
