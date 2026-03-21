package main

import (
	"context"
	"testing"

	nopv1 "github.com/crossplane-contrib/provider-nop/apis/v1alpha1"
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
		req          *fnv1.RunFunctionRequest
		wantAPIVer   string
		wantKind     string
		wantValue    string
		wantReadyIn  string
		wantSyncedIn string
	}{
		"SetsValueAndTimings": {
			req: &fnv1.RunFunctionRequest{
				Meta: &fnv1.RequestMeta{Tag: "test"},
				Observed: &fnv1.State{
					Composite: &fnv1.Resource{
						Resource: mustStruct(map[string]interface{}{
							"apiVersion": "internal.platform.org/v1alpha1",
							"kind":       "Numbers",
							"metadata":   map[string]interface{}{"name": "test-numbers"},
							"spec":       map[string]interface{}{"value": "3"},
						}),
					},
				},
			},
			wantAPIVer:   nopv1.SchemeGroupVersion.String(),
			wantKind:     nopv1.NopResourceKind,
			wantValue:    "3",
			wantReadyIn:  "10s",
			wantSyncedIn: "45s",
		},
		"EmptyValueWhenSpecValueMissing": {
			req: &fnv1.RunFunctionRequest{
				Meta: &fnv1.RequestMeta{Tag: "test"},
				Observed: &fnv1.State{
					Composite: &fnv1.Resource{
						Resource: mustStruct(map[string]interface{}{
							"apiVersion": "internal.platform.org/v1alpha1",
							"kind":       "Numbers",
							"metadata":   map[string]interface{}{"name": "test-numbers"},
							"spec":       map[string]interface{}{},
						}),
					},
				},
			},
			wantAPIVer:   nopv1.SchemeGroupVersion.String(),
			wantKind:     nopv1.NopResourceKind,
			wantValue:    "",
			wantReadyIn:  "10s",
			wantSyncedIn: "45s",
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

			nopRes, ok := rsp.GetDesired().GetResources()["nop-resource"]
			if !ok {
				t.Fatal("desired resources missing 'nop-resource'")
			}

			nop := composed.New()
			nop.Object = nopRes.GetResource().AsMap()

			if diff := cmp.Diff(tc.wantAPIVer, mustGetString(nop, "apiVersion")); diff != "" {
				t.Errorf("apiVersion mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantKind, mustGetString(nop, "kind")); diff != "" {
				t.Errorf("kind mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantValue, mustGetString(nop, "spec.forProvider.fields.value")); diff != "" {
				t.Errorf("spec.forProvider.fields.value mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantReadyIn, mustGetString(nop, "spec.forProvider.conditionAfter[0].time")); diff != "" {
				t.Errorf("ready time mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantSyncedIn, mustGetString(nop, "spec.forProvider.conditionAfter[1].time")); diff != "" {
				t.Errorf("synced time mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func mustGetString(cd *composed.Unstructured, fp string) string {
	v, _ := cd.GetString(fp)
	return v
}
