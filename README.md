# upp-aggregate-healthcheck
[![Circle CI](https://circleci.com/gh/Financial-Times/upp-aggregate-healthcheck.svg?style=shield)](https://circleci.com/gh/Financial-Times/upp-aggregate-healthcheck) [![Go Report Card](https://goreportcard.com/badge/github.com/Financial-Times/upp-aggregate-healthcheck)](https://goreportcard.com/report/github.com/Financial-Times/upp-aggregate-healthcheck) [![Coverage Status](https://coveralls.io/repos/github/Financial-Times/upp-aggregate-healthcheck/badge.svg)](https://coveralls.io/github/Financial-Times/upp-aggregate-healthcheck)

The purpose of this service is to aggregate the healthchecks from services and pods in the Kubernetes cluster.

## Usage
 In this section, the aggregate-healthcheck functionalities are described.
### Get services health
 A service is considered to be healthy if it has at least one pod that is able to serve requests. To determine which pods are able to serve requests,
 there is a readinessProbe configured on the deployment, which checks the GoodToGo endpoint of the app running inside the pod. If the GoodToGo responds
 with a 503 Service Unavailable status code, the pod will not serve requests anymore, until it will receive 200 OK status code on GoodToGo endpoint.

 For a service, if there is at least one pod that can serve requests, the service will be considered healthy, but if there are pods that are unavailable,
 a message will be displayed in the "Output" section of the corresponding service.
 As an exception, if a service is marked as non-resilient (it has the __isResilient: "false"__ label), it will be considered unhealthy if there is at least one pod which is unhealthy.

 Not that for services are grouped into categories, therefore there is the possibility to query the aggregate-healthcheck only for a certain list of categories.
 If no category is provided, the healthchecks of all services will be displayed.

### Get pods health for a service
 The healths of the pods are evaluated by querying the __health endpoint of apps inside the pods. Given a pod, if there is at least one check that fails,
 the pod health will be considered warning or critical, based on the severity level of the check that fails.
### Acknowledge a service
 When a service is unhealthy, there is a possibility to acknowledge the warning. By acknowledging all the services that are unhealthy,
 the general status of the aggregate-healthcheck will become healthy (it will also mention that there are 'n' services acknowledged).
### Sticky categories
 Categories can be sticky, meaning that if one of the services become unhealthy, the category will be disabled, meaning that it will be unhealthy,
  until manual re-enabling it. There is an endpoint for enabling a category.
## Running locally
To run the service locally, you will need to run the following commands first to get the vendored dependencies for this project:
  `go get github.com/kardianos/govendor` and
  `govendor sync`
 
 There is a limited number of functionalities that can be used locally, because we are querying all the apps, inside the pods and there is no current
  solution of accessing them outside of the cluster, without using port-forwarding.
 The list of functionalities that can be used outside of the cluster are:
  * Add/Remove acknowledge
  * Enable/Disable sticky categories
## How to configure services for aggregate-healthcheck
 For a service to be taken into consideration by aggregate-healthcheck it needs to have the following:
 * The Kubernetes service should have __hasHealthcheck: "true"__ label.
 * The container should have Kubernetes `readinessProbe` configured to check the `__gtg` endpoint of the app
 * The app should have `__gtg` and `__health` endpoints.
 * Optionally the Kubernetes service can have:
   - `isResilient: "false"` label which will cause the service to be unhealthy if there is at least one pod that is unhealthy
   - `isDaemon: "true"` label which indicates that the pods are managed by a daemonSet instead of a deployment.

## Endpoints
 In the following section, aggregate-healthcheck endpoints are described.
 Note that this app has two options of retrieving healthchecks:
  - `JSON format` - to get the results in JSON format, provide the `"Accept: application/json"` header
  - `HTML format` - this is the default format of displaying healthchecks.

### Service endpoints
 Note that there is a configurable __pathPrefix__ which will be the prefix of each endpoint's path (E.g. if the
 prefix is `__health`, the endpoint path for add-ack is `__health/add-ack`. The default value for __pathPrefix__ is the empty string.
 * `<pathPrefix>/__health` - Perform services healthcheck.
    - params:
       - `categories` - the healthcheck will be performed on the services belonging to the provided categories.
       - `cache` - if set to false, the healthchecks will be performed without the help of cache. By default, the cache is used.
 * `<pathPrefix>/__pods-health` - Perform pods healthcheck for a service.
    - params:
       - `service-name` - The healthcheck will be performed only for pods belonging to the provided service.
 * `<pathPrefix>/__pod-individual-health` - Retrieves the healthchecks of the app running inside the pod.
    - params:
       - `pod-name` - The name of the pod for which the healthchecks will be retrieved.
 * `<pathPrefix>/add-ack` - (POST) Acknowledges a service
    - params:
       - `service-name` - The service to be acknowledged.
    - request body:
       - `ack-msg` the acknowledge message.
 * `<pathPrefix>/rem-ack` - Removes the acknowledge of a service
    - params:
       - `service-name` - The service to be updated.
 * `<pathPrefix>/enable-category` - Enables a category. This is used for sticky categories which are unhealthy.
    - params:
       - `category-name` - The category to be enabled.
 * `<pathPrefix>/disable-category` - Disables a category. This is useful when doing a failover.
    - params:
       - `category-name` - The category to be disabled.

### Admin endpoints
 * `__health`
 * `__gtg`
    - params:
       - `categories` - the healthcheck will be performed on the services belonging to the provided categories.
       - `cache` - if set to false, the healthchecks will be performed without the help of cache. By default, the cache is used.
    - returns a __503 Service Unavailable__ status code in the following cases:
       - if at least one of the provided categories is disabled (see sticky functionality)
       - if at least one of the checked services is unhealthy
    - returns a __200 OK__ status code otherwise
