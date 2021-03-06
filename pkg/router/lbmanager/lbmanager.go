package lbmanager

import (
	"strings"
	"sync"
	"time"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	kapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	kclient "github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/watch"
	"github.com/golang/glog"

	osclient "github.com/openshift/origin/pkg/client"
	routeapi "github.com/openshift/origin/pkg/route/api"
	"github.com/openshift/origin/pkg/router"
)

// LBManager is responsible for synchronizing endpoint objects stored
// in the system with actual running pods.
type LBManager struct {
	routes          router.Router
	endpointWatcher kclient.EndpointsInterface
	routeWatcher    osclient.Interface
	lock            sync.Mutex
}

// NewLBManager creates a new LBManager.
func NewLBManager(routes router.Router, endpointWatcher kclient.EndpointsInterface, routeWatcher osclient.Interface) *LBManager {
	lm := &LBManager{
		routes:          routes,
		endpointWatcher: endpointWatcher,
		routeWatcher:    routeWatcher,
	}
	return lm
}

// Run begins watching and syncing.
func (lm *LBManager) Run(period time.Duration) {
	resourceVersion := "0"
	go util.Forever(func() { lm.watchEndpoints(resourceVersion) }, period)
	go util.Forever(func() { lm.watchRoutes(resourceVersion) }, period)
}

// resourceVersion is a pointer to the resource version to use/update.
func (lm *LBManager) watchRoutes(resourceVersion string) {
	ctx := kapi.NewContext()
	watching, err := lm.routeWatcher.WatchRoutes(
		ctx,
		labels.Everything(),
		labels.Everything(),
		resourceVersion,
	)
	if err != nil {
		glog.Errorf("Unexpected failure to watch: %v", err)
		time.Sleep(5 * time.Second)
		return
	}

	glog.V(4).Infof("Now entering watch mode.\n")
	for {
		select {
		case event, open := <-watching.ResultChan():
			if !open {
				// watchChannel has been closed, or something else went
				// wrong with our etcd watch call. Let the util.Forever()
				// that called us call us again.
				return
			}
			rc, ok := event.Object.(*routeapi.Route)
			if !ok {
				glog.Errorf("unexpected object: %#v", event.Object)
				continue
			}
			// Sync even if this is a deletion event, to ensure that we leave
			// it in the desired state.
			//glog.Infof("About to sync from watch: %v", *rc)
			lm.syncRoutes(event.Type, *rc)
		}
	}
}

// resourceVersion is a pointer to the resource version to use/update.
func (lm *LBManager) watchEndpoints(resourceVersion string) {
	ctx := kapi.NewContext()
	watching, err := lm.endpointWatcher.WatchEndpoints(
		ctx,
		labels.Everything(),
		labels.Everything(),
		resourceVersion,
	)
	if err != nil {
		glog.Errorf("Unexpected failure to watch: %v", err)
		time.Sleep(5 * time.Second)
		return
	}

	glog.V(4).Infof("Now entering watch mode.\n")
	for {
		select {
		case event, open := <-watching.ResultChan():
			if !open {
				// watchChannel has been closed, or something else went
				// wrong with our etcd watch call. Let the util.Forever()
				// that called us call us again.
				return
			}
			rc, ok := event.Object.(*api.Endpoints)
			if !ok {
				glog.Errorf("unexpected object: %#v", event.Object)
				continue
			}
			// Sync even if this is a deletion event, to ensure that we leave
			// it in the desired state.
			//glog.Infof("About to sync from watch: %v", *rc)
			if event.Type != watch.Error {
				lm.syncEndpoints(event.Type, *rc)
			} else {
				break
			}
		}
	}
}

func (lm *LBManager) syncRoutes(event watch.EventType, app routeapi.Route) {
	lm.lock.Lock()
	defer lm.lock.Unlock()
	glog.V(4).Infof("App Name : %s\n", app.ServiceName)
	glog.V(4).Infof("\tAlias : %s\n", app.Host)
	glog.V(4).Infof("\tEvent : %s\n", event)

	_, ok := lm.routes.FindFrontend(app.ServiceName)
	if !ok {
		lm.routes.CreateFrontend(app.ServiceName, "")
	}

	if event == watch.Added || event == watch.Modified {
		glog.V(4).Infof("Modifying routes for %s\n", app.ServiceName)
		lm.routes.AddAlias(app.Host, app.ServiceName)
	} else if event == watch.Deleted {
		lm.routes.DeleteFrontend(app.ServiceName)
	}
	lm.routes.WriteConfig()
	lm.routes.ReloadRouter()
}

func (lm *LBManager) syncEndpoints(event watch.EventType, app api.Endpoints) {
	lm.lock.Lock()
	defer lm.lock.Unlock()
	glog.V(4).Infof("App Name : %s\n", app.ID)
	glog.V(4).Infof("\tNumber of endpoints : %d\n", len(app.Endpoints))
	for i, e := range app.Endpoints {
		glog.V(4).Infof("\tEndpoint %d : %s\n", i, e)
	}
	_, ok := lm.routes.FindFrontend(app.ID)
	if !ok {
		lm.routes.CreateFrontend(app.ID, "") //"www."+app.ID+".com"
	}

	// Delete the endpoints only
	lm.routes.DeleteBackends(app.ID)

	if event == watch.Added || event == watch.Modified {
		glog.V(4).Infof("Modifying endpoints for %s\n", app.ID)
		eps := make([]router.Endpoint, len(app.Endpoints))
		for i, e := range app.Endpoints {
			ep := router.Endpoint{}
			if strings.Contains(e, ":") {
				eArr := strings.Split(e, ":")
				ep.IP = eArr[0]
				ep.Port = eArr[1]
			} else if e == "" {
				continue
			} else {
				ep.IP = e
				ep.Port = "80"
			}
			eps[i] = ep
		}
		lm.routes.AddRoute(app.ID, "", "", nil, eps)
	}
	lm.routes.WriteConfig()
	lm.routes.ReloadRouter()
}
