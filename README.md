# About

Repo for the Kubernetes Leipzig User Group meeting 05-04-2023 showcasing Kubernetes SIG
[`e2e-framework`](https://github.com/kubernetes-sigs/e2e-framework).

## Tools and Versions used

- [ko](https://github.com/ko-build/ko) (0.13.0)
- [helm](https://helm.sh/docs/helm/helm_install/) (v3.11.2)
- [kind](https://kind.sigs.k8s.io/) (v0.18.0)
- [go](https://go.dev/) (go1.20.3)

# Run the E2E Tests

The E2E tests assert that [NATS](https://nats.io/) is running in Kubernetes (deployed via `helm` as part of the test
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

# run tests
go test -race -count=1 -v ./e2e -args -v 4
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