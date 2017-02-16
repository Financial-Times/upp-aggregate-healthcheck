package main

import (
	"errors"
	"fmt"
	fthealth "github.com/Financial-Times/go-fthealth/v1a"
	"io/ioutil"
	"net/http"
	"sort"
)

func (c *healthCheckController) buildPodsHealthResult(serviceName string, useCache bool) (fthealth.HealthResult, error) {
	desc := fmt.Sprintf("Health of pods that are under service %s served without cache.", serviceName)

	checkResults, err := c.runPodChecksFor(serviceName)

	if err != nil {
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

func (c *healthCheckController) runPodChecksFor(serviceName string) ([]fthealth.CheckResult, error) {
	pods, err := c.healthCheckService.getPodsForService(serviceName)

	if err != nil {
		return []fthealth.CheckResult{}, fmt.Errorf("Cannot get pods for service %s, error was: %s", serviceName, err.Error())
	}

	services := c.healthCheckService.getServicesByNames([]string{serviceName})

	if len(services) == 0 {
		return []fthealth.CheckResult{}, fmt.Errorf("Cannot find service with name %s", serviceName)
	}

	var checks []fthealth.Check
	service := services[0]
	for _, pod := range pods {
		check := newPodHealthCheck(pod, service, c.healthCheckService)
		checks = append(checks, check)
	}

	healthChecks := fthealth.RunCheck("Forced check run", "", true, checks...).Checks

	for i, check := range healthChecks {
		if check.Ok != true {
			severity := c.getSeverityForPod(check.Name)
			healthChecks[i].Severity = severity
		}

		if service.ack != "" {
			healthChecks[i].Ack = service.ack
		}
	}

	return healthChecks, nil
}

func (c *healthCheckController) getIndividualPodHealth(podName string) ([]byte, error) {

	pod, err := c.healthCheckService.getPodByName(podName)
	if err != nil {
		return nil, errors.New("Error retrieving pod: " + err.Error())
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:8080/__health", pod.ip), nil)
	if err != nil {
		return nil, errors.New("Error constructing healthcheck request: " + err.Error())
	}

	resp, err := c.healthCheckService.getHTTPClient().Do(req)
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
