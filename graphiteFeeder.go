package main

import (
	"errors"
	"fmt"
	fthealth "github.com/Financial-Times/go-fthealth/v1a"
	"net"
	"strings"
	"time"
)

const (
	pilotLightFormat = "coco.health.%s.pilot-light 1 %d\n"
	metricFormat = "coco.health.%s.services.%s %d %d\n"
)

type graphiteFeeder struct {
	url         string
	environment string
	connection  net.Conn
	ticker      *time.Ticker
	controller  controller
}

type bufferedHealths struct {
	buffer chan fthealth.CheckResult
}

func newBufferedHealths() *bufferedHealths {
	buffer := make(chan fthealth.CheckResult, 60)
	return &bufferedHealths{buffer}
}

func newGraphiteFeeder(url string, environment string, controller controller) *graphiteFeeder {
	connection := tcpConnect(url)
	ticker := time.NewTicker(60 * time.Second)
	return &graphiteFeeder{
		url: url, environment:environment,
		connection: connection,
		ticker:ticker,
		controller:controller,
	}
}

func (g graphiteFeeder) feed() {
	for range g.ticker.C {
		errPilot := g.sendPilotLight()
		if errPilot != nil {
			warnLogger.Printf("Problem encountered while sending pilot light to Graphite. [%v]", errPilot.Error())
			g.reconnect()
		}

		errBuff := g.sendBuffers()
		if errBuff != nil {
			warnLogger.Printf("Problem encountered while sending buffered health to Graphite. [%v]", errBuff.Error())
			g.reconnect()
		}
	}
}

func (g graphiteFeeder) sendPilotLight() error {
	if g.connection == nil {
		return errors.New("Can't send pilot light, no Graphite connection is set")
	}

	_, err := fmt.Fprintf(g.connection, pilotLightFormat, g.environment, time.Now().Unix())
	return err
}

func (g graphiteFeeder) sendBuffers() error {
	for _, mService := range g.controller.getMeasuredServices() {
		err := g.sendOneBuffer(mService)
		if err != nil {
			return err
		}
	}
	return nil
}

func (g graphiteFeeder) sendOneBuffer(mService measuredService) error {
	for {
		select {
		case checkResult := <-mService.bufferedHealths.buffer:
			err := g.sendOne(checkResult)
			if err != nil {
				addBack(mService.bufferedHealths, checkResult)
				return err
			}
		default:
			return nil
		}
	}
}

func (g *graphiteFeeder) sendOne(check fthealth.CheckResult) error {
	if g.connection == nil {
		return errors.New("Can't send results, no Graphite connection")
	}
	name := strings.Replace(check.Name, ".", "-", -1)
	_, err := fmt.Fprintf(g.connection, metricFormat, g.environment, name, inverseBoolToInt(check.Ok), check.LastUpdated.Unix())
	return err
}

func addBack(bufferedHealths *bufferedHealths, checkResult fthealth.CheckResult) {
	select {
	case bufferedHealths.buffer <- checkResult:
	default:
	}
}

func (g *graphiteFeeder) reconnect() {
	infoLogger.Println("Reconnecting to Graphite host.")
	if g.connection != nil {
		g.connection.Close()
	}
	g.connection = tcpConnect(g.url)
}

func tcpConnect(url string) net.Conn {
	conn, err := net.Dial("tcp", url)
	if err != nil {
		warnLogger.Printf("Error while creating TCP connection [%v]", err)
		return nil
	}
	tcpConn := conn.(*net.TCPConn)
	tcpConn.SetKeepAlive(true)
	tcpConn.SetKeepAlivePeriod(30 * time.Minute)
	return conn
}

func inverseBoolToInt(b bool) int {
	if b {
		return 0
	}
	return 1
}
