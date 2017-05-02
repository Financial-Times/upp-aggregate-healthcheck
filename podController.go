package main

import (
	"errors"
	"fmt"
	fthealth "github.com/Financial-Times/go-fthealth/v1a"
	"io/ioutil"
	"net/http"
	"sort"
	"sync"
)

func (c *healthCheckController) buildPodsHealthResult(serviceName string) (fthealth.HealthResult, error) {
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
	serviceToBeChecked, err := c.healthCheckService.getServiceByName(serviceName)
	if err != nil {
		return []fthealth.CheckResult{}, err
	}

	pods, err := c.healthCheckService.getPodsForService(serviceName)
	if err != nil {
		return []fthealth.CheckResult{}, fmt.Errorf("Cannot get pods for service %s, error was: %s", serviceName, err.Error())
	}

	var checks []fthealth.Check
	for _, pod := range pods {
		check := newPodHealthCheck(pod, serviceToBeChecked, c.healthCheckService)
		checks = append(checks, check)
	}

	healthChecks := fthealth.RunCheck("Forced check run", "", true, checks...).Checks
	wg := sync.WaitGroup{}
	for i := range healthChecks {
		wg.Add(1)
		go func(i int, serviceToBeChecked service) {
			healthCheck := healthChecks[i]
			if !healthCheck.Ok {
				severity := c.getSeverityForPod(healthCheck.Name, serviceToBeChecked.appPort)
				healthChecks[i].Severity = severity
			}

			if serviceToBeChecked.ack != "" {
				healthChecks[i].Ack = serviceToBeChecked.ack
			}
			wg.Done()
		}(i, serviceToBeChecked)
	}
	wg.Wait()

	return healthChecks, nil
}

func (c *healthCheckController) getIndividualPodHealth(podName string) ([]byte, string, error) {
	podToBeChecked, err := c.healthCheckService.getPodByName(podName)
	if err != nil {
		return nil, "", errors.New("Error retrieving pod: " + err.Error())
	}

	service, err := c.healthCheckService.getServiceByName(podToBeChecked.serviceName)

	appPort := defaultAppPort
	if err != nil {
		warnLogger.Printf("Cannot get service with name %s. Using default app port [%d]", podToBeChecked.serviceName, defaultAppPort)
	} else {
		appPort = service.appPort
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
		return nil, "", fmt.Errorf("Healthcheck endpoint returned non-200 status (%v)", resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	defer func() {
		error := resp.Body.Close()
		if error != nil {
			errorLogger.Printf("Cannot close response body reader. Error was: %v", error.Error())
		}
	}()

	if err != nil {
		return nil, "", errors.New("Error reading healthcheck response: " + err.Error())
	}

	contentTypeResponseHeader := resp.Header.Get("Content-Type")

	return body, contentTypeResponseHeader, nil
}
