package main

import (
	"context"

	log "github.com/Financial-Times/go-logger"
)

func (c *healthCheckController) getSeverityForService(ctx context.Context, serviceName string, appPort int32) uint8 {
	pods, err := c.healthCheckService.getPodsForService(ctx, serviceName)
	if err != nil {
		log.WithError(err).Warnf("Cannot get pods for service with name %s in order to get severity level, using default severity: %d.", serviceName, defaultSeverity)
		return defaultSeverity
	}

	var isResilient bool
	service, err := c.healthCheckService.getServiceByName(serviceName)
	if err != nil {
		log.WithError(err).Warnf("Cannot get service with name %s in order to get resiliency, using default resiliency: %t.", serviceName, defaultResiliency)
		isResilient = defaultResiliency
	} else {
		isResilient = service.isResilient
	}

	if !isResilient {
		return c.computeSeverityByPods(pods, appPort)
	}

	finalSeverity := defaultSeverity
	for _, pod := range pods {
		individualPodSeverity, checkFailed, err := c.healthCheckService.getIndividualPodSeverity(pod, appPort)

		if err != nil {
			log.WithError(err).Error("Cannot get individual pod severity, skipping pod.")
			continue
		}
		// if at least one pod of the resilient service is healthy, we return the default severity,
		// regardless of the status of other pods
		if !checkFailed {
			return defaultSeverity
		}
		if individualPodSeverity < finalSeverity {
			finalSeverity = individualPodSeverity
		}
	}
	return finalSeverity
}

func (c *healthCheckController) getSeverityForPod(ctx context.Context, podName string, appPort int32) uint8 {
	podToBeChecked, err := c.healthCheckService.getPodByName(ctx, podName)

	if err != nil {
		log.WithError(err).Errorf("Cannot get pod by name: %s in order to get severity level, using default severity: %d.", podName, defaultSeverity)
		return defaultSeverity
	}

	return c.computeSeverityByPods([]pod{podToBeChecked}, appPort)
}

func (c *healthCheckController) computeSeverityByPods(pods []pod, appPort int32) uint8 {
	finalSeverity := defaultSeverity
	for _, pod := range pods {
		individualPodSeverity, _, err := c.healthCheckService.getIndividualPodSeverity(pod, appPort)

		if err != nil {
			log.WithError(err).Warn("Cannot get individual pod severity, skipping pod.")
			continue
		}

		if individualPodSeverity < finalSeverity {
			return individualPodSeverity
		}
	}

	return finalSeverity
}
