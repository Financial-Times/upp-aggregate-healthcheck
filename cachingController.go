package main

import (
	"fmt"
	fthealth "github.com/Financial-Times/go-fthealth/v1a"
	"math"
	"reflect"
	"time"
)

const (
	defaultRefreshPeriod = time.Duration(60 * time.Second)
)

func newMeasuredService(service service) measuredService {
	cachedHealth := newCachedHealth()
	go cachedHealth.maintainLatest()
	return measuredService{service, cachedHealth}
}

func (c *healthCheckController) collectChecksFromCachesFor(categories map[string]category) ([]fthealth.CheckResult, map[string][]fthealth.CheckResult) {
	var checkResults []fthealth.CheckResult
	categorisedResults := make(map[string][]fthealth.CheckResult)
	serviceNames := getServiceNamesFromCategories(categories)
	services := c.healthCheckService.getServicesByNames(serviceNames)
	var servicesThatAreNotInCache []service
	for _, service := range services {
		if mService, ok := c.measuredServices[service.name]; ok {
			infoLogger.Printf("Found service with name %s in cache", service.name)
			checkResult := <-mService.cachedHealth.toReadFromCache
			checkResults = append(checkResults, checkResult)
		} else {
			infoLogger.Printf("Service with name %s was not found in cache", service.name)
			servicesThatAreNotInCache = append(servicesThatAreNotInCache, service)
		}
	}

	if len(servicesThatAreNotInCache) != 0 {
		notCachedChecks := c.runServiceChecksByServiceNames(servicesThatAreNotInCache, categories)

		for _, check := range notCachedChecks {
			checkResults = append(checkResults, check)
		}
	}

	return checkResults, categorisedResults
}

func (c *healthCheckController) updateCachedHealth(services []service, categories map[string]category) {
	// adding new services, not touching existing
	for _, service := range services {
		if mService, ok := c.measuredServices[service.name]; !ok || !reflect.DeepEqual(service, c.measuredServices[service.name].service) {
			if ok {
				mService.cachedHealth.terminate <- true
			}
			newMService := newMeasuredService(service)
			c.measuredServices[service.name] = newMService
			refreshPeriod := findShortestPeriod(categories)
			go c.scheduleCheck(newMService, refreshPeriod, time.NewTimer(0))
		}
	}
}

func (c *healthCheckController) scheduleCheck(mService measuredService, refreshPeriod time.Duration, timer *time.Timer) {
	// wait
	select {
	case <-mService.cachedHealth.terminate:
		return
	case <-timer.C:
	}

	// run check
	healthResult := fthealth.RunCheck(mService.service.name,
		fmt.Sprintf("Checks the health of %v", mService.service.name),
		true,
		newServiceHealthCheck(mService.service, c.healthCheckService)).Checks[0]

	healthResult.Ack = mService.service.ack

	if healthResult.Ok != true {
		severity := c.getSeverityForService(healthResult.Name)
		healthResult.Severity = severity
	}

	mService.cachedHealth.toWriteToCache <- healthResult

	go c.scheduleCheck(mService, refreshPeriod, time.NewTimer(refreshPeriod))
}

func findShortestPeriod(categories map[string]category) time.Duration {
	if len(categories) == 0 {
		return defaultRefreshPeriod
	}

	minRefreshPeriod := time.Duration(math.MaxInt32 * time.Second)

	for _, category := range categories {
		if category.refreshPeriod.Seconds() < minRefreshPeriod.Seconds() {
			minRefreshPeriod = category.refreshPeriod
		}
	}

	return minRefreshPeriod
}
