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
	services    servicesMap
}

type healthcheckService interface {
	getCategories() (map[string]category, error)
	updateCategory(string, bool) error
	getServiceByName(serviceName string) (service, error)
	getServicesMapByNames([]string) map[string]service
	isServicePresent(string) bool
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
	defaultRefreshRate = 60
	defaultSeverity = uint8(2)
	ackMessagesConfigMapName = "healthcheck.ack.messages"
	ackMessagesConfigMapLabelSelector = "healthcheck-acknowledgements-for=aggregate-healthcheck"
	defaultAppPort = int32(8080)
)

func (hs *k8sHealthcheckService) updateAcksForServices(acksMap map[string]string) {
	hs.services.Lock()
	for serviceName, service := range hs.services.m {
		if ackMsg, ok := acksMap[serviceName]; ok {
			service.ack = ackMsg
		} else {
			service.ack = ""
		}
		hs.services.m[serviceName] = service
	}
	hs.services.Unlock()
}

func (hs *k8sHealthcheckService) watchAcks() {
	watcher, err := hs.k8sClient.CoreV1().ConfigMaps("default").Watch(v1.ListOptions{LabelSelector: ackMessagesConfigMapLabelSelector})

	if err != nil {
		errorLogger.Printf("Error while starting to watch acks configMap with label selector: %s. Error was: %s", ackMessagesConfigMapLabelSelector, err.Error())
	}

	infoLogger.Print("Started watching services")
	resultChannel := watcher.ResultChan()
	for msg := range resultChannel {
		switch msg.Type {
		case watch.Added, watch.Modified:
			k8sConfigMap := msg.Object.(*v1.ConfigMap)
			hs.updateAcksForServices(k8sConfigMap.Data)
			infoLogger.Printf("Acks configMap has been updated: %s", k8sConfigMap.Data)
		case watch.Deleted:
			errorLogger.Print("Acks configMap has been deleted. From now on the acks will no longer be available.")
		default:
			errorLogger.Print("Error received on watch acks configMap. Channel may be full")
		}
	}

	infoLogger.Print("Acks configMap watching terminated. Reconnecting...")
	hs.watchAcks()
}

func (hs *k8sHealthcheckService) watchServices() {
	watcher, err := hs.k8sClient.CoreV1().Services("default").Watch(v1.ListOptions{LabelSelector: "hasHealthcheck=true"})
	if err != nil {
		errorLogger.Printf("Error while starting to watch services: %s", err.Error())
	}

	infoLogger.Print("Started watching services")
	resultChannel := watcher.ResultChan()
	for msg := range resultChannel {
		switch msg.Type {
		case watch.Added, watch.Modified:
			k8sService := msg.Object.(*v1.Service)
			service := populateService(k8sService)

			hs.services.Lock()
			hs.services.m[service.name] = service
			hs.services.Unlock()

			infoLogger.Printf("Service with name %s added or updated.", service.name)
		case watch.Deleted:
			k8sService := msg.Object.(*v1.Service)
			hs.services.Lock()
			delete(hs.services.m, k8sService.Name)
			hs.services.Unlock()
			infoLogger.Printf("Service with name %s has been removed", k8sService.Name)
		default:
			errorLogger.Print("Error received on watch services. Channel may be full")
		}
	}

	infoLogger.Print("Services watching terminated. Reconnecting...")
	hs.watchServices()
}

func (hs *k8sHealthcheckService) watchDeployments() {
	watcher, err := hs.k8sClient.ExtensionsV1beta1().Deployments("default").Watch(v1.ListOptions{})

	if err != nil {
		errorLogger.Printf("Error while starting to watch deployments: %s", err.Error())
	}

	infoLogger.Print("Started watching deployments")
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
			errorLogger.Print("Error received on watch deployments. Channel may be full")
		}
	}

	infoLogger.Print("Deployments watching terminated. Reconnecting...")
	hs.watchDeployments()
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
	services := make(map[string]service)

	k8sService := &k8sHealthcheckService{
		httpClient:  httpClient,
		k8sClient:   k8sClient,
		deployments: deploymentsMap{m: deployments},
		services: servicesMap{m:services},
	}

	go k8sService.watchDeployments()
	go k8sService.watchServices()
	go k8sService.watchAcks()

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

func (hs *k8sHealthcheckService) isServicePresent(serviceName string) bool {
	hs.services.RLock()
	_, ok := hs.services.m[serviceName]
	hs.services.RUnlock()
	return ok
}

func (hs *k8sHealthcheckService) getServiceByName(serviceName string) (service, error) {
	hs.services.RLock()
	defer hs.services.RUnlock()

	if service, ok := hs.services.m[serviceName]; ok {
		return service,nil
	}

	return service{}, fmt.Errorf("Cannot find service with name %s", serviceName)
}
func (hs *k8sHealthcheckService) getServicesMapByNames(serviceNames []string) map[string]service {
	//if the list of service names is empty, it means that we are in the default category so we take all the services that have healthcheck
	if len(serviceNames) == 0 {
		hs.services.RLock()
		defer hs.services.RUnlock()
		//TODO: check if this map can be modified after it is returned.
		return hs.services.m
	}

	services := make(map[string]service)
	hs.services.RLock()
	for _, serviceName := range serviceNames {
		if service, ok := hs.services.m[serviceName]; ok {
			services[serviceName] = service
		} else {
			errorLogger.Printf("Service with name [%s] not found.", serviceName)
		}
	}

	hs.services.RUnlock()
	return services
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

func populateService(k8sService *v1.Service) service {
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
		appPort:     getAppPortForService(k8sService),
		isDaemon:    isDaemon,
		isResilient: isResilient,
	}
}

func getAppPortForService(k8sService *v1.Service) int32 {
	servicePorts := k8sService.Spec.Ports
	for _, port := range servicePorts {
		if port.Name == "app" {
			return port.TargetPort.IntVal
		}
	}

	return defaultAppPort
}

func getAcksConfigMap(k8sClient kubernetes.Interface) (v1.ConfigMap, error) {
	k8sAckConfigMap, err := k8sClient.CoreV1().ConfigMaps("default").Get(ackMessagesConfigMapName)

	if err != nil {
		return v1.ConfigMap{}, fmt.Errorf("Cannot fin configMap with name: %s. Error was: %s", ackMessagesConfigMapName, err.Error())
	}

	return *k8sAckConfigMap, nil
}
