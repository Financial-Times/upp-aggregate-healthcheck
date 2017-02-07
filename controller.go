package main

import (
	fthealth "github.com/Financial-Times/go-fthealth/v1a"
	"sort"
	"fmt"
	"net/http"
	"errors"
	"io/ioutil"
)

type healthCheckController struct {
	healthCheckService healthcheckService
	environment        *string
	measuredServices   map[string]MeasuredService
}

type MeasuredService struct {
	service      *service
	cachedHealth *cachedHealth //latest healthiness measurement
				   //todo: check if we will use graphite
				   //bufferedHealths *BufferedHealths //up to 60 healthiness measurements to be buffered and sent at once graphite
}

type controller interface {
	buildServicesHealthResult([]string, bool) (fthealth.HealthResult, map[string]category, map[string]category,error)
	buildPodsHealthResult(string, bool) (fthealth.HealthResult)
	runServiceChecksFor(map[string]category) ([]fthealth.CheckResult, map[string][]fthealth.CheckResult)
	runPodChecksFor(string) ([]fthealth.CheckResult, map[string][]fthealth.CheckResult)
	collectChecksFromCachesFor(map[string]category) ([]fthealth.CheckResult, map[string][]fthealth.CheckResult)
	getIndividualPodHealth(string) ([]byte, error)
}

func (c *healthCheckController)  collectChecksFromCachesFor(categories map[string]category) ([]fthealth.CheckResult, map[string][]fthealth.CheckResult) {
	var checkResults []fthealth.CheckResult
	categorisedResults := make(map[string][]fthealth.CheckResult)

	return checkResults, categorisedResults
}

func (c *healthCheckController) buildServicesHealthResult(providedCategories []string, useCache bool) (fthealth.HealthResult, map[string]category, map[string]category, error) {
	var checkResults []fthealth.CheckResult
	desc := "Health of the whole cluster of the moment served without cache."
	availableCategories, err := c.healthCheckService.getCategories()
	if err != nil {
		return fthealth.HealthResult{},nil,nil,errors.New(fmt.Sprintf("Cannot build health check result for services. Error was: %v", err.Error()))
	}

	matchingCategories := getMatchingCategories(providedCategories, availableCategories)

	if useCache {
		desc = "Health of the whole cluster served from cache."
		checkResults, _ = c.collectChecksFromCachesFor(matchingCategories)

	} else {
		checkResults, _ = c.runServiceChecksFor(matchingCategories)
	}

	finalOk, finalSeverity := getFinalResult(checkResults)

	health := fthealth.HealthResult{
		Checks:        checkResults,
		Description:   desc,
		Name:          *c.environment + " cluster health",
		SchemaVersion: 1,
		Ok:            finalOk,
		Severity:      finalSeverity,
	}

	sort.Sort(ByNameComparator(health.Checks))

	//todo: add unhealthy categories here.
	return health, matchingCategories, nil, nil
}

func (c *healthCheckController)buildPodsHealthResult(serviceName string, useCache bool) (fthealth.HealthResult) {
	var checkResults []fthealth.CheckResult
	desc := fmt.Sprintf("Health of pods that are under service %s served without cache.", serviceName)

	if useCache {
		desc = fmt.Sprintf("Health of pods that are under service %s served from cache.", serviceName)
		//todo: check if we will use cache also for pods.
		checkResults, _ = c.runPodChecksFor(serviceName)
	} else {
		checkResults, _ = c.runPodChecksFor(serviceName)
	}

	finalOk, finalSeverity := getFinalResult(checkResults)

	health := fthealth.HealthResult{
		Checks:        checkResults,
		Description:   desc,
		Name:          *c.environment + " cluster health",
		SchemaVersion: 1,
		Ok:            finalOk,
		Severity:      finalSeverity,
	}

	sort.Sort(ByNameComparator(health.Checks))

	return health
}

func (c *healthCheckController) runPodChecksFor(serviceName string) ([]fthealth.CheckResult, map[string][]fthealth.CheckResult) {
	categorisedResults := make(map[string][]fthealth.CheckResult)

	pods := c.healthCheckService.getPodsForService(serviceName)
	var checks []fthealth.Check

	for _, pod := range pods {
		check := NewPodHealthCheck(pod, c.healthCheckService)
		checks = append(checks, check)
	}

	healthChecks := fthealth.RunCheck("Forced check run", "", true, checks...).Checks

	return healthChecks, categorisedResults
}

func (c *healthCheckController) runServiceChecksFor(categories map[string]category) ([]fthealth.CheckResult, map[string][]fthealth.CheckResult) {
	categorisedResults := make(map[string][]fthealth.CheckResult)

	//for category := range categories {
	//	categorisedResults[category] = []fthealth.CheckResult{}
	//}

	serviceNames := getServiceNamesFromCategories(categories)
	services := c.healthCheckService.getServicesByNames(serviceNames)
	var checks []fthealth.Check

	for _, service := range services {
		check := NewServiceHealthCheck(service, c.healthCheckService)
		checks = append(checks, check)
	}

	healthChecks := fthealth.RunCheck("Forced check run", "", true, checks...).Checks

	//todo: populate categorisedResults if we will use graphite.
	return healthChecks, categorisedResults
}

func (c *healthCheckController) getIndividualPodHealth(podName string) ([]byte, error) {
	//todo: change this url.
	req, err := http.NewRequest("GET", "https://prod-us-up.ft.com/health/document-store-api-2/__health", nil)
	if err != nil {
		return nil, errors.New("Error constructing healthcheck request: " + err.Error())
	}

	resp, err := c.healthCheckService.getHttpClient().Do(req)
	if err != nil {
		return nil, errors.New("Error performing healthcheck: " + err.Error())
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Healthcheck endpoint returned non-200 status (%v)", resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return nil, errors.New("Error reading healthcheck response: " + err.Error())
	}

	return body, nil
}

func getFinalResult(checkResults []fthealth.CheckResult) (bool, uint8) {
	finalOk := true
	var finalSeverity uint8 = 2

	for _, checkResult := range checkResults {
		if !checkResult.Ok && checkResult.Ack == "" {
			finalOk = false

			if checkResult.Severity < finalSeverity {
				finalSeverity = checkResult.Severity
			}
		}
	}

	return finalOk, finalSeverity
}

func InitializeController(environment *string) *healthCheckController {
	service := InitializeHealthCheckService()
	measuredServices := make(map[string]MeasuredService)

	return &healthCheckController{
		healthCheckService: service,
		environment: environment,
		measuredServices: measuredServices,
	}
}

func getMatchingCategories(providedCategories []string, availableCategories map[string]category) map[string]category {
	result := make(map[string]category)
	for _, providedCat := range providedCategories {
		if _, ok := availableCategories[providedCat]; ok {
			result[providedCat] = availableCategories[providedCat]
		}
	}

	return result
}

func getServiceNamesFromCategories(categories map[string]category) []string {
	var services []string

	if _, ok := categories["default"]; ok {
		infoLogger.Print("Using default category")
		return services
	}

	for categoryName := range categories {
		servicesForCategory := categories[categoryName].services
		for _, service := range servicesForCategory {
			if !isStringInSlice(service, services) {
				services = append(services, service)
			}
		}
	}

	return services
}

func isStringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}

	return false
}

//used for sorting checks
type ByNameComparator []fthealth.CheckResult

func (s ByNameComparator) Less(i, j int) bool {
	return s[i].Name < s[j].Name
}

func (s ByNameComparator) Len() int {
	return len(s)
}
func (s ByNameComparator) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
