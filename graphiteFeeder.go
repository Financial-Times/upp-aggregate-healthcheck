package main

import (
	"time"
	"net"
	"strconv"
	"fmt"
	"errors"
	fthealth "github.com/Financial-Times/go-fthealth/v1a"
	"strings"
)

const (
	pilotLightFormat = "coco.health.%s.pilot-light 1 %d\n"
	metricFormat     = "coco.health.%s.services.%s %d %d\n"
)

type GraphiteFeeder struct {
	host        string
	port        int
	environment string
	connection  net.Conn
	ticker      *time.Ticker
	controller  controller //todo: add here controller interface to get the map of measured services.
			       //todo: ensure that ALL services are checked.
}

type BufferedHealths struct {
	buffer chan fthealth.CheckResult
}

func NewBufferedHealths() *BufferedHealths {
	buffer := make(chan fthealth.CheckResult, 60)
	return &BufferedHealths{buffer}
}

func NewGraphiteFeeder(host string, port int, environment string, controller controller) *GraphiteFeeder {
	connection := tcpConnect(host, port)
	ticker := time.NewTicker(60 * time.Second) //todo: should this value be configurable?
	return &GraphiteFeeder{host, port, environment, connection, ticker, controller}
}

func (g GraphiteFeeder) feed() {
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

func (g GraphiteFeeder) sendPilotLight() error {
	if g.connection == nil {
		return errors.New("Can't send pilot light, no Graphite connection is set.")
	}

	_, err := fmt.Fprintf(g.connection, pilotLightFormat, g.environment, time.Now().Unix())
	if err != nil {
		return err
	}

	return nil
}

func (g GraphiteFeeder) sendBuffers() error {
	//todo: ensure that all services are measured and cached when starting the agg-hc service
	for _, mService := range g.controller.getMeasuredServices() {
		err := g.sendOneBuffer(mService)
		if err != nil {
			return err
		}
	}
	return nil
}

func (g GraphiteFeeder) sendOneBuffer(mService measuredService) error {
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

func (g *GraphiteFeeder) sendOne(check fthealth.CheckResult) error {
	if g.connection == nil {
		return errors.New("Can't send results, no Graphite connection.")
	}
	name := strings.Replace(check.Name, ".", "-", -1)
	_, err := fmt.Fprintf(g.connection, metricFormat, g.environment, name, inverseBoolToInt(check.Ok), check.LastUpdated.Unix())
	if err != nil {
		return err
	}
	return nil
}

func addBack(bufferedHealths *BufferedHealths, checkResult fthealth.CheckResult) {
	select {
	case bufferedHealths.buffer <- checkResult:
	default:
	}
}

func (g *GraphiteFeeder) reconnect() {
	infoLogger.Println("Reconnecting to Graphite host.")
	if g.connection != nil {
		g.connection.Close()
	}
	g.connection = tcpConnect(g.host, g.port)
}

func tcpConnect(host string, port int) net.Conn {
	conn, err := net.Dial("tcp", host + ":" + strconv.Itoa(port))
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
