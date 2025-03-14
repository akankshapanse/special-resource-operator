package clients

import (
	"context"
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	clientconfigv1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

//go:generate mockgen -source=clients.go -package=clients -destination=mock_clients_api.go

const (
	clusterVersionName = "version"
)

var (
	// TODO need to remove this global variable
	Namespace string
)

type ClientsInterface interface {
	Update(ctx context.Context, obj client.Object) error
	Get(ctx context.Context, key client.ObjectKey, obj client.Object) error
	Delete(ctx context.Context, obj client.Object) error
	List(ctx context.Context, obj client.ObjectList, opts ...client.ListOption) error
	Create(ctx context.Context, obj client.Object) error
	GetPodLogs(namespace, podName string, podLogOpts *v1.PodLogOptions) *restclient.Request
	GetNamespace(ctx context.Context, name string, opts metav1.GetOptions) (*v1.Namespace, error)
	GetSecret(ctx context.Context, namespace, name string, opts metav1.GetOptions) (*v1.Secret, error)
	GetConfigMap(ctx context.Context, namespace, name string, opts metav1.GetOptions) (*v1.ConfigMap, error)
	GetImage(ctx context.Context, name string, opts metav1.GetOptions) (*configv1.Image, error)
	ClusterVersionGet(ctx context.Context, opts metav1.GetOptions) (result *configv1.ClusterVersion, err error)
	Invalidate()
	ServerGroups() (*metav1.APIGroupList, error)
	ServerGroupsAndResources() ([]*metav1.APIGroup, []*metav1.APIResourceList, error)
	StatusUpdate(ctx context.Context, obj client.Object) error
	StatusPatch(ctx context.Context, original, modified client.Object) error
	CreateOrUpdate(ctx context.Context, obj client.Object, fn controllerutil.MutateFn) (controllerutil.OperationResult, error)
	HasResource(resource schema.GroupVersionResource) (bool, error)
	GetNodesByLabels(ctx context.Context, matchingLabels map[string]string) (*v1.NodeList, error)
}

type k8sClients struct {
	runtimeClient   client.Client
	clientset       kubernetes.Clientset
	configV1Client  clientconfigv1.ConfigV1Client
	eventRecorder   record.EventRecorder
	cachedDiscovery discovery.CachedDiscoveryInterface
	restConfig      *restclient.Config
}

func NewClients(runtimeClient client.Client, restConfig *restclient.Config, eventRecorder record.EventRecorder) (ClientsInterface, error) {
	kubeClientSet, err := getKubeClientSet(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failure of getKubeClientSet in NewClients: %w", err)
	}
	configClient, err := getConfigClient(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failure of getConfigClient in NewClients: %w", err)
	}
	cachedDiscoveryClient, err := getCachedDiscoveryClient()
	if err != nil {
		return nil, fmt.Errorf("failure of getCachedDiscoveryClient in NewClients: %w", err)
	}
	return &k8sClients{
		runtimeClient:   runtimeClient,
		clientset:       *kubeClientSet,
		configV1Client:  *configClient,
		eventRecorder:   eventRecorder,
		cachedDiscovery: cachedDiscoveryClient,
		restConfig:      restConfig,
	}, nil
}

func (k *k8sClients) Update(ctx context.Context, obj client.Object) error {
	return k.runtimeClient.Update(ctx, obj)
}

func (k *k8sClients) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	return k.runtimeClient.Get(ctx, key, obj)
}

func (k *k8sClients) Delete(ctx context.Context, obj client.Object) error {
	return k.runtimeClient.Delete(ctx, obj)
}

func (k *k8sClients) List(ctx context.Context, obj client.ObjectList, opts ...client.ListOption) error {
	return k.runtimeClient.List(ctx, obj, opts...)
}

func (k *k8sClients) Create(ctx context.Context, obj client.Object) error {
	return k.runtimeClient.Create(ctx, obj)
}

func (k *k8sClients) GetPodLogs(namespace, podName string, podLogOpts *v1.PodLogOptions) *restclient.Request {
	return k.clientset.CoreV1().Pods(namespace).GetLogs(podName, podLogOpts)
}

func (k *k8sClients) GetNamespace(ctx context.Context, name string, opts metav1.GetOptions) (*v1.Namespace, error) {
	return k.clientset.CoreV1().Namespaces().Get(ctx, name, opts)
}

func (k *k8sClients) GetSecret(ctx context.Context, namespace, name string, opts metav1.GetOptions) (*v1.Secret, error) {
	return k.clientset.CoreV1().Secrets(namespace).Get(ctx, name, opts)
}

func (k *k8sClients) GetConfigMap(ctx context.Context, namespace, name string, opts metav1.GetOptions) (*v1.ConfigMap, error) {
	return k.clientset.CoreV1().ConfigMaps(namespace).Get(ctx, name, opts)
}

func (k *k8sClients) GetImage(ctx context.Context, name string, opts metav1.GetOptions) (*configv1.Image, error) {
	return k.configV1Client.Images().Get(ctx, name, opts)
}

func (k *k8sClients) ClusterVersionGet(ctx context.Context, opts metav1.GetOptions) (result *configv1.ClusterVersion, err error) {
	return k.configV1Client.ClusterVersions().Get(ctx, clusterVersionName, opts)
}

func (k *k8sClients) Invalidate() {
	k.cachedDiscovery.Invalidate()
}

func (k *k8sClients) ServerGroups() (*metav1.APIGroupList, error) {
	return k.cachedDiscovery.ServerGroups()
}

func (k *k8sClients) ServerGroupsAndResources() ([]*metav1.APIGroup, []*metav1.APIResourceList, error) {
	return k.cachedDiscovery.ServerGroupsAndResources()
}

func (k *k8sClients) StatusUpdate(ctx context.Context, obj client.Object) error {
	return k.runtimeClient.Status().Update(ctx, obj)
}

func (k *k8sClients) StatusPatch(ctx context.Context, original, modified client.Object) error {
	patch := client.MergeFrom(original)
	return k.runtimeClient.Status().Patch(ctx, modified, patch)
}

func (k *k8sClients) CreateOrUpdate(ctx context.Context, obj client.Object, fn controllerutil.MutateFn) (controllerutil.OperationResult, error) {
	return controllerruntime.CreateOrUpdate(ctx, k.runtimeClient, obj, fn)
}

func (k *k8sClients) HasResource(resource schema.GroupVersionResource) (bool, error) {
	dclient, err := discovery.NewDiscoveryClientForConfig(k.restConfig)
	if err != nil {
		return false, fmt.Errorf("Cannot retrieve a DiscoveryClient in HasResource: %w", err)
	}
	if dclient == nil {
		return false, nil
	}

	resources, err := dclient.ServerResourcesForGroupVersion(resource.GroupVersion().String())
	if apierrors.IsNotFound(err) {
		// entire group is missing
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("Cannot query ServerResources in HasResource: %w", err)
	} else {
		for _, serverResource := range resources.APIResources {
			if serverResource.Name == resource.Resource {
				return true, nil
			}
		}
	}
	return false, nil
}

func (k *k8sClients) GetNodesByLabels(ctx context.Context, matchingLabels map[string]string) (*v1.NodeList, error) {
	opts := []client.ListOption{
		client.MatchingLabels(matchingLabels),
	}
	nodes := v1.NodeList{}
	err := k.runtimeClient.List(ctx, &nodes, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	// filter nodes by taints
	nodesWithoutTaints := nodes.Items[:0]
	for _, node := range nodes.Items {
		if k.isNodeNotExecOrSchedule(&node) {
			continue
		}
		nodesWithoutTaints = append(nodesWithoutTaints, node)
	}
	nodes.Items = nodesWithoutTaints
	return &nodes, nil
}

func (k *k8sClients) isNodeNotExecOrSchedule(node *v1.Node) bool {
	for _, taint := range node.Spec.Taints {
		if taint.Effect == v1.TaintEffectNoSchedule || taint.Effect == v1.TaintEffectNoExecute {
			return true
		}
	}
	return false
}

// getKubeClientSet returns a native non-caching client for advanced CRUD operations
func getKubeClientSet(restConfig *restclient.Config) (*kubernetes.Clientset, error) {
	return kubernetes.NewForConfig(restConfig)
}

// getConfigClient returns a configv1 client to the reconciler
func getConfigClient(restConfig *restclient.Config) (*clientconfigv1.ConfigV1Client, error) {
	return clientconfigv1.NewForConfig(restConfig)
}

func getCachedDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	var config genericclioptions.ConfigFlags
	return config.ToDiscoveryClient()
}
