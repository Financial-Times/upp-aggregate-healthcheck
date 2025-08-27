package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	log "github.com/Financial-Times/go-logger"

	"github.com/gorilla/mux"
	cli "github.com/jawher/mow.cli"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	app := cli.App("upp-aggregate-healthcheck", "Monitoring health of multiple services in cluster.")

	appName := app.String(cli.StringOpt{
		Name:   "app-name",
		Value:  "upp-aggregate-healthcheck",
		Desc:   "Application name",
		EnvVar: "APP_NAME",
	})

	port := app.Int(cli.IntOpt{
		Name:   "port",
		Value:  8080,
		Desc:   "Port to listen on",
		EnvVar: "APP_PORT",
	})

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

	clusterURL := app.String(cli.StringOpt{
		Name:   "cluster-url",
		Value:  "",
		Desc:   "Cluster URL",
		EnvVar: "CLUSTER_URL",
	})

	logLevel := app.String(cli.StringOpt{
		Name:   "log-level",
		Value:  "INFO",
		Desc:   "App log level",
		EnvVar: "LOG_LEVEL",
	})

	healthcheckTimeoutSeconds := app.Int(cli.IntOpt{
		Name:   "healthcheck-timeout-seconds",
		Value:  20,
		Desc:   "Number of seconds to wait before a healthcheck request times out",
		EnvVar: "TIMEOUT_SECONDS",
	})

	log.InitLogger(*appName, *logLevel)

	app.Action = func() {
		log.Infof("Starting app with params: [environment: %s], [pathPrefix: %s]", *environment, *pathPrefix)

		controller := initializeController(*environment, *healthcheckTimeoutSeconds)
		handler := &httpHandler{
			controller: controller,
			pathPrefix: *pathPrefix,
			clusterURL: *clusterURL,
		}

		prometheusFeeder := newPrometheusFeeder(*environment, controller)
		go prometheusFeeder.feed()

		listen(handler, *pathPrefix, *port)
	}

	err := app.Run(os.Args)
	if err != nil {
		panic(fmt.Sprintf("Cannot run the app. Error was: %v", err))
	}
}

func listen(httpHandler *httpHandler, pathPrefix string, port int) {
	r := mux.NewRouter()
	r.HandleFunc("/__gtg", httpHandler.handleGoodToGo)
	r.Handle("/metrics", promhttp.Handler())
	s := r.PathPrefix(pathPrefix).Subrouter()
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

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		WriteTimeout: time.Second * 90,
		ReadTimeout:  time.Second * 90,
		IdleTimeout:  time.Second * 90,
		Handler:      r,
	}

	err := srv.ListenAndServe()
	if err != nil {
		panic(fmt.Sprintf("Cannot set up HTTP listener. Error was: %v", err))
	}
}
