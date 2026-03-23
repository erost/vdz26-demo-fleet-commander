package main

import (
	"context"
	"encoding/json"
	"fmt"

	s3v1beta1 "github.com/upbound/provider-aws/v2/apis/namespaced/s3/v1beta1"
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/response"
	corev1 "k8s.io/api/core/v1"
)

// Function returns composed S3 resources for the internal Bucket XR.
type Function struct {
	fnv1.UnimplementedFunctionRunnerServiceServer
	log logging.Logger
}

func ptr[T any](v T) *T { return &v }

func (f *Function) RunFunction(_ context.Context, req *fnv1.RunFunctionRequest) (*fnv1.RunFunctionResponse, error) {
	f.log.Info("Running function", "tag", req.GetMeta().GetTag())

	rsp := response.To(req, response.DefaultTTL)

	oxr, err := request.GetObservedCompositeResource(req)
	if err != nil {
		response.Fatal(rsp, fmt.Errorf("cannot get observed composite resource: %w", err))
		return rsp, nil
	}

	bucketName, err := oxr.Resource.GetString("spec.bucketName")
	if err != nil || bucketName == "" {
		response.Fatal(rsp, fmt.Errorf("spec.bucketName is required"))
		return rsp, nil
	}

	region, err := oxr.Resource.GetString("spec.region")
	if err != nil || region == "" {
		region = "eu-west-1"
	}

	providerConfigName := "aws"

	desired, err := request.GetDesiredComposedResources(req)
	if err != nil {
		response.Fatal(rsp, fmt.Errorf("cannot get desired composed resources: %w", err))
		return rsp, nil
	}

	observed, err := request.GetObservedComposedResources(req)
	if err != nil {
		response.Fatal(rsp, fmt.Errorf("cannot get observed composed resources: %w", err))
		return rsp, nil
	}

	bucket, err := buildBucket(bucketName, region, providerConfigName)
	if err != nil {
		response.Fatal(rsp, fmt.Errorf("cannot build bucket: %w", err))
		return rsp, nil
	}

	publicAccessBlock, err := buildPublicAccessBlock(bucketName, region, providerConfigName)
	if err != nil {
		response.Fatal(rsp, fmt.Errorf("cannot build public access block: %w", err))
		return rsp, nil
	}

	versioning, err := buildVersioning(bucketName, region, providerConfigName)
	if err != nil {
		response.Fatal(rsp, fmt.Errorf("cannot build versioning: %w", err))
		return rsp, nil
	}

	desired[resource.Name("bucket")] = &resource.DesiredComposed{Resource: bucket, Ready: observedReadiness(observed, "bucket")}
	desired[resource.Name("bucket-public-access-block")] = &resource.DesiredComposed{Resource: publicAccessBlock, Ready: observedReadiness(observed, "bucket-public-access-block")}
	desired[resource.Name("bucket-versioning")] = &resource.DesiredComposed{Resource: versioning, Ready: observedReadiness(observed, "bucket-versioning")}

	if err := response.SetDesiredComposedResources(rsp, desired); err != nil {
		response.Fatal(rsp, fmt.Errorf("cannot set desired composed resources: %w", err))
		return rsp, nil
	}

	return rsp, nil
}

func buildBucket(name, region, providerConfigName string) (*composed.Unstructured, error) {
	fp := s3v1beta1.BucketParameters{
		Region: ptr(region),
	}
	return buildResource(s3v1beta1.CRDGroupVersion.String(), "Bucket", name, fp, "", providerConfigName)
}

func buildPublicAccessBlock(bucketName, region, providerConfigName string) (*composed.Unstructured, error) {
	fp := s3v1beta1.BucketPublicAccessBlockParameters{
		Region:                ptr(region),
		BlockPublicAcls:       ptr(true),
		BlockPublicPolicy:     ptr(true),
		IgnorePublicAcls:      ptr(true),
		RestrictPublicBuckets: ptr(true),
	}
	return buildResource(s3v1beta1.CRDGroupVersion.String(), "BucketPublicAccessBlock", bucketName, fp, bucketName, providerConfigName)
}

func buildVersioning(bucketName, region, providerConfigName string) (*composed.Unstructured, error) {
	fp := s3v1beta1.BucketVersioningParameters{
		Region: ptr(region),
		VersioningConfiguration: &s3v1beta1.VersioningConfigurationParameters{
			Status: ptr("Enabled"),
		},
	}
	return buildResource(s3v1beta1.CRDGroupVersion.String(), "BucketVersioning", bucketName, fp, bucketName, providerConfigName)
}

// observedReadiness returns the readiness of a named composed resource based on its observed Ready condition.
func observedReadiness(observed map[resource.Name]resource.ObservedComposed, name string) resource.Ready {
	ocd, ok := observed[resource.Name(name)]
	if !ok {
		return resource.ReadyUnspecified
	}
	if ocd.Resource.GetCondition(xpv1.TypeReady).Status == corev1.ConditionTrue {
		return resource.ReadyTrue
	}
	return resource.ReadyFalse
}

// buildResource marshals a typed ForProvider struct into a composed.Unstructured resource.
// bucketRef is the name of the bucket to reference; leave empty for the Bucket itself.
// providerConfigName is the cluster-scoped ProviderConfig to use (convention: "aws-<namespace>").
func buildResource(apiVersion, kind, name string, forProvider interface{}, bucketRef, providerConfigName string) (*composed.Unstructured, error) {
	fpRaw, err := json.Marshal(forProvider)
	if err != nil {
		return nil, fmt.Errorf("cannot marshal forProvider: %w", err)
	}

	var fpMap map[string]interface{}
	if err := json.Unmarshal(fpRaw, &fpMap); err != nil {
		return nil, fmt.Errorf("cannot unmarshal forProvider: %w", err)
	}

	if bucketRef != "" {
		fpMap["bucketRef"] = map[string]interface{}{"name": bucketRef}
	}

	u := composed.New()
	u.Object = map[string]interface{}{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   map[string]interface{}{"name": name},
		"spec": map[string]interface{}{
			"forProvider":       fpMap,
			"providerConfigRef": map[string]interface{}{"name": providerConfigName, "kind": "ProviderConfig"},
		},
	}
	return u, nil
}
