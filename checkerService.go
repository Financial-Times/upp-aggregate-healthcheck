package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
)

type healthcheckResponse struct {
	Name   string
	Checks []struct {
		Name     string
		OK       bool
		Severity uint8
	}
}

func (hs *k8sHealthcheckService) checkServiceHealth(service service, deployments map[string]deployment) (string, error) {
	var err error
	pods, err := hs.getPodsForService(service.name)
	if err != nil {
		return "", fmt.Errorf("cannot retrieve pods for service with name %s to perform healthcheck: %s", service.name, err)
	}

	noOfUnavailablePods := 0
	for _, pod := range pods {
		err := hs.checkPodHealth(pod, service.appPort)
		if err != nil {
			noOfUnavailablePods++
		}
	}

	totalNoOfPods := len(pods)
	outputMsg := fmt.Sprintf("%v/%v pods available", totalNoOfPods-noOfUnavailablePods, totalNoOfPods)

	if noOfUnavailablePods != 0 {
		return "", errors.New(outputMsg)
	}
	if service.isDaemon {
		if totalNoOfPods == 0 {
			return "", errors.New(outputMsg)
		}
	} else {
		if _, exists := deployments[service.name]; !exists {
			return "", fmt.Errorf("cannot find deployment for service with name %s: %s", service.name, err)
		}
		if totalNoOfPods == 0 && deployments[service.name].desiredReplicas != 0 {
			return "", errors.New(outputMsg)
		}
	}

	return outputMsg, nil
}

func (hs *k8sHealthcheckService) checkPodHealth(pod pod, appPort int32) error {
	health, err := hs.getHealthChecksForPod(pod, appPort)
	if err != nil {
		errorLogger.Printf("Cannot perform healthcheck for pod with name %s: %s", pod.name, err.Error())
		return errors.New("cannot perform healthcheck for pod")
	}

	for _, check := range health.Checks {
		if !check.OK {
			return fmt.Errorf("failing check is: %s", check.Name)
		}
	}

	return nil
}

func (hs *k8sHealthcheckService) getIndividualPodSeverity(pod pod, appPort int32) (uint8, error) {
	health, err := hs.getHealthChecksForPod(pod, appPort)

	if err != nil {
		return defaultSeverity, fmt.Errorf("cannot get severity for pod with name %s: %s", pod.name, err.Error())
	}

	finalSeverity := uint8(2)
	for _, check := range health.Checks {
		if !check.OK {
			if check.Severity < finalSeverity {
				return check.Severity, nil
			}
		}
	}

	return finalSeverity, nil
}

func (hs *k8sHealthcheckService) getHealthChecksForPod(pod pod, appPort int32) (healthcheckResponse, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:%d/__health", pod.ip, appPort), nil)
	if err != nil {
		return healthcheckResponse{}, errors.New("Error constructing healthcheck request: " + err.Error())
	}

	req.Header.Set("Accept", "application/json")
	resp, err := hs.httpClient.Do(req)
	if err != nil {
		return healthcheckResponse{}, errors.New("Error performing healthcheck request: " + err.Error())
	}

	defer func() {
		error := resp.Body.Close()
		if error != nil {
			errorLogger.Printf("Cannot close response body reader. Error was: %v", error.Error())
		}
	}()

	if resp.StatusCode != 200 {
		return healthcheckResponse{}, fmt.Errorf("healthcheck endpoint returned non-200 status (%v)", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return healthcheckResponse{}, errors.New("Error reading healthcheck response: " + err.Error())
	}

	health := &healthcheckResponse{}
	if err := json.Unmarshal(body, &health); err != nil {
		return healthcheckResponse{}, errors.New("Error parsing healthcheck response: " + err.Error())
	}

	return *health, nil
}

func newPodHealthCheck(pod pod, service service, healthcheckService healthcheckService) fthealth.Check {
	var checkName string
	if service.isDaemon {
		checkName = fmt.Sprintf("%s (%s)", pod.name, pod.node)
	} else {
		checkName = pod.name
	}

	return fthealth.Check{
		BusinessImpact:   "On its own this failure does not have a business impact but it represents a degradation of the cluster health.",
		Name:             checkName,
		PanicGuide:       "https://sites.google.com/a/ft.com/technology/systems/dynamic-semantic-publishing/coco/runbook",
		Severity:         defaultSeverity,
		TechnicalSummary: "The pod is not healthy. Please check the panic guide.",
		Checker: func() (string, error) {
			return "", healthcheckService.checkPodHealth(pod, service.appPort)
		},
	}
}

func newServiceHealthCheck(service service, deployments map[string]deployment, healthcheckService healthcheckService) fthealth.Check {
	return fthealth.Check{
		BusinessImpact:   "On its own this failure does not have a business impact but it represents a degradation of the cluster health.",
		Name:             service.name,
		PanicGuide:       "https://sites.google.com/a/ft.com/technology/systems/dynamic-semantic-publishing/coco/runbook",
		Severity:         defaultSeverity,
		TechnicalSummary: "The service is not healthy. Please check the panic guide.",
		Checker: func() (string, error) {
			return healthcheckService.checkServiceHealth(service, deployments)
		},
	}
}
