package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
	log "github.com/Financial-Times/go-logger"
	"github.com/Financial-Times/kafka-client-go/v3"
)

type healthcheckResponse struct {
	Name   string
	Checks []struct {
		Name             string
		OK               bool
		Severity         uint8
		TechnicalSummary string
	}
}

func (hs *k8sHealthcheckService) checkServiceHealth(ctx context.Context, service service, deployments map[string]deployment, ignoreLagWarning bool) (string, error) {
	pods, err := hs.getPodsForService(ctx, service.name)
	if err != nil {
		return "", fmt.Errorf("cannot retrieve pods for service with name %s to perform healthcheck: %s", service.name, err.Error())
	}

	noOfUnavailablePods := 0
	for _, pod := range pods {
		if err := hs.checkPodHealth(pod, service.appPort, ignoreLagWarning); err != nil {
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
			return "", fmt.Errorf("cannot find deployment for service with name %s", service.name)
		}
		if totalNoOfPods == 0 && deployments[service.name].desiredReplicas != 0 {
			return "", errors.New(outputMsg)
		}
	}

	return outputMsg, nil
}

func (hs *k8sHealthcheckService) checkPodHealth(pod pod, appPort int32, ignoreLagWarning bool) error {
	health, err := hs.getHealthChecksForPod(pod, appPort)
	if err != nil {
		log.WithError(err).Errorf("Cannot perform healthcheck for pod with name %s", pod.name)
		return errors.New("cannot perform healthcheck for pod")
	}

	for _, check := range health.Checks {
		if !check.OK {
			// checks if the error is lag and if it should be skipped
			if ignoreLagWarning && check.TechnicalSummary == kafka.LagTechnicalSummary {
				log.Debugf("Service %s is lagging behind when reading from Kafka", health.Name)
				continue
			}
			return fmt.Errorf("failing check is: %s", check.Name)
		}
	}

	return nil
}

func (hs *k8sHealthcheckService) getIndividualPodSeverity(pod pod, appPort int32) (uint8, bool, error) {
	health, err := hs.getHealthChecksForPod(pod, appPort)

	if err != nil {
		return defaultSeverity, false, fmt.Errorf("cannot get severity for pod with name %s: %s", pod.name, err.Error())
	}

	finalSeverity := uint8(2)
	checkFailed := false
	for _, check := range health.Checks {
		if !check.OK {
			checkFailed = true
			if check.Severity < finalSeverity {
				return check.Severity, checkFailed, nil
			}
		}
	}

	return finalSeverity, checkFailed, nil
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
		err := resp.Body.Close()
		if err != nil {
			log.WithError(err).Errorf("Cannot close response body reader.")
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
		PanicGuide:       "https://runbooks.in.ft.com/upp-aggregate-healthcheck",
		Severity:         defaultSeverity,
		TechnicalSummary: "The pod is not healthy. Please check the panic guide.",
		Checker: func() (string, error) {
			return "", healthcheckService.checkPodHealth(pod, service.appPort, true)
		},
	}
}

func newServiceHealthCheck(ctx context.Context, service service, deployments map[string]deployment, healthcheckService healthcheckService, ignoreLagWarning bool) fthealth.Check {
	return fthealth.Check{
		BusinessImpact:   "On its own this failure does not have a business impact but it represents a degradation of the cluster health.",
		Name:             service.name,
		PanicGuide:       "https://runbooks.in.ft.com/upp-aggregate-healthcheck",
		Severity:         defaultSeverity,
		TechnicalSummary: "The service is not healthy. Please check the panic guide.",
		Checker: func() (string, error) {
			return healthcheckService.checkServiceHealth(ctx, service, deployments, ignoreLagWarning)
		},
	}
}
