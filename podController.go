package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"

	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
	log "github.com/Financial-Times/go-logger"
)

func (c *healthCheckController) buildPodsHealthResult(ctx context.Context, serviceName string) (fthealth.HealthResult, error) {
	desc := fmt.Sprintf("Health of pods that are under service %s served without cache.", serviceName)

	checkResults, err := c.runPodChecksFor(ctx, serviceName)

	if err != nil {
		// nolint:staticcheck
		return fthealth.HealthResult{}, fmt.Errorf("Cannot perform pod checks for service %s, error was: %s", serviceName, err.Error())
	}

	finalOk, finalSeverity := getFinalResult(checkResults, nil)

	health := fthealth.HealthResult{
		Checks:        checkResults,
		Description:   desc,
		Name:          c.environment + " cluster health",
		SchemaVersion: 1,
		Ok:            finalOk,
		Severity:      finalSeverity,
	}

	sort.Sort(byNameComparator(health.Checks))

	return health, nil
}

func (c *healthCheckController) runPodChecksFor(ctx context.Context, serviceName string) ([]fthealth.CheckResult, error) {
	serviceToBeChecked, err := c.healthCheckService.getServiceByName(serviceName)
	if err != nil {
		return []fthealth.CheckResult{}, err
	}

	pods, err := c.healthCheckService.getPodsForService(ctx, serviceName)
	if err != nil {
		// nolint:staticcheck
		return []fthealth.CheckResult{}, fmt.Errorf("Cannot get pods for service %s, error was: %s", serviceName, err.Error())
	}

	checks := make([]fthealth.Check, len(pods))
	for i, currentPod := range pods {
		check := newPodHealthCheck(currentPod, serviceToBeChecked, c.healthCheckService)
		checks[i] = check
	}

	healthChecks := fthealth.RunCheck(fthealth.HealthCheck{
		SystemCode:  "aggregate-healthcheck",
		Name:        "Aggregate Healthcheck",
		Description: "Forced check run",
		Checks:      checks,
	}).Checks

	wg := sync.WaitGroup{}
	wg.Add(len(healthChecks))
	for i := range healthChecks {
		go func(i int, serviceToBeChecked service) {
			defer wg.Done()
			healthCheck := healthChecks[i]
			if !healthCheck.Ok {
				severity := c.getSeverityForPod(ctx, healthCheck.Name, serviceToBeChecked.appPort)
				healthChecks[i].Severity = severity
			}

			if serviceToBeChecked.ack != "" {
				healthChecks[i].Ack = serviceToBeChecked.ack
			}
		}(i, serviceToBeChecked)
	}
	wg.Wait()

	return healthChecks, nil
}

func (c *healthCheckController) getIndividualPodHealth(ctx context.Context, podName string) ([]byte, string, error) {
	podToBeChecked, err := c.healthCheckService.getPodByName(ctx, podName)
	if err != nil {
		return nil, "", errors.New("Error retrieving pod: " + err.Error())
	}

	srv, err := c.healthCheckService.getServiceByName(podToBeChecked.serviceName)

	appPort := defaultAppPort
	if err != nil {
		log.Warnf("Cannot get service with name %s. Using default app port [%d]", podToBeChecked.serviceName, defaultAppPort)
	} else {
		appPort = srv.appPort
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:%d/__health", podToBeChecked.ip, appPort), nil)
	if err != nil {
		return nil, "", errors.New("Error constructing healthcheck request: " + err.Error())
	}

	resp, err := c.healthCheckService.getHTTPClient().Do(req)
	if err != nil {
		return nil, "", errors.New("Error performing healthcheck: " + err.Error())
	}

	if resp.StatusCode != 200 {
		// nolint:staticcheck
		return nil, "", fmt.Errorf("Healthcheck endpoint returned non-200 status (%v)", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	defer func() {
		if err = resp.Body.Close(); err != nil {
			log.WithError(err).Error("Cannot close response body reader.")
		}
	}()

	if err != nil {
		return nil, "", errors.New("Error reading healthcheck response: " + err.Error())
	}

	contentTypeResponseHeader := resp.Header.Get("Content-Type")

	return body, contentTypeResponseHeader, err
}
