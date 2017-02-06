package main

import (
	"time"
	"net/http"
	fthealth "github.com/Financial-Times/go-fthealth/v1a"
	"errors"
	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/rest"
)

type k8sHealthcheckService struct {
	k8sClient  *kubernetes.Clientset
	httpClient *http.Client
}

type healthcheckService interface {
	getCategories() map[string]category
	getServicesByNames([]string) []service
	getPodsForService(string) []pod
	checkServiceHealth(string) error
	checkPodHealth(pod) error
	getHttpClient() *http.Client
}

type pod struct {
	name string
	ip   string
}

type service struct {
	name      string
	severity  uint8
	isEnabled bool
}

type category struct {
	name          string
	services      []string
	refreshPeriod time.Duration
	isSticky      bool
}

func (healthCheckService *k8sHealthcheckService) checkServiceHealth(string) error {
	return errors.New("Error reading healthcheck response: ")
}

func (healthCheckService *k8sHealthcheckService) checkPodHealth(pod pod) error {
	return errors.New("Error reading healthcheck response: ")
}

//todo: take only the services that have healthcheck
//TODO: if the list of service names is empty, it means that we are in the default category so take all the services that have healthcheck
func (healthCheckService *k8sHealthcheckService) getServicesByNames(serviceNames []string) []service {
	services := []service{
		{
			name: "test-service-name",
			severity: 1,
		},
		{
			name: "test-service-name-2",
			severity: 2,
		},
	}

	return services
}

func (healthCheckService *k8sHealthcheckService) getPodsForService(serviceName string) []pod {
	//todo: take only the pods that belong to the service with name serviceName
	pods := []pod{
		{
			name: "test-pod-name-8425234-9hdfg ",
			ip: "10.2.51.2",
		},
		{
			name: "test2-pod-name-8425234-9hdfg ",
			ip: "10.2.51.3",
		},
	}

	return pods
}

func (healthCheckService *k8sHealthcheckService) getCategories() map[string]category {
	categories := make(map[string]category)

	categories["default"] = category{
		name: "default",
	}

	categories["content-read"] = category{
		name: "content-read",
	}
	return categories
}

func (healthCheckService *k8sHealthcheckService) getHttpClient() *http.Client {
	return healthCheckService.httpClient
}

func InitializeHealthCheckService() *k8sHealthcheckService {
	httpClient := &http.Client{
		Timeout:   5 * time.Second,
	}

	//todo: use kubernetes client from branch release-1.5
	//todo: uncomment this
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	// creates the clientset
	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		errorLogger.Printf("Failed to create k8s client, error was: %v", err.Error())
	}

	return &k8sHealthcheckService{
		httpClient:httpClient,
		k8sClient : k8sClient,
	}
}

func NewPodHealthCheck(pod pod, healthcheckService healthcheckService) fthealth.Check {
	//severity := service.severity

	return fthealth.Check{
		BusinessImpact:   "On its own this failure does not have a business impact but it represents a degradation of the cluster health.",
		Name:             pod.name,
		PanicGuide:       "https://sites.google.com/a/ft.com/technology/systems/dynamic-semantic-publishing/coco/runbook",
		Severity:         1, //todo:
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

