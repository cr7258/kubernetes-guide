/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package certificates

import (
	"context"
	"reflect"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apiserver/pkg/authentication/user"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	certapi "k8s.io/kubernetes/pkg/apis/certificates"
	"k8s.io/kubernetes/pkg/features"
	"k8s.io/utils/pointer"
)

func TestStrategyCreate(t *testing.T) {
	tests := map[string]struct {
		ctx                context.Context
		disableFeatureGate bool
		obj                runtime.Object
		expectedObj        runtime.Object
	}{
		"no user in context, no user in obj": {
			ctx: genericapirequest.NewContext(),
			obj: &certapi.CertificateSigningRequest{},
			expectedObj: &certapi.CertificateSigningRequest{
				Status: certapi.CertificateSigningRequestStatus{Conditions: []certapi.CertificateSigningRequestCondition{}},
			},
		},
		"user in context, no user in obj": {
			ctx: genericapirequest.WithUser(
				genericapirequest.NewContext(),
				&user.DefaultInfo{
					Name:   "bob",
					UID:    "123",
					Groups: []string{"group1"},
					Extra:  map[string][]string{"foo": {"bar"}},
				},
			),
			obj: &certapi.CertificateSigningRequest{},
			expectedObj: &certapi.CertificateSigningRequest{
				Spec: certapi.CertificateSigningRequestSpec{
					Username: "bob",
					UID:      "123",
					Groups:   []string{"group1"},
					Extra:    map[string]certapi.ExtraValue{"foo": {"bar"}},
				},
				Status: certapi.CertificateSigningRequestStatus{Conditions: []certapi.CertificateSigningRequestCondition{}},
			},
		},
		"no user in context, user in obj": {
			ctx: genericapirequest.NewContext(),
			obj: &certapi.CertificateSigningRequest{
				Spec: certapi.CertificateSigningRequestSpec{
					Username: "bob",
					UID:      "123",
					Groups:   []string{"group1"},
				},
			},
			expectedObj: &certapi.CertificateSigningRequest{
				Status: certapi.CertificateSigningRequestStatus{Conditions: []certapi.CertificateSigningRequestCondition{}},
			},
		},
		"user in context, user in obj": {
			ctx: genericapirequest.WithUser(
				genericapirequest.NewContext(),
				&user.DefaultInfo{
					Name: "alice",
					UID:  "234",
				},
			),
			obj: &certapi.CertificateSigningRequest{
				Spec: certapi.CertificateSigningRequestSpec{
					Username: "bob",
					UID:      "123",
					Groups:   []string{"group1"},
				},
			},
			expectedObj: &certapi.CertificateSigningRequest{
				Spec: certapi.CertificateSigningRequestSpec{
					Username: "alice",
					UID:      "234",
					Groups:   nil,
				},
				Status: certapi.CertificateSigningRequestStatus{Conditions: []certapi.CertificateSigningRequestCondition{}},
			},
		},
		"pre-approved status": {
			ctx: genericapirequest.NewContext(),
			obj: &certapi.CertificateSigningRequest{
				Status: certapi.CertificateSigningRequestStatus{
					Conditions: []certapi.CertificateSigningRequestCondition{
						{Type: certapi.CertificateApproved},
					},
				},
			},
			expectedObj: &certapi.CertificateSigningRequest{
				Status: certapi.CertificateSigningRequestStatus{Conditions: []certapi.CertificateSigningRequestCondition{}},
			},
		},
		"expirationSeconds set with gate enabled": {
			ctx: genericapirequest.NewContext(),
			obj: &certapi.CertificateSigningRequest{
				Spec: certapi.CertificateSigningRequestSpec{
					ExpirationSeconds: pointer.Int32(1234),
				},
			},
			expectedObj: &certapi.CertificateSigningRequest{
				Spec: certapi.CertificateSigningRequestSpec{
					ExpirationSeconds: pointer.Int32(1234),
				},
				Status: certapi.CertificateSigningRequestStatus{Conditions: []certapi.CertificateSigningRequestCondition{}},
			},
		},
		"expirationSeconds set with gate disabled": {
			ctx:                genericapirequest.NewContext(),
			disableFeatureGate: true,
			obj: &certapi.CertificateSigningRequest{
				Spec: certapi.CertificateSigningRequestSpec{
					ExpirationSeconds: pointer.Int32(5678),
				},
			},
			expectedObj: &certapi.CertificateSigningRequest{
				Spec: certapi.CertificateSigningRequestSpec{
					ExpirationSeconds: nil,
				},
				Status: certapi.CertificateSigningRequestStatus{Conditions: []certapi.CertificateSigningRequestCondition{}},
			},
		},
	}

	for k, tc := range tests {
		tc := tc
		t.Run(k, func(t *testing.T) {
			if tc.disableFeatureGate {
				defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.CSRDuration, false)()
			}
			obj := tc.obj
			Strategy.PrepareForCreate(tc.ctx, obj)
			if !reflect.DeepEqual(obj, tc.expectedObj) {
				t.Errorf("object diff: %s", diff.ObjectDiff(obj, tc.expectedObj))
			}
		})
	}
}

func TestStatusUpdate(t *testing.T) {
	now := metav1.Now()
	later := metav1.NewTime(now.Add(time.Hour))
	nowFunc = func() metav1.Time { return now }
	defer func() {
		nowFunc = metav1.Now
	}()

	tests := []struct {
		name         string
		newObj       *certapi.CertificateSigningRequest
		oldObj       *certapi.CertificateSigningRequest
		expectedObjs map[string]*certapi.CertificateSigningRequest
	}{
		{
			name:   "no-op",
			newObj: &certapi.CertificateSigningRequest{},
			oldObj: &certapi.CertificateSigningRequest{},
			expectedObjs: map[string]*certapi.CertificateSigningRequest{
				"v1":      {},
				"v1beta1": {},
			},
		},
		{
			name: "adding failed condition to existing approved/denied conditions",
			newObj: &certapi.CertificateSigningRequest{
				Status: certapi.CertificateSigningRequestStatus{
					Conditions: []certapi.CertificateSigningRequestCondition{
						{Type: certapi.CertificateFailed},
						{Type: certapi.CertificateDenied},
						{Type: certapi.CertificateApproved},
						{Type: certapi.CertificateDenied},
						{Type: certapi.CertificateApproved},
					},
				},
			},
			oldObj: &certapi.CertificateSigningRequest{
				Status: certapi.CertificateSigningRequestStatus{
					Conditions: []certapi.CertificateSigningRequestCondition{
						{Type: certapi.CertificateDenied, Reason: "because1"},
						{Type: certapi.CertificateApproved, Reason: "because2"},
						{Type: certapi.CertificateDenied, Reason: "because3", LastUpdateTime: later, LastTransitionTime: later},
						{Type: certapi.CertificateApproved, Reason: "because4", LastUpdateTime: later, LastTransitionTime: later},
					},
				},
			},
			expectedObjs: map[string]*certapi.CertificateSigningRequest{
				// preserve existing Approved/Denied conditions
				"v1": {
					Status: certapi.CertificateSigningRequestStatus{
						Conditions: []certapi.CertificateSigningRequestCondition{
							{Type: certapi.CertificateFailed, LastUpdateTime: now, LastTransitionTime: now},
							{Type: certapi.CertificateDenied, LastUpdateTime: now, LastTransitionTime: later, Reason: "because1"},
							{Type: certapi.CertificateApproved, LastUpdateTime: now, LastTransitionTime: later, Reason: "because2"},
							{Type: certapi.CertificateDenied, LastUpdateTime: later, LastTransitionTime: later, Reason: "because3"},
							{Type: certapi.CertificateApproved, LastUpdateTime: later, LastTransitionTime: later, Reason: "because4"},
						},
					},
				},
				// preserve existing Approved/Denied conditions
				"v1beta1": {
					Status: certapi.CertificateSigningRequestStatus{
						Conditions: []certapi.CertificateSigningRequestCondition{
							{Type: certapi.CertificateFailed, LastUpdateTime: now, LastTransitionTime: now},
							{Type: certapi.CertificateDenied, LastUpdateTime: now, LastTransitionTime: later, Reason: "because1"},
							{Type: certapi.CertificateApproved, LastUpdateTime: now, LastTransitionTime: later, Reason: "because2"},
							{Type: certapi.CertificateDenied, LastUpdateTime: later, LastTransitionTime: later, Reason: "because3"},
							{Type: certapi.CertificateApproved, LastUpdateTime: later, LastTransitionTime: later, Reason: "because4"},
						},
					},
				},
			},
		},
		{
			name: "add approved condition",
			newObj: &certapi.CertificateSigningRequest{
				Status: certapi.CertificateSigningRequestStatus{
					Conditions: []certapi.CertificateSigningRequestCondition{
						{Type: certapi.CertificateApproved},
					},
				},
			},
			oldObj: &certapi.CertificateSigningRequest{},
			expectedObjs: map[string]*certapi.CertificateSigningRequest{
				// preserved submitted conditions if existing Approved/Denied conditions could not be copied over (will fail validation)
				"v1": {
					Status: certapi.CertificateSigningRequestStatus{
						Conditions: []certapi.CertificateSigningRequestCondition{
							{Type: certapi.CertificateApproved, LastUpdateTime: now, LastTransitionTime: now},
						},
					},
				},
				// reset conditions to existing conditions if Approved/Denied conditions could not be copied over
				"v1beta1": {
					Status: certapi.CertificateSigningRequestStatus{},
				},
			},
		},
	}

	for _, tt := range tests {
		for _, version := range []string{"v1", "v1beta1"} {
			t.Run(tt.name+"_"+version, func(t *testing.T) {
				ctx := genericapirequest.WithRequestInfo(context.TODO(), &genericapirequest.RequestInfo{APIGroup: "certificates.k8s.io", APIVersion: version})
				obj := tt.newObj.DeepCopy()
				StatusStrategy.PrepareForUpdate(ctx, obj, tt.oldObj.DeepCopy())
				if !reflect.DeepEqual(obj, tt.expectedObjs[version]) {
					t.Errorf("object diff: %s", diff.ObjectDiff(obj, tt.expectedObjs[version]))
				}
			})
		}
	}
}
