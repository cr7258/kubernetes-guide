#! /bin/sh
# 由于 Kubernetes 签发的证书也是自签名证书，因此默认情况下 Kubernetes 也不信任
# 在访问 Webhook Server 的时候会收到 : x509: certificate signed by unknown authority 错误
# 需要将 Kubernetes 的 CA 添加到 WebhookConfiguration 的 caBundle 中让 Kubernetes 信任自己签发的证书
set -uo errexit

export APP="${1}"
export NAMESPACE="${2}"
export CSR_NAME="${APP}.${NAMESPACE}.svc"

echo "... creating tls.key"
openssl genrsa -out tls.key 2048

echo "... creating tls.csr"
cat >csr.conf<<EOF
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name
[req_distinguished_name]
[ v3_req ]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names
[alt_names]
DNS.1 = ${APP}
DNS.2 = ${APP}.${NAMESPACE}
DNS.3 = ${CSR_NAME}
DNS.4 = ${CSR_NAME}.cluster.local
EOF
echo "openssl req -new -key tls.key -subj \"/CN=${CSR_NAME}\" -out tls.csr -config csr.conf"
openssl req -new -key tls.key -subj "/CN=${CSR_NAME}" -out tls.csr -config csr.conf

echo "... creating kubernetes CSR object"
echo """
apiVersion: certificates.k8s.io/v1
kind: CertificateSigningRequest
metadata:
  name: ${CSR_NAME}
spec:
  groups:
  - system:authenticated
  request: $(cat tls.csr | base64 | tr -d '\n')
  signerName: kubernetes.io/kube-apiserver-client
  usages:
  - client auth
""" > csr.yaml
kubectl apply -f csr.yaml

SECONDS=0
while true; do
  echo "... waiting for csr to be present in kubernetes"
  echo "kubectl get csr ${CSR_NAME}"
  kubectl get csr ${CSR_NAME} > /dev/null 2>&1
  if [ "$?" -eq 0 ]; then
      break
  fi
  if [[ $SECONDS -ge 60 ]]; then
    echo "[!] timed out waiting for csr"
    exit 1
  fi
  sleep 2
done

kubectl certificate approve ${CSR_NAME}

SECONDS=0
while true; do
  echo "... waiting for serverCert to be present in kubernetes"
  echo "kubectl get csr ${CSR_NAME} -o jsonpath='{.status.certificate}'"
  serverCert=$(kubectl get csr ${CSR_NAME} -o jsonpath='{.status.certificate}')
  if [[ $serverCert != "" ]]; then
    break
  fi
  if [[ $SECONDS -ge 60 ]]; then
    echo "[!] timed out waiting for serverCert"
    exit 1
  fi
  sleep 2
done

echo "... creating tls.crt cert file"
echo ${serverCert} | openssl base64 -d -A -out tls.crt