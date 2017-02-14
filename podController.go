package main

import (
	"sort"
	fthealth "github.com/Financial-Times/go-fthealth/v1a"
	"fmt"
	"errors"
	"io/ioutil"
	"net/http"
)

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

	finalOk, finalSeverity := getFinalResult(checkResults, nil)

	health := fthealth.HealthResult{
		Checks:        checkResults,
		Description:   desc,
		Name:          c.environment + " cluster health",
		SchemaVersion: 1,
		Ok:            finalOk,
		Severity:      finalSeverity,
	}

	sort.Sort(ByNameComparator(health.Checks))

	return health
}

func (c *healthCheckController) runPodChecksFor(serviceName string) ([]fthealth.CheckResult, map[string][]fthealth.CheckResult) {
	categorisedResults := make(map[string][]fthealth.CheckResult)

	pods, err := c.healthCheckService.getPodsForService(serviceName)

	if err != nil {
		//TODO: send the error further
	}

	services := c.healthCheckService.getServicesByNames([]string{serviceName})

	if len(services) == 0 {
		//todo: throw error
	}

	var checks []fthealth.Check

	for _, pod := range pods {
		//todo: add services[0] to this call to take severity+ack from it
		check := NewPodHealthCheck(pod, services[0], c.healthCheckService)
		checks = append(checks, check)
	}

	healthChecks := fthealth.RunCheck("Forced check run", "", true, checks...).Checks

	for i, check := range healthChecks {
		if check.Ok != true {
			severity := c.getSeverityForPod(check.Name)
			//todo: remove this log
			infoLogger.Printf("Severity for pod with name [%s] is [%s]", check.Name, severity)
			healthChecks[i].Severity = severity
		}
	}
	return healthChecks, categorisedResults
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
