go run cmd/kubelet/kubelet.go --v=1 --hostname-override=myjtthink \
--kubeconfig=./mykubelet/kubelet.config \
--bootstrap-kubeconfig=./mykubelet/bootstrap.yaml \
--config=./mykubelet/kubelet.config.yaml \
--cert-dir=/Users/I576375/Code/kubernetes-guide/kubelet/kubernetes-1.22.15/mykubelet/certs/kubelet