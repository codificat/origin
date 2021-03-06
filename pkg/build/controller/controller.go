package controller

import (
	"fmt"

	"github.com/golang/glog"

	kapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	errors "github.com/GoogleCloudPlatform/kubernetes/pkg/api/errors"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client/cache"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"

	buildapi "github.com/openshift/origin/pkg/build/api"
)

// BuildController watches build resources and manages their state
type BuildController struct {
	BuildStore    cache.Store
	NextBuild     func() *buildapi.Build
	NextPod       func() *kapi.Pod
	BuildUpdater  buildUpdater
	PodCreator    podCreator
	BuildStrategy BuildStrategy
}

// BuildStrategy knows how to create a pod spec for a pod which can execute a build.
type BuildStrategy interface {
	CreateBuildPod(build *buildapi.Build) (*kapi.Pod, error)
}

type buildUpdater interface {
	UpdateBuild(ctx kapi.Context, build *buildapi.Build) (*buildapi.Build, error)
}

type podCreator interface {
	CreatePod(ctx kapi.Context, pod *kapi.Pod) (*kapi.Pod, error)
}

// Run begins watching and syncing build jobs onto the cluster.
func (bc *BuildController) Run() {
	go util.Forever(func() { bc.HandleBuild(bc.NextBuild()) }, 0)
	go util.Forever(func() { bc.HandlePod(bc.NextPod()) }, 0)
}

func (bc *BuildController) HandleBuild(build *buildapi.Build) {
	glog.V(4).Infof("Handling build %s", build.ID)

	// We only deal with new builds here
	if build.Status != buildapi.BuildStatusNew {
		return
	}

	nextStatus := buildapi.BuildStatusFailed

	build.PodID = fmt.Sprintf("build-%s", build.ID)
	ctx := kapi.WithNamespace(kapi.NewContext(), build.Namespace)

	var podSpec *kapi.Pod
	var err error
	if podSpec, err = bc.BuildStrategy.CreateBuildPod(build); err != nil {
		glog.V(2).Infof("Strategy failed to create build pod definition: %v", err)
		nextStatus = buildapi.BuildStatusFailed
	} else {
		if _, err := bc.PodCreator.CreatePod(ctx, podSpec); err != nil {
			if !errors.IsAlreadyExists(err) {
				glog.V(2).Infof("Failed to create pod for build %s: %#v", build.ID, err)
				nextStatus = buildapi.BuildStatusFailed
			}
		} else {
			glog.V(2).Infof("Created build pod: %#v", podSpec)
			nextStatus = buildapi.BuildStatusPending
		}
	}

	build.Status = nextStatus
	if _, err := bc.BuildUpdater.UpdateBuild(kapi.WithNamespace(kapi.NewContext(), build.Namespace), build); err != nil {
		glog.V(2).Infof("Failed to update build %s: %#v", build.ID, err)
	}
}

func (bc *BuildController) HandlePod(pod *kapi.Pod) {
	// Find the build for this pod
	var build *buildapi.Build
	for _, obj := range bc.BuildStore.List() {
		b := obj.(*buildapi.Build)
		if b.PodID == pod.ID {
			build = b
			break
		}
	}

	if build == nil {
		return
	}

	nextStatus := build.Status

	switch pod.CurrentState.Status {
	case kapi.PodRunning:
		// The pod's still running
		nextStatus = buildapi.BuildStatusRunning
	case kapi.PodTerminated:
		// Check the exit codes of all the containers in the pod
		nextStatus = buildapi.BuildStatusComplete
		for _, info := range pod.CurrentState.Info {
			if info.State.Termination != nil && info.State.Termination.ExitCode != 0 {
				nextStatus = buildapi.BuildStatusFailed
				break
			}
		}
	}

	if build.Status != nextStatus {
		glog.V(4).Infof("Updating build %s status %s -> %s", build.ID, build.Status, nextStatus)
		build.Status = nextStatus
		if _, err := bc.BuildUpdater.UpdateBuild(kapi.WithNamespace(kapi.NewContext(), build.Namespace), build); err != nil {
			glog.V(2).Infof("Failed to update build %s: %#v", build.ID, err)
		}
	}
}
