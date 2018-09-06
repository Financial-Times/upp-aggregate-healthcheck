package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/jawher/mow.cli"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const logPattern = log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile | log.LUTC

var infoLogger *log.Logger
var warnLogger *log.Logger
var errorLogger *log.Logger

func main() {
	app := cli.App("aggregate-healthcheck", "Monitoring health of multiple services in cluster.")

	environment := app.String(cli.StringOpt{
		Name:   "environment",
		Value:  "Default-environment",
		Desc:   "Environment tag (e.g. local, pre-prod, prod-uk)",
		EnvVar: "ENVIRONMENT",
	})

	pathPrefix := app.String(cli.StringOpt{
		Name:   "pathPrefix",
		Value:  "",
		Desc:   "Path prefix for all endpoints",
		EnvVar: "PATH_PREFIX",
	})

	graphiteURL := app.String(cli.StringOpt{
		Name:   "graphite-host",
		Value:  "graphite.ft.com:2003",
		Desc:   "Graphite url",
		EnvVar: "GRAPHITE_URL",
	})

	clusterURL := app.String(cli.StringOpt{
		Name:   "cluster-url",
		Value:  "",
		Desc:   "Cluster URL",
		EnvVar: "CLUSTER_URL",
	})

	app.Action = func() {
		initLogs(os.Stdout, os.Stdout, os.Stderr)
		infoLogger.Printf("Starting app with params: [environment: %s], [pathPrefix: %s], [graphiteURL: %s]", *environment, *pathPrefix, *graphiteURL)

		controller := initializeController(*environment)
		handler := &httpHandler{
			controller: controller,
			pathPrefix: *pathPrefix,
			clusterURL: *clusterURL,
		}

		graphiteFeeder := newGraphiteFeeder(*graphiteURL, *environment, controller)
		go graphiteFeeder.feed()

		pilotLight := prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "upp",
				Subsystem: "health",
				Name:      "pilotlight",
				Help:      "Pilot light for the service monitoring UPP service health",
			},
			[]string{
				"environment",
			})
		prometheus.MustRegister(pilotLight)
		pilotLight.With(prometheus.Labels{"environment": *environment}).Set(1)
		listen(handler, *pathPrefix)
	}

	err := app.Run(os.Args)
	if err != nil {
		panic(fmt.Sprintf("Cannot run the app. Error was: %v", err))
	}
}

func listen(httpHandler *httpHandler, pathPrefix string) {
	r := mux.NewRouter()
	r.HandleFunc("/__gtg", httpHandler.handleGoodToGo)
	s := r.PathPrefix(pathPrefix).Subrouter()
	r.Handle("/metrics", promhttp.Handler())
	s.HandleFunc("/add-ack", httpHandler.handleAddAck).Methods("POST")
	s.HandleFunc("/enable-category", httpHandler.handleEnableCategory)
	s.HandleFunc("/disable-category", httpHandler.handleDisableCategory)
	s.HandleFunc("/rem-ack", httpHandler.handleRemoveAck)
	s.HandleFunc("/add-ack-form", httpHandler.handleAddAckForm)
	s.HandleFunc("", httpHandler.handleServicesHealthCheck)
	s.HandleFunc("/", httpHandler.handleServicesHealthCheck)
	s.HandleFunc("/__pods-health", httpHandler.handlePodsHealthCheck)
	s.HandleFunc("/__pod-individual-health", httpHandler.handleIndividualPodHealthCheck)
	s.PathPrefix("/").Handler(http.StripPrefix("/", http.FileServer(http.Dir("resources/"))))
	err := http.ListenAndServe(":8080", r)
	if err != nil {
		panic(fmt.Sprintf("Cannot set up HTTP listener. Error was: %v", err))
	}
}

func initLogs(infoHandle io.Writer, warnHandle io.Writer, errorHandle io.Writer) {
	infoLogger = log.New(infoHandle, "INFO  - ", logPattern)
	warnLogger = log.New(warnHandle, "WARN  - ", logPattern)
	errorLogger = log.New(errorHandle, "ERROR - ", logPattern)
}
