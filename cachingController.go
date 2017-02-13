package main

import (
	fthealth "github.com/Financial-Times/go-fthealth/v1a"
	"time"
	"fmt"
	"reflect"
)

const (
	defaultRefreshPeriod = time.Duration(60 * time.Second)
)

func NewMeasuredService(service *service) MeasuredService {
	cachedHealth := NewCachedHealth()
	//bufferedHealths := NewBufferedHealths()
	go cachedHealth.maintainLatest()
	return MeasuredService{service, cachedHealth}
}

func (c *healthCheckController)  collectChecksFromCachesFor(categories map[string]category) ([]fthealth.CheckResult, map[string][]fthealth.CheckResult) {
	var checkResults []fthealth.CheckResult
	var servicesThatAreNotInCache []string
	categorisedResults := make(map[string][]fthealth.CheckResult)
	serviceNames := getServiceNamesFromCategories(categories)
	for _, serviceName := range serviceNames {
		if mService, ok := c.measuredServices[serviceName]; ok {
			infoLogger.Printf("Found service with name %s in cache", serviceName)
			checkResult := <-mService.cachedHealth.toReadFromCache
			checkResults = append(checkResults, checkResult)
		} else {
			infoLogger.Printf("Service with name %s was not found in cache", serviceName)
			servicesThatAreNotInCache = append(servicesThatAreNotInCache, serviceName)
		}
	}

	notCachedChecks := c.runServiceChecksByServiceNames(servicesThatAreNotInCache)

	for _, check := range notCachedChecks {
		checkResults = append(checkResults, check)
	}

	//todo: add sticky functionality here. see line with for catIndex, category := range categories {

	return checkResults, categorisedResults
}

func (c *healthCheckController) updateCachedHealth(services []service) {
	// adding new services, not touching existing
	for _, service := range services {
		if mService, ok := c.measuredServices[service.name]; !ok || !reflect.DeepEqual(service, c.measuredServices[service.name].service) {
			if ok {
				mService.cachedHealth.terminate <- true
			}
			newMService := NewMeasuredService(&service)
			c.measuredServices[service.name] = newMService
			go c.scheduleCheck(&newMService, time.NewTimer(0))
		}
	}
}

func (c *healthCheckController) scheduleCheck(mService *MeasuredService, timer *time.Timer) {
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
		NewServiceHealthCheck(*mService.service, c.healthCheckService))

	healthResult.Checks[0].Ack = mService.service.ack

	mService.cachedHealth.toWriteToCache <- healthResult

	waitDuration := findShortestPeriod(*mService.service)
	go c.scheduleCheck(mService, time.NewTimer(waitDuration))
}

func findShortestPeriod(service service) time.Duration {
	return defaultRefreshPeriod
	//TODO: implement this.
}
