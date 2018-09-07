package main

import (
	"testing"

	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

const ENV = "foobar"

func TestNewPrometheusFeeder(t *testing.T) {
	feeder := newPrometheusFeeder(ENV, &healthCheckController{})
	assert.NotNil(t, feeder)
	assert.Equal(t, ENV, feeder.environment, "Environment should be set correctly.")
}

func TestInitServiceStatusMetrics(t *testing.T) {
	gauge := initServiceStatusMetrics()
	assert.NotNil(t, gauge)
	duplicateGauge := prom.NewGaugeVec(
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

	err := prom.Register(duplicateGauge)
	assert.NotNil(t, err, "Gauge should've been registered already.")
	_, ok := err.(prom.AlreadyRegisteredError)
	assert.True(t, ok, "Expecting an 'AlreadyRegisteredError'.")
}
func TestIgnitePilotLight(t *testing.T) {
	ignitePilotLight(ENV)
	duplicateGauge := prom.NewGaugeVec(
		prom.GaugeOpts{
			Namespace: "upp",
			Subsystem: "health",
			Name:      "pilotlight",
			Help:      "Pilot light for the service monitoring UPP service health",
		},
		[]string{
			"environment",
		})

	err := prom.Register(duplicateGauge)
	assert.NotNil(t, err, "Gauge should've been registered already.")
	_, ok := err.(prom.AlreadyRegisteredError)
	assert.True(t, ok, "Expecting an 'AlreadyRegisteredError'.")
}

func TestInverseBoolToFloat64(t *testing.T) {
	one := inverseBoolToFloat64(false)
	assert.Equal(t, float64(1), one)

	zero := inverseBoolToFloat64(true)
	assert.Equal(t, float64(0), zero)
}
