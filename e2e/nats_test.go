package e2e

import (
	"context"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"k8s.io/api/apps/v1"
	v12 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
	"sigs.k8s.io/e2e-framework/third_party/helm"
)

const (
	natsRepo         = "nats"
	natsChart        = "https://nats-io.github.io/k8s/helm/charts/"
	natsChartVersion = "0.19.12"
	natsRelease      = "nats/nats"
	natsConfig       = "./testdata/nats.config"
	natsDeployment   = "nats-server"
	natsStream       = "e2e-topic"
)

func setupNats() features.Func {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		hm := helm.New(cfg.KubeconfigFile())

		klog.Infof("adding nats repo %q using chart %q", natsRepo, natsChart)
		opts := []helm.Option{helm.WithArgs("add", natsRepo, natsChart)}
		err := hm.RunRepo(opts...)
		assert.NilError(t, err)

		err = hm.RunRepo(helm.WithArgs("update"))
		assert.NilError(t, err)

		ns := getTestNamespaceFromContext(ctx, t)
		klog.Infof("installing nats %q in namespace %q with version %q", natsDeployment, ns, natsChartVersion)
		opts = []helm.Option{
			helm.WithName(natsDeployment),
			helm.WithNamespace(ns),
			helm.WithVersion(natsChartVersion),
			helm.WithReleaseName(natsRelease),
			helm.WithArgs("-f", natsConfig),
		}
		err = hm.RunInstall(opts...)
		assert.NilError(t, err)

		return ctx
	}
}

func natsRunning() features.Func {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		ns := getTestNamespaceFromContext(ctx, t)

		var server v1.StatefulSet
		err := cfg.Client().Resources().Get(ctx, natsDeployment, ns, &server)
		assert.NilError(t, err)

		klog.Infof("waiting for nats %q in namespace %q to become ready", natsDeployment, ns)
		serverReady := conditions.New(cfg.Client().Resources()).ResourceMatch(&server, func(object k8s.Object) bool {
			s := object.(*v1.StatefulSet)
			return s.Status.ReadyReplicas == 1
		})

		err = wait.For(serverReady, wait.WithTimeout(time.Minute*5))
		assert.NilError(t, err)

		return ctx
	}
}

func publisherRunning() features.Func {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		ns := getTestNamespaceFromContext(ctx, t)

		name := "publisher"
		publisher := newDeployment(ns, name, 1, envCfg.Publisher)
		klog.Infof("creating deployment %q", name)
		err := cfg.Client().Resources().Create(ctx, &publisher)
		assert.NilError(t, err)

		err = cfg.Client().Resources().Get(ctx, name, ns, &publisher)
		assert.NilError(t, err)

		klog.Infof("waiting for deployment %q in namespace %q to become ready", name, ns)
		ready := conditions.New(cfg.Client().Resources()).DeploymentConditionMatch(&publisher, v1.DeploymentAvailable, v12.ConditionTrue)
		err = wait.For(ready, wait.WithTimeout(time.Minute))
		assert.NilError(t, err)

		return ctx
	}
}

func subscriberRunning() features.Func {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		ns := getTestNamespaceFromContext(ctx, t)

		name := "subscriber"
		subscriber := newDeployment(ns, name, 1, envCfg.Subscriber)
		klog.Infof("creating deployment %q", name)
		err := cfg.Client().Resources().Create(ctx, &subscriber)
		assert.NilError(t, err)

		err = cfg.Client().Resources().Get(ctx, name, ns, &subscriber)
		assert.NilError(t, err)

		klog.Infof("waiting for deployment %q in namespace %q to become ready", name, ns)
		ready := conditions.New(cfg.Client().Resources()).DeploymentConditionMatch(&subscriber, v1.DeploymentAvailable, v12.ConditionTrue)
		err = wait.For(ready, wait.WithTimeout(time.Minute))
		assert.NilError(t, err)

		return ctx
	}
}
