<h1>Quickstart</h1>

This guide aims to give you a quick look and feel for using the Database Interface on a local Kubernetes environment.

## Prerequisites

### Kubernetes
For local tests we recommend to use one of the following
solutions:

* [minikube](https://github.com/kubernetes/minikube/releases), which creates a
  single-node K8s cluster inside a VM (requires KVM or VirtualBox),
* [kind](https://kind.sigs.k8s.io/) and [k3d](https://k3d.io), which allows creating multi-nodes K8s
  clusters running on Docker (requires Docker)

### Postgres

For the quick installation you can use [Zalando Postgres Operator](https://github.com/zalando/postgres-operator)

The Postgres Operator can be deployed in the following way:

```bash
kubectl apply -k github.com/zalando/postgres-operator/manifests
```

Check if Postgres Operator is running

```bash
kubectl get pod -l name=postgres-operator
```
If the operator pod is running it listens to new events regarding `postgresql`
resources. Now, it's time to submit your first Postgres cluster manifest.
```bash
kubectl create -f https://raw.githubusercontent.com/zalando/postgres-operator/master/manifests/minimal-postgres-manifest.yaml
```

Now you can retrieve the database password:

```bash
kubectl get secret postgres.acid-minimal-cluster.credentials.postgresql.acid.zalan.do -o 'jsonpath={.data.password}' | base64 -d
```

The database user is `postgres`

## Database Interface CRDs

Before you deploy database-controller and sidecar-controller you have to deploy the Database Interface API CRDs.
Clone [database-interface-api](https://github.com/pluralsh/database-interface-api)

Deploy CRDs from manifests:
```bash
kubectl create -f config/crd/bases/
```



