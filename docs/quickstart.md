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

Now it's time to deploy database and sidecar controllers. 

First deploy database-controller
```bash
kubectl create -f config/resources/database-controller
```

Go to `config/resources/sidecar-controller` and update `secret.yaml` file with Postgres parameters:

```yaml
...
...
stringData:
  DB_HOST: "acid-minimal-cluster"
  DB_PASSWORD: "password"
  DB_PORT: "5432"
  DB_USER: "postgres"
```

Deploy sidecar-controller together with postgres driver:

```bash
kubectl create -f config/resources/sidecar-controller
```

The next step is to create database and get secret wit the credentials. All necessary files you can find here: `config/samples`.
You have to install DatabaseClass and DatabaseAccessClass:

```bash
kubectl create -f config/samples/database_class.yaml
kubectl create -f config/samples/database_access_class.yaml
```

Now you can deploy database:
```bash
kubectl create -f config/samples/database_request.yaml
```

Check database status:

```bash
kubectl get databases
```

When it's ready you can request for the database credentials:

```bash
kubectl create -f config/samples/database_access_request.yaml
```

You should be able to get the secret:

```bash
kubectl get secret database-sample
```


