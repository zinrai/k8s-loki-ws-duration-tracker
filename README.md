# k8s-loki-ws-duration-tracker

This Go program checks the log availability duration for Kubernetes Pods in a specified namespace prefix. It uses the Loki logging system to retrieve the logs and determines how long it takes for the logs to become available after the Pod starts.

**The program uses the tail endpoint provided by the Loki HTTP API as a WebSocket.**

https://grafana.com/docs/loki/latest/reference/api/#stream-logs

## Futures

This program needs to be run at the same time as [zinrai/k8s-pod-log-generator](https://github.com/zinrai/k8s-loki-logline-verifier), as it calculates the difference between the start time of the k8s Pod and the difference that could be logged from Loki.

If you know of a better way to do this, please let me know.

It would be nice if we could make it so that it can calculate the time it takes to get the logs from Loki even after the [zinrai/k8s-pod-log-generator](https://github.com/zinrai/k8s-loki-logline-verifier) has been executed.

## Motivation

I wanted to measure the time it took for the logs to become searchable at Loki.

## Tested Version

- `Loki`: 2.9.5
    - https://grafana.com/docs/loki/latest/setup/install/helm/install-scalable/
- `Promtail`: 2.9.3
    - https://grafana.com/docs/loki/latest/send-data/promtail/installation/#install-using-helm

## Requirements

- Access to a Grafana Loki instance with `auth_enabled: true`
    - https://grafana.com/docs/loki/latest/configure/#supported-contents-and-default-values-of-lokiyaml
- Access to Loki search endpoints deployed on k8s is required.
- k8s Namespace is set as the unit of tenant id in Loki.

Example of access to a Loki search endpoint using port forwarding:

```
$ kubectl get service -n loki
NAME                        TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)             AGE
loki-backend                ClusterIP   10.24.124.202   <none>        3100/TCP,9095/TCP   20d
loki-backend-headless       ClusterIP   None            <none>        3100/TCP,9095/TCP   20d
loki-gateway                ClusterIP   10.24.120.118   <none>        80/TCP              20d
loki-memberlist             ClusterIP   None            <none>        7946/TCP            20d
loki-read                   ClusterIP   10.24.123.180   <none>        3100/TCP,9095/TCP   20d
loki-read-headless          ClusterIP   None            <none>        3100/TCP,9095/TCP   20d
loki-write                  ClusterIP   10.24.120.24    <none>        3100/TCP,9095/TCP   20d
loki-write-headless         ClusterIP   None            <none>        3100/TCP,9095/TCP   20d
query-scheduler-discovery   ClusterIP   None            <none>        3100/TCP,9095/TCP   20d
$
```
```
$ kubectl port-forward svc/loki-gateway 8080:80 -n loki
Forwarding from 127.0.0.1:8080 -> 8080
Forwarding from [::1]:8080 -> 8080
Handling connection for 8080
Handling connection for 8080
```

Example values.yaml for Promtail using Helm when logging from Cloud Pub/Sub:

```
daemonset:
  enabled: false
deployment:
  enabled: true
serviceAccount:
  create: false
  name: ksa-cloudpubsub
configmap:
  enabled: true
config:
  clients:
    - url: http://loki-gateway.loki.svc.cluster.local/loki/api/v1/push
      tenant_id: default
  snippets:
    scrapeConfigs: |
      - job_name: gcplog
        pipeline_stages:
          - tenant:
              label: "namespace"
        gcplog:
          subscription_type: "pull"
          project_id: "project-id"
          subscription: "subscription-id"
          use_incoming_timestamp: false
          use_full_line: false
          labels:
            job: "gcplog"
        relabel_configs:
          - source_labels: ['__gcp_resource_labels_namespace_name']
            target_label: 'namespace'
          - source_labels: ['__gcp_resource_labels_pod_name']
            target_label: 'pod_name'
```

## Configuration

The program reads configurations from a YAML file named `config.yaml`. The following configuration options are available:

- `kubeconfig_path`: (Optional) Path to the Kubernetes cluster configuration file. If not provided, the default path will be used.
- `namespace_prefix`: (Optional) Prefix for the namespaces created by the tool. Defaults to logger-ns.
- `loki_address`: URL of the Loki server.
- `loki_websocket_address`: URL of the Loki server.
- `delay_for`: The number of seconds to delay retrieving logs to let slow loggers catch up. Defaults to 0 and cannot be larger than 5.
    - https://grafana.com/docs/loki/latest/reference/api/#stream-logs
- `poll_interval`: Interval in seconds at which the list of k8s Pods to be logged is retrieved.

## Usage

```bash
$ cat << EOF > config.yaml
loki_address: "http://localhost:8080"
loki_websocket_address: "ws://localhost:8080"
delay_for: 1
poll_interval: 5
EOF
```

```bash
$ go run main.go
2024/04/16 09:32:24 First log line for pod logger-pod-1 in namespace logger-ns-1: (Time difference: 89h4m9.609003064s)
2024/04/16 09:32:25 First log line for pod logger-pod-16 in namespace logger-ns-1: (Time difference: 89h3m7.572050856s)
2024/04/16 09:32:26 First log line for pod logger-pod-18 in namespace logger-ns-1: (Time difference: 89h3m4.457078398s)
2024/04/16 09:32:27 First log line for pod logger-pod-36 in namespace logger-ns-1: (Time difference: 89h0m6.137843315s)
2024/04/16 09:32:27 First log line for pod logger-pod-39 in namespace logger-ns-1: (Time difference: 88h59m29.53378119s)
2024/04/16 09:32:28 First log line for pod logger-pod-46 in namespace logger-ns-1: (Time difference: 88h58m2.288602982s)
2024/04/16 09:32:28 First log line for pod logger-pod-60 in namespace logger-ns-1: (Time difference: 88h54m23.646311357s)
2024/04/16 09:32:29 First log line for pod logger-pod-8 in namespace logger-ns-1: (Time difference: 89h3m59.110662816s)
...
```

## License

This project is licensed under the MIT License - see the [LICENSE](https://opensource.org/license/mit) for details.
