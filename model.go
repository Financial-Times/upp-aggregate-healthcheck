package main

import "time"

type pod struct {
	name string
	ip   string
}

type service struct {
	name     string
	severity uint8
	ack      string
}

type category struct {
	name          string
	services      []string
	refreshPeriod time.Duration
	isSticky      bool
	isEnabled     bool
}
