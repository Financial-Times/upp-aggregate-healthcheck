package main

import fthealth "github.com/Financial-Times/go-fthealth/v1a"

type cachedHealth struct {
	toWriteToCache  chan fthealth.HealthResult
	toReadFromCache chan fthealth.HealthResult
	terminate       chan bool
}

func NewCachedHealth() *cachedHealth {
	a := make(chan fthealth.HealthResult)
	b := make(chan fthealth.HealthResult)
	terminate := make(chan bool)
	return &cachedHealth{a, b, terminate}
}

func (c *cachedHealth) maintainLatest() {
	var aux fthealth.HealthResult
	for {
		select {
		case aux = <-c.toWriteToCache:
		case c.toReadFromCache <- aux:
		}
	}
}
