package main

import (
	"fmt"
	"math"
	"reflect"
	"time"

	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
	log "github.com/Financial-Times/go-logger"
)

const (
	defaultRefreshPeriod = 60 * time.Second
)

func newMeasuredService(service service) measuredService {
	cachedHealth := newCachedHealth()
	go cachedHealth.maintainLatest()

	cachedHealthMetric := newCachedHealth()
	go cachedHealthMetric.maintainLatest()
	return measuredService{
		service:            service,
		cachedHealth:       cachedHealth,
		cachedHealthMetric: cachedHealthMetric,
	}
}

func (c *healthCheckController) collectChecksFromCachesFor(categories map[string]category) ([]fthealth.CheckResult, error) {
	var checkResults []fthealth.CheckResult
	serviceNames := getServiceNamesFromCategories(categories)
	services := c.healthCheckService.getServicesMapByNames(serviceNames)
	servicesThatAreNotInCache := make(map[string]service)
	for _, service := range services {
		if mService, ok := c.measuredServices[service.name]; ok {
			checkResult := <-mService.cachedHealth.toReadFromCache
			checkResults = append(checkResults, checkResult)
		} else {
			servicesThatAreNotInCache[service.name] = service
		}
	}

	if len(servicesThatAreNotInCache) != 0 {
		notCachedChecks, err := c.runServiceChecksByServiceNames(servicesThatAreNotInCache, categories)
		if err != nil {
			return nil, err
		}
		checkResults = append(checkResults, notCachedChecks...)
	}

	return checkResults, nil
}

func (c *healthCheckController) updateCachedHealth(services map[string]service, categories map[string]category) {
	// adding new services, not touching existing
	refreshPeriod := findShortestPeriod(categories)
	categories, err := c.healthCheckService.getCategories()
	if err != nil {
		log.WithError(err).Warn("Cannot read categories. Using minimum refresh period for services")
	}
	for _, service := range services {
		if mService, ok := c.measuredServices[service.name]; !ok || !reflect.DeepEqual(service, c.measuredServices[service.name].service) {
			if ok {
				mService.cachedHealth.terminate <- true
			}
			newMService := newMeasuredService(service)
			c.measuredServices[service.name] = newMService
			if categories != nil {
				for _, category := range categories {
					if isStringInSlice(service.name, category.services) {
						refreshPeriod = category.refreshPeriod
						break
					}
				}
			}
			log.Infof("Scheduling check for service [%s] with refresh period [%v].\n", service.name, refreshPeriod)
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

	if !c.healthCheckService.isServicePresent(mService.service.name) {
		log.Infof("Service with name %s doesn't exist anymore, removing it from cache", mService.service.name)
		delete(c.measuredServices, mService.service.name)
		mService.cachedHealth.terminate <- true
		return
	}

	// run check
	deployments, err := c.healthCheckService.getDeployments()
	if err != nil {
		log.WithError(err).Error("Cannot run scheduled health check")
		return
	}

	serviceToBeChecked := mService.service

	checks := []fthealth.Check{newServiceHealthCheck(serviceToBeChecked, deployments, c.healthCheckService)}

	checkResult := fthealth.RunCheck(fthealth.HealthCheck{
		SystemCode:  serviceToBeChecked.name,
		Name:        serviceToBeChecked.name,
		Description: fmt.Sprintf("Checks the health of %v", serviceToBeChecked.name),
		Checks:      checks,
	}).Checks[0]

	checkResult.Ack = serviceToBeChecked.ack

	if !checkResult.Ok {
		severity := c.getSeverityForService(checkResult.Name, serviceToBeChecked.appPort)
		checkResult.Severity = severity
	}

	mService.cachedHealth.toWriteToCache <- checkResult
	mService.cachedHealthMetric.toWriteToCache <- checkResult

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
