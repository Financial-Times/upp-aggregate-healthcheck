package main

import (
	"strings"
	"time"

	prom "github.com/prometheus/client_golang/prometheus"
)

type prometheusFeeder struct {
	environment string
	ticker      *time.Ticker
	controller  controller
}

func newPrometheusFeeder(environment string, controller controller) *prometheusFeeder {
	ticker := time.NewTicker(60 * time.Second)
	return &prometheusFeeder{
		environment: environment,
		ticker:      ticker,
		controller:  controller,
	}
}

func (p prometheusFeeder) feed() {
	ignitePilotLight(p.environment)
	serviceStatus := initServiceStatusMetrics()

	for range p.ticker.C {
		p.recordMetrics(serviceStatus)
	}
}

func (p prometheusFeeder) recordMetrics(serviceStatus *prom.GaugeVec) {
	for _, service := range p.controller.getMeasuredServices() {
		select {
		case checkResult := <-service.bufferedMetrics.buffer:
			name := strings.Replace(checkResult.Name, ".", "-", -1)
			checkStatus := inverseBoolToFloat64(checkResult.Ok)
			serviceStatus.
				With(prom.Labels{"environment": p.environment, "service": name}).
				Set(checkStatus)
		default:
			continue
		}
	}
}

func initServiceStatusMetrics() *prom.GaugeVec {
	serviceStatus := prom.NewGaugeVec(
		prom.GaugeOpts{
			Namespace: "upp",
			Subsystem: "health",
			Name:      "servicestatus",
			Help:      "Status of the service: 0 - healthy; 1 - unhealthy",
		},
		[]string{
			"environment",
			"service",
		})
	prom.MustRegister(serviceStatus)
	return serviceStatus
}

func ignitePilotLight(environment string) {
	pilotLight := prom.NewGaugeVec(
		prom.GaugeOpts{
			Namespace: "upp",
			Subsystem: "health",
			Name:      "pilotlight",
			Help:      "Pilot light for the service monitoring UPP service health",
		},
		[]string{
			"environment",
		})
	prom.MustRegister(pilotLight)
	pilotLight.With(prom.Labels{"environment": environment}).Set(1)
}

func inverseBoolToFloat64(b bool) float64 {
	if b {
		return 0
	}
	return 1
}
