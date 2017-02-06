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
	IndividualHealthChecks  []IndividualHealthcheckParams
}

var defaultCategories = []string{"default"}

const timeLayout = "15:04:05 MST"

func (h *httpHandler) handleServicesHealthCheck(w http.ResponseWriter, r *http.Request) {
	categories := parseCategories(r.URL)
	healthResult, validCategories, _ := h.controller.buildServicesHealthResult(categories, useCache(r.URL))

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
		buildServicesCheckHtmlResponse(w, healthResult, "ADD ENV HERE", getCategoriesString(validCategories))
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
		buildPodsCheckHtmlResponse(w, healthResult)
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
	htmlTemplate := parseHtmlTemplate(w)
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

func buildPodsCheckHtmlResponse(w http.ResponseWriter, healthResult fthealth.HealthResult) {
	w.Header().Add("Content-Type", "text/html")
	htmlTemplate := parseHtmlTemplate(w)
	if htmlTemplate == nil {
		return
	}

	aggregateHealthcheckParams := populateAggregatePodChecks(healthResult)

	if err := htmlTemplate.Execute(w, aggregateHealthcheckParams); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		errorLogger.Printf("Cannot apply params to html template, error was: %v", err.Error())
		w.Write([]byte("Couldn't render template file for html response"))
		return
	}
}

func parseHtmlTemplate(w http.ResponseWriter) *template.Template {
	htmlTemplate, err := template.ParseFiles("html-templates\\services-healthcheck-template.html")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Couldn't open template file for html response"))
		errorLogger.Printf("Could not parse html template, error was: %v", err.Error())

		return nil
	}

	return htmlTemplate
}

func populateAggregateServiceChecks(healthResult fthealth.HealthResult, environment string, categories string) *AggregateHealthcheckParams {
	indiviualServiceChecks := populateIndividualServiceChecks(healthResult.Checks)
	aggregateChecks := &AggregateHealthcheckParams{
		PageTitle: buildPageTitle(environment, categories),
		GeneralStatus: getGeneralStatus(healthResult),
		RefreshFromCachePath: "#",
		RefreshWithoutCachePath: "#",
		IndividualHealthChecks: indiviualServiceChecks,
	}

	return aggregateChecks
}

func populateIndividualServiceChecks(checks []fthealth.CheckResult) []IndividualHealthcheckParams {
	var indiviualServiceChecks []IndividualHealthcheckParams

	for _, individualCheck := range checks {
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

	return indiviualServiceChecks
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

func populateAggregatePodChecks(healthResult  fthealth.HealthResult) *AggregateHealthcheckParams {
	aggregateChecks := &AggregateHealthcheckParams{
		PageTitle: "CoCo prod-uk service service-name-test pods",
		GeneralStatus: getGeneralStatus(healthResult),
		RefreshFromCachePath: "#",
		RefreshWithoutCachePath: "#",
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
		formattedCategoryNames += categoryName + " "
	}

	return formattedCategoryNames
}
