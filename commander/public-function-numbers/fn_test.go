package main

import (
	"context"
	"testing"

	kubeobj "github.com/crossplane-contrib/provider-kubernetes/apis/namespaced/object/v1alpha1"
	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/types/known/structpb"
)

func mustStruct(m map[string]interface{}) *structpb.Struct {
	s, err := structpb.NewStruct(m)
	if err != nil {
		panic(err)
	}
	return s
}

func TestRunFunction(t *testing.T) {
	f := &Function{log: logging.NewNopLogger()}

	cases := map[string]struct {
		req                   *fnv1.RunFunctionRequest
		wantAPIVer            string
		wantKind              string
		wantManifestAPIVer    string
		wantManifestKind      string
		wantManifestName      string
		wantManifestNamespace string
		wantValue             string
		wantProviderConfig    string
	}{
		"CreatesObjectWithValue": {
			req: &fnv1.RunFunctionRequest{
				Meta: &fnv1.RequestMeta{Tag: "test"},
				Observed: &fnv1.State{
					Composite: &fnv1.Resource{
						Resource: mustStruct(map[string]interface{}{
							"apiVersion": "platform.org/v1alpha1",
							"kind":       "Number",
							"metadata":   map[string]interface{}{"name": "test-numbers", "namespace": "team-a"},
							"spec":       map[string]interface{}{"value": "42"},
						}),
					},
				},
			},
			wantAPIVer:            kubeobj.SchemeGroupVersion.String(),
			wantKind:              kubeobj.ObjectKind,
			wantManifestAPIVer:    "internal.platform.org/v1alpha1",
			wantManifestKind:      "Number",
			wantManifestName:      "test-numbers",
			wantManifestNamespace: "team-a",
			wantValue:             "42",
			wantProviderConfig:    "unit-numbers",
		},
		"EmptyValueWhenSpecValueMissing": {
			req: &fnv1.RunFunctionRequest{
				Meta: &fnv1.RequestMeta{Tag: "test"},
				Observed: &fnv1.State{
					Composite: &fnv1.Resource{
						Resource: mustStruct(map[string]interface{}{
							"apiVersion": "platform.org/v1alpha1",
							"kind":       "Number",
							"metadata":   map[string]interface{}{"name": "test-numbers", "namespace": "team-a"},
							"spec":       map[string]interface{}{},
						}),
					},
				},
			},
			wantAPIVer:            kubeobj.SchemeGroupVersion.String(),
			wantKind:              kubeobj.ObjectKind,
			wantManifestAPIVer:    "internal.platform.org/v1alpha1",
			wantManifestKind:      "Number",
			wantManifestName:      "test-numbers",
			wantManifestNamespace: "team-a",
			wantValue:             "",
			wantProviderConfig:    "unit-numbers",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rsp, err := f.RunFunction(context.Background(), tc.req)
			if err != nil {
				t.Fatalf("RunFunction returned error: %v", err)
			}
			if len(rsp.GetResults()) > 0 && rsp.GetResults()[0].GetSeverity() == fnv1.Severity_SEVERITY_FATAL {
				t.Fatalf("RunFunction returned fatal result: %s", rsp.GetResults()[0].GetMessage())
			}

			objRes, ok := rsp.GetDesired().GetResources()["numbers-object"]
			if !ok {
				t.Fatal("desired resources missing 'numbers-object'")
			}

			obj := composed.New()
			obj.Object = objRes.GetResource().AsMap()

			if diff := cmp.Diff(tc.wantAPIVer, mustGetString(obj, "apiVersion")); diff != "" {
				t.Errorf("apiVersion mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantKind, mustGetString(obj, "kind")); diff != "" {
				t.Errorf("kind mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantManifestAPIVer, mustGetString(obj, "spec.forProvider.manifest.apiVersion")); diff != "" {
				t.Errorf("manifest apiVersion mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantManifestKind, mustGetString(obj, "spec.forProvider.manifest.kind")); diff != "" {
				t.Errorf("manifest kind mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantManifestName, mustGetString(obj, "spec.forProvider.manifest.metadata.name")); diff != "" {
				t.Errorf("manifest name mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantManifestNamespace, mustGetString(obj, "spec.forProvider.manifest.metadata.namespace")); diff != "" {
				t.Errorf("manifest namespace mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantValue, mustGetString(obj, "spec.forProvider.manifest.spec.value")); diff != "" {
				t.Errorf("manifest spec.value mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantProviderConfig, mustGetString(obj, "spec.providerConfigRef.name")); diff != "" {
				t.Errorf("providerConfigRef.name mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func mustGetString(cd *composed.Unstructured, fp string) string {
	v, _ := cd.GetString(fp)
	return v
}
