package e2e

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

type (
	namespaceCtxKey string
)

const (
	namespaceKey = namespaceCtxKey("featureNamespace")
)

var commonLabels = map[string]string{"app": "e2e"}

// createNSForFeature creates a random namespace with the runID as a prefix. It is stored in the context
// so that the deleteNSForFeature routine can look it up and delete it.
func createNSForFeature(ctx context.Context, cfg *envconf.Config, feature string) (context.Context, error) {
	ns := envconf.RandomName("e2e-feature", 15)
	ctx = context.WithValue(ctx, namespaceKey, ns)

	klog.Infof("creating namespace %q for feature %q", ns, feature)
	nsObj := corev1.Namespace{}
	nsObj.Name = ns

	return ctx, cfg.Client().Resources().Create(ctx, &nsObj)
}

// deleteNSForFeature looks up the namespace corresponding to the given test and deletes it.
func deleteNSForFeature(ctx context.Context, cfg *envconf.Config, t *testing.T, feature string) (context.Context, error) {
	ns := getTestNamespaceFromContext(ctx, t)

	klog.Infof("deleting namespace %q for feature %q", ns, feature)

	nsObj := corev1.Namespace{}
	nsObj.Name = ns

	return ctx, cfg.Client().Resources().Delete(ctx, &nsObj)
}

func getTestNamespaceFromContext(ctx context.Context, t *testing.T) string {
	ns, ok := ctx.Value(namespaceKey).(string)
	assert.Equal(t, ok, true, "retrieve namespace from context: value not found for key %q", namespaceKey)
	return ns
}

func newDeployment(namespace string, name string, replicas int32, image string) v1.Deployment {
	labels := copyMap(commonLabels)
	labels["app"] = "name"

	env := []corev1.EnvVar{
		{
			Name:  "NATS_SERVER",
			Value: fmt.Sprintf("%s.%s.svc.cluster.local", natsDeployment, namespace),
		},
		{
			Name:  "NATS_TOPIC",
			Value: natsStream,
		},
		{
			Name:  "HEALTHZ_ADDRESS",
			Value: ":8080",
		},
		{
			Name:  "HEALTHZ_PATH",
			Value: "/healthz",
		},
	}

	health := corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/healthz",
				Port: intstr.IntOrString{
					IntVal: 8080,
				},
			},
		},
		InitialDelaySeconds: 1,
		TimeoutSeconds:      3,
		PeriodSeconds:       1,
		SuccessThreshold:    1,
		FailureThreshold:    1,
	}

	return v1.Deployment{
		ObjectMeta: v12.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: v1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &v12.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: v12.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "publisher",
							Image:           image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Env:             env,
							ReadinessProbe:  &health,
						},
					},
				},
			},
		},
	}
}

func copyMap(in map[string]string) map[string]string {
	out := make(map[string]string)

	for k, v := range in {
		out[k] = v
	}
	return out
}
