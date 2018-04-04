package main

func (c *healthCheckController) getSeverityForService(serviceName string, appPort int32) uint8 {
	pods, err := c.healthCheckService.getPodsForService(serviceName)
	if err != nil {
		warnLogger.Printf("Cannot get pods for service with name %s in order to get severity level, using default severity: %d. Problem was: %s", serviceName, defaultSeverity, err.Error())
		return defaultSeverity
	}

	var isResilient bool
	service, err := c.healthCheckService.getServiceByName(serviceName)
	if err != nil {
		warnLogger.Printf("Cannot get service with name %s in order to get resiliency, using default resiliency: %t. Problem was: %s", serviceName, defaultResiliency, err.Error())
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
			warnLogger.Printf("Cannot get individual pod severity, skipping pod. Problem was: %s", err.Error())
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

func (c *healthCheckController) getSeverityForPod(podName string, appPort int32) uint8 {
	podToBeChecked, err := c.healthCheckService.getPodByName(podName)

	if err != nil {
		warnLogger.Printf("Cannot get pod by name: %s in order to get severity level, using default severity: %d. Problem was: %s", podName, defaultSeverity, err.Error())
		return defaultSeverity
	}

	return c.computeSeverityByPods([]pod{podToBeChecked}, appPort)
}

func (c *healthCheckController) computeSeverityByPods(pods []pod, appPort int32) uint8 {
	finalSeverity := defaultSeverity
	for _, pod := range pods {
		individualPodSeverity, _, err := c.healthCheckService.getIndividualPodSeverity(pod, appPort)

		if err != nil {
			warnLogger.Printf("Cannot get individual pod severity, skipping pod. Problem was: %s", err.Error())
			continue
		}

		if individualPodSeverity < finalSeverity {
			return individualPodSeverity
		}
	}

	return finalSeverity
}
