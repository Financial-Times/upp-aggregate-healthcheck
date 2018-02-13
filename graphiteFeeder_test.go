package main

import (
	"net"
	"testing"
	"time"

	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
	"github.com/stretchr/testify/assert"
)

type DummyConnection struct {
	b []byte
}

func (c *DummyConnection) Read(b []byte) (n int, err error) {
	return 0, nil
}

func (c *DummyConnection) Write(b []byte) (n int, err error) {
	c.b = b
	return 0, nil
}

func (c DummyConnection) Close() error {
	return nil
}

func (c *DummyConnection) LocalAddr() net.Addr {
	return nil
}

func (c *DummyConnection) RemoteAddr() net.Addr {
	return nil
}

func (c *DummyConnection) SetDeadline(t time.Time) error {
	return nil
}

func (c *DummyConnection) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *DummyConnection) SetWriteDeadline(t time.Time) error {
	return nil
}

func NewMockGraphiteFeeder(conn *DummyConnection) *graphiteFeeder {
	ticker := time.NewTicker(1 * time.Second)
	return &graphiteFeeder{
		url:         "dummy-url",
		environment: "test-env",
		connection:  conn,
		ticker:      ticker,
		controller:  nil}
}

func TestPilotLightHappyFlow(t *testing.T) {
	conn := &DummyConnection{}
	graphiteFeeder := NewMockGraphiteFeeder(conn)
	err := graphiteFeeder.sendPilotLight()

	assert.NotNil(t, conn.b)
	assert.Nil(t, err)
	graphiteFeeder.ticker.Stop()
}

func TestPilotLightNilConnection(t *testing.T) {
	graphiteFeeder := &graphiteFeeder{}
	err := graphiteFeeder.sendPilotLight()

	assert.NotNil(t, err)
}

func TestAddBackHappyFlow(t *testing.T) {
	bufferedHealths := newBufferedHealths()
	addBack(bufferedHealths, fthealth.CheckResult{})

	select {
	case <-bufferedHealths.buffer:
	default:
		assert.Fail(t, "Buffer should not be empty")
	}
}

func TestSendBuffersWithNoHealths(t *testing.T) {
	mServices := make(map[string]measuredService)
	hc := &healthCheckController{measuredServices: mServices}
	graphiteFeeder := &graphiteFeeder{controller: hc}
	assert.Nil(t, graphiteFeeder.connection)
	err := graphiteFeeder.sendBuffers()

	assert.Nil(t, err)
}

func TestSendBuffersHappyFlow(t *testing.T) {
	mServices := make(map[string]measuredService)
	bufferedHealths := newBufferedHealths()
	mServices["test"] = measuredService{bufferedHealths: bufferedHealths}
	select {
	case bufferedHealths.buffer <- fthealth.CheckResult{}:
	default:
	}

	hc := &healthCheckController{measuredServices: mServices}
	graphiteFeeder := &graphiteFeeder{controller: hc}
	assert.Nil(t, graphiteFeeder.connection)
	err := graphiteFeeder.sendBuffers()

	assert.NotNil(t, err)
	select {
	case <-bufferedHealths.buffer:
	default:
		assert.Fail(t, "Buffer should not be empty")
	}
}
