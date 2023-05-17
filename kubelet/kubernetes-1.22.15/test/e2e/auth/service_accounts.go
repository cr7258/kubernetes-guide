/*
Copyright 2014 The Kubernetes Authors.

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

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	watch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/kubernetes/plugin/pkg/admission/serviceaccount"
	"k8s.io/kubernetes/test/e2e/framework"
	e2ekubectl "k8s.io/kubernetes/test/e2e/framework/kubectl"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	e2eskipper "k8s.io/kubernetes/test/e2e/framework/skipper"
	imageutils "k8s.io/kubernetes/test/utils/image"
	utilptr "k8s.io/utils/pointer"

	"github.com/onsi/ginkgo"
)

var _ = SIGDescribe("ServiceAccounts", func() {
	f := framework.NewDefaultFramework("svcaccounts")

	ginkgo.It("should ensure a single API token exists", func() {
		// wait for the service account to reference a single secret
		var secrets []v1.ObjectReference
		framework.ExpectNoError(wait.Poll(time.Millisecond*500, time.Second*10, func() (bool, error) {
			ginkgo.By("waiting for a single token reference")
			sa, err := f.ClientSet.CoreV1().ServiceAccounts(f.Namespace.Name).Get(context.TODO(), "default", metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				framework.Logf("default service account was not found")
				return false, nil
			}
			if err != nil {
				framework.Logf("error getting default service account: %v", err)
				return false, err
			}
			switch len(sa.Secrets) {
			case 0:
				framework.Logf("default service account has no secret references")
				return false, nil
			case 1:
				framework.Logf("default service account has a single secret reference")
				secrets = sa.Secrets
				return true, nil
			default:
				return false, fmt.Errorf("default service account has too many secret references: %#v", sa.Secrets)
			}
		}))

		// make sure the reference doesn't flutter
		{
			ginkgo.By("ensuring the single token reference persists")
			time.Sleep(2 * time.Second)
			sa, err := f.ClientSet.CoreV1().ServiceAccounts(f.Namespace.Name).Get(context.TODO(), "default", metav1.GetOptions{})
			framework.ExpectNoError(err)
			framework.ExpectEqual(sa.Secrets, secrets)
		}

		// delete the referenced secret
		ginkgo.By("deleting the service account token")
		framework.ExpectNoError(f.ClientSet.CoreV1().Secrets(f.Namespace.Name).Delete(context.TODO(), secrets[0].Name, metav1.DeleteOptions{}))

		// wait for the referenced secret to be removed, and another one autocreated
		framework.ExpectNoError(wait.Poll(time.Millisecond*500, framework.ServiceAccountProvisionTimeout, func() (bool, error) {
			ginkgo.By("waiting for a new token reference")
			sa, err := f.ClientSet.CoreV1().ServiceAccounts(f.Namespace.Name).Get(context.TODO(), "default", metav1.GetOptions{})
			if err != nil {
				framework.Logf("error getting default service account: %v", err)
				return false, err
			}
			switch len(sa.Secrets) {
			case 0:
				framework.Logf("default service account has no secret references")
				return false, nil
			case 1:
				if sa.Secrets[0] == secrets[0] {
					framework.Logf("default service account still has the deleted secret reference")
					return false, nil
				}
				framework.Logf("default service account has a new single secret reference")
				secrets = sa.Secrets
				return true, nil
			default:
				return false, fmt.Errorf("default service account has too many secret references: %#v", sa.Secrets)
			}
		}))

		// make sure the reference doesn't flutter
		{
			ginkgo.By("ensuring the single token reference persists")
			time.Sleep(2 * time.Second)
			sa, err := f.ClientSet.CoreV1().ServiceAccounts(f.Namespace.Name).Get(context.TODO(), "default", metav1.GetOptions{})
			framework.ExpectNoError(err)
			framework.ExpectEqual(sa.Secrets, secrets)
		}

		// delete the reference from the service account
		ginkgo.By("deleting the reference to the service account token")
		{
			sa, err := f.ClientSet.CoreV1().ServiceAccounts(f.Namespace.Name).Get(context.TODO(), "default", metav1.GetOptions{})
			framework.ExpectNoError(err)
			sa.Secrets = nil
			_, updateErr := f.ClientSet.CoreV1().ServiceAccounts(f.Namespace.Name).Update(context.TODO(), sa, metav1.UpdateOptions{})
			framework.ExpectNoError(updateErr)
		}

		// wait for another one to be autocreated
		framework.ExpectNoError(wait.Poll(time.Millisecond*500, framework.ServiceAccountProvisionTimeout, func() (bool, error) {
			ginkgo.By("waiting for a new token to be created and added")
			sa, err := f.ClientSet.CoreV1().ServiceAccounts(f.Namespace.Name).Get(context.TODO(), "default", metav1.GetOptions{})
			if err != nil {
				framework.Logf("error getting default service account: %v", err)
				return false, err
			}
			switch len(sa.Secrets) {
			case 0:
				framework.Logf("default service account has no secret references")
				return false, nil
			case 1:
				framework.Logf("default service account has a new single secret reference")
				secrets = sa.Secrets
				return true, nil
			default:
				return false, fmt.Errorf("default service account has too many secret references: %#v", sa.Secrets)
			}
		}))

		// make sure the reference doesn't flutter
		{
			ginkgo.By("ensuring the single token reference persists")
			time.Sleep(2 * time.Second)
			sa, err := f.ClientSet.CoreV1().ServiceAccounts(f.Namespace.Name).Get(context.TODO(), "default", metav1.GetOptions{})
			framework.ExpectNoError(err)
			framework.ExpectEqual(sa.Secrets, secrets)
		}
	})

	/*
	   Release: v1.9
	   Testname: Service Account Tokens Must AutoMount
	   Description: Ensure that Service Account keys are mounted into the Container. Pod
	                contains three containers each will read Service Account token,
	                root CA and default namespace respectively from the default API
	                Token Mount path. All these three files MUST exist and the Service
	                Account mount path MUST be auto mounted to the Container.
	*/
	framework.ConformanceIt("should mount an API token into pods ", func() {
		var rootCAContent string

		sa, err := f.ClientSet.CoreV1().ServiceAccounts(f.Namespace.Name).Create(context.TODO(), &v1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "mount-test"}}, metav1.CreateOptions{})
		framework.ExpectNoError(err)

		// Standard get, update retry loop
		framework.ExpectNoError(wait.Poll(time.Millisecond*500, framework.ServiceAccountProvisionTimeout, func() (bool, error) {
			ginkgo.By("getting the auto-created API token")
			sa, err := f.ClientSet.CoreV1().ServiceAccounts(f.Namespace.Name).Get(context.TODO(), "mount-test", metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				framework.Logf("mount-test service account was not found")
				return false, nil
			}
			if err != nil {
				framework.Logf("error getting mount-test service account: %v", err)
				return false, err
			}
			if len(sa.Secrets) == 0 {
				framework.Logf("mount-test service account has no secret references")
				return false, nil
			}
			for _, secretRef := range sa.Secrets {
				secret, err := f.ClientSet.CoreV1().Secrets(f.Namespace.Name).Get(context.TODO(), secretRef.Name, metav1.GetOptions{})
				if err != nil {
					framework.Logf("Error getting secret %s: %v", secretRef.Name, err)
					continue
				}
				if secret.Type == v1.SecretTypeServiceAccountToken {
					rootCAContent = string(secret.Data[v1.ServiceAccountRootCAKey])
					return true, nil
				}
			}

			framework.Logf("default service account has no secret references to valid service account tokens")
			return false, nil
		}))

		zero := int64(0)
		pod, err := f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pod-service-account-" + string(uuid.NewUUID()),
			},
			Spec: v1.PodSpec{
				ServiceAccountName: sa.Name,
				Containers: []v1.Container{{
					Name:    "test",
					Image:   imageutils.GetE2EImage(imageutils.BusyBox),
					Command: []string{"sleep", "100000"},
				}},
				TerminationGracePeriodSeconds: &zero,
				RestartPolicy:                 v1.RestartPolicyNever,
			},
		}, metav1.CreateOptions{})
		framework.ExpectNoError(err)
		framework.ExpectNoError(e2epod.WaitForPodRunningInNamespace(f.ClientSet, pod))

		tk := e2ekubectl.NewTestKubeconfig(framework.TestContext.CertDir, framework.TestContext.Host, framework.TestContext.KubeConfig, framework.TestContext.KubeContext, framework.TestContext.KubectlPath, f.Namespace.Name)
		mountedToken, err := tk.ReadFileViaContainer(pod.Name, pod.Spec.Containers[0].Name, path.Join(serviceaccount.DefaultAPITokenMountPath, v1.ServiceAccountTokenKey))
		framework.ExpectNoError(err)
		mountedCA, err := tk.ReadFileViaContainer(pod.Name, pod.Spec.Containers[0].Name, path.Join(serviceaccount.DefaultAPITokenMountPath, v1.ServiceAccountRootCAKey))
		framework.ExpectNoError(err)
		mountedNamespace, err := tk.ReadFileViaContainer(pod.Name, pod.Spec.Containers[0].Name, path.Join(serviceaccount.DefaultAPITokenMountPath, v1.ServiceAccountNamespaceKey))
		framework.ExpectNoError(err)

		// CA and namespace should be identical
		framework.ExpectEqual(mountedCA, rootCAContent)
		framework.ExpectEqual(mountedNamespace, f.Namespace.Name)
		// Token should be a valid credential that identifies the pod's service account
		tokenReview := &authenticationv1.TokenReview{Spec: authenticationv1.TokenReviewSpec{Token: mountedToken}}
		tokenReview, err = f.ClientSet.AuthenticationV1().TokenReviews().Create(context.TODO(), tokenReview, metav1.CreateOptions{})
		framework.ExpectNoError(err)
		framework.ExpectEqual(tokenReview.Status.Authenticated, true)
		framework.ExpectEqual(tokenReview.Status.Error, "")
		framework.ExpectEqual(tokenReview.Status.User.Username, "system:serviceaccount:"+f.Namespace.Name+":"+sa.Name)
		groups := sets.NewString(tokenReview.Status.User.Groups...)
		framework.ExpectEqual(groups.Has("system:authenticated"), true, fmt.Sprintf("expected system:authenticated group, had %v", groups.List()))
		framework.ExpectEqual(groups.Has("system:serviceaccounts"), true, fmt.Sprintf("expected system:serviceaccounts group, had %v", groups.List()))
		framework.ExpectEqual(groups.Has("system:serviceaccounts:"+f.Namespace.Name), true, fmt.Sprintf("expected system:serviceaccounts:"+f.Namespace.Name+" group, had %v", groups.List()))
	})

	/*
	   Release: v1.9
	   Testname: Service account tokens auto mount optionally
	   Description: Ensure that Service Account keys are mounted into the Pod only
	                when AutoMountServiceToken is not set to false. We test the
	                following scenarios here.
	   1. Create Pod, Pod Spec has AutomountServiceAccountToken set to nil
	      a) Service Account with default value,
	      b) Service Account is an configured AutomountServiceAccountToken set to true,
	      c) Service Account is an configured AutomountServiceAccountToken set to false
	   2. Create Pod, Pod Spec has AutomountServiceAccountToken set to true
	      a) Service Account with default value,
	      b) Service Account is configured with AutomountServiceAccountToken set to true,
	      c) Service Account is configured with AutomountServiceAccountToken set to false
	   3. Create Pod, Pod Spec has AutomountServiceAccountToken set to false
	      a) Service Account with default value,
	      b) Service Account is configured with AutomountServiceAccountToken set to true,
	      c) Service Account is configured with AutomountServiceAccountToken set to false

	   The Containers running in these pods MUST verify that the ServiceTokenVolume path is
	   auto mounted only when Pod Spec has AutomountServiceAccountToken not set to false
	   and ServiceAccount object has AutomountServiceAccountToken not set to false, this
	   include test cases 1a,1b,2a,2b and 2c.
	   In the test cases 1c,3a,3b and 3c the ServiceTokenVolume MUST not be auto mounted.
	*/
	framework.ConformanceIt("should allow opting out of API token automount ", func() {

		var err error
		trueValue := true
		falseValue := false
		mountSA := &v1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "mount"}, AutomountServiceAccountToken: &trueValue}
		nomountSA := &v1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "nomount"}, AutomountServiceAccountToken: &falseValue}
		mountSA, err = f.ClientSet.CoreV1().ServiceAccounts(f.Namespace.Name).Create(context.TODO(), mountSA, metav1.CreateOptions{})
		framework.ExpectNoError(err)
		nomountSA, err = f.ClientSet.CoreV1().ServiceAccounts(f.Namespace.Name).Create(context.TODO(), nomountSA, metav1.CreateOptions{})
		framework.ExpectNoError(err)

		// Standard get, update retry loop
		framework.ExpectNoError(wait.Poll(time.Millisecond*500, framework.ServiceAccountProvisionTimeout, func() (bool, error) {
			ginkgo.By("getting the auto-created API token")
			sa, err := f.ClientSet.CoreV1().ServiceAccounts(f.Namespace.Name).Get(context.TODO(), mountSA.Name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				framework.Logf("mount service account was not found")
				return false, nil
			}
			if err != nil {
				framework.Logf("error getting mount service account: %v", err)
				return false, err
			}
			if len(sa.Secrets) == 0 {
				framework.Logf("mount service account has no secret references")
				return false, nil
			}
			for _, secretRef := range sa.Secrets {
				secret, err := f.ClientSet.CoreV1().Secrets(f.Namespace.Name).Get(context.TODO(), secretRef.Name, metav1.GetOptions{})
				if err != nil {
					framework.Logf("Error getting secret %s: %v", secretRef.Name, err)
					continue
				}
				if secret.Type == v1.SecretTypeServiceAccountToken {
					return true, nil
				}
			}

			framework.Logf("default service account has no secret references to valid service account tokens")
			return false, nil
		}))

		testcases := []struct {
			PodName            string
			ServiceAccountName string
			AutomountPodSpec   *bool
			ExpectTokenVolume  bool
		}{
			{
				PodName:            "pod-service-account-defaultsa",
				ServiceAccountName: "default",
				AutomountPodSpec:   nil,
				ExpectTokenVolume:  true, // default is true
			},
			{
				PodName:            "pod-service-account-mountsa",
				ServiceAccountName: mountSA.Name,
				AutomountPodSpec:   nil,
				ExpectTokenVolume:  true,
			},
			{
				PodName:            "pod-service-account-nomountsa",
				ServiceAccountName: nomountSA.Name,
				AutomountPodSpec:   nil,
				ExpectTokenVolume:  false,
			},

			// Make sure pod spec trumps when opting in
			{
				PodName:            "pod-service-account-defaultsa-mountspec",
				ServiceAccountName: "default",
				AutomountPodSpec:   &trueValue,
				ExpectTokenVolume:  true,
			},
			{
				PodName:            "pod-service-account-mountsa-mountspec",
				ServiceAccountName: mountSA.Name,
				AutomountPodSpec:   &trueValue,
				ExpectTokenVolume:  true,
			},
			{
				PodName:            "pod-service-account-nomountsa-mountspec",
				ServiceAccountName: nomountSA.Name,
				AutomountPodSpec:   &trueValue,
				ExpectTokenVolume:  true, // pod spec trumps
			},

			// Make sure pod spec trumps when opting out
			{
				PodName:            "pod-service-account-defaultsa-nomountspec",
				ServiceAccountName: "default",
				AutomountPodSpec:   &falseValue,
				ExpectTokenVolume:  false, // pod spec trumps
			},
			{
				PodName:            "pod-service-account-mountsa-nomountspec",
				ServiceAccountName: mountSA.Name,
				AutomountPodSpec:   &falseValue,
				ExpectTokenVolume:  false, // pod spec trumps
			},
			{
				PodName:            "pod-service-account-nomountsa-nomountspec",
				ServiceAccountName: nomountSA.Name,
				AutomountPodSpec:   &falseValue,
				ExpectTokenVolume:  false, // pod spec trumps
			},
		}

		for _, tc := range testcases {
			pod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: tc.PodName},
				Spec: v1.PodSpec{
					Containers:                   []v1.Container{{Name: "token-test", Image: imageutils.GetE2EImage(imageutils.Agnhost)}},
					RestartPolicy:                v1.RestartPolicyNever,
					ServiceAccountName:           tc.ServiceAccountName,
					AutomountServiceAccountToken: tc.AutomountPodSpec,
				},
			}
			createdPod, err := f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), pod, metav1.CreateOptions{})
			framework.ExpectNoError(err)
			framework.Logf("created pod %s", tc.PodName)

			hasServiceAccountTokenVolume := false
			for _, c := range createdPod.Spec.Containers {
				for _, vm := range c.VolumeMounts {
					if vm.MountPath == serviceaccount.DefaultAPITokenMountPath {
						hasServiceAccountTokenVolume = true
					}
				}
			}

			if hasServiceAccountTokenVolume != tc.ExpectTokenVolume {
				framework.Failf("%s: expected volume=%v, got %v (%#v)", tc.PodName, tc.ExpectTokenVolume, hasServiceAccountTokenVolume, createdPod)
			} else {
				framework.Logf("pod %s service account token volume mount: %v", tc.PodName, hasServiceAccountTokenVolume)
			}
		}
	})

	/*
	  Release : v1.20
	  Testname: TokenRequestProjection should mount a projected volume with token using TokenRequest API.
	  Description: Ensure that projected service account token is mounted.
	*/
	framework.ConformanceIt("should mount projected service account token", func() {

		var (
			podName         = "test-pod-" + string(uuid.NewUUID())
			volumeName      = "test-volume"
			volumeMountPath = "/test-volume"
			tokenVolumePath = "/test-volume/token"
		)

		volumes := []v1.Volume{
			{
				Name: volumeName,
				VolumeSource: v1.VolumeSource{
					Projected: &v1.ProjectedVolumeSource{
						Sources: []v1.VolumeProjection{
							{
								ServiceAccountToken: &v1.ServiceAccountTokenProjection{
									Path:              "token",
									ExpirationSeconds: utilptr.Int64Ptr(60 * 60),
								},
							},
						},
					},
				},
			},
		}
		volumeMounts := []v1.VolumeMount{
			{
				Name:      volumeName,
				MountPath: volumeMountPath,
				ReadOnly:  true,
			},
		}
		mounttestArgs := []string{
			"mounttest",
			fmt.Sprintf("--file_content=%v", tokenVolumePath),
		}

		pod := e2epod.NewAgnhostPod(f.Namespace.Name, podName, volumes, volumeMounts, nil, mounttestArgs...)
		pod.Spec.RestartPolicy = v1.RestartPolicyNever

		output := []string{
			fmt.Sprintf("content of file \"%v\": %s", tokenVolumePath, `[A-Za-z0-9-_=]+\.[A-Za-z0-9-_=]+\.?[A-Za-z0-9-_.+/=]*`),
		}

		f.TestContainerOutputRegexp("service account token: ", pod, 0, output)
	})

	/*
	   Testname: Projected service account token file ownership and permission.
	   Description: Ensure that Projected Service Account Token is mounted with
	               correct file ownership and permission mounted. We test the
	               following scenarios here.
	   1. RunAsUser is set,
	   2. FsGroup is set,
	   3. RunAsUser and FsGroup are set,
	   4. Default, neither RunAsUser nor FsGroup is set,

	   Containers MUST verify that the projected service account token can be
	   read and has correct file mode set including ownership and permission.
	*/
	ginkgo.It("should set ownership and permission when RunAsUser or FsGroup is present [LinuxOnly] [NodeFeature:FSGroup]", func() {
		e2eskipper.SkipIfNodeOSDistroIs("windows")

		var (
			podName         = "test-pod-" + string(uuid.NewUUID())
			volumeName      = "test-volume"
			volumeMountPath = "/test-volume"
			tokenVolumePath = "/test-volume/token"
		)

		volumes := []v1.Volume{
			{
				Name: volumeName,
				VolumeSource: v1.VolumeSource{
					Projected: &v1.ProjectedVolumeSource{
						Sources: []v1.VolumeProjection{
							{
								ServiceAccountToken: &v1.ServiceAccountTokenProjection{
									Path:              "token",
									ExpirationSeconds: utilptr.Int64Ptr(60 * 60),
								},
							},
						},
					},
				},
			},
		}
		volumeMounts := []v1.VolumeMount{
			{
				Name:      volumeName,
				MountPath: volumeMountPath,
				ReadOnly:  true,
			},
		}
		mounttestArgs := []string{
			"mounttest",
			fmt.Sprintf("--file_perm=%v", tokenVolumePath),
			fmt.Sprintf("--file_owner=%v", tokenVolumePath),
			fmt.Sprintf("--file_content=%v", tokenVolumePath),
		}

		pod := e2epod.NewAgnhostPod(f.Namespace.Name, podName, volumes, volumeMounts, nil, mounttestArgs...)
		pod.Spec.RestartPolicy = v1.RestartPolicyNever

		testcases := []struct {
			runAsUser bool
			fsGroup   bool
			wantPerm  string
			wantUID   int64
			wantGID   int64
		}{
			{
				runAsUser: true,
				wantPerm:  "-rw-------",
				wantUID:   1000,
				wantGID:   0,
			},
			{
				fsGroup:  true,
				wantPerm: "-rw-r-----",
				wantUID:  0,
				wantGID:  10000,
			},
			{
				runAsUser: true,
				fsGroup:   true,
				wantPerm:  "-rw-r-----",
				wantUID:   1000,
				wantGID:   10000,
			},
			{
				wantPerm: "-rw-r--r--",
				wantUID:  0,
				wantGID:  0,
			},
		}

		for _, tc := range testcases {
			pod.Spec.SecurityContext = &v1.PodSecurityContext{}
			if tc.runAsUser {
				pod.Spec.SecurityContext.RunAsUser = &tc.wantUID
			}
			if tc.fsGroup {
				pod.Spec.SecurityContext.FSGroup = &tc.wantGID
			}

			output := []string{
				fmt.Sprintf("perms of file \"%v\": %s", tokenVolumePath, tc.wantPerm),
				fmt.Sprintf("content of file \"%v\": %s", tokenVolumePath, `[A-Za-z0-9-_=]+\.[A-Za-z0-9-_=]+\.?[A-Za-z0-9-_.+/=]*`),
				fmt.Sprintf("owner UID of \"%v\": %d", tokenVolumePath, tc.wantUID),
				fmt.Sprintf("owner GID of \"%v\": %d", tokenVolumePath, tc.wantGID),
			}
			f.TestContainerOutputRegexp("service account token: ", pod, 0, output)
		}
	})

	ginkgo.It("should support InClusterConfig with token rotation [Slow]", func() {
		tenMin := int64(10 * 60)
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "inclusterclient"},
			Spec: v1.PodSpec{
				Containers: []v1.Container{{
					Name:  "inclusterclient",
					Image: imageutils.GetE2EImage(imageutils.Agnhost),
					Args:  []string{"inclusterclient"},
					VolumeMounts: []v1.VolumeMount{{
						MountPath: "/var/run/secrets/kubernetes.io/serviceaccount",
						Name:      "kube-api-access-e2e",
						ReadOnly:  true,
					}},
				}},
				RestartPolicy:      v1.RestartPolicyNever,
				ServiceAccountName: "default",
				Volumes: []v1.Volume{{
					Name: "kube-api-access-e2e",
					VolumeSource: v1.VolumeSource{
						Projected: &v1.ProjectedVolumeSource{
							Sources: []v1.VolumeProjection{
								{
									ServiceAccountToken: &v1.ServiceAccountTokenProjection{
										Path:              "token",
										ExpirationSeconds: &tenMin,
									},
								},
								{
									ConfigMap: &v1.ConfigMapProjection{
										LocalObjectReference: v1.LocalObjectReference{
											Name: "kube-root-ca.crt",
										},
										Items: []v1.KeyToPath{
											{
												Key:  "ca.crt",
												Path: "ca.crt",
											},
										},
									},
								},
								{
									DownwardAPI: &v1.DownwardAPIProjection{
										Items: []v1.DownwardAPIVolumeFile{
											{
												Path: "namespace",
												FieldRef: &v1.ObjectFieldSelector{
													APIVersion: "v1",
													FieldPath:  "metadata.namespace",
												},
											},
										},
									},
								},
							},
						},
					},
				}},
			},
		}
		pod, err := f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), pod, metav1.CreateOptions{})
		framework.ExpectNoError(err)

		framework.Logf("created pod")
		if !e2epod.CheckPodsRunningReady(f.ClientSet, f.Namespace.Name, []string{pod.Name}, time.Minute) {
			framework.Failf("pod %q in ns %q never became ready", pod.Name, f.Namespace.Name)
		}

		framework.Logf("pod is ready")

		var logs string
		if err := wait.Poll(1*time.Minute, 20*time.Minute, func() (done bool, err error) {
			framework.Logf("polling logs")
			logs, err = e2epod.GetPodLogs(f.ClientSet, f.Namespace.Name, "inclusterclient", "inclusterclient")
			if err != nil {
				framework.Logf("Error pulling logs: %v", err)
				return false, nil
			}
			tokenCount, err := ParseInClusterClientLogs(logs)
			if err != nil {
				return false, fmt.Errorf("inclusterclient reported an error: %v", err)
			}
			if tokenCount < 2 {
				framework.Logf("Retrying. Still waiting to see more unique tokens: got=%d, want=2", tokenCount)
				return false, nil
			}
			return true, nil
		}); err != nil {
			framework.Failf("Unexpected error: %v\n%s", err, logs)
		}
	})

	/*
	   Release: v1.21
	   Testname: OIDC Discovery (ServiceAccountIssuerDiscovery)
	   Description: Ensure kube-apiserver serves correct OIDC discovery
	   endpoints by deploying a Pod that verifies its own
	   token against these endpoints.
	*/
	framework.ConformanceIt("ServiceAccountIssuerDiscovery should support OIDC discovery of service account issuer", func() {

		// Allow the test pod access to the OIDC discovery non-resource URLs.
		// The role should have already been automatically created as part of the
		// RBAC bootstrap policy, but not the role binding. If RBAC is disabled,
		// we skip creating the binding. We also make sure we clean up the
		// binding after the test.
		const clusterRoleName = "system:service-account-issuer-discovery"
		crbName := fmt.Sprintf("%s-%s", f.Namespace.Name, clusterRoleName)
		if crb, err := f.ClientSet.RbacV1().ClusterRoleBindings().Create(
			context.TODO(),
			&rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: crbName,
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      rbacv1.ServiceAccountKind,
						APIGroup:  "",
						Name:      "default",
						Namespace: f.Namespace.Name,
					},
				},
				RoleRef: rbacv1.RoleRef{
					Name:     clusterRoleName,
					APIGroup: rbacv1.GroupName,
					Kind:     "ClusterRole",
				},
			},
			metav1.CreateOptions{}); err != nil {
			// Tolerate RBAC not being enabled
			framework.Logf("error granting ClusterRoleBinding %s: %v", crbName, err)
		} else {
			defer func() {
				framework.ExpectNoError(
					f.ClientSet.RbacV1().ClusterRoleBindings().Delete(
						context.TODO(),
						crb.Name, metav1.DeleteOptions{}))
			}()
		}

		// Create the pod with tokens.
		tokenPath := "/var/run/secrets/tokens"
		tokenName := "sa-token"
		audience := "oidc-discovery-test"
		tenMin := int64(10 * 60)

		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "oidc-discovery-validator"},
			Spec: v1.PodSpec{
				Containers: []v1.Container{{
					Name:  "oidc-discovery-validator",
					Image: imageutils.GetE2EImage(imageutils.Agnhost),
					Args: []string{
						"test-service-account-issuer-discovery",
						"--token-path", path.Join(tokenPath, tokenName),
						"--audience", audience,
					},
					VolumeMounts: []v1.VolumeMount{{
						MountPath: tokenPath,
						Name:      tokenName,
						ReadOnly:  true,
					}},
				}},
				RestartPolicy:      v1.RestartPolicyNever,
				ServiceAccountName: "default",
				Volumes: []v1.Volume{{
					Name: tokenName,
					VolumeSource: v1.VolumeSource{
						Projected: &v1.ProjectedVolumeSource{
							Sources: []v1.VolumeProjection{
								{
									ServiceAccountToken: &v1.ServiceAccountTokenProjection{
										Path:              tokenName,
										ExpirationSeconds: &tenMin,
										Audience:          audience,
									},
								},
							},
						},
					},
				}},
			},
		}
		pod, err := f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), pod, metav1.CreateOptions{})
		framework.ExpectNoError(err)

		framework.Logf("created pod")
		podErr := e2epod.WaitForPodSuccessInNamespace(f.ClientSet, pod.Name, f.Namespace.Name)

		// Get the logs before calling ExpectNoError, so we can debug any errors.
		var logs string
		if err := wait.Poll(30*time.Second, 2*time.Minute, func() (done bool, err error) {
			framework.Logf("polling logs")
			logs, err = e2epod.GetPodLogs(f.ClientSet, f.Namespace.Name, pod.Name, pod.Spec.Containers[0].Name)
			if err != nil {
				framework.Logf("Error pulling logs: %v", err)
				return false, nil
			}
			return true, nil
		}); err != nil {
			framework.Failf("Unexpected error getting pod logs: %v\n%s", err, logs)
		} else {
			framework.Logf("Pod logs: \n%v", logs)
		}

		framework.ExpectNoError(podErr)
		framework.Logf("completed pod")
	})

	/*
			   Release: v1.19
			   Testname: ServiceAccount lifecycle test
			   Description: Creates a ServiceAccount with a static Label MUST be added as shown in watch event.
		                        Patching the ServiceAccount MUST return it's new property.
		                        Listing the ServiceAccounts MUST return the test ServiceAccount with it's patched values.
		                        ServiceAccount will be deleted and MUST find a deleted watch event.
	*/
	framework.ConformanceIt("should run through the lifecycle of a ServiceAccount", func() {
		testNamespaceName := f.Namespace.Name
		testServiceAccountName := "testserviceaccount"
		testServiceAccountStaticLabels := map[string]string{"test-serviceaccount-static": "true"}
		testServiceAccountStaticLabelsFlat := "test-serviceaccount-static=true"

		ginkgo.By("creating a ServiceAccount")
		testServiceAccount := v1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:   testServiceAccountName,
				Labels: testServiceAccountStaticLabels,
			},
		}
		_, err := f.ClientSet.CoreV1().ServiceAccounts(testNamespaceName).Create(context.TODO(), &testServiceAccount, metav1.CreateOptions{})
		framework.ExpectNoError(err, "failed to create a ServiceAccount")

		ginkgo.By("watching for the ServiceAccount to be added")
		resourceWatchTimeoutSeconds := int64(180)
		resourceWatch, err := f.ClientSet.CoreV1().ServiceAccounts(testNamespaceName).Watch(context.TODO(), metav1.ListOptions{LabelSelector: testServiceAccountStaticLabelsFlat, TimeoutSeconds: &resourceWatchTimeoutSeconds})
		if err != nil {
			fmt.Println(err, "failed to setup watch on newly created ServiceAccount")
			return
		}

		resourceWatchChan := resourceWatch.ResultChan()
		eventFound := false
		for watchEvent := range resourceWatchChan {
			if watchEvent.Type == watch.Added {
				eventFound = true
				break
			}
		}
		framework.ExpectEqual(eventFound, true, "failed to find %v event", watch.Added)

		ginkgo.By("patching the ServiceAccount")
		boolFalse := false
		testServiceAccountPatchData, err := json.Marshal(v1.ServiceAccount{
			AutomountServiceAccountToken: &boolFalse,
		})
		framework.ExpectNoError(err, "failed to marshal JSON patch for the ServiceAccount")
		_, err = f.ClientSet.CoreV1().ServiceAccounts(testNamespaceName).Patch(context.TODO(), testServiceAccountName, types.StrategicMergePatchType, []byte(testServiceAccountPatchData), metav1.PatchOptions{})
		framework.ExpectNoError(err, "failed to patch the ServiceAccount")
		eventFound = false
		for watchEvent := range resourceWatchChan {
			if watchEvent.Type == watch.Modified {
				eventFound = true
				break
			}
		}
		framework.ExpectEqual(eventFound, true, "failed to find %v event", watch.Modified)

		ginkgo.By("finding ServiceAccount in list of all ServiceAccounts (by LabelSelector)")
		serviceAccountList, err := f.ClientSet.CoreV1().ServiceAccounts("").List(context.TODO(), metav1.ListOptions{LabelSelector: testServiceAccountStaticLabelsFlat})
		framework.ExpectNoError(err, "failed to list ServiceAccounts by LabelSelector")
		foundServiceAccount := false
		for _, serviceAccountItem := range serviceAccountList.Items {
			if serviceAccountItem.ObjectMeta.Name == testServiceAccountName && serviceAccountItem.ObjectMeta.Namespace == testNamespaceName && *serviceAccountItem.AutomountServiceAccountToken == boolFalse {
				foundServiceAccount = true
				break
			}
		}
		framework.ExpectEqual(foundServiceAccount, true, "failed to find the created ServiceAccount")

		ginkgo.By("deleting the ServiceAccount")
		err = f.ClientSet.CoreV1().ServiceAccounts(testNamespaceName).DeleteCollection(context.TODO(), metav1.DeleteOptions{}, metav1.ListOptions{})
		framework.ExpectNoError(err, "failed to delete the ServiceAccount by Collection")
		eventFound = false
		for watchEvent := range resourceWatchChan {
			if watchEvent.Type == watch.Deleted {
				eventFound = true
				break
			}
		}
		framework.ExpectEqual(eventFound, true, "failed to find %v event", watch.Deleted)
	})

	/*
		Release: v1.21
		Testname: RootCA ConfigMap test
		Description: Ensure every namespace exist a ConfigMap for root ca cert.
			1. Created automatically
			2. Recreated if deleted
			3. Reconciled if modified
	*/
	framework.ConformanceIt("should guarantee kube-root-ca.crt exist in any namespace", func() {
		const rootCAConfigMapName = "kube-root-ca.crt"

		framework.ExpectNoError(wait.PollImmediate(500*time.Millisecond, wait.ForeverTestTimeout, func() (bool, error) {
			_, err := f.ClientSet.CoreV1().ConfigMaps(f.Namespace.Name).Get(context.TODO(), rootCAConfigMapName, metav1.GetOptions{})
			if err == nil {
				return true, nil
			}
			if apierrors.IsNotFound(err) {
				ginkgo.By("root ca configmap not found, retrying")
				return false, nil
			}
			return false, err
		}))
		framework.Logf("Got root ca configmap in namespace %q", f.Namespace.Name)

		framework.ExpectNoError(f.ClientSet.CoreV1().ConfigMaps(f.Namespace.Name).Delete(context.TODO(), rootCAConfigMapName, metav1.DeleteOptions{GracePeriodSeconds: utilptr.Int64Ptr(0)}))
		framework.Logf("Deleted root ca configmap in namespace %q", f.Namespace.Name)

		framework.ExpectNoError(wait.Poll(500*time.Millisecond, wait.ForeverTestTimeout, func() (bool, error) {
			ginkgo.By("waiting for a new root ca configmap created")
			_, err := f.ClientSet.CoreV1().ConfigMaps(f.Namespace.Name).Get(context.TODO(), rootCAConfigMapName, metav1.GetOptions{})
			if err == nil {
				return true, nil
			}
			if apierrors.IsNotFound(err) {
				ginkgo.By("root ca configmap not found, retrying")
				return false, nil
			}
			return false, err
		}))
		framework.Logf("Recreated root ca configmap in namespace %q", f.Namespace.Name)

		_, err := f.ClientSet.CoreV1().ConfigMaps(f.Namespace.Name).Update(context.TODO(), &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: rootCAConfigMapName,
			},
			Data: map[string]string{
				"ca.crt": "",
			},
		}, metav1.UpdateOptions{})
		framework.ExpectNoError(err)
		framework.Logf("Updated root ca configmap in namespace %q", f.Namespace.Name)

		framework.ExpectNoError(wait.Poll(500*time.Millisecond, wait.ForeverTestTimeout, func() (bool, error) {
			ginkgo.By("waiting for the root ca configmap reconciled")
			cm, err := f.ClientSet.CoreV1().ConfigMaps(f.Namespace.Name).Get(context.TODO(), rootCAConfigMapName, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					ginkgo.By("root ca configmap not found, retrying")
					return false, nil
				}
				return false, err
			}
			if value, ok := cm.Data["ca.crt"]; !ok || value == "" {
				ginkgo.By("root ca configmap is not reconciled yet, retrying")
				return false, nil
			}
			return true, nil
		}))
		framework.Logf("Reconciled root ca configmap in namespace %q", f.Namespace.Name)
	})
})

var reportLogsParser = regexp.MustCompile("([a-zA-Z0-9-_]*)=([a-zA-Z0-9-_]*)$")

// ParseInClusterClientLogs parses logs of pods using inclusterclient.
func ParseInClusterClientLogs(logs string) (int, error) {
	seenTokens := map[string]struct{}{}

	lines := strings.Split(logs, "\n")
	for _, line := range lines {
		parts := reportLogsParser.FindStringSubmatch(line)
		if len(parts) != 3 {
			continue
		}

		key, value := parts[1], parts[2]
		switch key {
		case "authz_header":
			if value == "<empty>" {
				return 0, fmt.Errorf("saw empty Authorization header")
			}
			seenTokens[value] = struct{}{}
		case "status":
			if value == "failed" {
				return 0, fmt.Errorf("saw status=failed")
			}
		}
	}

	return len(seenTokens), nil
}
