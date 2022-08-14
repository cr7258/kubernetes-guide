# 创建 CA 证书和服务器证书
```bash
cat > ca-config.json << EOF
{
  "signing": {
    "default": {
      "expiry": "8760h"
    },
    "profiles": {
      "server": {
        "usages": ["signing"],
        "expiry": "8760h"
      }
    }
  }
}
EOF

cat > ca-csr.json  << EOF
{
  "CN": "Kubernetes",
  "key": {
    "algo": "rsa",
    "size": 2048
  },
  "names": [
    {
      "C": "zh",
      "L": "bj",
      "O": "bj",
      "OU": "CA"
   }
  ]
}
EOF

cfssl gencert -initca ca-csr.json | cfssljson -bare ca

cat > server-csr.json << EOF
{
  "CN": "admission",
  "key": {
    "algo": "rsa",
    "size": 2048
  },
  "names": [
    {
      "C": "zh",
      "L": "bj",
      "O": "bj",
      "OU": "bj"
    }
  ]
}
EOF

cfssl gencert \
  -ca=ca.pem \
  -ca-key=ca-key.pem \
  -config=ca-config.json \
  -hostname=myhook.kube-system.svc \
  -profile=server \
  server-csr.json | cfssljson -bare server

kubectl create secret tls myhook --cert=server.pem --key=server-key.pem  -n kube-system
```

# 获取 MutatingWebhookConfiguration 的 caBundle
```bash
cat ca.pem | base64
```

