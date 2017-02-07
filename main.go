package main

import (
	"github.com/jawher/mow.cli"
	"os"
	"io"
	"log"
	"github.com/gorilla/mux"
	"net/http"
	"fmt"
)

const logPattern = log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile | log.LUTC

var infoLogger *log.Logger
var warnLogger *log.Logger
var errorLogger *log.Logger

func main() {
	app := cli.App("aggregate-healthcheck", "Monitoring health of multiple services in cluster.")

	environment := app.String(cli.StringOpt{
		Name:   "environment",
		Value:  "local",
		Desc:   "Environment tag (e.g. local, pre-prod, prod-uk)",
		EnvVar: "ENVIRONMENT",
	})

	app.Action = func() {
		initLogs(os.Stdout, os.Stdout, os.Stderr)

		controller := InitializeController(environment)
		handler := &httpHandler{
			controller: controller,
		}

		listen(handler)
	}

	err := app.Run(os.Args)
	if err != nil {
		panic(fmt.Sprintf("Cannot run the app. Error was: %v", err))
	}
}

func listen(httpHandler *httpHandler) {
	r := mux.NewRouter()

	r.HandleFunc("/", httpHandler.handleServicesHealthCheck)
	r.HandleFunc("/__health", httpHandler.handleServicesHealthCheck)
	r.HandleFunc("/__pods-health", httpHandler.handlePodsHealthCheck)
	r.HandleFunc("/__pod-individual-health", httpHandler.handleIndividualPodHealthCheck)
	r.HandleFunc("/__gtg", httpHandler.handleGoodToGo)

	err := http.ListenAndServe(":8080", r)
	if err != nil {
		panic(fmt.Sprintf("Cannotset up HTTP listener. Error was: %v", err))
	}
}

func initLogs(infoHandle io.Writer, warnHandle io.Writer, errorHandle io.Writer) {
	infoLogger = log.New(infoHandle, "INFO  - ", logPattern)
	warnLogger = log.New(warnHandle, "WARN  - ", logPattern)
	errorLogger = log.New(errorHandle, "ERROR - ", logPattern)
}
