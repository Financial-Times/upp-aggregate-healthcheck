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

	app.Action = func() {
		initLogs(os.Stdout, os.Stdout, os.Stderr)

		controller := initializeController(*environment)
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
	r.HandleFunc("/add-ack", httpHandler.handleAddAck).Methods("POST")
	r.HandleFunc("/enable-category", httpHandler.handleEnableCategory)
	r.HandleFunc("/disable-category", httpHandler.handleDisableCategory)
	r.HandleFunc("/rem-ack", httpHandler.handleRemoveAck)
	r.HandleFunc("/add-ack-form", httpHandler.handleAddAckForm)
	r.HandleFunc("/", httpHandler.handleServicesHealthCheck)
	r.HandleFunc("/__health", httpHandler.handleServicesHealthCheck)
	r.HandleFunc("/__pods-health", httpHandler.handlePodsHealthCheck)
	r.HandleFunc("/__pod-individual-health", httpHandler.handleIndividualPodHealthCheck)
	r.HandleFunc("/__gtg", httpHandler.handleGoodToGo)
	r.PathPrefix("/").Handler(http.StripPrefix("/", http.FileServer(http.Dir("resources/"))))

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
