# Kubectl Real Name Diff

![CI](https://github.com/hhiroshell/kubectl-realname-diff/actions/workflows/ci.yaml/badge.svg)

A kubectl plugin that diffs live and local resources ignoring Kustomize
hash-suffixes.

## What's this ?
`kubectl realname-diff` works the same as `kubectl diff`, but if you set "real
name" as a label, local and live resources with the same label will be
compared.

This is especially beneficial if you use the Kustomize and enable hash
suffixing ConfigMap/Secret names. In case of `kubectl diff`, local and live
resources with hash suffixed name are considered as irrelevant. So you will not
be able to get any results comparing them.

With realname-diff, you can compare the resources with hash suffixed name by
specifying the comparison target with "real name" labels.

## Usage
First, you have set the label `realname-diff/realname: [real name]` to the
resources you want to diff ignoring difference of `{.metadata.name}`.

In Kustomize, you can set the label in ConfigMap/Secret Generator fields.

```yaml
resources:
  # resources that have references to the ConfigMap "nginx-conf"
  - deployment.yaml

configMapGenerator:
  - name: nginx-conf
    files:
      - conf/nginx.conf
    options:
      labels:
        realname-diff/realname: nginx-conf
      # hash suffix is enabled by default
```

Then, apply the manifests to your kubernetes cluster.

```bash
# assume you have placed the kustomization.yaml in "./example" directory
$ kustomize build -f ./example | kubectl apply -f -

# you can get the ConfigMap with hash suffixed name.
$ kubectl get configmap --show-labels
NAME                    DATA   AGE   LABELS
nginx-conf-m5d2cggb7k   1      18s   realname-diff/realname=nginx-conf
```

Next, edit the content you want to pack into ConfigMap and use `kubectl realname-diff`
to diff the local ConfigMap and the live one.

```bash
$ echo "# test" >> ./example/conf/nginx.conf

$ kustomize build -f ./example | kubectl realname-diff -f -
```

You can see the diff result comparing contents in the ConfigMap. The local
ConfigMap is not treated as new one.

```diff
diff -u -N /var/folders/2n/lgqgy6f151l5mw1x4dj_7ztw0000gn/T/LIVE-116798495/apps.v1.Deployment.default.nginx /var/folders/2n/lgqgy6f151l5mw1x4dj_7ztw0000gn/T/MERGED-2431241138/apps.v1.Deployment.default.nginx
--- /var/folders/2n/lgqgy6f151l5mw1x4dj_7ztw0000gn/T/LIVE-116798495/apps.v1.Deployment.default.nginx	2021-12-24 00:04:23.000000000 +0900
+++ /var/folders/2n/lgqgy6f151l5mw1x4dj_7ztw0000gn/T/MERGED-2431241138/apps.v1.Deployment.default.nginx	2021-12-24 00:04:23.000000000 +0900
@@ -6,7 +6,7 @@
     kubectl.kubernetes.io/last-applied-configuration: |
       {"apiVersion":"apps/v1","kind":"Deployment","metadata":{"annotations":{},"labels":{"app":"nginx","prunable":"true"},"name":"nginx","namespace":"default"},"spec":{"replicas":2,"selector":{"matchLabels":{"app":"nginx","prunable":"true"}},"template":{"metadata":{"labels":{"app":"nginx","prunable":"true"}},"spec":{"containers":[{"image":"nginx:latest","name":"nginx","ports":[{"containerPort":80}],"volumeMounts":[{"mountPath":"/etc/nginx/htpasswd","name":"htpasswd","readOnly":true},{"mountPath":"/etc/nginx","name":"nginx-conf","readOnly":true}]}],"volumes":[{"name":"htpasswd","secret":{"secretName":"htpasswd-k7mbh9mm68"}},{"configMap":{"name":"nginx-conf-m5d2cggb7k"},"name":"nginx-conf"}]}}}}
   creationTimestamp: "2021-12-23T15:00:12Z"
-  generation: 1
+  generation: 2
   labels:
     app: nginx
     prunable: "true"
@@ -177,7 +177,7 @@
           secretName: htpasswd-k7mbh9mm68
       - configMap:
           defaultMode: 420
-          name: nginx-conf-m5d2cggb7k
+          name: nginx-conf-b6gmtkgcd5
         name: nginx-conf
 status:
   availableReplicas: 2
diff -u -N /var/folders/2n/lgqgy6f151l5mw1x4dj_7ztw0000gn/T/LIVE-116798495/v1.ConfigMap.default.nginx-conf-m5d2cggb7k /var/folders/2n/lgqgy6f151l5mw1x4dj_7ztw0000gn/T/MERGED-2431241138/v1.ConfigMap.default.nginx-conf-m5d2cggb7k
--- /var/folders/2n/lgqgy6f151l5mw1x4dj_7ztw0000gn/T/LIVE-116798495/v1.ConfigMap.default.nginx-conf-m5d2cggb7k	2021-12-24 00:04:23.000000000 +0900
+++ /var/folders/2n/lgqgy6f151l5mw1x4dj_7ztw0000gn/T/MERGED-2431241138/v1.ConfigMap.default.nginx-conf-m5d2cggb7k	2021-12-24 00:04:23.000000000 +0900
@@ -16,12 +16,10 @@
             }
         }
     }
+    # test
 kind: ConfigMap
 metadata:
   ...(snip last-applied-configuration label)..
   labels:
     prunable: "true"
     realname-diff/realname: nginx-conf
@@ -33,17 +31,13 @@
     ...(snip managed fields)...
     manager: kubectl-client-side-apply
     operation: Update
-    time: "2021-12-23T15:00:12Z"
-  name: nginx-conf-m5d2cggb7k
+    time: "2021-12-23T15:04:23Z"
+  name: nginx-conf-b6gmtkgcd5
   namespace: default
-  resourceVersion: "82594"
-  uid: 6a5d347c-d936-49c6-825b-70e2e39eb676
+  uid: 0d69cf40-d201-47fe-bad2-8c3333ef0d07
```

For a complete example, see the [example directory](./example). 

## Installation

### by `go install`
Use go install as follows:

```bash
$ go install github.com/hhiroshell/kubectl-realname-diff/cmd/kubectl-realname_diff@latest
```

### as a kubectl plugin
Make sure the [krew](https://github.com/kubernetes-sigs/krew) is already installed.

Then install it via the krew as follows:

```bash
$ kubectl krew install realname-diff
```

## License
Kubectl Real Name Diff is licensed under the Apache License 2.0, and includes
works distributed under same one.
