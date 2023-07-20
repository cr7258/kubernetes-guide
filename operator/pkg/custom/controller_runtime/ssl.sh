#! /bin/sh

export APP="${1}"
export NAMESPACE="${2}"
export CSR_NAME="${APP}.${NAMESPACE}.svc"

cd $(dirname "$0")/certs

echo "Create CA cert"

cat > ca-config.json <<EOF
{
  "signing": {
    "default": {
      "expiry": "8760h"
    },
    "profiles": {
      "server": {
        "usages": ["signing", "key encipherment", "server auth", "client auth"],
        "expiry": "8760h"
      }
    }
  }
}
EOF

cat > ca-csr.json <<EOF
{
  "CN": "Kubernetes",
  "key": {
    "algo": "rsa",
    "size": 2048
  },
  "names": [
    {
      "C": "US",
      "L": "Portland",
      "O": "Kubernetes",
      "OU": "CA",
      "ST": "Oregon"
    }
  ]
}
EOF

cfssl gencert -initca ca-csr.json | cfssljson -bare ca

echo "Create Server Cert"

cat > server-csr.json <<EOF
{
  "CN": "admission",
  "key": {
    "algo": "rsa",
    "size": 2048
  },
  "names": [
    {
      "C": "US",
      "L": "Portland",
      "O": "Kubernetes",
      "OU": "Kubernetes",
      "ST": "Oregon"
    }
  ]
}
EOF

cfssl gencert \
  -ca=ca.pem \
  -ca-key=ca-key.pem \
  -config=ca-config.json \
  -hostname=${CSR_NAME} \
  -profile=server \
  server-csr.json | cfssljson -bare server

cp server.pem tls.crt
cp server-key.pem tls.key

echo "Action Required: Get CA Bundle, Please copy the output to your webhook configuration caBundle."
openssl base64 -A -in ca.pem | tr -d '\n'