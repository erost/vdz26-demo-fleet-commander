package main

import (
	"context"
	"encoding/json"
	"fmt"

	kubeobj "github.com/crossplane-contrib/provider-kubernetes/apis/namespaced/object/v1alpha1"
	kubepc "github.com/crossplane-contrib/provider-kubernetes/apis/namespaced/v1alpha1"
	xpcommon "github.com/crossplane/crossplane-runtime/v2/apis/common"
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/response"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Function returns composed resources for the public Bucket XR.
type Function struct {
	fnv1.UnimplementedFunctionRunnerServiceServer
	log logging.Logger
}

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

	name := oxr.Resource.GetName()
	namespace := oxr.Resource.GetNamespace()

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

	manifest, err := json.Marshal(map[string]interface{}{
		"apiVersion": "internal.platform.org/v1alpha1",
		"kind":       "Bucket",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]interface{}{
			"bucketName": bucketName,
			"region":     region,
		},
	})
	if err != nil {
		response.Fatal(rsp, fmt.Errorf("cannot marshal manifest: %w", err))
		return rsp, nil
	}

	obj := &kubeobj.Object{
		TypeMeta: metav1.TypeMeta{
			APIVersion: kubeobj.SchemeGroupVersion.String(),
			Kind:       kubeobj.ObjectKind,
		},
		Spec: kubeobj.ObjectSpec{
			ManagedResourceSpec: xpv2.ManagedResourceSpec{
				ProviderConfigReference: &xpcommon.ProviderConfigReference{
					Kind: kubepc.ClusterProviderConfigKind,
					Name: "unit-aws",
				},
			},
			ForProvider: kubeobj.ObjectParameters{
				Manifest: runtime.RawExtension{Raw: manifest},
			},
			Readiness: kubeobj.Readiness{
				Policy: kubeobj.ReadinessPolicyDeriveFromObject,
			},
		},
	}

	objMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		response.Fatal(rsp, fmt.Errorf("cannot convert object to unstructured: %w", err))
		return rsp, nil
	}

	composedObj := composed.New()
	composedObj.Object = objMap

	ready := resource.ReadyUnspecified
	if ocd, ok := observed[resource.Name("bucket-object")]; ok {
		if ocd.Resource.GetCondition(xpv1.TypeReady).Status == corev1.ConditionTrue {
			ready = resource.ReadyTrue
		} else {
			ready = resource.ReadyFalse
		}
	}

	desired[resource.Name("bucket-object")] = &resource.DesiredComposed{Resource: composedObj, Ready: ready}

	if err := response.SetDesiredComposedResources(rsp, desired); err != nil {
		response.Fatal(rsp, fmt.Errorf("cannot set desired composed resources: %w", err))
		return rsp, nil
	}

	return rsp, nil
}
