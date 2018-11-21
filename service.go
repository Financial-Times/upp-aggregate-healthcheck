package main

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	log "github.com/Financial-Times/go-logger"
)

type k8sHealthcheckService struct {
	k8sClient  kubernetes.Interface
	httpClient *http.Client
	services   servicesMap
	acks       map[string]string
}

type healthcheckService interface {
	getCategories() (map[string]category, error)
	updateCategory(string, bool) error
	getDeployments() (map[string]deployment, error)
	getServiceByName(serviceName string) (service, error)
	getServicesMapByNames([]string) map[string]service
	isServicePresent(string) bool
	getPodsForService(string) ([]pod, error)
	getPodByName(string) (pod, error)
	checkServiceHealth(service, map[string]deployment) (string, error)
	checkPodHealth(pod, int32) error
	getIndividualPodSeverity(pod, int32) (uint8, bool, error)
	getHealthChecksForPod(pod, int32) (healthcheckResponse, error)
	addAck(string, string) error
	removeAck(string) error
	getHTTPClient() *http.Client
}

const (
	defaultRefreshRate                = 60
	defaultFailureThreshold           = 3
	defaultSeverity                   = uint8(2)
	defaultResiliency                 = true
	ackMessagesConfigMapName          = "healthcheck.ack.messages"
	ackMessagesConfigMapLabelSelector = "healthcheck-acknowledgements-for=aggregate-healthcheck"
	defaultAppPort                    = int32(8080)
	namespace                         = "default"
)

func (hs *k8sHealthcheckService) updateAcksForServices(acksMap map[string]string) {
	hs.services.Lock()
	for serviceName, service := range hs.services.m {
		if ackMsg, found := acksMap[serviceName]; found {
			service.ack = ackMsg
		} else {
			service.ack = ""
		}
		hs.services.m[serviceName] = service
	}
	hs.services.Unlock()
}

func (hs *k8sHealthcheckService) watchAcks() {
	watcher, err := hs.k8sClient.CoreV1().ConfigMaps(namespace).Watch(v1.ListOptions{LabelSelector: ackMessagesConfigMapLabelSelector})

	if err != nil {
		log.WithError(err).Errorf("Error while starting to watch acks configMap with label selector %s", ackMessagesConfigMapLabelSelector)
	}

	log.Info("Started watching acks configMap")
	resultChannel := watcher.ResultChan()
	for msg := range resultChannel {
		switch msg.Type {
		case watch.Added, watch.Modified:
			k8sConfigMap := msg.Object.(*core.ConfigMap)
			hs.updateAcksForServices(k8sConfigMap.Data)
			hs.acks = k8sConfigMap.Data
			log.Infof("Acks configMap has been updated: %s", k8sConfigMap.Data)
		case watch.Deleted:
			hs.acks = make(map[string]string)
			log.Error("Acks configMap has been deleted. From now on the acks will no longer be available.")
		default:
			log.Error("Error received on watch acks configMap. Channel may be full")
		}
	}

	log.Info("Acks configMap watching terminated. Reconnecting...")
	hs.watchAcks()
}

func (hs *k8sHealthcheckService) watchServices() {
	watcher, err := hs.k8sClient.CoreV1().Services(namespace).Watch(v1.ListOptions{LabelSelector: "hasHealthcheck=true"})
	if err != nil {
		log.WithError(err).Error("Error while starting to watch services")
	}

	log.Info("Started watching services")
	resultChannel := watcher.ResultChan()
	for msg := range resultChannel {
		switch msg.Type {
		case watch.Added, watch.Modified:
			k8sService := msg.Object.(*core.Service)
			service := populateService(k8sService, hs.acks)

			hs.services.Lock()
			hs.services.m[service.name] = service
			hs.services.Unlock()

			log.Infof("Service with name %s added or updated.", service.name)
		case watch.Deleted:
			k8sService := msg.Object.(*core.Service)
			hs.services.Lock()
			delete(hs.services.m, k8sService.Name)
			hs.services.Unlock()
			log.Infof("Service with name %s has been removed", k8sService.Name)
		default:
			log.Error("Error received on watch services. Channel may be full")
		}
	}

	log.Info("Services watching terminated. Reconnecting...")
	hs.watchServices()
}

func initializeHealthCheckService() *k8sHealthcheckService {
	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
				DualStack: true,
			}).DialContext,
			MaxIdleConns:          256,
			MaxIdleConnsPerHost:   16,               // Each service will have their own host
			IdleConnTimeout:       90 * time.Second, // from DefaultTransport
			TLSHandshakeTimeout:   10 * time.Second, // from DefaultTransport
			ExpectContinueTimeout: 1 * time.Second,  // from DefaultTransport
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
		panic(fmt.Sprintf("Failed to create k8s client: %v", err.Error()))
	}

	services := make(map[string]service)

	k8sService := &k8sHealthcheckService{
		httpClient: httpClient,
		k8sClient:  k8sClient,
		services:   servicesMap{m: services},
	}

	go k8sService.watchAcks()
	go k8sService.watchServices()

	return k8sService
}

func (hs *k8sHealthcheckService) updateCategory(categoryName string, isEnabled bool) error {
	categoryConfigMapName := fmt.Sprintf("category.%s", categoryName)
	k8sCategory, err := hs.k8sClient.CoreV1().ConfigMaps(namespace).Get(categoryConfigMapName, v1.GetOptions{})

	if err != nil {
		return fmt.Errorf("cannot retrieve configMap for category with name %s: %s", categoryName, err.Error())
	}

	k8sCategory.Data["category.enabled"] = strconv.FormatBool(isEnabled)
	_, err = hs.k8sClient.CoreV1().ConfigMaps(namespace).Update(k8sCategory)

	if err != nil {
		return fmt.Errorf("cannot update configMap for category with name %s: %s", categoryName, err.Error())
	}

	return nil
}

func (hs *k8sHealthcheckService) removeAck(serviceName string) error {
	log.Infof("Removing ack for service with name %s ", serviceName)
	k8sAcksConfigMap, err := getAcksConfigMap(hs.k8sClient)

	if err != nil {
		return fmt.Errorf("failed to retrieve the current list of acks: %s", err.Error())
	}

	delete(k8sAcksConfigMap.Data, serviceName)

	if k8sAcksConfigMap.Data[serviceName] != "" {
		return fmt.Errorf("the ack for service %s has not been removed from configmap", serviceName)
	}

	k8sAcksConfigMap2, err := hs.k8sClient.CoreV1().ConfigMaps(namespace).Update(&k8sAcksConfigMap)

	if k8sAcksConfigMap2.Data[serviceName] != "" {
		return fmt.Errorf("the ack for service %s has not been removed from configmap. This check has been performed on the retrieved service", serviceName)
	}

	if err != nil {
		return fmt.Errorf("failed to remove the ack for service %s", serviceName)
	}

	return nil
}

func (hs *k8sHealthcheckService) addAck(serviceName string, ackMessage string) error {
	k8sAcksConfigMap, err := getAcksConfigMap(hs.k8sClient)

	if err != nil {
		return fmt.Errorf("failed to retrieve the current list of acks: %s", err.Error())
	}

	if k8sAcksConfigMap.Data == nil {
		k8sAcksConfigMap.Data = make(map[string]string)
	}

	k8sAcksConfigMap.Data[serviceName] = ackMessage

	_, err = hs.k8sClient.CoreV1().ConfigMaps(namespace).Update(&k8sAcksConfigMap)

	if err != nil {
		return fmt.Errorf("failed to update the acks config map for service %s and ack message [%s]: %v", serviceName, ackMessage, err)
	}

	return nil
}

func (hs *k8sHealthcheckService) getDeployments() (deployments map[string]deployment, err error) {
	deploymentList, err := hs.k8sClient.ExtensionsV1beta1().Deployments(namespace).List(v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve deployments: %v", err.Error())
	}

	deployments = make(map[string]deployment)
	for _, d := range deploymentList.Items {
		deployments[d.GetName()] = deployment{
			desiredReplicas: *d.Spec.Replicas,
		}
	}

	dl, err := hs.k8sClient.AppsV1beta1().StatefulSets(namespace).List(v1.ListOptions{})
	if err != nil {
		log.WithError(err).Warn("error getting StatefulSet list")
		return deployments, nil
	}

	for _, d := range dl.Items {
		deployments[d.Spec.ServiceName] = deployment{
			desiredReplicas: *d.Spec.Replicas,
		}
	}
	return deployments, nil
}

func (hs *k8sHealthcheckService) getPodByName(podName string) (pod, error) {
	k8sPod, err := hs.k8sClient.CoreV1().Pods(namespace).Get(podName, v1.GetOptions{})
	if err != nil {
		return pod{}, fmt.Errorf("failed to get the pod with name %s from k8s cluster: %v", podName, err.Error())
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
		return service, nil
	}

	return service{}, fmt.Errorf("cannot find service with name %s", serviceName)
}
func (hs *k8sHealthcheckService) getServicesMapByNames(serviceNames []string) map[string]service {
	//if the list of service names is empty, it means that we are in the default category so we take all the services that have healthcheck
	if len(serviceNames) == 0 {
		hs.services.RLock()
		defer hs.services.RUnlock()
		return hs.services.m
	}

	services := make(map[string]service)
	hs.services.RLock()
	for _, serviceName := range serviceNames {
		if service, ok := hs.services.m[serviceName]; ok {
			services[serviceName] = service
		} else {
			log.Errorf("Service with name [%s] not found.", serviceName)
		}
	}

	hs.services.RUnlock()
	return services
}

func (hs *k8sHealthcheckService) getPodsForService(serviceName string) ([]pod, error) {
	k8sPods, err := hs.k8sClient.CoreV1().Pods(namespace).List(v1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", serviceName)})
	if err != nil {
		return []pod{}, fmt.Errorf("failed to get the list of pods from k8s cluster: %v", err.Error())
	}

	pods := make([]pod, len(k8sPods.Items))
	for i, k8sPod := range k8sPods.Items {
		p := populatePod(k8sPod)
		pods[i] = p
	}

	return pods, nil
}

func (hs *k8sHealthcheckService) getCategories() (map[string]category, error) {
	categories := make(map[string]category)
	k8sCategories, err := hs.k8sClient.CoreV1().ConfigMaps(namespace).List(v1.ListOptions{LabelSelector: "healthcheck-categories-for=aggregate-healthcheck"})
	if err != nil {
		return nil, fmt.Errorf("failed to get the categories from kubernetes: %v", err.Error())
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
		isSticky = false
	}

	isEnabled, err := strconv.ParseBool(k8sCatData["category.enabled"])
	if err != nil {
		isEnabled = true
	}

	refreshRateSeconds, err := strconv.ParseInt(k8sCatData["category.refreshrate"], 10, 64)
	if err != nil {
		log.Infof("refreshRate is not set for category with name [%s]. Using default refresh rate.", categoryName)
		refreshRateSeconds = defaultRefreshRate
	}

	failureThreshold, err := strconv.Atoi(k8sCatData["category.failureThreshold"])
	if err != nil {
		failureThreshold = defaultFailureThreshold
	}

	refreshRatePeriod := time.Duration(refreshRateSeconds * int64(time.Second))
	categories := strings.Replace(k8sCatData["category.services"], " ", "", -1)
	return category{
		name:             categoryName,
		services:         strings.Split(categories, ","),
		refreshPeriod:    refreshRatePeriod,
		isSticky:         isSticky,
		isEnabled:        isEnabled,
		failureThreshold: failureThreshold,
	}
}

func populatePod(k8sPod core.Pod) pod {
	return pod{
		name:        k8sPod.Name,
		node:        k8sPod.Spec.NodeName,
		ip:          k8sPod.Status.PodIP,
		serviceName: k8sPod.Labels["app"],
	}
}

func populateService(k8sService *core.Service, acks map[string]string) service {
	//services are resilient by default.
	isResilient := true
	isDaemon := false
	serviceName := k8sService.Name
	var err error
	if isResilientLabelValue, ok := k8sService.Labels["isResilient"]; ok {
		isResilient, err = strconv.ParseBool(isResilientLabelValue)
		if err != nil {
			log.WithError(err).Warnf("Cannot parse isResilient label value for service with name %s.", serviceName)
		}
	}

	if isDaemonLabelValue, ok := k8sService.Labels["isDaemon"]; ok {
		isDaemon, err = strconv.ParseBool(isDaemonLabelValue)
		if err != nil {
			log.WithError(err).Warnf("Cannot parse isDaemon label value for service with name %s.", serviceName)
		}
	}

	return service{
		name:        serviceName,
		appPort:     getAppPortForService(k8sService),
		isDaemon:    isDaemon,
		isResilient: isResilient,
		ack:         acks[serviceName],
	}
}

func getAppPortForService(k8sService *core.Service) int32 {
	servicePorts := k8sService.Spec.Ports
	for _, port := range servicePorts {
		if port.Name == "app" {
			return port.TargetPort.IntVal
		}
	}

	return defaultAppPort
}

func getAcksConfigMap(k8sClient kubernetes.Interface) (core.ConfigMap, error) {
	k8sAckConfigMap, err := k8sClient.CoreV1().ConfigMaps(namespace).Get(ackMessagesConfigMapName, v1.GetOptions{})

	if err != nil {
		return core.ConfigMap{}, fmt.Errorf("cannot find configMap with name %s: %s", ackMessagesConfigMapName, err.Error())
	}

	return *k8sAckConfigMap, nil
}
