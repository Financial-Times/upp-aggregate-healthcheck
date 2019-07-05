package main

import (
	"fmt"
	"sort"
	"sync"
	"time"

	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
	log "github.com/Financial-Times/go-logger"
)

type healthCheckController struct {
	healthCheckService             healthcheckService
	environment                    string
	measuredServices               map[string]measuredService
	stickyCategoriesFailedServices map[string]int
}

type controller interface {
	buildServicesHealthResult([]string, bool) (fthealth.HealthResult, map[string]category, error)
	runServiceChecksByServiceNames(map[string]service, map[string]category) ([]fthealth.CheckResult, error)
	runServiceChecksFor(map[string]category) ([]fthealth.CheckResult, error)
	buildPodsHealthResult(string) (fthealth.HealthResult, error)
	runPodChecksFor(string) ([]fthealth.CheckResult, error)
	collectChecksFromCachesFor(map[string]category) ([]fthealth.CheckResult, error)
	updateCachedHealth(map[string]service, map[string]category)
	scheduleCheck(measuredService, time.Duration, *time.Timer)
	getIndividualPodHealth(string) ([]byte, string, error)
	addAck(string, string) error
	updateStickyCategory(string, bool) error
	removeAck(string) error
	getEnvironment() string
	getSeverityForService(string, int32) uint8
	getSeverityForPod(string, int32) uint8
	getMeasuredServices() map[string]measuredService
}

func initializeController(environment string) *healthCheckController {
	service := initializeHealthCheckService()
	measuredServices := make(map[string]measuredService)
	stickyCategoriesFailedServices := make(map[string]int)

	return &healthCheckController{
		healthCheckService:             service,
		environment:                    environment,
		measuredServices:               measuredServices,
		stickyCategoriesFailedServices: stickyCategoriesFailedServices,
	}
}

func (c *healthCheckController) getEnvironment() string {
	return c.environment
}

func (c *healthCheckController) updateStickyCategory(categoryName string, isEnabled bool) error {
	return c.healthCheckService.updateCategory(categoryName, isEnabled)
}

func (c *healthCheckController) removeAck(serviceName string) error {
	if !c.healthCheckService.isServicePresent(serviceName) {
		return fmt.Errorf("cannot find service with name %s", serviceName)
	}

	err := c.healthCheckService.removeAck(serviceName)

	if err != nil {
		return fmt.Errorf("failed to remove ack for service %s: %s", serviceName, err.Error())
	}

	return nil
}

func (c *healthCheckController) addAck(serviceName string, ackMessage string) error {
	if !c.healthCheckService.isServicePresent(serviceName) {
		return fmt.Errorf("cannot find service with name %s", serviceName)
	}

	err := c.healthCheckService.addAck(serviceName, ackMessage)

	if err != nil {
		return fmt.Errorf("failed to add ack message [%s] for service %s: %s", ackMessage, serviceName, err.Error())
	}

	return nil
}

func (c *healthCheckController) buildServicesHealthResult(providedCategories []string, useCache bool) (fthealth.HealthResult, map[string]category, error) {
	var checkResults []fthealth.CheckResult
	desc := "Health of the whole cluster of the moment served without cache."
	availableCategories, err := c.healthCheckService.getCategories()
	if err != nil {
		return fthealth.HealthResult{}, nil, fmt.Errorf("cannot build health check result for services: %v", err.Error())
	}

	matchingCategories := getMatchingCategories(providedCategories, availableCategories)

	if useCache {
		desc = "Health of the whole cluster served from cache."
		checkResults, err = c.collectChecksFromCachesFor(matchingCategories)
	} else {
		checkResults, err = c.runServiceChecksFor(matchingCategories)
	}
	if err != nil {
		return fthealth.HealthResult{}, nil, fmt.Errorf("cannot build health check result for services: %v", err.Error())
	}

	c.disableStickyFailingCategories(matchingCategories, checkResults)

	finalOk, finalSeverity := getFinalResult(checkResults, matchingCategories)

	health := fthealth.HealthResult{
		SystemCode:    c.environment,
		Checks:        checkResults,
		Description:   desc,
		Name:          c.environment + " cluster health",
		SchemaVersion: 1,
		Ok:            finalOk,
		Severity:      finalSeverity,
	}

	sort.Sort(byNameComparator(health.Checks))

	return health, matchingCategories, nil
}

func (c *healthCheckController) runServiceChecksByServiceNames(services map[string]service, categories map[string]category) ([]fthealth.CheckResult, error) {
	deployments, err := c.healthCheckService.getDeployments()
	if err != nil {
		return nil, err
	}

	checks := make([]fthealth.Check, 0, len(services))
	for _, service := range services {
		check := newServiceHealthCheck(service, deployments, c.healthCheckService)
		checks = append(checks, check)
	}

	healthChecks := fthealth.RunCheck(fthealth.HealthCheck{
		SystemCode:  "aggregate-healthcheck",
		Name:        "Aggregate Healthcheck",
		Description: "Forced check run",
		Checks:      checks,
	}).Checks

	wg := sync.WaitGroup{}
	for i := range healthChecks {
		wg.Add(1)
		go func(i int) {
			healthCheck := healthChecks[i]
			if !healthCheck.Ok {
				if unhealthyService, ok := services[healthCheck.Name]; ok {
					severity := c.getSeverityForService(healthCheck.Name, unhealthyService.appPort)
					healthChecks[i].Severity = severity
				} else {
					log.Warnf("Cannot compute severity for service with name %s because it was not found. Using default value.", healthCheck.Name)
				}
			}
			wg.Done()
		}(i)
	}
	wg.Wait()

	for _, service := range services {
		if service.ack != "" {
			updateHealthCheckWithAckMsg(healthChecks, service.name, service.ack)
		}
	}

	c.updateCachedHealth(services, categories)

	return healthChecks, nil
}

func (c *healthCheckController) runServiceChecksFor(categories map[string]category) (healthChecks []fthealth.CheckResult, err error) {
	serviceNames := getServiceNamesFromCategories(categories)
	services := c.healthCheckService.getServicesMapByNames(serviceNames)
	healthChecks, err = c.runServiceChecksByServiceNames(services, categories)
	if err != nil {
		return nil, err
	}

	return healthChecks, err
}

func (c *healthCheckController) disableStickyFailingCategories(categories map[string]category, healthChecks []fthealth.CheckResult) {
	for catIndex, category := range categories {
		if !isEnabledAndSticky(category) {
			continue
		}

		for _, serviceName := range category.services {
			for _, healthCheck := range healthChecks {
				if healthCheck.Name == serviceName && !healthCheck.Ok {
					c.stickyCategoriesFailedServices[serviceName]++
					log.Infof("Sticky category [%s] is unhealthy -- check %v/%v.", category.name, c.stickyCategoriesFailedServices[serviceName], category.failureThreshold)

					if c.isCategoryThresholdExceeded(serviceName, category.failureThreshold) {
						log.Infof("Sticky category [%s] is unhealthy, disabling it.", category.name)
						category.isEnabled = false
						categories[catIndex] = category

						err := c.healthCheckService.updateCategory(category.name, false)
						if err != nil {
							log.WithError(err).Errorf("Cannot disable sticky category with name %s.", category.name)
						} else {
							log.Infof("Category [%s] disabled", category.name)
							c.stickyCategoriesFailedServices[serviceName] = 0
						}
					}
				}
			}
		}
	}
}

func (c *healthCheckController) isCategoryThresholdExceeded(serviceName string, failureThreshold int) bool {
	return c.stickyCategoriesFailedServices[serviceName] >= failureThreshold
}

func isEnabledAndSticky(category category) bool {
	return category.isSticky && category.isEnabled
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
	finalSeverity := defaultSeverity

	if len(checkResults) == 0 {
		return false, finalSeverity
	}

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
type byNameComparator []fthealth.CheckResult

func (s byNameComparator) Less(i, j int) bool {
	return s[i].Name < s[j].Name
}

func (s byNameComparator) Len() int {
	return len(s)
}
func (s byNameComparator) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
