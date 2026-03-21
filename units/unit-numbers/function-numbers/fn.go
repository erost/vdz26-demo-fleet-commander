package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	nopv1 "github.com/crossplane-contrib/provider-nop/apis/v1alpha1"
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
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

// Function returns composed resources for the Numbers XR.
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

	value, err := oxr.Resource.GetString("spec.value")
	if err != nil {
		// spec.value may not be set on first reconcile; treat as empty.
		value = ""
	}

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

	fields, err := json.Marshal(map[string]interface{}{"value": value})
	if err != nil {
		response.Fatal(rsp, fmt.Errorf("cannot marshal fields: %w", err))
		return rsp, nil
	}

	nopObj := &nopv1.NopResource{
		TypeMeta: metav1.TypeMeta{
			APIVersion: nopv1.SchemeGroupVersion.String(),
			Kind:       nopv1.NopResourceKind,
		},
		Spec: nopv1.NopSpec{
			ForProvider: nopv1.NopParameters{
				ConditionAfter: []nopv1.ConditionAfter{
					{
						ConditionType:   xpv1.TypeReady,
						ConditionStatus: corev1.ConditionTrue,
						Time:            metav1.Duration{Duration: 10 * time.Second},
					},
					{
						ConditionType:   xpv1.TypeSynced,
						ConditionStatus: corev1.ConditionTrue,
						Time:            metav1.Duration{Duration: 45 * time.Second},
					},
				},
				Fields: runtime.RawExtension{Raw: fields},
			},
		},
	}

	nopMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(nopObj)
	if err != nil {
		response.Fatal(rsp, fmt.Errorf("cannot convert NopResource to unstructured: %w", err))
		return rsp, nil
	}

	composedObj := composed.New()
	composedObj.Object = nopMap

	ready := resource.ReadyUnspecified
	if ocd, ok := observed[resource.Name("nop-resource")]; ok {
		if ocd.Resource.GetCondition(xpv1.TypeReady).Status == corev1.ConditionTrue {
			ready = resource.ReadyTrue
		} else {
			ready = resource.ReadyFalse
		}
	}

	desired[resource.Name("nop-resource")] = &resource.DesiredComposed{Resource: composedObj, Ready: ready}

	if err := response.SetDesiredComposedResources(rsp, desired); err != nil {
		response.Fatal(rsp, fmt.Errorf("cannot set desired composed resources: %w", err))
		return rsp, nil
	}

	return rsp, nil
}
