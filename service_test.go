package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Financial-Times/go-logger"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8smeta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8swatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
)

func init() {
	logger.InitLogger("upp-aggregate-healthcheck", "debug")
}

type mockTransport struct {
	responseStatusCode int
	responseBody       string
}

const (
	validIP                             = "1.0.0.0"
	validK8sServiceName                 = "validServiceName"
	validK8sServiceNameWithAck          = "validK8sServiceNameWithAck"
	validSeverity                       = uint8(1)
	ackMsg                              = "ack-msg"
	validFailingHealthCheckResponseBody = `{
  "schemaVersion": 1,
  "name": "CMSNotifierApplication",
  "description": "CMSNotifierApplication",
  "checks": [
    {
      "checkOutput": "",
      "panicGuide": "",
      "lastUpdated": "",
      "ok": false,
      "severity": 2,
      "businessImpact": "",
      "technicalSummary": "",
      "name": ""
    },
	{
      "checkOutput": "",
      "panicGuide": "",
      "lastUpdated": "",
      "ok": false,
      "severity": 1,
      "businessImpact": "",
      "technicalSummary": "",
      "name": ""
    }
  ]
}`
	validFailingHealthCheckResponseBodyWithSeverity2 = `{
  "schemaVersion": 1,
  "name": "CMSNotifierApplication",
  "description": "CMSNotifierApplication",
  "checks": [
    {
      "checkOutput": "",
      "panicGuide": "",
      "lastUpdated": "",
      "ok": false,
      "severity": 2,
      "businessImpact": "",
      "technicalSummary": "",
      "name": ""
    }
  ]
}`
	//nolint:gosec // test fixture, not a real credential
	validPassingHealthCheckResponseBody = `{
  "schemaVersion": 1,
  "name": "CMSNotifierApplication",
  "description": "CMSNotifierApplication",
  "checks": [
    {
      "checkOutput": "",
      "panicGuide": "",
      "lastUpdated": "",
      "ok": true,
      "severity": 2,
      "businessImpact": "",
      "technicalSummary": "",
      "name": ""
    }
  ]
}`
)

func initializeMockServiceWithK8sServices() *k8sHealthcheckService {
	services := make(map[string]service)
	services[validK8sServiceName] = service{
		name: validServiceName,
	}
	services[validK8sServiceNameWithAck] = service{
		name: validK8sServiceNameWithAck,
		ack:  "test",
	}
	return &k8sHealthcheckService{
		services: servicesMap{
			m: services,
		},
	}
}

func initializeMockService(httpClient *http.Client) *k8sHealthcheckService {
	mockK8sClient := fake.NewSimpleClientset()
	mockCategoriesK8sClient := fake.NewSimpleClientset()

	return &k8sHealthcheckService{
		k8sClient:           mockK8sClient,
		categoriesK8sClient: mockCategoriesK8sClient,
		httpClient:          httpClient,
	}
}

func initializeMockHTTPClient(responseStatusCode int, responseBody string) *http.Client {
	client := getDefaultClient()
	client.Transport = &mockTransport{
		responseStatusCode: responseStatusCode,
		responseBody:       responseBody,
	}

	return client
}

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	response := &http.Response{
		Header:     make(http.Header),
		Request:    req,
		StatusCode: t.responseStatusCode,
	}

	response.Header.Set("Content-Type", "application/json")
	response.Body = io.NopCloser(strings.NewReader(t.responseBody))

	return response, nil
}

func TestGetHealthChecksForPodInternalServerErr(t *testing.T) {
	service := initializeMockService(initializeMockHTTPClient(http.StatusInternalServerError, ""))
	_, err := service.getHealthChecksForPod(pod{name: "test", ip: validIP}, 8080)
	assert.NotNil(t, err)
}

func TestGetHealthChecksForPodHealthAvailable(t *testing.T) {
	service := initializeMockService(initializeMockHTTPClient(http.StatusOK, validFailingHealthCheckResponseBody))
	healthCheckResponse, err := service.getHealthChecksForPod(pod{name: "test", ip: validIP}, 8080)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(healthCheckResponse.Checks))
}

func TestGetIndividualPodSeverityErrorWhilePodHealthCheck(t *testing.T) {
	service := initializeMockService(initializeMockHTTPClient(http.StatusInternalServerError, ""))
	severity, _, err := service.getIndividualPodSeverity(pod{name: "test", ip: validIP}, 8080)
	assert.NotNil(t, err)
	assert.Equal(t, defaultSeverity, severity)
}

func TestGetIndividualPodSeverityValidPodHealth(t *testing.T) {
	service := initializeMockService(initializeMockHTTPClient(http.StatusOK, validFailingHealthCheckResponseBody))
	severity, checkFailed, err := service.getIndividualPodSeverity(pod{name: "test", ip: validIP}, 8080)
	assert.Nil(t, err)
	assert.True(t, checkFailed)
	assert.Equal(t, validSeverity, severity)
}

func TestGetIndividualPodSeverityValidPodHealth_Severity2(t *testing.T) {
	service := initializeMockService(initializeMockHTTPClient(http.StatusOK, validFailingHealthCheckResponseBodyWithSeverity2))
	severity, checkFailed, err := service.getIndividualPodSeverity(pod{name: "test", ip: validIP}, 8080)
	assert.Nil(t, err)
	assert.True(t, checkFailed)
	assert.Equal(t, uint8(2), severity)
}

func TestCheckPodHealthFailingChecks(t *testing.T) {
	service := initializeMockService(initializeMockHTTPClient(http.StatusOK, validFailingHealthCheckResponseBody))
	err := service.checkPodHealth(pod{name: "test", ip: validIP}, 8080)
	assert.NotNil(t, err)
}

func TestCheckPodHealthWithInvalidUrl(t *testing.T) {
	service := initializeMockService(nil)
	err := service.checkPodHealth(pod{name: "test", ip: "%s"}, 8080)
	assert.NotNil(t, err)
}

func TestCheckPodHealthPassingChecks(t *testing.T) {
	service := initializeMockService(initializeMockHTTPClient(http.StatusOK, validPassingHealthCheckResponseBody))
	err := service.checkPodHealth(pod{name: "test", ip: validIP}, 8080)
	assert.Nil(t, err)
}

func TestGetCategories(t *testing.T) {
	service := initializeMockService(nil)
	_, err := service.getCategories(context.TODO())
	assert.Nil(t, err)
}

func TestRefreshCategoriesCachesKubernetesConfigMaps(t *testing.T) {
	service := initializeMockService(nil)
	_, err := service.categoriesK8sClient.CoreV1().ConfigMaps(apiv1.NamespaceDefault).Create(
		context.TODO(),
		buildCategoryConfigMap("category.test", "test", "service-a, service-b"),
		k8smeta.CreateOptions{},
	)
	assert.Nil(t, err)

	_, err = service.refreshCategories(context.TODO())
	assert.Nil(t, err)

	categories, err := service.getCategories(context.TODO())
	assert.Nil(t, err)
	assert.Equal(t, 1, len(categories))
	assert.Equal(t, []string{"service-a", "service-b"}, categories["test"].services)
}

func TestGetCategoriesUsesCache(t *testing.T) {
	service := initializeMockService(nil)
	service.replaceCategories(map[string]category{
		"default": {
			name:     "default",
			services: []string{"service-a"},
		},
	})

	mock := &fake.Clientset{}
	mock.AddReactor("list", "configmaps", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInternalError(assert.AnError)
	})
	service.k8sClient = mock

	categories, err := service.getCategories(context.TODO())
	assert.Nil(t, err)
	assert.Equal(t, []string{"service-a"}, categories["default"].services)
}

func TestGetCategoriesReturnsCopyOfCache(t *testing.T) {
	service := initializeMockService(nil)
	service.replaceCategories(map[string]category{
		"default": {
			name:     "default",
			services: []string{"service-a"},
		},
	})

	categories, err := service.getCategories(context.TODO())
	assert.Nil(t, err)
	categories["default"] = category{name: "mutated", services: []string{"mutated-service"}}

	categories, err = service.getCategories(context.TODO())
	assert.Nil(t, err)
	assert.Equal(t, "default", categories["default"].name)
	assert.Equal(t, []string{"service-a"}, categories["default"].services)
}

func TestHandleCategoryWatchEventUpdatesCache(t *testing.T) {
	service := initializeMockService(nil)
	service.replaceCategories(map[string]category{})

	categoryConfigMap := buildCategoryConfigMap("category.test", "test", "service-a")
	service.handleCategoryWatchEvent(k8swatch.Event{Type: k8swatch.Added, Object: categoryConfigMap})

	categories, err := service.getCategories(context.TODO())
	assert.Nil(t, err)
	assert.Equal(t, []string{"service-a"}, categories["test"].services)

	categoryConfigMap.Data["category.services"] = "service-b"
	service.handleCategoryWatchEvent(k8swatch.Event{Type: k8swatch.Modified, Object: categoryConfigMap})

	categories, err = service.getCategories(context.TODO())
	assert.Nil(t, err)
	assert.Equal(t, []string{"service-b"}, categories["test"].services)
}

func TestHandleCategoryWatchEventDeletesFromCache(t *testing.T) {
	service := initializeMockService(nil)
	service.replaceCategories(map[string]category{
		"test": {
			name:     "test",
			services: []string{"service-a"},
		},
	})

	service.handleCategoryWatchEvent(k8swatch.Event{
		Type:   k8swatch.Deleted,
		Object: buildCategoryConfigMap("category.test", "test", "service-a"),
	})

	categories, err := service.getCategories(context.TODO())
	assert.Nil(t, err)
	_, found := categories["test"]
	assert.False(t, found)
}

func TestUpdateCategoryInvalidConfigMap(t *testing.T) {
	service := initializeMockService(nil)
	err := service.updateCategory(context.TODO(), "validCategoryName", true)
	assert.NotNil(t, err)
}

func TestUpdateCategoryUpdatesCache(t *testing.T) {
	service := initializeMockService(nil)
	_, err := service.k8sClient.CoreV1().ConfigMaps(apiv1.NamespaceDefault).Create(
		context.TODO(),
		buildCategoryConfigMap("category.test", "test", "service-a"),
		k8smeta.CreateOptions{},
	)
	assert.Nil(t, err)

	err = service.updateCategory(context.TODO(), "test", false)
	assert.Nil(t, err)

	categories, err := service.getCategories(context.TODO())
	assert.Nil(t, err)
	assert.False(t, categories["test"].isEnabled)
}

func TestAddAckConfigMapNotFound(t *testing.T) {
	service := initializeMockService(nil)
	err := service.addAck(context.TODO(), "invalidServiceName", "ack message")
	assert.NotNil(t, err)
}

func TestUpdateAcksForServicesEmptyAckList(t *testing.T) {
	hcService := initializeMockServiceWithK8sServices()
	acks := make(map[string]string)
	acks[validK8sServiceName] = ackMsg
	hcService.updateAcksForServices(acks)

	assert.Equal(t, hcService.services.m[validK8sServiceNameWithAck].ack, "")
	assert.Equal(t, hcService.services.m[validK8sServiceName].ack, ackMsg)
}

func TestGetDeploymentsReturnsDeployments(t *testing.T) {
	service := initializeMockService(nil)
	var replicas int32 = 1
	_, err := service.k8sClient.AppsV1().Deployments(apiv1.NamespaceDefault).Create(
		context.TODO(),
		&appsv1.Deployment{
			ObjectMeta: k8smeta.ObjectMeta{
				Name:      "deployment1",
				Namespace: apiv1.NamespaceDefault,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
			},
		},
		k8smeta.CreateOptions{})
	assert.Nil(t, err)

	_, err = service.k8sClient.AppsV1().Deployments(apiv1.NamespaceDefault).Create(
		context.TODO(),
		&appsv1.Deployment{
			ObjectMeta: k8smeta.ObjectMeta{
				Name:      "deployment2",
				Namespace: apiv1.NamespaceDefault,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
			},
		}, k8smeta.CreateOptions{})
	assert.Nil(t, err)

	deployments, err := service.getDeployments(context.TODO())

	assert.Nil(t, err)
	assert.Equal(t, 2, len(deployments))
	assertDeploymentsHas(t, deployments, "deployment1")
	assertDeploymentsHas(t, deployments, "deployment2")
}

func TestGetDeploymentsReturnsDeploymentsAndStatefulSets(t *testing.T) {
	service := initializeMockService(nil)
	var replicas int32 = 1
	_, err := service.k8sClient.AppsV1().Deployments(apiv1.NamespaceDefault).Create(
		context.TODO(),
		&appsv1.Deployment{
			ObjectMeta: k8smeta.ObjectMeta{
				Name:      "deployment1",
				Namespace: apiv1.NamespaceDefault,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
			},
		}, k8smeta.CreateOptions{})
	assert.Nil(t, err)

	_, err = service.k8sClient.AppsV1().Deployments(apiv1.NamespaceDefault).Create(
		context.TODO(),
		&appsv1.Deployment{
			ObjectMeta: k8smeta.ObjectMeta{
				Name:      "deployment2",
				Namespace: apiv1.NamespaceDefault,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
			},
		}, k8smeta.CreateOptions{})
	assert.Nil(t, err)

	_, err = service.k8sClient.AppsV1().StatefulSets(apiv1.NamespaceDefault).Create(
		context.TODO(),
		&appsv1.StatefulSet{
			ObjectMeta: k8smeta.ObjectMeta{
				Name:      "deployment3",
				Namespace: apiv1.NamespaceDefault,
			},
			Spec: appsv1.StatefulSetSpec{
				ServiceName: "special-stateful-service",
				Replicas:    &replicas,
			},
		}, k8smeta.CreateOptions{})
	assert.Nil(t, err)

	deployments, err := service.getDeployments(context.TODO())

	assert.Nil(t, err)
	assert.Equal(t, 3, len(deployments))
	assertDeploymentsHas(t, deployments, "deployment1")
	assertDeploymentsHas(t, deployments, "deployment2")
	assertDeploymentsHas(t, deployments, "special-stateful-service")
}

func assertDeploymentsHas(t *testing.T, deployments map[string]deployment, key string) {
	_, present := deployments[key]
	assert.True(t, present, "Expected deployments to have %s", key)
}

func TestGetDeploymentsReturnsErrorForDeployments(t *testing.T) {
	service := initializeMockService(nil)
	mock := &fake.Clientset{}
	service.k8sClient = mock
	mock.AddReactor("list", "deployments", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(action.GetResource().GroupResource(), action.GetResource().Resource)
	})
	deployments, err := service.getDeployments(context.TODO())
	assert.Error(t, err)
	assert.Nil(t, deployments)
}

func TestGetDeploymentsReturnsErrorForStatefulSets(t *testing.T) {
	service := initializeMockService(nil)
	mock := &fake.Clientset{}
	service.k8sClient = mock
	mock.AddReactor("list", "deployments", func(action core.Action) (bool, runtime.Object, error) {
		return true, &appsv1.DeploymentList{}, nil
	})
	mock.AddReactor("list", "statefulsets", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(action.GetResource().GroupResource(), action.GetResource().Resource)
	})
	deployments, err := service.getDeployments(context.TODO())
	assert.Error(t, err)
	assert.Nil(t, deployments)
}

func TestGetDefaultClient(t *testing.T) {
	hc := getDefaultClient()
	assert.Equal(t, hc.Timeout, 12*time.Second, "Expected time out to be 12 seconds")
}

func TestGetCategoriesUsesDedicatedK8sClient(t *testing.T) {
	service := initializeMockService(nil)
	_, err := service.categoriesK8sClient.CoreV1().ConfigMaps(apiv1.NamespaceDefault).Create(
		context.TODO(),
		&apiv1.ConfigMap{
			ObjectMeta: k8smeta.ObjectMeta{
				Name:      "category.publishing",
				Namespace: apiv1.NamespaceDefault,
				Labels: map[string]string{
					"healthcheck-categories-for": "aggregate-healthcheck",
				},
			},
			Data: map[string]string{
				"category.name":     "publishing",
				"category.services": validK8sServiceName,
			},
		},
		k8smeta.CreateOptions{},
	)
	assert.Nil(t, err)

	categories, err := service.getCategories(context.TODO())

	assert.Nil(t, err)
	assert.Contains(t, categories, "publishing")
	assert.Equal(t, []string{validK8sServiceName}, categories["publishing"].services)
}

//nolint:unparam
func buildCategoryConfigMap(configMapName string, categoryName string, services string) *apiv1.ConfigMap {
	return &apiv1.ConfigMap{
		ObjectMeta: k8smeta.ObjectMeta{
			Name:      configMapName,
			Namespace: apiv1.NamespaceDefault,
			Labels: map[string]string{
				"healthcheck-categories-for": "aggregate-healthcheck",
			},
		},
		Data: map[string]string{
			"category.name":        categoryName,
			"category.services":    services,
			"category.refreshrate": "60",
		},
	}
}
