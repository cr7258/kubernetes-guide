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

package serviceaccount

// This file tests authentication and (soon) authorization of HTTP requests to an API server object.
// It does not use the client in pkg/client/... because authentication and authorization needs
// to work for any client of the HTTP interface.

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/group"
	"k8s.io/apiserver/pkg/authentication/request/bearertoken"
	"k8s.io/apiserver/pkg/authentication/request/union"
	serviceaccountapiserver "k8s.io/apiserver/pkg/authentication/serviceaccount"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	clientinformers "k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"k8s.io/kubernetes/pkg/controller"
	serviceaccountcontroller "k8s.io/kubernetes/pkg/controller/serviceaccount"
	"k8s.io/kubernetes/pkg/controlplane"
	kubefeatures "k8s.io/kubernetes/pkg/features"
	"k8s.io/kubernetes/pkg/serviceaccount"
	serviceaccountadmission "k8s.io/kubernetes/plugin/pkg/admission/serviceaccount"
	"k8s.io/kubernetes/test/integration/framework"
)

const (
	rootUserName = "root"
	rootToken    = "root-user-token" // Fake value for testing.

	readOnlyServiceAccountName  = "ro"
	readWriteServiceAccountName = "rw"
)

func TestServiceAccountAutoCreate(t *testing.T) {
	c, _, stopFunc, err := startServiceAccountTestServerAndWaitForCaches(t)
	defer stopFunc()
	if err != nil {
		t.Fatalf("failed to setup ServiceAccounts server: %v", err)
	}

	ns := "test-service-account-creation"

	// Create namespace
	_, err = c.CoreV1().Namespaces().Create(context.TODO(), &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("could not create namespace: %v", err)
	}

	// Get service account
	defaultUser, err := getServiceAccount(c, ns, "default", true)
	if err != nil {
		t.Fatalf("Default serviceaccount not created: %v", err)
	}

	// Delete service account
	err = c.CoreV1().ServiceAccounts(ns).Delete(context.TODO(), defaultUser.Name, metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("Could not delete default serviceaccount: %v", err)
	}

	// Get recreated service account
	defaultUser2, err := getServiceAccount(c, ns, "default", true)
	if err != nil {
		t.Fatalf("Default serviceaccount not created: %v", err)
	}
	if defaultUser2.UID == defaultUser.UID {
		t.Fatalf("Expected different UID with recreated serviceaccount")
	}
}

func TestServiceAccountTokenAutoCreate(t *testing.T) {
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, kubefeatures.LegacyServiceAccountTokenNoAutoGeneration, false)()
	c, _, stopFunc, err := startServiceAccountTestServerAndWaitForCaches(t)
	defer stopFunc()
	if err != nil {
		t.Fatalf("failed to setup ServiceAccounts server: %v", err)
	}

	ns := "test-service-account-token-creation"
	name := "my-service-account"

	// Create namespace
	_, err = c.CoreV1().Namespaces().Create(context.TODO(), &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("could not create namespace: %v", err)
	}

	// Create service account
	_, err = c.CoreV1().ServiceAccounts(ns).Create(context.TODO(), &v1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Service Account not created: %v", err)
	}

	// Get token
	token1Name, token1, err := getReferencedServiceAccountToken(c, ns, name, true)
	if err != nil {
		t.Fatal(err)
	}

	// Delete token
	err = c.CoreV1().Secrets(ns).Delete(context.TODO(), token1Name, metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("Could not delete token: %v", err)
	}

	// Get recreated token
	token2Name, token2, err := getReferencedServiceAccountToken(c, ns, name, true)
	if err != nil {
		t.Fatal(err)
	}
	if token1Name == token2Name {
		t.Fatalf("Expected new auto-created token name")
	}
	if token1 == token2 {
		t.Fatalf("Expected new auto-created token value")
	}

	// Trigger creation of a new referenced token
	serviceAccount, err := c.CoreV1().ServiceAccounts(ns).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	serviceAccount.Secrets = []v1.ObjectReference{}
	_, err = c.CoreV1().ServiceAccounts(ns).Update(context.TODO(), serviceAccount, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Get rotated token
	token3Name, token3, err := getReferencedServiceAccountToken(c, ns, name, true)
	if err != nil {
		t.Fatal(err)
	}
	if token3Name == token2Name {
		t.Fatalf("Expected new auto-created token name")
	}
	if token3 == token2 {
		t.Fatalf("Expected new auto-created token value")
	}

	// Delete service account
	err = c.CoreV1().ServiceAccounts(ns).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Wait for tokens to be deleted
	tokensToCleanup := sets.NewString(token1Name, token2Name, token3Name)
	err = wait.Poll(time.Second, 10*time.Second, func() (bool, error) {
		// Get all secrets in the namespace
		secrets, err := c.CoreV1().Secrets(ns).List(context.TODO(), metav1.ListOptions{})
		// Retrieval errors should fail
		if err != nil {
			return false, err
		}
		for _, s := range secrets.Items {
			if tokensToCleanup.Has(s.Name) {
				// Still waiting for tokens to be cleaned up
				return false, nil
			}
		}
		// All clean
		return true, nil
	})
	if err != nil {
		t.Fatalf("Error waiting for tokens to be deleted: %v", err)
	}
}

func TestServiceAccountTokenAutoMount(t *testing.T) {
	c, _, stopFunc, err := startServiceAccountTestServerAndWaitForCaches(t)
	defer stopFunc()
	if err != nil {
		t.Fatalf("failed to setup ServiceAccounts server: %v", err)
	}

	ns := "auto-mount-ns"

	// Create "my" namespace
	_, err = c.CoreV1().Namespaces().Create(context.TODO(), &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("could not create namespace: %v", err)
	}

	// Pod to create
	protoPod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "protopod"},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "container",
					Image: "container-image",
				},
			},
		},
	}

	createdPod, err := c.CoreV1().Pods(ns).Create(context.TODO(), &protoPod, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	expectedServiceAccount := serviceaccountadmission.DefaultServiceAccountName
	if createdPod.Spec.ServiceAccountName != expectedServiceAccount {
		t.Fatalf("Expected %s, got %s", expectedServiceAccount, createdPod.Spec.ServiceAccountName)
	}
	if len(createdPod.Spec.Volumes) == 0 || createdPod.Spec.Volumes[0].Projected == nil {
		t.Fatal("Expected projected volume for service account token inserted")
	}
}

func TestServiceAccountTokenAuthentication(t *testing.T) {
	c, config, stopFunc, err := startServiceAccountTestServerAndWaitForCaches(t)
	defer stopFunc()
	if err != nil {
		t.Fatalf("failed to setup ServiceAccounts server: %v", err)
	}

	myns := "auth-ns"
	otherns := "other-ns"

	// Create "my" namespace
	_, err = c.CoreV1().Namespaces().Create(context.TODO(), &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: myns}}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("could not create namespace: %v", err)
	}

	// Create "other" namespace
	_, err = c.CoreV1().Namespaces().Create(context.TODO(), &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: otherns}}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("could not create namespace: %v", err)
	}

	// Create "ro" user in myns
	roSA, err := c.CoreV1().ServiceAccounts(myns).Create(context.TODO(), &v1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: readOnlyServiceAccountName}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Service Account not created: %v", err)
	}

	roTokenName := "ro-test-token"
	secret, err := createServiceAccountToken(c, roSA, myns, roTokenName)
	if err != nil {
		t.Fatalf("Secret not created: %v", err)
	}
	roClientConfig := *config
	roClientConfig.BearerToken = string(secret.Data[v1.ServiceAccountTokenKey])
	roClient := clientset.NewForConfigOrDie(&roClientConfig)
	doServiceAccountAPIRequests(t, roClient, myns, true, true, false)
	doServiceAccountAPIRequests(t, roClient, otherns, true, false, false)
	err = c.CoreV1().Secrets(myns).Delete(context.TODO(), roTokenName, metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("could not delete token: %v", err)
	}
	// wait for delete to be observed and reacted to via watch
	err = wait.PollImmediate(100*time.Millisecond, 30*time.Second, func() (bool, error) {
		_, err := roClient.CoreV1().Secrets(myns).List(context.TODO(), metav1.ListOptions{})
		if err == nil {
			t.Logf("token is still valid, waiting")
			return false, nil
		}
		if !apierrors.IsUnauthorized(err) {
			t.Logf("expected unauthorized error, got %v", err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("waiting for token to be invalidated: %v", err)
	}
	doServiceAccountAPIRequests(t, roClient, myns, false, false, false)

	// Create "rw" user in myns
	rwSA, err := c.CoreV1().ServiceAccounts(myns).Create(context.TODO(), &v1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: readWriteServiceAccountName}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Service Account not created: %v", err)
	}
	rwTokenName := "rw-test-token"
	secret, err = createServiceAccountToken(c, rwSA, myns, rwTokenName)
	if err != nil {
		t.Fatalf("Secret not created: %v", err)
	}
	rwClientConfig := *config
	rwClientConfig.BearerToken = string(secret.Data[v1.ServiceAccountTokenKey])
	rwClient := clientset.NewForConfigOrDie(&rwClientConfig)
	doServiceAccountAPIRequests(t, rwClient, myns, true, true, true)
	doServiceAccountAPIRequests(t, rwClient, otherns, true, false, false)
}

// startServiceAccountTestServerAndWaitForCaches returns a started server
// It is the responsibility of the caller to ensure the returned stopFunc is called
func startServiceAccountTestServerAndWaitForCaches(t *testing.T) (*clientset.Clientset, *restclient.Config, func(), error) {
	rootTokenAuth := authenticator.TokenFunc(func(ctx context.Context, token string) (*authenticator.Response, bool, error) {
		if token == rootToken {
			return &authenticator.Response{User: &user.DefaultInfo{Name: rootUserName}}, true, nil
		}
		return nil, false, nil
	})
	serviceAccountKey, _ := rsa.GenerateKey(rand.Reader, 2048)

	var informers clientinformers.SharedInformerFactory
	var externalInformers clientinformers.SharedInformerFactory
	var rootClientset *clientset.Clientset

	// Set up a API server
	_, clientConfig, tearDownFn := framework.StartTestServer(t, framework.TestServerSetup{
		ModifyServerConfig: func(config *controlplane.Config) {
			rootConfig := restclient.CopyConfig(config.GenericConfig.LoopbackClientConfig)
			rootConfig.BearerToken = rootToken
			rootClientset = clientset.NewForConfigOrDie(rootConfig)
			externalRootClientset := clientset.NewForConfigOrDie(rootConfig)

			externalInformers = clientinformers.NewSharedInformerFactory(externalRootClientset, controller.NoResyncPeriodFunc())
			informers = clientinformers.NewSharedInformerFactory(rootClientset, controller.NoResyncPeriodFunc())

			// Set up two authenticators:
			// 1. A token authenticator that maps the rootToken to the "root" user
			// 2. A ServiceAccountToken authenticator that validates ServiceAccount tokens
			serviceAccountTokenGetter := serviceaccountcontroller.NewGetterFromClient(
				rootClientset,
				externalInformers.Core().V1().Secrets().Lister(),
				externalInformers.Core().V1().ServiceAccounts().Lister(),
				externalInformers.Core().V1().Pods().Lister(),
			)
			serviceAccountTokenAuth := serviceaccount.JWTTokenAuthenticator(
				[]string{serviceaccount.LegacyIssuer},
				[]interface{}{&serviceAccountKey.PublicKey},
				nil,
				serviceaccount.NewLegacyValidator(true, serviceAccountTokenGetter),
			)
			authenticator := group.NewAuthenticatedGroupAdder(union.New(
				bearertoken.New(rootTokenAuth),
				bearertoken.New(serviceAccountTokenAuth),
			))

			// Set up a stub authorizer:
			// 1. The "root" user is allowed to do anything
			// 2. ServiceAccounts named "ro" are allowed read-only operations in their namespace
			// 3. ServiceAccounts named "rw" are allowed any operation in their namespace
			authorizer := authorizer.AuthorizerFunc(func(ctx context.Context, attrs authorizer.Attributes) (authorizer.Decision, string, error) {
				username := ""
				if user := attrs.GetUser(); user != nil {
					username = user.GetName()
				}
				ns := attrs.GetNamespace()

				// If the user is "root"...
				if username == rootUserName {
					// allow them to do anything
					return authorizer.DecisionAllow, "", nil
				}

				// If the user is a service account...
				if serviceAccountNamespace, serviceAccountName, err := serviceaccountapiserver.SplitUsername(username); err == nil {
					// Limit them to their own namespace
					if serviceAccountNamespace == ns {
						switch serviceAccountName {
						case readOnlyServiceAccountName:
							if attrs.IsReadOnly() {
								return authorizer.DecisionAllow, "", nil
							}
						case readWriteServiceAccountName:
							return authorizer.DecisionAllow, "", nil
						}
					}
				}

				return authorizer.DecisionNoOpinion, fmt.Sprintf("User %s is denied (ns=%s, readonly=%v, resource=%s)", username, ns, attrs.IsReadOnly(), attrs.GetResource()), nil
			})

			// Set up admission plugin to auto-assign serviceaccounts to pods
			serviceAccountAdmission := serviceaccountadmission.NewServiceAccount()
			serviceAccountAdmission.SetExternalKubeClientSet(externalRootClientset)
			serviceAccountAdmission.SetExternalKubeInformerFactory(externalInformers)

			config.GenericConfig.EnableIndex = true
			config.GenericConfig.Authentication.Authenticator = authenticator
			config.GenericConfig.Authorization.Authorizer = authorizer
			config.GenericConfig.AdmissionControl = serviceAccountAdmission
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	stop := func() {
		cancel()
		tearDownFn()
	}

	// Start the service account and service account token controllers
	tokenGenerator, err := serviceaccount.JWTTokenGenerator(serviceaccount.LegacyIssuer, serviceAccountKey)
	if err != nil {
		return rootClientset, clientConfig, stop, err
	}
	tokenController, err := serviceaccountcontroller.NewTokensController(
		informers.Core().V1().ServiceAccounts(),
		informers.Core().V1().Secrets(),
		rootClientset,
		serviceaccountcontroller.TokensControllerOptions{
			TokenGenerator: tokenGenerator,
			AutoGenerate:   !utilfeature.DefaultFeatureGate.Enabled(kubefeatures.LegacyServiceAccountTokenNoAutoGeneration),
		},
	)
	if err != nil {
		return rootClientset, clientConfig, stop, err
	}
	go tokenController.Run(1, ctx.Done())

	serviceAccountController, err := serviceaccountcontroller.NewServiceAccountsController(
		informers.Core().V1().ServiceAccounts(),
		informers.Core().V1().Namespaces(),
		rootClientset,
		serviceaccountcontroller.DefaultServiceAccountsControllerOptions(),
	)
	if err != nil {
		return rootClientset, clientConfig, stop, err
	}
	informers.Start(ctx.Done())
	externalInformers.Start(ctx.Done())
	go serviceAccountController.Run(ctx, 5)

	// since this method starts the controllers in a separate goroutine
	// and the tests don't check /readyz there is no way
	// the tests can tell it is safe to call the server and requests won't be rejected
	// thus we wait until caches have synced
	informers.WaitForCacheSync(ctx.Done())
	externalInformers.WaitForCacheSync(ctx.Done())

	return rootClientset, clientConfig, stop, nil
}

func getServiceAccount(c *clientset.Clientset, ns string, name string, shouldWait bool) (*v1.ServiceAccount, error) {
	if !shouldWait {
		return c.CoreV1().ServiceAccounts(ns).Get(context.TODO(), name, metav1.GetOptions{})
	}

	var user *v1.ServiceAccount
	var err error
	err = wait.Poll(time.Second, 10*time.Second, func() (bool, error) {
		user, err = c.CoreV1().ServiceAccounts(ns).Get(context.TODO(), name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return true, nil
	})
	return user, err
}

func createServiceAccountToken(c *clientset.Clientset, sa *v1.ServiceAccount, ns string, name string) (*v1.Secret, error) {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Annotations: map[string]string{
				v1.ServiceAccountNameKey: sa.GetName(),
				v1.ServiceAccountUIDKey:  string(sa.UID),
			},
		},
		Type: v1.SecretTypeServiceAccountToken,
		Data: map[string][]byte{},
	}
	secret, err := c.CoreV1().Secrets(ns).Create(context.TODO(), secret, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create secret (%s:%s) %+v, err: %v", ns, secret.Name, *secret, err)
	}
	err = wait.Poll(time.Second, 10*time.Second, func() (bool, error) {
		if len(secret.Data[v1.ServiceAccountTokenKey]) != 0 {
			return false, nil
		}
		secret, err = c.CoreV1().Secrets(ns).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			return true, fmt.Errorf("failed to get secret (%s:%s) %+v, err: %v", ns, secret.Name, *secret, err)
		}
		return true, nil
	})
	return secret, nil
}

func getReferencedServiceAccountToken(c *clientset.Clientset, ns string, name string, shouldWait bool) (string, string, error) {
	tokenName := ""
	token := ""

	findToken := func() (bool, error) {
		user, err := c.CoreV1().ServiceAccounts(ns).Get(context.TODO(), name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}

		for _, ref := range user.Secrets {
			secret, err := c.CoreV1().Secrets(ns).Get(context.TODO(), ref.Name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return false, err
			}
			if secret.Type != v1.SecretTypeServiceAccountToken {
				continue
			}
			name := secret.Annotations[v1.ServiceAccountNameKey]
			uid := secret.Annotations[v1.ServiceAccountUIDKey]
			tokenData := secret.Data[v1.ServiceAccountTokenKey]
			if name == user.Name && uid == string(user.UID) && len(tokenData) > 0 {
				tokenName = secret.Name
				token = string(tokenData)
				return true, nil
			}
		}

		return false, nil
	}

	if shouldWait {
		err := wait.Poll(time.Second, 10*time.Second, findToken)
		if err != nil {
			return "", "", err
		}
	} else {
		ok, err := findToken()
		if err != nil {
			return "", "", err
		}
		if !ok {
			return "", "", fmt.Errorf("No token found for %s/%s", ns, name)
		}
	}
	return tokenName, token, nil
}

type testOperation func() error

func doServiceAccountAPIRequests(t *testing.T, c *clientset.Clientset, ns string, authenticated bool, canRead bool, canWrite bool) {
	testSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "testSecret"},
		Data:       map[string][]byte{"test": []byte("data")},
	}

	readOps := []testOperation{
		func() error {
			_, err := c.CoreV1().Secrets(ns).List(context.TODO(), metav1.ListOptions{})
			return err
		},
		func() error {
			_, err := c.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
			return err
		},
	}
	writeOps := []testOperation{
		func() error {
			_, err := c.CoreV1().Secrets(ns).Create(context.TODO(), testSecret, metav1.CreateOptions{})
			return err
		},
		func() error {
			return c.CoreV1().Secrets(ns).Delete(context.TODO(), testSecret.Name, metav1.DeleteOptions{})
		},
	}

	for _, op := range readOps {
		err := op()
		unauthorizedError := apierrors.IsUnauthorized(err)
		forbiddenError := apierrors.IsForbidden(err)

		switch {
		case !authenticated && !unauthorizedError:
			t.Fatalf("expected unauthorized error, got %v", err)
		case authenticated && unauthorizedError:
			t.Fatalf("unexpected unauthorized error: %v", err)
		case authenticated && canRead && forbiddenError:
			t.Fatalf("unexpected forbidden error: %v", err)
		case authenticated && !canRead && !forbiddenError:
			t.Fatalf("expected forbidden error, got: %v", err)
		}
	}

	for _, op := range writeOps {
		err := op()
		unauthorizedError := apierrors.IsUnauthorized(err)
		forbiddenError := apierrors.IsForbidden(err)

		switch {
		case !authenticated && !unauthorizedError:
			t.Fatalf("expected unauthorized error, got %v", err)
		case authenticated && unauthorizedError:
			t.Fatalf("unexpected unauthorized error: %v", err)
		case authenticated && canWrite && forbiddenError:
			t.Fatalf("unexpected forbidden error: %v", err)
		case authenticated && !canWrite && !forbiddenError:
			t.Fatalf("expected forbidden error, got: %v", err)
		}
	}
}
