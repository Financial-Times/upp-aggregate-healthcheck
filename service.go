package main

import (
	"fmt"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	k8s "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/pkg/watch"
	"k8s.io/client-go/rest"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type k8sHealthcheckService struct {
	k8sClient   kubernetes.Interface
	httpClient  *http.Client
	deployments deploymentsMap
}

type healthcheckService interface {
	getCategories() (map[string]category, error)
	updateCategory(string, bool) error
	getServicesByNames([]string) []service
	getPodsForService(string) ([]pod, error)
	getPodByName(string) (pod, error)
	checkServiceHealth(service) (string, error)
	getPodAvailabilityForDeployment(service) (int32, int32, error)
	getPodAvailabilityForDaemonSet(service) (int32, int32, error)
	checkPodHealth(pod, int32) error
	getIndividualPodSeverity(pod, int32) (uint8, error)
	getHealthChecksForPod(pod, int32) (healthcheckResponse, error)
	addAck(string, string) error
	removeAck(string) error
	getHTTPClient() *http.Client
}

const (
	defaultRefreshRate       = 60
	defaultSeverity          = uint8(2)
	ackMessagesConfigMapName = "healthcheck.ack.messages"
	defaultAppPort           = int32(8080)
)

func (hs *k8sHealthcheckService) watchDeployments() {
	watcher, err := hs.k8sClient.ExtensionsV1beta1().Deployments("default").Watch(v1.ListOptions{})

	if err != nil {
		errorLogger.Printf("Error while starting to watch deployments: %s", err.Error())
	}

	resultChannel := watcher.ResultChan()
	for msg := range resultChannel {
		switch msg.Type {
		case watch.Added, watch.Modified:
			k8sDeployment := msg.Object.(*k8s.Deployment)
			deployment := deployment{
				numberOfAvailableReplicas:   k8sDeployment.Status.AvailableReplicas,
				numberOfUnavailableReplicas: k8sDeployment.Status.UnavailableReplicas,
			}

			hs.deployments.Lock()
			hs.deployments.m[k8sDeployment.Name] = deployment
			hs.deployments.Unlock()

			infoLogger.Printf("Deployment %s has been added or updated: No of available replicas: %d, no of unavailable replicas: %d", k8sDeployment.Name, k8sDeployment.Status.AvailableReplicas, k8sDeployment.Status.UnavailableReplicas)

		case watch.Deleted:
			k8sDeployment := msg.Object.(*k8s.Deployment)
			hs.deployments.Lock()
			delete(hs.deployments.m, k8sDeployment.Name)
			hs.deployments.Unlock()
			infoLogger.Printf("Deployment %s has been removed", k8sDeployment.Name)
		default:
			errorLogger.Print("Error received on watch deployments. Channel may be full ")
		}
	}
}

func initializeHealthCheckService() *k8sHealthcheckService {
	httpClient := &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 100,
			Dial: (&net.Dialer{
				KeepAlive: 30 * time.Second,
			}).Dial,
		},
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

	deployments := make(map[string]deployment)

	k8sService := &k8sHealthcheckService{
		httpClient:  httpClient,
		k8sClient:   k8sClient,
		deployments: deploymentsMap{m: deployments},
	}

	go k8sService.watchDeployments()

	return k8sService
}

func (hs *k8sHealthcheckService) updateCategory(categoryName string, isEnabled bool) error {
	categoryConfigMapName := fmt.Sprintf("category.%s", categoryName)
	k8sCategory, err := hs.k8sClient.CoreV1().ConfigMaps("default").Get(categoryConfigMapName)

	if err != nil {
		return fmt.Errorf("Cannot retrieve configMap for category with name %s. Error was: %s", categoryName, err.Error())
	}

	k8sCategory.Data["category.enabled"] = strconv.FormatBool(isEnabled)
	_, err = hs.k8sClient.CoreV1().ConfigMaps("default").Update(k8sCategory)

	if err != nil {
		return fmt.Errorf("Cannot update configMap for category with name %s. Error was: %s", categoryName, err.Error())
	}

	return nil
}

func (hs *k8sHealthcheckService) removeAck(serviceName string) error {
	infoLogger.Printf("Removing ack for service with name %s ", serviceName)
	k8sAcksConfigMap, err := getAcksConfigMap(hs.k8sClient)

	if err != nil {
		return fmt.Errorf("Failed to retrieve the current list of acks. Error was: %s", err.Error())
	}

	delete(k8sAcksConfigMap.Data, serviceName)

	if k8sAcksConfigMap.Data[serviceName] != "" {
		return fmt.Errorf("The ack for service %s has not been removed from configmap", serviceName)
	}

	k8sAcksConfigMap2, err := hs.k8sClient.CoreV1().ConfigMaps("default").Update(&k8sAcksConfigMap)

	if k8sAcksConfigMap2.Data[serviceName] != "" {
		return fmt.Errorf("The ack for service %s has not been removed from configmap. This check has been performed on the retrieved service", serviceName)
	}

	if err != nil {
		return fmt.Errorf("Failed to remove the ack for service %s", serviceName)
	}

	return nil
}

func (hs *k8sHealthcheckService) addAck(serviceName string, ackMessage string) error {
	k8sAcksConfigMap, err := getAcksConfigMap(hs.k8sClient)

	if err != nil {
		return fmt.Errorf("Failed to retrieve the current list of acks. Error was: %s", err.Error())
	}

	if k8sAcksConfigMap.Data == nil {
		k8sAcksConfigMap.Data = make(map[string]string)
	}

	k8sAcksConfigMap.Data[serviceName] = ackMessage

	_, err = hs.k8sClient.CoreV1().ConfigMaps("default").Update(&k8sAcksConfigMap)

	if err != nil {
		return fmt.Errorf("Failed to update the acks config map for service %s and ack message [%s]", serviceName, ackMessage)
	}

	return nil
}

func (hs *k8sHealthcheckService) getPodByName(podName string) (pod, error) {
	k8sPod, err := hs.k8sClient.CoreV1().Pods("default").Get(podName)
	if err != nil {
		return pod{}, fmt.Errorf("Failed to get the pod with name %s from k8s cluster, error was %v", podName, err.Error())
	}

	p := populatePod(*k8sPod)
	return p, nil
}

func (hs *k8sHealthcheckService) getServicesByNames(serviceNames []string) []service {
	k8sServices, err := hs.k8sClient.CoreV1().Services("default").List(v1.ListOptions{LabelSelector: "hasHealthcheck=true"})

	if err != nil {
		errorLogger.Printf("Failed to get the list of services from k8s cluster, error was %v", err.Error())
		return []service{}
	}

	acks, err := getAcks(hs.k8sClient)

	if err != nil {
		warnLogger.Printf("Cannot get acks. There will be no acks at all. Problem was: %s", err.Error())
	}

	//if the list of service names is empty, it means that we are in the default category so we take all the services that have healthcheck
	if len(serviceNames) == 0 {
		return getAllServices(k8sServices.Items, acks)
	}

	return getServicesWithNames(k8sServices.Items, serviceNames, acks)
}

func (hs *k8sHealthcheckService) getPodsForService(serviceName string) ([]pod, error) {
	k8sPods, err := hs.k8sClient.CoreV1().Pods("default").List(v1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", serviceName)})
	if err != nil {
		return []pod{}, fmt.Errorf("Failed to get the list of pods from k8s cluster, error was %v", err.Error())
	}

	pods := []pod{}
	for _, k8sPod := range k8sPods.Items {
		p := populatePod(k8sPod)
		pods = append(pods, p)
	}

	return pods, nil
}

func (hs *k8sHealthcheckService) getCategories() (map[string]category, error) {
	categories := make(map[string]category)
	k8sCategories, err := hs.k8sClient.CoreV1().ConfigMaps("default").List(v1.ListOptions{LabelSelector: "healthcheck-categories-for=aggregate-healthcheck"})
	if err != nil {
		return nil, fmt.Errorf("Failed to get the categories from kubernetes. Error was: %v", err)
	}

	for _, k8sCategory := range k8sCategories.Items {
		c := populateCategory(k8sCategory.Data)
		categories[c.name] = c
	}

	return categories, nil
}

func (hs *k8sHealthcheckService) getHTTPClient() *http.Client {
	return hs.httpClient
}

func populateCategory(k8sCatData map[string]string) category {
	categoryName := k8sCatData["category.name"]
	isSticky, err := strconv.ParseBool(k8sCatData["category.issticky"])
	if err != nil {
		infoLogger.Printf("isSticky flag is not set for category with name [%s]. Using default value of false.", categoryName)
		isSticky = false
	}

	isEnabled, err := strconv.ParseBool(k8sCatData["category.enabled"])
	if err != nil {
		infoLogger.Printf("isEnabled flag is not set for category with name for category with name [%s]. Using default value of true.", categoryName)
		isEnabled = true
	}

	refreshRateSeconds, err := strconv.ParseInt(k8sCatData["category.refreshrate"], 10, 64)
	if err != nil {
		infoLogger.Printf("refreshRate is not set for category with name [%s]. Using default refresh rate.", categoryName)
		refreshRateSeconds = defaultRefreshRate
	}

	refreshRatePeriod := time.Duration(refreshRateSeconds * int64(time.Second))
	return category{
		name:          categoryName,
		services:      strings.Split(k8sCatData["category.services"], ","),
		refreshPeriod: refreshRatePeriod,
		isSticky:      isSticky,
		isEnabled:     isEnabled,
	}
}

func populatePod(k8sPod v1.Pod) pod {
	return pod{
		name:        k8sPod.Name,
		ip:          k8sPod.Status.PodIP,
		serviceName: k8sPod.Labels["app"],
	}
}

func getServiceByName(k8sServices []v1.Service, serviceName string) (v1.Service, error) {
	for _, k8sService := range k8sServices {
		if k8sService.Name == serviceName {
			return k8sService, nil
		}
	}

	return v1.Service{}, fmt.Errorf("Cannot find k8sService with name %s", serviceName)
}

func getServicesWithNames(k8sServices []v1.Service, serviceNames []string, acks map[string]string) []service {
	services := []service{}

	for _, serviceName := range serviceNames {
		k8sService, err := getServiceByName(k8sServices, serviceName)
		if err != nil {
			errorLogger.Printf("Service with name [%s] cannot be found in k8s services. Error was: %v", serviceName, err)
		} else {
			s := populateService(k8sService, acks)
			services = append(services, s)
		}
	}

	return services
}

func getAllServices(k8sServices []v1.Service, acks map[string]string) []service {
	infoLogger.Print("Using category default, retrieving all services.")
	services := []service{}
	for _, k8sService := range k8sServices {
		s := populateService(k8sService, acks)
		services = append(services, s)
	}

	return services
}

func populateService(k8sService v1.Service, acks map[string]string) service {
	//services are resilient by default.
	isResilient := true
	isDaemon := false
	serviceName := k8sService.Name
	var err error
	if isResilientLabelValue, ok := k8sService.Labels["isResilient"]; ok {
		isResilient, err = strconv.ParseBool(isResilientLabelValue)
		if err != nil {
			warnLogger.Printf("Cannot parse isResilient label value for service with name %s. Problem was: %s", serviceName, err.Error())
		}
	}

	if isDaemonLabelValue, ok := k8sService.Labels["isDaemon"]; ok {
		isDaemon, err = strconv.ParseBool(isDaemonLabelValue)
		if err != nil {
			warnLogger.Printf("Cannot parse isDaemon label value for service with name %s. Problem was: %s", serviceName, err.Error())
		}
	}

	return service{
		name:        serviceName,
		ack:         acks[k8sService.Name],
		appPort:     getAppPortForService(k8sService),
		isDaemon:    isDaemon,
		isResilient: isResilient,
	}
}
func getAppPortForService(k8sService v1.Service) int32 {
	servicePorts := k8sService.Spec.Ports
	for _, port := range servicePorts {
		if port.Name == "app" {
			return port.TargetPort.IntVal
		}
	}

	return defaultAppPort
}

func getAcks(k8sClient kubernetes.Interface) (map[string]string, error) {
	k8sAckConfigMap, err := getAcksConfigMap(k8sClient)

	if err != nil {
		return nil, err
	}

	return k8sAckConfigMap.Data, nil
}

func getAcksConfigMap(k8sClient kubernetes.Interface) (v1.ConfigMap, error) {
	k8sAckConfigMap, err := k8sClient.CoreV1().ConfigMaps("default").Get(ackMessagesConfigMapName)

	if err != nil {
		return v1.ConfigMap{}, fmt.Errorf("Cannot fin configMap with name: %s. Error was: %s", ackMessagesConfigMapName, err.Error())
	}

	return *k8sAckConfigMap, nil
}
