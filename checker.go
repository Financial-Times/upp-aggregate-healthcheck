package main

import (
	"net/http"
	"errors"
	"fmt"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/labels"
	fthealth "github.com/Financial-Times/go-fthealth/v1a"
)

func (hs *k8sHealthcheckService) checkServiceHealth(serviceName string) error {
	k8sDeployments, err := hs.k8sClient.Extensions().Deployments("default").List(api.ListOptions{LabelSelector: labels.SelectorFromSet(labels.Set{"app":serviceName})})
	if err != nil {
		return errors.New(fmt.Sprintf("Cannot get deployment for service with name: [%s] ", serviceName))
	}

	if len(k8sDeployments.Items) == 0 {
		return errors.New(fmt.Sprintf("Cannot find deployment for service with name [%s]", serviceName))
	}

	noOfUnavailablePods := k8sDeployments.Items[0].Status.UnavailableReplicas

	if noOfUnavailablePods != 0 {
		return errors.New(fmt.Sprintf("There are %v pods unavailable for service with name: [%s] ", noOfUnavailablePods, serviceName))
	}

	return nil
}
func (hs *k8sHealthcheckService) checkPodHealth(pod pod) error {

	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:8080/__gtg", pod.ip), nil)
	if err != nil {
		return errors.New("Error constructing GTG request: " + err.Error())
	}

	resp, err := hs.httpClient.Do(req)
	if err != nil {
		return errors.New("Error performing healthcheck: " + err.Error())
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("GTG endpoint returned non-200 status (%v)", resp.Status)
	}

	return nil
}

func NewPodHealthCheck(pod pod, service service, healthcheckService healthcheckService) fthealth.Check {
	//severity := service.severity

	return fthealth.Check{
		BusinessImpact:   "On its own this failure does not have a business impact but it represents a degradation of the cluster health.",
		Name:             pod.name,
		PanicGuide:       "https://sites.google.com/a/ft.com/technology/systems/dynamic-semantic-publishing/coco/runbook",
		Severity:         service.severity,
		TechnicalSummary: "The service is not healthy. Please check the panic guide.",
		Checker: func() (string, error) {
			return "", healthcheckService.checkPodHealth(pod)
		},
	}
}

func NewServiceHealthCheck(service service, healthcheckService healthcheckService) fthealth.Check {
	return fthealth.Check{
		BusinessImpact:   "On its own this failure does not have a business impact but it represents a degradation of the cluster health.",
		Name:             service.name,
		PanicGuide:       "https://sites.google.com/a/ft.com/technology/systems/dynamic-semantic-publishing/coco/runbook",
		Severity:         service.severity,
		TechnicalSummary: "The service is not healthy. Please check the panic guide.",
		Checker: func() (string, error) {
			return "", healthcheckService.checkServiceHealth(service.name)
		},
	}
}
