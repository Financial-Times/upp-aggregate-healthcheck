package main

import (
	"time"
	"net/http"
	"errors"
	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"fmt"
	"strings"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/labels"
	"strconv"
	"k8s.io/client-go/1.5/pkg/fields"
	"k8s.io/client-go/1.5/rest"
)

type k8sHealthcheckService struct {
	k8sClient  *kubernetes.Clientset
	httpClient *http.Client
}

type healthcheckService interface {
	getCategories() (map[string]category, error)
	updateCategory(string, bool) error
	getServicesByNames([]string) []service
	getPodsForService(string) ([]pod, error)
	getPodByName(string) (pod, error)
	checkServiceHealth(string) error
	checkPodHealth(pod) error
	getIndividualPodSeverity(pod) (uint8, error)
	getHealthChecksForPod(pod) (healthcheckResponse, error)
	addAck(string, string) error
	removeAck(string) error
	getHttpClient() *http.Client
}

const (
	defaultRefreshRate = 60
	defaultSeverity = uint8(2)
	ackMessagesConfigMapName = "healthcheck.ack.messages"
)

func InitializeHealthCheckService() *k8sHealthcheckService {
	httpClient := &http.Client{
		Timeout:   50 * time.Second,
	}

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

func (hs *k8sHealthcheckService) updateCategory(categoryName string, isEnabled bool) error {
	categoryConfigMapName := fmt.Sprintf("category.%s", categoryName)
	k8sCategory, err := hs.k8sClient.Core().ConfigMaps("default").Get(categoryConfigMapName)

	if err != nil {
		return errors.New(fmt.Sprintf("Cannot retrieve configMap for category with name %s. Error was: %s", categoryName, err.Error()))
	}

	k8sCategory.Data["category.enabled"] = strconv.FormatBool(isEnabled)
	_, err = hs.k8sClient.Core().ConfigMaps("default").Update(k8sCategory)

	if err != nil {
		return errors.New(fmt.Sprintf("Cannot update configMap for category with name %s. Error was: %s", categoryName, err.Error()))
	}

	return nil
}

func (hs *k8sHealthcheckService) removeAck(serviceName string) error {
	infoLogger.Printf("Removing ack for service with name %s ", serviceName)
	k8sAcksConfigMap, err := getAcksConfigMap(hs.k8sClient)

	if err != nil {
		return errors.New(fmt.Sprintf("Failed to retrieve the current list of acks. Error was: %s", err.Error()))
	}

	delete(k8sAcksConfigMap.Data, serviceName);

	if k8sAcksConfigMap.Data[serviceName] != "" {
		return errors.New(fmt.Sprintf("The ack for service %s has not been removed from configmap.", serviceName))
	}

	k8sAcksConfigMap2, err := hs.k8sClient.Core().ConfigMaps("default").Update(&k8sAcksConfigMap)

	if k8sAcksConfigMap2.Data[serviceName] != "" {
		//todo: delete this log:
		errorLogger.Printf("The ack for service %s has not been removed from configmap. This check has been performed on the retrieved service.", serviceName)
		//todo: remove this log.
		return errors.New(fmt.Sprintf("The ack for service %s has not been removed from configmap. This check has been performed on the retrieved service.", serviceName))
	}

	if err != nil {
		return errors.New(fmt.Sprintf("Failed to remove the ack for service %s.", serviceName))
	}

	return nil
}

func (hs *k8sHealthcheckService) addAck(serviceName string, ackMessage string) error {
	k8sAcksConfigMap, err := getAcksConfigMap(hs.k8sClient)

	if err != nil {
		return errors.New(fmt.Sprintf("Failed to retrieve the current list of acks. Error was: %s", err.Error()))
	}

	if k8sAcksConfigMap.Data == nil {
		k8sAcksConfigMap.Data = make(map[string]string)
	}

	k8sAcksConfigMap.Data[serviceName] = ackMessage

	_, err = hs.k8sClient.Core().ConfigMaps("default").Update(&k8sAcksConfigMap)

	if err != nil {
		return errors.New(fmt.Sprintf("Failed to update the acks config map for service %s and ack message [%s]", serviceName, ackMessage))
	}

	return nil
}

func (hs *k8sHealthcheckService) getPodByName(podName string) (pod, error) {

	k8sPods, err := hs.k8sClient.Core().Pods("default").List(api.ListOptions{FieldSelector: fields.SelectorFromSet(fields.Set{"metadata.name":podName})})
	if err != nil {
		return pod{}, errors.New(fmt.Sprintf("Failed to get the pod from k8s cluster, error was %v", err.Error()))
	}

	if len(k8sPods.Items) == 0 {
		return pod{}, errors.New(fmt.Sprintf("Pod with name %s was not found in cluster, error was %v", podName, err.Error()))
	}

	pod := populatePod(k8sPods.Items[0])
	return pod, nil
}

func (hs *k8sHealthcheckService) getServicesByNames(serviceNames []string) []service {
	k8sServices, err := hs.k8sClient.Core().Services("default").List(api.ListOptions{LabelSelector: labels.SelectorFromSet(labels.Set{"hasHealthcheck":"true"})})

	acks, err := getAcks(hs.k8sClient)

	if err != nil {
		warnLogger.Printf("Cannot get acks. There will be no acks at all. Error was: %s", err.Error())
	}

	//todo: return _,err instead of empty services list in case of error.

	if err != nil {
		errorLogger.Printf("Failed to get the list of services from k8s cluster, error was %v", err.Error())
		return []service{}
	}

	//if the list of service names is empty, it means that we are in the default category so we take all the services that have healthcheck
	if len(serviceNames) == 0 {
		return getAllServices(k8sServices.Items, acks)
	}

	return getServicesWithNames(k8sServices.Items, serviceNames, acks)
}

func (hs *k8sHealthcheckService) getPodsForService(serviceName string) ([]pod, error) {

	//todo: return _,err instead of empty services list in case of error.
	k8sPods, err := hs.k8sClient.Core().Pods("default").List(api.ListOptions{LabelSelector: labels.SelectorFromSet(labels.Set{"app":serviceName})})
	if err != nil {
		return []pod{}, errors.New(fmt.Sprintf("Failed to get the list of pods from k8s cluster, error was %v", err.Error()))
	}

	pods := []pod{}
	for _, k8sPod := range k8sPods.Items {
		pod := populatePod(k8sPod)
		pods = append(pods, pod)
	}

	return pods, nil
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

	isEnabled, err := strconv.ParseBool(k8sCatData["category.enabled"])
	if err != nil {
		warnLogger.Printf("Failed to convert isEnabled flag from string to bool for category with name [%s]. Using default value of true. Error was: %v", categoryName, err)
		isEnabled = true
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
		isEnabled: isEnabled,
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

func getServicesWithNames(k8sServices []v1.Service, serviceNames []string, acks map[string]string) []service {
	services := []service{}

	for _, serviceName := range serviceNames {
		k8sService, err := getServiceByName(k8sServices, serviceName)
		if err != nil {
			errorLogger.Printf("Service with name [%s] cannot be found in k8s services. Error was: %v", serviceName, err)
		} else {
			service := populateService(k8sService, acks)
			services = append(services, service)
		}
	}

	return services
}

func getAllServices(k8sServices []v1.Service, acks map[string]string) []service {
	infoLogger.Print("Using category default, retrieving all services.")
	services := []service{}
	for _, k8sService := range k8sServices {
		service := populateService(k8sService, acks)
		services = append(services, service)
	}

	return services
}

func populateService(k8sService v1.Service, acks map[string]string) service {
	service := service{
		name: k8sService.Name,
		ack: acks[k8sService.Name],
	}

	return service
}

func getAcks(k8sClient *kubernetes.Clientset) (map[string]string, error) {
	k8sAckConfigMap, err := getAcksConfigMap(k8sClient)

	if err != nil {
		return nil, err
	}

	return k8sAckConfigMap.Data, nil
}

func getAcksConfigMap(k8sClient *kubernetes.Clientset) (v1.ConfigMap, error) {
	k8sAckConfigMaps, err := k8sClient.Core().ConfigMaps("default").List(api.ListOptions{FieldSelector: fields.SelectorFromSet(fields.Set{"metadata.name":ackMessagesConfigMapName})})

	if err != nil {
		return v1.ConfigMap{}, errors.New(fmt.Sprintf("Cannot get configMap with name: %s from k8s cluster. Error was: %s", ackMessagesConfigMapName, err.Error()))
	}

	if len(k8sAckConfigMaps.Items) == 0 {
		return v1.ConfigMap{}, errors.New(fmt.Sprintf("Cannot find configMap with name: %s", ackMessagesConfigMapName))
	}

	return k8sAckConfigMaps.Items[0], nil
}

