# About

[![Tests](https://github.com/embano1/k8s-meetup-04-05-2023/actions/workflows/e2e.yaml/badge.svg)](https://github.com/embano1/k8s-meetup-04-05-2023/actions/workflows/e2e.yaml)
[![go.mod Go version](https://img.shields.io/github/go-mod/go-version/embano1/k8s-meetup-04-05-2023)](https://github.com/embano1/k8s-meetup-04-05-2023)

Repo for the Kubernetes Leipzig User Group meeting 05-04-2023 showcasing Kubernetes SIG
[`e2e-framework`](https://github.com/kubernetes-sigs/e2e-framework).

The `e2e-framework` in this repo is used to run a suite of end-to-end tests:

- Showcase how to use the framework for business applications 
    - Deploy a [NATS](https://nats.io/) JetStream server
    - Deploy a message publisher (Golang app)
    - Deploy a message consumer (Golang app)
    - Assert the producer sends messages (*)
    - Assert the consumer receives messages (*)
    - Assert successful cleanup of test resources
- Showcase how to use the framework with Kubernetes controllers/operators
  - Deploy the AWS [ACK Controller for EventBridge](https://aws.amazon.com/about-aws/whats-new/2023/03/ack-controllers-amazon-eventbridge-pipes/)
  - Create an `EventBus` resource
  - Assert the resource is synchronized and successfully created in the AWS service control plane (backend)
  - Assert successful cleanup of test resources

(*) The producer and consumer expose an HTTP health check which is flipped when a message was successfully
sent/received. The pods will only enter the ready state if the health check probe succeeds, i.e., the deployment is
considered `available` which is reflected in the resource's status and can be asserted on with a `Wait()` function in
the framework. This is a very naive way of doing this, but shows how to use Kubernetes conditions for asynchronous
assertations (works very nicely with Kubernetes `Jobs`).

## Tools and Versions used

- [ko](https://github.com/ko-build/ko) (0.13.0)
- [helm](https://helm.sh/docs/helm/helm_install/) (v3.11.2)
- [kind](https://kind.sigs.k8s.io/) (v0.18.0 - uses Kubernetes 1.26)
- [go](https://go.dev/) (go1.20.3)
- [Docker](https://docker.com/) (20.10.23)

# Run the E2E Tests

## NATS

This E2E test asserts that [NATS](https://nats.io/) is running in Kubernetes (deployed via `helm` as part of the test
suite), a `publisher` Kubernetes deployment (Go) can send messages to a NATS JetStream topic, and a `subscriber`
Kubernetes deployment (Go) successfully consumes messages from the stream.

```console
# create kind cluster
kind create cluster --name e2e-meetup
export KIND_CLUSTER_NAME=e2e-meetup 
export KO_DOCKER_REPO=kind.local

# build and upload images to kind
# replace platform with your environment
export PUBLISHER_IMAGE=$(ko build -B --platform=linux/arm64 ./publisher) \
export SUBSCRIBER_IMAGE=$(ko build -B --platform=linux/arm64 ./subscriber)

# run nats tests
go test -race -count=1 -v ./e2e -args -v 4 -labels=feature=e2e-nats
```

Your output should be similar to

```console
I0419 13:32:41.075876   72332 main_test.go:39] setting up test environment with kind cluster "e2e-meetup"
I0419 13:32:41.076098   72332 kind.go:93] Creating kind cluster e2e-meetup
I0419 13:32:41.867656   72332 kind.go:99] Skipping Kind Cluster.Create: cluster already created: e2e-meetup
=== RUN   TestSimpleE2E
I0419 13:32:52.556332   72332 helper_test.go:31] creating namespace "e2e-feature-f53" for feature "e2e demo with nats"
=== RUN   TestSimpleE2E/e2e_demo_with_nats
I0419 13:32:52.567830   72332 suite_test.go:48] adding nats repo "nats" using chart "https://nats-io.github.io/k8s/helm/charts/"

<snip>

=== RUN   TestSimpleE2E/e2e_demo_with_nats/nats_server_running
I0419 13:32:53.772049   72332 suite_test.go:80] waiting for nats "nats-server" in namespace "e2e-feature-f53" to become ready
=== RUN   TestSimpleE2E/e2e_demo_with_nats/publisher_running
I0419 13:33:08.791521   72332 suite_test.go:99] creating deployment "publisher"
I0419 13:33:08.812503   72332 suite_test.go:106] waiting for deployment "publisher" in namespace "e2e-feature-f53" to become ready
=== RUN   TestSimpleE2E/e2e_demo_with_nats/subscriber_received_message
I0419 13:33:13.826676   72332 suite_test.go:121] creating deployment "subscriber"
I0419 13:33:13.849208   72332 suite_test.go:128] waiting for deployment "subscriber" in namespace "e2e-feature-f53" to become ready
I0419 13:33:18.881446   72332 helper_test.go:42] deleting namespace "e2e-feature-f53" for feature "e2e demo with nats"
--- PASS: TestSimpleE2E (26.33s)
    --- PASS: TestSimpleE2E/e2e_demo_with_nats (26.31s)
        --- PASS: TestSimpleE2E/e2e_demo_with_nats/nats_server_running (15.02s)
        --- PASS: TestSimpleE2E/e2e_demo_with_nats/publisher_running (5.03s)
        --- PASS: TestSimpleE2E/e2e_demo_with_nats/subscriber_received_message (5.05s)
PASS
ok      k8s-meetup-04-05-2023/e2e       38.501s
```

## AWS EventBridge

This E2E test asserts that the AWS EventBridge controller Helm [chart](https://gallery.ecr.aws/aws-controllers-k8s/eventbridge-chart) is running in Kubernetes (deployed via `helm` as part of the test
suite), an `EventBus` custom resource is created, synchronized (status), and created in the AWS service control plane (backend).

```console
# create kind cluster unless it already exists
kind create cluster --name e2e-meetup
export KIND_CLUSTER_NAME=e2e-meetup 
export KO_DOCKER_REPO=kind.local

# define AWS environment variables with IAM permissions to manage event buses and authenticate with ECR
AWS_DEFAULT_REGION=eu-central-1
AWS_ACCESS_KEY_ID=<ID>
AWS_SECRET_ACCESS_KEY=<KEY>
AWS_SESSION_TOKEN=<TOKEN>

# run eventbridge tests
go test -race -count=1 -v ./e2e -args -v 4 -labels=feature=e2e-eventbridge
```

Your output should be similar to

```console
I0426 10:18:31.452472   37678 main_test.go:39] setting up test environment with kind cluster "e2e-meetup"
I0426 10:18:31.452674   37678 kind.go:93] Creating kind cluster e2e-meetup
I0426 10:18:31.686098   37678 kind.go:99] Skipping Kind Cluster.Create: cluster already created: e2e-meetup
=== RUN   TestDemoMultipleE2E
I0426 10:18:42.356441   37678 helper_test.go:39] creating namespace "e2e-feature-01a" for feature "e2e demo with nats"
=== RUN   TestDemoMultipleE2E/e2e_demo_with_nats
    env.go:432: Skipping feature "e2e demo with nats": unmatched labels "[feature=[e2e-nats]]"
I0426 10:18:42.369002   37678 helper_test.go:50] deleting namespace "e2e-feature-01a" for feature "e2e demo with nats"
I0426 10:18:42.377324   37678 helper_test.go:39] creating namespace "e2e-feature-46e" for feature "e2e demo with eventbridge"
=== RUN   TestDemoMultipleE2E/e2e_demo_with_eventbridge
I0426 10:18:42.382857   37678 eventbridge_test.go:91] creating aws credentials secret "eventbridge-credentials" in namespace "e2e-feature-46e"
I0426 10:18:42.388304   37678 eventbridge_test.go:110] retrieving ecr authorization token
I0426 10:18:43.127327   37678 eventbridge_test.go:120] logging in to ecr registry "public.ecr.aws"
I0426 10:18:49.928574   37678 eventbridge_test.go:126] installing eventbridge controller "ack-eventbridge-controller" in namespace "e2e-feature-46e" with version "v1.0.0"
I0426 10:18:49.928878   37678 helm.go:241] "Running Helm Operation" command="helm install ack-eventbridge-controller oci://public.ecr.aws/aws-controllers-k8s/eventbridge-chart --namespace e2e-feature-46e --version v1.0.0 --create-namespace -f ./testdata/eventbridge.config --set aws.region=eu-central-1 --wait --kubeconfig /var/folders/62/qd6v4j797gjg9ygkwtdvt97c0000gr/T/kind-cluser-e2e-meetup-kubecfg3819137103"

<snip>

=== RUN   TestDemoMultipleE2E/e2e_demo_with_eventbridge/event_bus_created
I0426 10:19:14.903570   37678 eventbridge_test.go:162] creating event bus "e2e-feature-4a0" in namespace "e2e-feature-46e"
I0426 10:19:14.910773   37678 eventbridge_test.go:167] waiting for event bus "e2e-feature-4a0" in namespace "e2e-feature-46e" to become ready
I0426 10:20:09.920583   37678 eventbridge_test.go:182] asserting event bus "e2e-feature-4a0" in namespace "e2e-feature-46e" exists in aws service control plane
=== RUN   TestDemoMultipleE2E/e2e_demo_with_eventbridge/event_bus_deleted
I0426 10:20:10.059323   37678 eventbridge_test.go:204] deleting event bus "e2e-feature-4a0" in namespace "e2e-feature-46e"
I0426 10:20:10.071365   37678 eventbridge_test.go:210] asserting event bus "e2e-feature-4a0" in namespace "e2e-feature-46e" is deleted in aws service control plane
I0426 10:20:15.096922   37678 eventbridge_test.go:246] uninstalling eventbridge controller "ack-eventbridge-controller" in namespace "e2e-feature-46e"
I0426 10:20:15.097669   37678 helm.go:241] "Running Helm Operation" command="helm uninstall ack-eventbridge-controller  --namespace e2e-feature-46e --kubeconfig /var/folders/62/qd6v4j797gjg9ygkwtdvt97c0000gr/T/kind-cluser-e2e-meetup-kubecfg3819137103"
I0426 10:20:15.365897   37678 helm.go:244] Helm Command output
release "ack-eventbridge-controller" uninstalled
I0426 10:20:15.366155   37678 helper_test.go:50] deleting namespace "e2e-feature-46e" for feature "e2e demo with eventbridge"
--- PASS: TestDemoMultipleE2E (93.03s)
    --- SKIP: TestDemoMultipleE2E/e2e_demo_with_nats (0.00s)
    --- PASS: TestDemoMultipleE2E/e2e_demo_with_eventbridge (93.00s)
        --- PASS: TestDemoMultipleE2E/e2e_demo_with_eventbridge/event_bus_created (55.16s)
        --- PASS: TestDemoMultipleE2E/e2e_demo_with_eventbridge/event_bus_deleted (5.05s)
PASS
ok      k8s-meetup-04-05-2023/e2e       104.869s
```