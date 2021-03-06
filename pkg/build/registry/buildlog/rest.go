package buildlog

import (
	"fmt"
	"net/url"

	kapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/apiserver"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/runtime"

	"github.com/openshift/origin/pkg/build/api"
	"github.com/openshift/origin/pkg/build/registry/build"
	"github.com/openshift/origin/pkg/cmd/server/kubernetes"
)

// REST is an implementation of RESTStorage for the api server.
type REST struct {
	BuildRegistry build.Registry
	PodClient     client.PodInterface
}

// NewREST creates a new REST for BuildLog
// Takes build registry and pod client to get neccessary attibutes to assamble
// URL to which the request shall be redirected in order to get build logs.
func NewREST(b build.Registry, c client.PodInterface) apiserver.RESTStorage {
	return &REST{
		BuildRegistry: b,
		PodClient:     c,
	}
}

// Redirector implementation
func (r *REST) ResourceLocation(ctx kapi.Context, id string) (string, error) {
	build, err := r.BuildRegistry.GetBuild(ctx, id)
	if err != nil {
		return "", fmt.Errorf("No such build: %v", err)
	}

	pod, err := r.PodClient.GetPod(ctx, build.PodID)
	if err != nil {
		return "", fmt.Errorf("No such pod: %v", err)
	}
	buildPodID := build.PodID
	buildPodHost := pod.CurrentState.Host
	buildPodNamespace := pod.Namespace
	// Build will take place only in one container
	buildContainerName := pod.DesiredState.Manifest.Containers[0].Name
	location := &url.URL{
		Host: fmt.Sprintf("%s:%d", buildPodHost, kubernetes.NodePort),
		Path: fmt.Sprintf("/containerLogs/%s/%s/%s", buildPodNamespace, buildPodID, buildContainerName),
	}
	if build.Status == api.BuildStatusRunning {
		params := url.Values{"follow": []string{"1"}}
		location.RawQuery = params.Encode()
	}
	if err != nil {
		return "", err
	}
	return location.String(), nil
}

func (r *REST) Get(ctx kapi.Context, id string) (runtime.Object, error) {
	return nil, fmt.Errorf("BuildLog can't be retrieved")
}

func (r *REST) New() runtime.Object {
	return nil
}

func (r *REST) List(ctx kapi.Context, selector, fields labels.Selector) (runtime.Object, error) {
	return nil, fmt.Errorf("BuildLog can't be listed")
}

func (r *REST) Delete(ctx kapi.Context, id string) (<-chan runtime.Object, error) {
	return nil, fmt.Errorf("BuildLog can't be deleted")
}

func (r *REST) Create(ctx kapi.Context, obj runtime.Object) (<-chan runtime.Object, error) {
	return nil, fmt.Errorf("BuildLog can't be created")
}

func (r *REST) Update(ctx kapi.Context, obj runtime.Object) (<-chan runtime.Object, error) {
	return nil, fmt.Errorf("BuildLog can't be updated")
}
