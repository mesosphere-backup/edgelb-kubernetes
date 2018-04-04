dep:
    dep ensure

run: KUBECONFIG?=${HOME}/.kube/config
    go run *.go -debug -kubeconfig=$(KUBECONFIG)
