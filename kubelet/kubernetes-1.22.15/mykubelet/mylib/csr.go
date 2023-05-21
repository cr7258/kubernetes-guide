package mylib

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	certificatesv1 "k8s.io/api/certificates/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/cert"
	"k8s.io/utils/pointer"
	"log"
	"os"
	"time"
)

/**
* @description 创建 CertificateSigningRequest 对象，保存 Private Key 到本地
* @author chengzw
* @since 2023/5/21
* @link
 */

const TEST_PRIVATEKEY_FILE = "../certs/kubelet.key"
const TEST_PEM_FILE = "../certs/kubelet.pem"

// 把私钥保存为文件
func savePrivateKeyToFile(key *ecdsa.PrivateKey) error {
	b, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	privkey_pem := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: b,
		},
	)
	_ = os.Remove(TEST_PRIVATEKEY_FILE)

	err = ioutil.WriteFile(TEST_PRIVATEKEY_FILE, privkey_pem, 0600)
	if err != nil {
		return err
	}
	return nil
}

// 生成 CSR 证书请求文件，用于填充 CertificateSigningRequest 对象的 reqeust 字段
func GenCSRPEM() []byte {
	cr := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   fmt.Sprintf("system:node:%s", "chengzw"),
			Organization: []string{"system:nodes"},
		},
	}
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatalln(err)
	}
	// 保存私钥为 kubelet.key 文件
	err = savePrivateKeyToFile(privateKey)
	if err != nil {
		log.Fatalln(err)
	}
	csrPEM, err := cert.MakeCSRFromTemplate(privateKey, cr)
	if err != nil {
		log.Fatalln(err)
	}
	return csrPEM
}

// 创建 certificates.k8s.io/v1 CertificateSigningRequest 对象
func CreateCSRCert(client *kubernetes.Clientset) *certificatesv1.CertificateSigningRequest {
	csr := &certificatesv1.CertificateSigningRequest{
		TypeMeta: metav1.TypeMeta{Kind: "CertificateSigningRequest", APIVersion: "certificates.k8s.io/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "testcsr",
		},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Request: GenCSRPEM(),
			Usages: []certificatesv1.KeyUsage{
				certificatesv1.UsageClientAuth,
			},
			ExpirationSeconds: DurationToExpirationSeconds(time.Second * 3600 * 10),
			SignerName:        certificatesv1.KubeAPIServerClientSignerName,
		},
	}

	csr_ret, err := client.CertificatesV1().CertificateSigningRequests().
		Create(context.Background(), csr, metav1.CreateOptions{})
	if err != nil {
		log.Fatalln(err)
	}
	return csr_ret
}

func DurationToExpirationSeconds(duration time.Duration) *int32 {
	return pointer.Int32(int32(duration / time.Second))
}
