<!--
    Written in the format prescribed by https://github.com/Financial-Times/runbook.md.
    Any future edits should abide by this format.
-->
# UPP - Aggregate Healthcheck

The purpose of this service is to aggregate the healthchecks from services and pods in the Kubernetes cluster.

## Code

upp-aggregate-healthcheck

## Primary URL

https://upp-prod-publish-glb.upp.ft.com/__upp-aggregate-healthcheck/

## Service Tier

Platinum

## Lifecycle Stage

Production

## Host Platform

AWS

## Architecture

Upp-aggregate-healthcheck exposes multiple endpoints, which provide health data on service or category level. This service is deployed twice in the Publish and Delivery clusters -
as `upp-aggregate-healthcheck` and as `upp-aggregate-healthcheck-second` to be able to handle heavy traffic.

## Contains Personal Data

No

## Contains Sensitive Data

No

<!-- Placeholder - remove HTML comment markers to activate
## Can Download Personal Data
Choose Yes or No

...or delete this placeholder if not applicable to this system
-->

<!-- Placeholder - remove HTML comment markers to activate
## Can Contact Individuals
Choose Yes or No

...or delete this placeholder if not applicable to this system
-->

## Failover Architecture Type

ActivePassive

## Failover Process Type

FullyAutomated

## Failback Process Type

FullyAutomated

## Failover Details

The service is deployed in Publish, Delivery and PAC clusters with different configs. So you can follow these guides:

For Publish: <https://github.com/Financial-Times/upp-docs/tree/master/failover-guides/publishing-cluster>
For Delivery: <https://github.com/Financial-Times/upp-docs/tree/master/failover-guides/delivery-cluster>
For PAC: <https://github.com/Financial-Times/upp-docs/tree/master/failover-guides/pac-cluster>

## Data Recovery Process Type

NotApplicable

## Data Recovery Details

The service does not store data, so it does not require any data recovery steps.

## Release Process Type

PartiallyAutomated

## Rollback Process Type

Manual

## Release Details

Manual failover is needed when a new version of
the service is deployed to production.
Otherwise, an automated failover is going to take place when releasing.
For more details about the failover process please see:

<https://github.com/Financial-Times/upp-docs/tree/master/failover-guides/publishing-cluster>

<https://github.com/Financial-Times/upp-docs/tree/master/failover-guides/delivery-cluster>

<https://github.com/Financial-Times/upp-docs/tree/master/failover-guides/pac-cluster>

<!-- Placeholder - remove HTML comment markers to activate
## Heroku Pipeline Name
Enter descriptive text satisfying the following:
This is the name of the Heroku pipeline for this system. If you don't have a pipeline, this is the name of the app in Heroku. A pipeline is a group of Heroku apps that share the same codebase where each app in a pipeline represents the different stages in a continuous delivery workflow, i.e. staging, production.

...or delete this placeholder if not applicable to this system
-->

## Key Management Process Type

Manual

## Key Management Details

To access the service clients need to provide basic auth credentials.
To rotate credentials you need to login to a particular cluster and update varnish-auth secrets.

## Monitoring

Service in UPP K8S publishing clusters:

*   Pub-Prod-EU health: <https://upp-prod-publish-eu.upp.ft.com/__upp-aggregate-healthcheck/__health>
*   Pub-Prod-US health: <https://upp-prod-publish-us.upp.ft.com/__upp-aggregate-healthcheck/__health>

Service in UPP K8S delivery clusters:

*   Delivery-Prod-EU health: <https://upp-prod-delivery-eu.upp.ft.com/__upp-aggregate-healthcheck/__health>
*   Delivery-Prod-US health: <https://upp-prod-delivery-us.upp.ft.com/__upp-aggregate-healthcheck/__health>

Service in UPP PAC clusters:

*   PAC-EU health: <https://pac-prod-eu.upp.ft.com/__health>
*   PAC-US health: <https://pac-prod-us.upp.ft.com/__health>

## First Line Troubleshooting

<https://github.com/Financial-Times/upp-docs/tree/master/guides/ops/first-line-troubleshooting>

## Second Line Troubleshooting

Please refer to the GitHub repository README for troubleshooting information.