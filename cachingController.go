package main

import (
	"fmt"
	fthealth "github.com/Financial-Times/go-fthealth/v1a"
	"math"
	"reflect"
	"time"
)

const (
	defaultRefreshPeriod = 60 * time.Second
)

func newMeasuredService(service service) measuredService {
	cachedHealth := newCachedHealth()
	bufferedHealths := NewBufferedHealths()
	go cachedHealth.maintainLatest()
	return measuredService{service, cachedHealth, bufferedHealths}
}

func (c *healthCheckController) collectChecksFromCachesFor(categories map[string]category) ([]fthealth.CheckResult, map[string][]fthealth.CheckResult) {
	var checkResults []fthealth.CheckResult
	categorisedResults := make(map[string][]fthealth.CheckResult)
	serviceNames := getServiceNamesFromCategories(categories)
	services := c.healthCheckService.getServicesByNames(serviceNames)
	var servicesThatAreNotInCache []service
	for _, service := range services {
		if mService, ok := c.measuredServices[service.name]; ok {
			checkResult := <-mService.cachedHealth.toReadFromCache
			checkResults = append(checkResults, checkResult)
		} else {
			servicesThatAreNotInCache = append(servicesThatAreNotInCache, service)
		}
	}

	if len(servicesThatAreNotInCache) != 0 {
		notCachedChecks := c.runServiceChecksByServiceNames(servicesThatAreNotInCache, categories)
		checkResults = append(checkResults, notCachedChecks...)
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

	//removing services that do not exist anymore
	if _, ok := categories["default"]; ok {
		for measuredServiceName, measuredService := range c.measuredServices {
			if !isServiceInList(measuredServiceName, services) {
				infoLogger.Printf("Deleting service with name %s from cache, since it doesn't exist anymore", measuredServiceName)
				delete(c.measuredServices, measuredServiceName)
				measuredService.cachedHealth.terminate <- true
			}
		}
	}
}

func isServiceInList(serviceName string, services []service) bool {
	for _, s := range services {
		if s.name == serviceName {
			return true
		}
	}

	return false
}


func (c *healthCheckController) scheduleCheck(mService measuredService, refreshPeriod time.Duration, timer *time.Timer) {
	// wait
	select {
	case <-mService.cachedHealth.terminate:
		return
	case <-timer.C:
	}

	// run check
	checkResult := fthealth.RunCheck(mService.service.name,
		fmt.Sprintf("Checks the health of %v", mService.service.name),
		true,
		newServiceHealthCheck(mService.service, c.healthCheckService)).Checks[0]

	checkResult.Ack = mService.service.ack

	if !checkResult.Ok {
		severity := c.getSeverityForService(checkResult.Name, mService.service.appPort)
		checkResult.Severity = severity
	}

	mService.cachedHealth.toWriteToCache <- checkResult
	select {
	case mService.bufferedHealths.buffer <- checkResult:
	default:
	}

	go c.scheduleCheck(mService, refreshPeriod, time.NewTimer(refreshPeriod))
}

func (c *healthCheckController) getMeasuredServices() map[string]measuredService {
	return c.measuredServices
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
