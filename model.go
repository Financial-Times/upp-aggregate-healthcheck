package main

import (
	"sync"
	"time"
)

type pod struct {
	name        string
	node        string
	ip          string
	serviceName string
}

type category struct {
	name             string
	services         []string
	refreshPeriod    time.Duration
	isSticky         bool
	isEnabled        bool
	failureThreshold int
}

type deployment struct {
	desiredReplicas int32
}

type service struct {
	name        string
	sysCode     string
	ack         string
	appPort     int32
	isResilient bool
	isDaemon    bool
}

type servicesMap struct {
	sync.RWMutex
	m map[string]service
}

type measuredService struct {
	service            service
	cachedHealth       *cachedHealth
	cachedHealthMetric *cachedHealth
}
