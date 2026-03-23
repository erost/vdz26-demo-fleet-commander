package main

import (
	"context"
	"testing"

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

func mustGetString(cd *composed.Unstructured, fp string) string {
	v, _ := cd.GetString(fp)
	return v
}

func mustGetBool(cd *composed.Unstructured, fp string) bool {
	v, _ := cd.GetBool(fp)
	return v
}

func TestRunFunction(t *testing.T) {
	f := &Function{log: logging.NewNopLogger()}

	cases := map[string]struct {
		req            *fnv1.RunFunctionRequest
		wantBucketName string
		wantRegion     string
	}{
		"CreatesBucketResources": {
			req: &fnv1.RunFunctionRequest{
				Meta: &fnv1.RequestMeta{Tag: "test"},
				Observed: &fnv1.State{
					Composite: &fnv1.Resource{
						Resource: mustStruct(map[string]interface{}{
							"apiVersion": "internal.platform.org/v1alpha1",
							"kind":       "Bucket",
							"metadata":   map[string]interface{}{"name": "test-bucket", "namespace": "team-a"},
							"spec": map[string]interface{}{
								"bucketName": "my-demo-bucket",
								"region":     "eu-west-1",
							},
						}),
					},
				},
			},
			wantBucketName: "my-demo-bucket",
			wantRegion:     "eu-west-1",
		},
		"DefaultsRegionToEuWest1": {
			req: &fnv1.RunFunctionRequest{
				Meta: &fnv1.RequestMeta{Tag: "test"},
				Observed: &fnv1.State{
					Composite: &fnv1.Resource{
						Resource: mustStruct(map[string]interface{}{
							"apiVersion": "internal.platform.org/v1alpha1",
							"kind":       "Bucket",
							"metadata":   map[string]interface{}{"name": "test-bucket", "namespace": "team-a"},
							"spec": map[string]interface{}{
								"bucketName": "my-demo-bucket",
							},
						}),
					},
				},
			},
			wantBucketName: "my-demo-bucket",
			wantRegion:     "eu-west-1",
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

			desired := rsp.GetDesired().GetResources()

			// --- Bucket ---
			bucketRes, ok := desired["bucket"]
			if !ok {
				t.Fatal("desired resources missing 'bucket'")
			}
			bucket := composed.New()
			bucket.Object = bucketRes.GetResource().AsMap()

			if diff := cmp.Diff("s3.aws.m.upbound.io/v1beta1", mustGetString(bucket, "apiVersion")); diff != "" {
				t.Errorf("bucket apiVersion mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff("Bucket", mustGetString(bucket, "kind")); diff != "" {
				t.Errorf("bucket kind mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantBucketName, mustGetString(bucket, "metadata.name")); diff != "" {
				t.Errorf("bucket name mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantRegion, mustGetString(bucket, "spec.forProvider.region")); diff != "" {
				t.Errorf("bucket region mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff("aws", mustGetString(bucket, "spec.providerConfigRef.name")); diff != "" {
				t.Errorf("bucket providerConfigRef mismatch (-want +got):\n%s", diff)
			}

			// --- BucketPublicAccessBlock ---
			pabRes, ok := desired["bucket-public-access-block"]
			if !ok {
				t.Fatal("desired resources missing 'bucket-public-access-block'")
			}
			pab := composed.New()
			pab.Object = pabRes.GetResource().AsMap()

			if diff := cmp.Diff("BucketPublicAccessBlock", mustGetString(pab, "kind")); diff != "" {
				t.Errorf("public access block kind mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantBucketName, mustGetString(pab, "spec.forProvider.bucketRef.name")); diff != "" {
				t.Errorf("public access block bucketRef mismatch (-want +got):\n%s", diff)
			}
			for _, field := range []string{
				"spec.forProvider.blockPublicAcls",
				"spec.forProvider.blockPublicPolicy",
				"spec.forProvider.ignorePublicAcls",
				"spec.forProvider.restrictPublicBuckets",
			} {
				if !mustGetBool(pab, field) {
					t.Errorf("public access block %s should be true", field)
				}
			}

			// --- BucketVersioning ---
			versioningRes, ok := desired["bucket-versioning"]
			if !ok {
				t.Fatal("desired resources missing 'bucket-versioning'")
			}
			versioning := composed.New()
			versioning.Object = versioningRes.GetResource().AsMap()

			if diff := cmp.Diff("BucketVersioning", mustGetString(versioning, "kind")); diff != "" {
				t.Errorf("versioning kind mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff("Enabled", mustGetString(versioning, "spec.forProvider.versioningConfiguration.status")); diff != "" {
				t.Errorf("versioning status mismatch (-want +got):\n%s", diff)
			}

		})
	}
}
