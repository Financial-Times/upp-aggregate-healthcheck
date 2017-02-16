package main

import fthealth "github.com/Financial-Times/go-fthealth/v1a"

type cachedHealth struct {
	toWriteToCache  chan fthealth.CheckResult
	toReadFromCache chan fthealth.CheckResult
	terminate       chan bool
}

func newCachedHealth() *cachedHealth {
	a := make(chan fthealth.CheckResult)
	b := make(chan fthealth.CheckResult)
	terminate := make(chan bool)
	return &cachedHealth{a, b, terminate}
}

func (c *cachedHealth) maintainLatest() {
	var aux fthealth.CheckResult
	for {
		select {
		case aux = <-c.toWriteToCache:
		case c.toReadFromCache <- aux:
		}
	}
}
