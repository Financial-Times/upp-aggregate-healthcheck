package main

import "time"

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
