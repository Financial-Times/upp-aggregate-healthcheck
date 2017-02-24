package main

func (c *healthCheckController) getSeverityForService(serviceName string, appPort int32) uint8 {
	pods, err := c.healthCheckService.getPodsForService(serviceName)

	if err != nil {
		warnLogger.Printf("Cannot get pods for service with name %s in order to get severity level, using default severity: %d. Error was: %s", serviceName, defaultSeverity, err.Error())
		return defaultSeverity
	}

	return c.computeSeverityByPods(pods, appPort)
}

func (c *healthCheckController) getSeverityForPod(podName string, appPort int32) uint8 {
	podToBeChecked, err := c.healthCheckService.getPodByName(podName)

	if err != nil {
		warnLogger.Printf("Cannot get pod by name: %s in order to get severity level, using default severity: %d. Error was: %s", podName, defaultSeverity, err.Error())
		return defaultSeverity
	}

	return c.computeSeverityByPods([]pod{podToBeChecked}, appPort)
}

func (c *healthCheckController) computeSeverityByPods(pods []pod, appPort int32) uint8 {
	finalSeverity := defaultSeverity
	for _, pod := range pods {
		individualPodSeverity, err := c.healthCheckService.getIndividualPodSeverity(pod, appPort)

		if err != nil {
			warnLogger.Printf("Cannot get individual pod severity, skipping pod. Error was: %s", err.Error())
			continue
		}

		if individualPodSeverity < finalSeverity {
			return individualPodSeverity
		}
	}

	return finalSeverity
}
