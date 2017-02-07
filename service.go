package main

import (
	"time"
	"net/http"
	fthealth "github.com/Financial-Times/go-fthealth/v1a"
	"errors"
	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/rest"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"fmt"
	"strings"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/labels"
	"strconv"
	"k8s.io/client-go/1.5/pkg/fields"
)

type k8sHealthcheckService struct {
	k8sClient  *kubernetes.Clientset
	httpClient *http.Client
}

type healthcheckService interface {
	getCategories() (map[string]category, error)
	getServicesByNames([]string) []service
	getPodsForService(string) []pod
	getPodByName(string) (pod, error)
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
	ack       string
}

type category struct {
	name          string
	services      []string
	refreshPeriod time.Duration
	isSticky      bool
}

const (
	defaultRefreshRate = 60
	defaultServiceSeverity = 2
)

func (hs *k8sHealthcheckService) getPodByName(podName string) (pod, error) {

	k8sPods, err := hs.k8sClient.Core().Pods("default").List(api.ListOptions{FieldSelector: fields.SelectorFromSet(fields.Set{"name":podName})})
	if err != nil {
		return pod{}, errors.New(fmt.Sprintf("Failed to get the pod from k8s cluster, error was %v", err.Error()))
	}

	if len(k8sPods.Items) == 0 {
		return pod{}, errors.New(fmt.Sprintf("Pod with name %s was not found in cluster, error was %v", podName, err.Error()))
	}

	pod := populatePod(k8sPods.Items[0])
	return pod, nil
}
func (hs *k8sHealthcheckService) checkServiceHealth(serviceName string) error {
	infoLogger.Printf("Checking service with name: %s", serviceName) //todo: delete this

	k8sPods, err := hs.k8sClient.Extensions().Deployments("default").List(api.ListOptions{LabelSelector: labels.SelectorFromSet(labels.Set{"app":serviceName})})
	if err != nil {
		return errors.New(fmt.Sprintf("Cannot get deployment for service with name: [%s] ", serviceName))
	}

	if len(k8sPods.Items) == 0 {
		return errors.New(fmt.Sprintf("Cannot find deployment for service with name [%s]", serviceName))
	}

	noOfUnavailablePods := k8sPods.Items[0].Status.UnavailableReplicas

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

//todo: take only the services that have healthcheck
func (hs *k8sHealthcheckService) getServicesByNames(serviceNames []string) []service {
	//todo: list only services that have hasHealthCheck=true label.
	//k8sServices, err := healthCheckService.k8sClient.Core().Services("default").List(api.ListOptions{LabelSelector: labels.SelectorFromSet(labels.Set{"hasHealthCheck":"true"})})

	//todo: return _,err instead of empty services list in case of error.
	k8sServices, err := hs.k8sClient.Core().Services("default").List(api.ListOptions{})
	if err != nil {
		errorLogger.Printf("Failed to get the list of services from k8s cluster, error was %v", err.Error())
		return []service{}
	}

	//if the list of service names is empty, it means that we are in the default category so we take all the services that have healthcheck
	if len(serviceNames) == 0 {
		return getAllServices(k8sServices.Items)
	}

	return getServicesWithNames(k8sServices.Items, serviceNames)
}

func (hs *k8sHealthcheckService) getPodsForService(serviceName string) []pod {

	//todo: return _,err instead of empty services list in case of error.
	k8sPods, err := hs.k8sClient.Core().Pods("default").List(api.ListOptions{LabelSelector: labels.SelectorFromSet(labels.Set{"app":serviceName})})
	if err != nil {
		errorLogger.Printf("Failed to get the list of services from k8s cluster, error was %v", err.Error())
		return []pod{}
	}

	pods := []pod{}
	for _, k8sPod := range k8sPods.Items {
		pod := populatePod(k8sPod)
		pods = append(pods, pod)
	}

	return pods
}

func (hs *k8sHealthcheckService) getCategories() (map[string]category, error) {
	categories := make(map[string]category)

	labelSelector := labels.SelectorFromSet(labels.Set{"healthcheck-categories-for":"aggregate-healthcheck"})
	k8sCategories, err := hs.k8sClient.Core().ConfigMaps("default").List(api.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Failed to get the categories from kubernetes. Error was: %v", err))
	}

	for _, k8sCategory := range k8sCategories.Items {
		category := populateCategory(k8sCategory.Data)
		warnLogger.Printf("Found category: %v \n", category) //TODO: remove this.
		categories[category.name] = category
	}

	return categories, nil
}

func (hs *k8sHealthcheckService) getHttpClient() *http.Client {
	return hs.httpClient
}

func populateCategory(k8sCatData map[string]string) category {
	categoryName := k8sCatData["category.name"]
	isSticky, err := strconv.ParseBool(k8sCatData["category.issticky"])
	if err != nil {
		warnLogger.Printf("Failed to convert isSticky flag from string to bool for category with name [%s]. Using default value of false. Error was: %v", categoryName, err)
		isSticky = false
	}

	refreshRateSeconds, err := strconv.ParseInt(k8sCatData["category.refreshrate"], 10, 64)
	if err != nil {
		warnLogger.Printf("Failed to convert refreshRate from string to int for category with name [%s]. Using default refresh rate. Error was: %v", categoryName, err)
		refreshRateSeconds = defaultRefreshRate
	}

	refreshRatePeriod := time.Duration(refreshRateSeconds * int64(time.Second))

	return category{
		name:categoryName,
		services:      strings.Split(k8sCatData["category.services"], ","), //todo: what if the array of strings will contain also white spaces near service names? remove the white spaces from the resulting array of strings.
		refreshPeriod: refreshRatePeriod,
		isSticky:      isSticky,
	}
}

func populatePod(k8sPod v1.Pod) pod {
	return pod{
		name:k8sPod.Name,
		ip:k8sPod.Status.PodIP,
	}
}

func getServiceByName(k8sServices []v1.Service, serviceName string) (v1.Service, error) {
	for _, k8sService := range k8sServices {
		if k8sService.Name == serviceName {
			return k8sService, nil
		}
	}

	return v1.Service{}, errors.New(fmt.Sprintf("Cannot find k8sService with name %s", serviceName))
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

func getServicesWithNames(k8sServices []v1.Service, serviceNames []string) []service {
	services := []service{}

	for _, serviceName := range serviceNames {
		k8sService, err := getServiceByName(k8sServices, serviceName)
		if err != nil {
			errorLogger.Printf("Service with name [%s] cannot be found in k8s services. Error was: %v", serviceName, err)
		} else {
			service := populateService(k8sService)
			services = append(services, service)
		}
	}

	return services
}

func getAllServices(k8sServices []v1.Service) []service {
	infoLogger.Print("Using category default, retrieving all services.")
	services := []service{}
	for _, k8sService := range k8sServices {
		service := populateService(k8sService)
		services = append(services, service)
	}

	return services
}

func populateService(k8sService v1.Service) service {
	severity, err := strconv.ParseUint(k8sService.GetLabels()["healthcheckSeverity"], 10, 8)
	if err != nil {
		warnLogger.Printf("Cannot parse severity level from k8s label for service with name [%s], using default severity level of 'warning', error was %v", k8sService.Name, err.Error())
		severity = defaultServiceSeverity
	}

	service := service{
		name: k8sService.Name,
		isEnabled: true, //TODO: add is enabled  functionality (used for isSticky functionality)
		severity: uint8(severity),
	}

	return service
}
