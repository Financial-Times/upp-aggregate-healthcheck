package main

import (
	fthealth "github.com/Financial-Times/go-fthealth/v1a"
	"sort"
	"fmt"
	"errors"
	"time"
)

type healthCheckController struct {
	healthCheckService healthcheckService
	environment        string
	measuredServices   map[string]MeasuredService
}

type MeasuredService struct {
	service      service
	cachedHealth *cachedHealth
}

type controller interface {
	buildServicesHealthResult([]string, bool) (fthealth.HealthResult, map[string]category, map[string]category, error)
	runServiceChecksByServiceNames([]service) []fthealth.CheckResult
	runServiceChecksFor(map[string]category) ([]fthealth.CheckResult, map[string][]fthealth.CheckResult)
	buildPodsHealthResult(string, bool) (fthealth.HealthResult)
	runPodChecksFor(string) ([]fthealth.CheckResult, map[string][]fthealth.CheckResult)
	collectChecksFromCachesFor(map[string]category) ([]fthealth.CheckResult, map[string][]fthealth.CheckResult)
	updateCachedHealth([]service)
	scheduleCheck(MeasuredService, *time.Timer)
	getIndividualPodHealth(string) ([]byte, error)
	addAck(string, string) error
	enableStickyCategory(string) error
	removeAck(string) error
	getEnvironment() string
	getSeverityForService(string) uint8
	getSeverityForPod(string) uint8
	computeSeverityByPods([]pod) uint8
}


func InitializeController(environment string) *healthCheckController {
	service := InitializeHealthCheckService()
	measuredServices := make(map[string]MeasuredService)

	return &healthCheckController{
		healthCheckService: service,
		environment: environment,
		measuredServices: measuredServices,
	}
}

func (c *healthCheckController) getEnvironment() string {
	return c.environment
}

func (c *healthCheckController) enableStickyCategory(serviceName string) error {
	return c.healthCheckService.updateCategory(serviceName, true)
}

func (c *healthCheckController) removeAck(serviceName string) error {
	services := c.healthCheckService.getServicesByNames([]string{serviceName})

	if len(services) == 0 {
		return errors.New(fmt.Sprintf("Cannot find service with name %s", serviceName))
	}

	err := c.healthCheckService.removeAck(serviceName)

	if err != nil {
		return errors.New(fmt.Sprintf("Failed to remove ack for service %s. Error was: %s", serviceName, err.Error()))
	}

	return nil
}

func (c *healthCheckController) addAck(serviceName string, ackMessage string) error {
	services := c.healthCheckService.getServicesByNames([]string{serviceName})

	if len(services) == 0 {
		return errors.New(fmt.Sprintf("Cannot find service with name %s", serviceName))
	}

	err := c.healthCheckService.addAck(serviceName, ackMessage)

	if err != nil {
		return errors.New(fmt.Sprintf("Failed to add ack message [%s] for service %s. Error was: %s", ackMessage, serviceName, err.Error()))
	}

	return nil
}

func (c *healthCheckController) buildServicesHealthResult(providedCategories []string, useCache bool) (fthealth.HealthResult, map[string]category, map[string]category, error) {
	var checkResults []fthealth.CheckResult
	desc := "Health of the whole cluster of the moment served without cache."
	availableCategories, err := c.healthCheckService.getCategories()
	if err != nil {
		return fthealth.HealthResult{}, nil, nil, errors.New(fmt.Sprintf("Cannot build health check result for services. Error was: %v", err.Error()))
	}

	matchingCategories := getMatchingCategories(providedCategories, availableCategories)

	if useCache {
		desc = "Health of the whole cluster served from cache."
		checkResults, _ = c.collectChecksFromCachesFor(matchingCategories)

	} else {
		checkResults, _ = c.runServiceChecksFor(matchingCategories)
	}

	finalOk, finalSeverity := getFinalResult(checkResults, matchingCategories)

	health := fthealth.HealthResult{
		Checks:        checkResults,
		Description:   desc,
		Name:          c.environment + " cluster health",
		SchemaVersion: 1,
		Ok:            finalOk,
		Severity:      finalSeverity,
	}

	sort.Sort(ByNameComparator(health.Checks))

	return health, matchingCategories, nil, nil
}

func (c *healthCheckController) runServiceChecksByServiceNames(services []service) []fthealth.CheckResult {
	var checks []fthealth.Check

	for _, service := range services {
		check := NewServiceHealthCheck(service, c.healthCheckService)
		checks = append(checks, check)
	}

	healthChecks := fthealth.RunCheck("Forced check run", "", true, checks...).Checks

	for _, service := range services {
		if service.ack != "" {
			updateHealthCheckWithAckMsg(healthChecks, service.name, service.ack)
		}
	}

	//todo: possibly we will need to add the check severity here too. (foreach healthCheck, if healthcheck.ok == false, then get severity for service.)

	c.updateCachedHealth(services)

	return healthChecks
}

func (c *healthCheckController) runServiceChecksFor(categories map[string]category) ([]fthealth.CheckResult, map[string][]fthealth.CheckResult) {
	categorisedResults := make(map[string][]fthealth.CheckResult)
	serviceNames := getServiceNamesFromCategories(categories)
	services := c.healthCheckService.getServicesByNames(serviceNames)
	healthChecks := c.runServiceChecksByServiceNames(services)

	for catIndex, category := range categories {
		if category.isSticky && category.isEnabled {
			for _, serviceName := range category.services {
				for _, healthCheck := range healthChecks {
					if healthCheck.Name == serviceName {
						infoLogger.Printf("Sticky category [%s] is unhealthy, disabling it.", category.name)
						category.isEnabled = false
						categories[catIndex] = category
						c.healthCheckService.updateCategory(category.name, false)
					}
				}
			}
		}
	}

	return healthChecks, categorisedResults
}

func updateHealthCheckWithAckMsg(healthChecks []fthealth.CheckResult, name string, ackMsg string) {
	for i, healthCheck := range healthChecks {
		if healthCheck.Name == name {
			healthChecks[i].Ack = ackMsg
			return
		}
	}
}

func getFinalResult(checkResults []fthealth.CheckResult, categories map[string]category) (bool, uint8) {
	finalOk := true
	var finalSeverity uint8 = 2

	for _, category := range categories {
		if !category.isEnabled {
			finalOk = false
		}
	}

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
