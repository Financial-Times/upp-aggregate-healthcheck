package main

import (
	"fmt"
	"github.com/gorilla/mux"
	"github.com/jawher/mow.cli"
	"io"
	"log"
	"net/http"
	"os"
)

const logPattern = log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile | log.LUTC

var infoLogger *log.Logger
var warnLogger *log.Logger
var errorLogger *log.Logger

func main() {
	app := cli.App("aggregate-healthcheck", "Monitoring health of multiple services in cluster.")

	environment := app.String(cli.StringOpt{
		Name:   "environment",
		Value:  "Kubernetes",
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

	app.Action = func() {
		initLogs(os.Stdout, os.Stdout, os.Stderr)

		controller := initializeController(*environment)
		handler := &httpHandler{
			controller: controller,
			pathPrefix: *pathPrefix,
		}

		graphiteFeeder := newGraphiteFeeder(*graphiteURL, *environment, controller)
		go graphiteFeeder.feed()

		listen(handler, *pathPrefix)
	}

	err := app.Run(os.Args)
	if err != nil {
		panic(fmt.Sprintf("Cannot run the app. Error was: %v", err))
	}
}

func listen(httpHandler *httpHandler, pathPrefix string) {
	r := mux.NewRouter()
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
	s.HandleFunc("/__gtg", httpHandler.handleGoodToGo)
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
