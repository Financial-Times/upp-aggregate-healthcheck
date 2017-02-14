package main

func (c *healthCheckController) getSeverityForService(serviceName string) uint8 {
	pods, err := c.healthCheckService.getPodsForService(serviceName)

	if err != nil {
		warnLogger.Printf("Cannot get pods for service with name %s in order to get severity level, using default severity: %s. Error was: %s", serviceName, defaultSeverity, err.Error())
		return defaultSeverity
	}

	return c.computeSeverityByPods(pods)
}

func (c *healthCheckController) getSeverityForPod(podName string) uint8 {
	pod, err := c.healthCheckService.getPodByName(podName)

	if err != nil {
		warnLogger.Printf("Cannot get pod by name: %s in order to get severity level, using default severity: %s. Error was: %s", podName, defaultSeverity, err.Error())
		return defaultSeverity
	}

	return c.computeSeverityByPods([]pod{pod})
}

func (c *healthCheckController) computeSeverityByPods(pods []pod) uint8 {
	finalSeverity := defaultSeverity
	for _, pod := range pods {
		individualPodSeverity, err := c.healthCheckService.getIndividualPodSeverity(pod)

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
