package etcd

import (
	"reflect"
	"testing"

	kapi "github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	_ "github.com/GoogleCloudPlatform/kubernetes/pkg/api/v1beta1"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/runtime"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/tools"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/watch"

	"github.com/openshift/origin/pkg/api/latest"
	"github.com/openshift/origin/pkg/build/api"

	"github.com/coreos/go-etcd/etcd"
)

func NewTestEtcd(client tools.EtcdClient) *Etcd {
	return New(tools.EtcdHelper{client, latest.Codec, tools.RuntimeVersionAdapter{latest.ResourceVersioner}})
}

// This copy and paste is not pure ignorance.  This is that we can be sure that the key is getting made as we
// expect it to. If someone changes the location of these resources by say moving all the resources to
// "/origin/resources" (which is a really good idea), then they've made a breaking change and something should
// fail to let them know they've change some significant change and that other dependent pieces may break.
func makeTestBuildListKey(namespace string) string {
	if len(namespace) != 0 {
		return "/builds/" + namespace
	}
	return "/builds"
}
func makeTestBuildKey(namespace, id string) string {
	return "/builds/" + namespace + "/" + id
}
func makeTestDefaultBuildKey(id string) string {
	return makeTestBuildKey(kapi.NamespaceDefault, id)
}
func makeTestDefaultBuildListKey() string {
	return makeTestBuildListKey(kapi.NamespaceDefault)
}
func makeTestBuildConfigListKey(namespace string) string {
	if len(namespace) != 0 {
		return "/buildConfigs/" + namespace
	}
	return "/buildConfigs"
}
func makeTestBuildConfigKey(namespace, id string) string {
	return "/buildConfigs/" + namespace + "/" + id
}
func makeTestDefaultBuildConfigKey(id string) string {
	return makeTestBuildConfigKey(kapi.NamespaceDefault, id)
}
func makeTestDefaultBuildConfigListKey() string {
	return makeTestBuildConfigListKey(kapi.NamespaceDefault)
}

func TestEtcdGetBuild(t *testing.T) {
	fakeClient := tools.NewFakeEtcdClient(t)
	fakeClient.Set(makeTestDefaultBuildKey("foo"), runtime.EncodeOrDie(latest.Codec, &api.Build{TypeMeta: kapi.TypeMeta{ID: "foo"}}), 0)
	registry := NewTestEtcd(fakeClient)
	build, err := registry.GetBuild(kapi.NewDefaultContext(), "foo")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	} else if build.ID != "foo" {
		t.Errorf("Unexpected build: %#v", build)
	}
}

func TestEtcdGetBuildNotFound(t *testing.T) {
	fakeClient := tools.NewFakeEtcdClient(t)
	fakeClient.Data[makeTestDefaultBuildKey("foo")] = tools.EtcdResponseWithError{
		R: &etcd.Response{
			Node: nil,
		},
		E: tools.EtcdErrorNotFound,
	}
	registry := NewTestEtcd(fakeClient)
	_, err := registry.GetBuild(kapi.NewDefaultContext(), "foo")
	if err == nil {
		t.Errorf("Unexpected non-error.")
	}
}

func TestEtcdCreateBuild(t *testing.T) {
	fakeClient := tools.NewFakeEtcdClient(t)
	fakeClient.TestIndex = true
	fakeClient.Data[makeTestDefaultBuildKey("foo")] = tools.EtcdResponseWithError{
		R: &etcd.Response{
			Node: nil,
		},
		E: tools.EtcdErrorNotFound,
	}
	registry := NewTestEtcd(fakeClient)
	err := registry.CreateBuild(kapi.NewDefaultContext(), &api.Build{
		TypeMeta: kapi.TypeMeta{
			ID: "foo",
		},
		Parameters: api.BuildParameters{
			Source: api.BuildSource{
				Git: &api.GitBuildSource{
					URI: "http://my.build.com/the/build/Dockerfile",
				},
			},
			Strategy: api.BuildStrategy{
				Type: api.STIBuildStrategyType,
				STIStrategy: &api.STIBuildStrategy{
					BuilderImage: "builder/image",
				},
			},
			Output: api.BuildOutput{
				ImageTag: "repository/dataBuild",
			},
		},
		Status: api.BuildStatusPending,
		PodID:  "-the-pod-id",
		Labels: map[string]string{
			"name": "dataBuild",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp, err := fakeClient.Get(makeTestDefaultBuildKey("foo"), false, false)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
	var build api.Build
	err = latest.Codec.DecodeInto([]byte(resp.Node.Value), &build)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if build.ID != "foo" {
		t.Errorf("Unexpected build: %#v %s", build, resp.Node.Value)
	}
}

func TestEtcdCreateBuildAlreadyExisting(t *testing.T) {
	fakeClient := tools.NewFakeEtcdClient(t)
	fakeClient.Data[makeTestDefaultBuildKey("foo")] = tools.EtcdResponseWithError{
		R: &etcd.Response{
			Node: &etcd.Node{
				Value: runtime.EncodeOrDie(latest.Codec, &api.Build{TypeMeta: kapi.TypeMeta{ID: "foo"}}),
			},
		},
		E: nil,
	}
	registry := NewTestEtcd(fakeClient)
	err := registry.CreateBuild(kapi.NewDefaultContext(), &api.Build{
		TypeMeta: kapi.TypeMeta{
			ID: "foo",
		},
	})
	if err == nil {
		t.Error("Unexpected non-error")
	}
}

func TestEtcdDeleteBuild(t *testing.T) {
	fakeClient := tools.NewFakeEtcdClient(t)
	fakeClient.TestIndex = true

	key := makeTestDefaultBuildKey("foo")
	fakeClient.Set(key, runtime.EncodeOrDie(latest.Codec, &api.Build{
		TypeMeta: kapi.TypeMeta{ID: "foo"},
	}), 0)
	registry := NewTestEtcd(fakeClient)
	err := registry.DeleteBuild(kapi.NewDefaultContext(), "foo")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(fakeClient.DeletedKeys) != 1 {
		t.Errorf("Expected 1 delete, found %#v", fakeClient.DeletedKeys)
	} else if fakeClient.DeletedKeys[0] != key {
		t.Errorf("Unexpected key: %s, expected %s", fakeClient.DeletedKeys[0], key)
	}
}

func TestEtcdEmptyListBuilds(t *testing.T) {
	fakeClient := tools.NewFakeEtcdClient(t)
	key := makeTestDefaultBuildListKey()
	fakeClient.Data[key] = tools.EtcdResponseWithError{
		R: &etcd.Response{
			Node: &etcd.Node{
				Nodes: []*etcd.Node{},
			},
		},
		E: nil,
	}
	registry := NewTestEtcd(fakeClient)
	builds, err := registry.ListBuilds(kapi.NewDefaultContext(), labels.Everything())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(builds.Items) != 0 {
		t.Errorf("Unexpected build list: %#v", builds)
	}
}

func TestEtcdListBuilds(t *testing.T) {
	fakeClient := tools.NewFakeEtcdClient(t)
	key := makeTestDefaultBuildListKey()
	fakeClient.Data[key] = tools.EtcdResponseWithError{
		R: &etcd.Response{
			Node: &etcd.Node{
				Nodes: []*etcd.Node{
					{
						Value: runtime.EncodeOrDie(latest.Codec, &api.Build{
							TypeMeta: kapi.TypeMeta{ID: "foo"},
						}),
					},
					{
						Value: runtime.EncodeOrDie(latest.Codec, &api.Build{
							TypeMeta: kapi.TypeMeta{ID: "bar"},
						}),
					},
				},
			},
		},
		E: nil,
	}
	registry := NewTestEtcd(fakeClient)
	builds, err := registry.ListBuilds(kapi.NewDefaultContext(), labels.Everything())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(builds.Items) != 2 || builds.Items[0].ID != "foo" || builds.Items[1].ID != "bar" {
		t.Errorf("Unexpected build list: %#v", builds)
	}
}

func TestEtcdWatchBuilds(t *testing.T) {
	fakeClient := tools.NewFakeEtcdClient(t)
	registry := NewTestEtcd(fakeClient)
	filterFields := labels.SelectorFromSet(labels.Set{"ID": "foo", "Status": string(api.BuildStatusRunning), "PodID": "bar"})

	watching, err := registry.WatchBuilds(kapi.NewContext(), labels.Everything(), filterFields, "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fakeClient.WaitForWatchCompletion()

	repo := &api.Build{TypeMeta: kapi.TypeMeta{ID: "foo"}, Status: api.BuildStatusRunning, PodID: "bar"}
	repoBytes, _ := latest.Codec.Encode(repo)
	fakeClient.WatchResponse <- &etcd.Response{
		Action: "set",
		Node: &etcd.Node{
			Value: string(repoBytes),
		},
	}

	event := <-watching.ResultChan()
	if e, a := watch.Added, event.Type; e != a {
		t.Errorf("Expected %v, got %v", e, a)
	}
	if e, a := repo, event.Object; !reflect.DeepEqual(e, a) {
		t.Errorf("Expected %v, got %v", e, a)
	}

	select {
	case _, ok := <-watching.ResultChan():
		if !ok {
			t.Errorf("watching channel should be open")
		}
	default:
	}

	fakeClient.WatchInjectError <- nil
	if _, ok := <-watching.ResultChan(); ok {
		t.Errorf("watching channel should be closed")
	}
	watching.Stop()
}

func TestEtcdGetBuildConfig(t *testing.T) {
	fakeClient := tools.NewFakeEtcdClient(t)
	fakeClient.Set(makeTestDefaultBuildConfigKey("foo"), runtime.EncodeOrDie(latest.Codec, &api.BuildConfig{TypeMeta: kapi.TypeMeta{ID: "foo"}}), 0)
	registry := NewTestEtcd(fakeClient)
	buildConfig, err := registry.GetBuildConfig(kapi.NewDefaultContext(), "foo")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	} else if buildConfig.ID != "foo" {
		t.Errorf("Unexpected build config: %#v", buildConfig)
	}
}

func TestEtcdGetBuildConfigNotFound(t *testing.T) {
	fakeClient := tools.NewFakeEtcdClient(t)
	fakeClient.Data[makeTestDefaultBuildConfigKey("foo")] = tools.EtcdResponseWithError{
		R: &etcd.Response{
			Node: nil,
		},
		E: tools.EtcdErrorNotFound,
	}
	registry := NewTestEtcd(fakeClient)
	_, err := registry.GetBuildConfig(kapi.NewDefaultContext(), "foo")
	if err == nil {
		t.Errorf("Unexpected non-error.")
	}
}

func TestEtcdCreateBuildConfig(t *testing.T) {
	fakeClient := tools.NewFakeEtcdClient(t)
	fakeClient.TestIndex = true
	fakeClient.Data[makeTestDefaultBuildConfigKey("foo")] = tools.EtcdResponseWithError{
		R: &etcd.Response{
			Node: nil,
		},
		E: tools.EtcdErrorNotFound,
	}
	registry := NewTestEtcd(fakeClient)
	err := registry.CreateBuildConfig(kapi.NewDefaultContext(), &api.BuildConfig{
		TypeMeta: kapi.TypeMeta{
			ID: "foo",
		},
		Parameters: api.BuildParameters{
			Source: api.BuildSource{
				Git: &api.GitBuildSource{
					URI: "http://my.build.com/the/build/Dockerfile",
				},
			},
			Strategy: api.BuildStrategy{
				Type: api.STIBuildStrategyType,
				STIStrategy: &api.STIBuildStrategy{
					BuilderImage: "builder/image",
				},
			},
			Output: api.BuildOutput{
				ImageTag: "repository/dataBuild",
			},
		},
		Labels: map[string]string{
			"name": "dataBuildConfig",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp, err := fakeClient.Get(makeTestDefaultBuildConfigKey("foo"), false, false)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}
	var buildConfig api.BuildConfig
	err = latest.Codec.DecodeInto([]byte(resp.Node.Value), &buildConfig)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if buildConfig.ID != "foo" {
		t.Errorf("Unexpected buildConfig: %#v %s", buildConfig, resp.Node.Value)
	}
}

func TestEtcdCreateBuildConfigAlreadyExisting(t *testing.T) {
	fakeClient := tools.NewFakeEtcdClient(t)
	fakeClient.Data[makeTestDefaultBuildConfigKey("foo")] = tools.EtcdResponseWithError{
		R: &etcd.Response{
			Node: &etcd.Node{
				Value: runtime.EncodeOrDie(latest.Codec, &api.BuildConfig{TypeMeta: kapi.TypeMeta{ID: "foo"}}),
			},
		},
		E: nil,
	}
	registry := NewTestEtcd(fakeClient)
	err := registry.CreateBuildConfig(kapi.NewDefaultContext(), &api.BuildConfig{
		TypeMeta: kapi.TypeMeta{
			ID: "foo",
		},
	})
	if err == nil {
		t.Error("Unexpected non-error")
	}
}

func TestEtcdDeleteBuildConfig(t *testing.T) {
	fakeClient := tools.NewFakeEtcdClient(t)
	fakeClient.TestIndex = true

	key := makeTestDefaultBuildConfigKey("foo")
	fakeClient.Set(key, runtime.EncodeOrDie(latest.Codec, &api.BuildConfig{
		TypeMeta: kapi.TypeMeta{ID: "foo"},
	}), 0)
	registry := NewTestEtcd(fakeClient)
	err := registry.DeleteBuildConfig(kapi.NewDefaultContext(), "foo")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(fakeClient.DeletedKeys) != 1 {
		t.Errorf("Expected 1 delete, found %#v", fakeClient.DeletedKeys)
	} else if fakeClient.DeletedKeys[0] != key {
		t.Errorf("Unexpected key: %s, expected %s", fakeClient.DeletedKeys[0], key)
	}
}

func TestEtcdEmptyListBuildConfigs(t *testing.T) {
	fakeClient := tools.NewFakeEtcdClient(t)
	key := makeTestDefaultBuildConfigListKey()
	fakeClient.Data[key] = tools.EtcdResponseWithError{
		R: &etcd.Response{
			Node: &etcd.Node{
				Nodes: []*etcd.Node{},
			},
		},
		E: nil,
	}
	registry := NewTestEtcd(fakeClient)
	buildConfigs, err := registry.ListBuildConfigs(kapi.NewDefaultContext(), labels.Everything())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(buildConfigs.Items) != 0 {
		t.Errorf("Unexpected buildConfig list: %#v", buildConfigs)
	}
}

func TestEtcdListBuildConfigs(t *testing.T) {
	fakeClient := tools.NewFakeEtcdClient(t)
	key := makeTestDefaultBuildConfigListKey()
	fakeClient.Data[key] = tools.EtcdResponseWithError{
		R: &etcd.Response{
			Node: &etcd.Node{
				Nodes: []*etcd.Node{
					{
						Value: runtime.EncodeOrDie(latest.Codec, &api.BuildConfig{
							TypeMeta: kapi.TypeMeta{ID: "foo"},
						}),
					},
					{
						Value: runtime.EncodeOrDie(latest.Codec, &api.BuildConfig{
							TypeMeta: kapi.TypeMeta{ID: "bar"},
						}),
					},
				},
			},
		},
		E: nil,
	}
	registry := NewTestEtcd(fakeClient)
	buildConfigs, err := registry.ListBuildConfigs(kapi.NewDefaultContext(), labels.Everything())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(buildConfigs.Items) != 2 || buildConfigs.Items[0].ID != "foo" || buildConfigs.Items[1].ID != "bar" {
		t.Errorf("Unexpected buildConfig list: %#v", buildConfigs)
	}
}

func TestEtcdCreateBuildConfigFailsWithoutNamespace(t *testing.T) {
	fakeClient := tools.NewFakeEtcdClient(t)
	fakeClient.TestIndex = true
	registry := NewTestEtcd(fakeClient)
	err := registry.CreateBuildConfig(kapi.NewContext(), &api.BuildConfig{
		TypeMeta: kapi.TypeMeta{
			ID: "foo",
		},
	})

	if err == nil {
		t.Errorf("expected error that namespace was missing from context")
	}
}

func TestEtcdCreateBuildFailsWithoutNamespace(t *testing.T) {
	fakeClient := tools.NewFakeEtcdClient(t)
	fakeClient.TestIndex = true
	registry := NewTestEtcd(fakeClient)
	err := registry.CreateBuild(kapi.NewContext(), &api.Build{
		TypeMeta: kapi.TypeMeta{
			ID: "foo",
		},
	})

	if err == nil {
		t.Errorf("expected error that namespace was missing from context")
	}
}

func TestEtcdListBuildsInDifferentNamespaces(t *testing.T) {
	fakeClient := tools.NewFakeEtcdClient(t)
	namespaceAlfa := kapi.WithNamespace(kapi.NewContext(), "alfa")
	namespaceBravo := kapi.WithNamespace(kapi.NewContext(), "bravo")
	fakeClient.Data["/builds/alfa"] = tools.EtcdResponseWithError{
		R: &etcd.Response{
			Node: &etcd.Node{
				Nodes: []*etcd.Node{
					{
						Value: runtime.EncodeOrDie(latest.Codec, &api.Build{TypeMeta: kapi.TypeMeta{ID: "foo1"}}),
					},
				},
			},
		},
		E: nil,
	}
	fakeClient.Data["/builds/bravo"] = tools.EtcdResponseWithError{
		R: &etcd.Response{
			Node: &etcd.Node{
				Nodes: []*etcd.Node{
					{
						Value: runtime.EncodeOrDie(latest.Codec, &api.Build{TypeMeta: kapi.TypeMeta{ID: "foo2"}}),
					},
					{
						Value: runtime.EncodeOrDie(latest.Codec, &api.Build{TypeMeta: kapi.TypeMeta{ID: "bar2"}}),
					},
				},
			},
		},
		E: nil,
	}
	registry := NewTestEtcd(fakeClient)

	buildsAlfa, err := registry.ListBuilds(namespaceAlfa, labels.Everything())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(buildsAlfa.Items) != 1 || buildsAlfa.Items[0].ID != "foo1" {
		t.Errorf("Unexpected builds list: %#v", buildsAlfa)
	}

	buildsBravo, err := registry.ListBuilds(namespaceBravo, labels.Everything())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(buildsBravo.Items) != 2 || buildsBravo.Items[0].ID != "foo2" || buildsBravo.Items[1].ID != "bar2" {
		t.Errorf("Unexpected builds list: %#v", buildsBravo)
	}
}

func TestEtcdListBuildConfigsInDifferentNamespaces(t *testing.T) {
	fakeClient := tools.NewFakeEtcdClient(t)
	namespaceAlfa := kapi.WithNamespace(kapi.NewContext(), "alfa")
	namespaceBravo := kapi.WithNamespace(kapi.NewContext(), "bravo")
	fakeClient.Data["/buildConfigs/alfa"] = tools.EtcdResponseWithError{
		R: &etcd.Response{
			Node: &etcd.Node{
				Nodes: []*etcd.Node{
					{
						Value: runtime.EncodeOrDie(latest.Codec, &api.BuildConfig{TypeMeta: kapi.TypeMeta{ID: "foo1"}}),
					},
				},
			},
		},
		E: nil,
	}
	fakeClient.Data["/buildConfigs/bravo"] = tools.EtcdResponseWithError{
		R: &etcd.Response{
			Node: &etcd.Node{
				Nodes: []*etcd.Node{
					{
						Value: runtime.EncodeOrDie(latest.Codec, &api.BuildConfig{TypeMeta: kapi.TypeMeta{ID: "foo2"}}),
					},
					{
						Value: runtime.EncodeOrDie(latest.Codec, &api.BuildConfig{TypeMeta: kapi.TypeMeta{ID: "bar2"}}),
					},
				},
			},
		},
		E: nil,
	}
	registry := NewTestEtcd(fakeClient)

	buildConfigsAlfa, err := registry.ListBuildConfigs(namespaceAlfa, labels.Everything())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(buildConfigsAlfa.Items) != 1 || buildConfigsAlfa.Items[0].ID != "foo1" {
		t.Errorf("Unexpected builds list: %#v", buildConfigsAlfa)
	}

	buildConfigsBravo, err := registry.ListBuildConfigs(namespaceBravo, labels.Everything())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(buildConfigsBravo.Items) != 2 || buildConfigsBravo.Items[0].ID != "foo2" || buildConfigsBravo.Items[1].ID != "bar2" {
		t.Errorf("Unexpected builds list: %#v", buildConfigsBravo)
	}
}

func TestEtcdGetBuildConfigInDifferentNamespaces(t *testing.T) {
	fakeClient := tools.NewFakeEtcdClient(t)
	namespaceAlfa := kapi.WithNamespace(kapi.NewContext(), "alfa")
	namespaceBravo := kapi.WithNamespace(kapi.NewContext(), "bravo")
	fakeClient.Set("/buildConfigs/alfa/foo", runtime.EncodeOrDie(latest.Codec, &api.BuildConfig{TypeMeta: kapi.TypeMeta{ID: "foo"}}), 0)
	fakeClient.Set("/buildConfigs/bravo/foo", runtime.EncodeOrDie(latest.Codec, &api.BuildConfig{TypeMeta: kapi.TypeMeta{ID: "foo"}}), 0)
	registry := NewTestEtcd(fakeClient)

	alfaFoo, err := registry.GetBuildConfig(namespaceAlfa, "foo")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if alfaFoo == nil || alfaFoo.ID != "foo" {
		t.Errorf("Unexpected buildConfig: %#v", alfaFoo)
	}

	bravoFoo, err := registry.GetBuildConfig(namespaceBravo, "foo")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if bravoFoo == nil || bravoFoo.ID != "foo" {
		t.Errorf("Unexpected buildConfig: %#v", bravoFoo)
	}
}

func TestEtcdGetBuildInDifferentNamespaces(t *testing.T) {
	fakeClient := tools.NewFakeEtcdClient(t)
	namespaceAlfa := kapi.WithNamespace(kapi.NewContext(), "alfa")
	namespaceBravo := kapi.WithNamespace(kapi.NewContext(), "bravo")
	fakeClient.Set("/builds/alfa/foo", runtime.EncodeOrDie(latest.Codec, &api.Build{TypeMeta: kapi.TypeMeta{ID: "foo"}}), 0)
	fakeClient.Set("/builds/bravo/foo", runtime.EncodeOrDie(latest.Codec, &api.Build{TypeMeta: kapi.TypeMeta{ID: "foo"}}), 0)
	registry := NewTestEtcd(fakeClient)

	alfaFoo, err := registry.GetBuild(namespaceAlfa, "foo")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if alfaFoo == nil || alfaFoo.ID != "foo" {
		t.Errorf("Unexpected buildConfig: %#v", alfaFoo)
	}

	bravoFoo, err := registry.GetBuild(namespaceBravo, "foo")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if bravoFoo == nil || bravoFoo.ID != "foo" {
		t.Errorf("Unexpected buildConfig: %#v", bravoFoo)
	}
}
