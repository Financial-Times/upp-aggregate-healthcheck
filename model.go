package main

import (
	"sync"
	"time"
)

type pod struct {
	name        string
	ip          string
	serviceName string
}

type service struct {
	name        string
	ack         string
	appPort     int32
	isResilient bool
	isDaemon    bool
}

type category struct {
	name          string
	services      []string
	refreshPeriod time.Duration
	isSticky      bool
	isEnabled     bool
}

type deployment struct {
	numberOfAvailableReplicas   int32
	numberOfUnavailableReplicas int32
}

type deploymentsMap struct {
	sync.RWMutex
	m map[string]deployment
}
