package main

import (
	"testing"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/testapi"
	"k8s.io/kubernetes/pkg/client/unversioned/testclient"
	"k8s.io/kubernetes/pkg/watch"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	definitions = map[string][]string{
		"single-port": []string{
			"test/single-port-endpoints.yaml",
			"test/single-port-svc.yaml",
		},
		"multi-port": []string{
			"test/multi-port-endpoints.yaml",
			"test/multi-port-svc.yaml",
		},
	}

	apiVersion = "v1"
	selector   = map[string]string{"type": "public"}

	singlePortID   = getVulcanID("singlePort", "test", "web")
	apiMultiPortID = getVulcanID("multiPort", "test", "api")
	webMultiPortID = getVulcanID("multiPort", "test", "web")
	singlePort     = []VulcanObject{
		NewBackend(singlePortID),
		NewFrontend(singlePortID, singlePortID, "Host(`www.example.com`)", "Path(`/web`)"),
	}
	multiPort = []VulcanObject{
		NewBackend(apiMultiPortID),
		NewBackend(webMultiPortID),
		NewFrontend(apiMultiPortID, apiMultiPortID, "Host(`www.example.com`)", "Path(`/api/v1`)"),
		NewFrontend(webMultiPortID, webMultiPortID, "Host(`www.example.com`)", "Path(`/blog`)"),
	}
)

func setup(t *testing.T) (*assert.Assertions, *require.Assertions) {
	if testing.Verbose() {
		*logLevel, *debug = "debug", true
		setupLog()
	}
	test = true
	cache = newCache()
	*vulcanKey = "/vulcand-test"
	return assert.New(t), require.New(t)
}

func fakeKubeClient(defs string) testclient.ObjectRetriever {
	c := &testclient.Fake{}
	o := testclient.NewObjects(api.Scheme, api.Scheme)
	c.AddReactor("*", "*", testclient.ObjectReaction(o, testapi.Default.RESTMapper()))
	for _, d := range definitions[defs] {
		e := testclient.AddObjectsFromPath(d, o, api.Scheme)
		if e != nil {
			panic(e)
		}
	}

	tKubeClient = c
	return o
}

func TestRegister(te *testing.T) {
	is, must := setup(te)

	var tests = []struct {
		defs, kind, name string
		event            watch.EventType
		valid            bool
		data             []VulcanObject
	}{
		{"single-port", "Endpoints", "singlePort", watch.Added, true, singlePort},
		{"multi-port", "Service", "multiPort", watch.Modified, true, multiPort},
	}

	for _, t := range tests {
		etcd = newFakeEtcdClient(*vulcanKey)
		o := fakeKubeClient(t.defs)

		obj, er := o.Kind(t.kind, t.name)
		is.NoError(er, "Unable to get object '%s-%s'", t.name, t.kind)

		w := watch.Event{t.event, obj}
		er = process(w)
		te.Logf("Fake Etcd: %v", spew.Sdump(etcd))
		if t.valid {
			must.NoError(er, "Event handling failed event=%v", spew.Sdump(w))
		} else {
			must.Error(er, "Expected error event=%v", spew.Sdump(w))
			continue
		}

		for _, d := range t.data {
			expVal, _ := d.Val()
			key := prefix(*vulcanKey, d.Key())
			val, er := etcd.Val(d.Key())
			is.NoError(er, "%q not written to etcd", key)
			is.Equal(expVal, val, "Encoding for '%s-%s' not expected", t.name, t.kind)
		}
	}
}
