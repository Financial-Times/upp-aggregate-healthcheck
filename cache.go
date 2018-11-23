package main

import fthealth "github.com/Financial-Times/go-fthealth/v1_1"

type cachedHealth struct {
	toWriteToCache  chan fthealth.CheckResult
	toReadFromCache chan fthealth.CheckResult
	terminate       chan bool
}

func newCachedHealth() *cachedHealth {
	a := make(chan fthealth.CheckResult)
	b := make(chan fthealth.CheckResult)
	terminate := make(chan bool)
	return &cachedHealth{
		toWriteToCache:  a,
		toReadFromCache: b,
		terminate:       terminate,
	}
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
