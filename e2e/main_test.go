package e2e

import (
	"context"
	"os"
	"testing"

	"github.com/kelseyhightower/envconfig"
	"k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

type config struct {
	Publisher   string `envconfig:"PUBLISHER_IMAGE" required:"true"`
	Subscriber  string `envconfig:"SUBSCRIBER_IMAGE" required:"true"`
	KindCluster string `envconfig:"KIND_CLUSTER_NAME" required:"true"`
	DockerRepo  string `envconfig:"KO_DOCKER_REPO" default:"kind.local"`
}

var (
	testEnv env.Environment
	envCfg  config
)

func TestMain(m *testing.M) {
	flags, err := envconf.NewFromFlags()
	if err != nil {
		klog.Fatalf("could not parse flags: %v", err)
	}
	testEnv = env.NewWithConfig(flags)

	if err := envconfig.Process("", &envCfg); err != nil {
		klog.Fatalf("could not parse environment variables: %v", err)
	}

	klog.Infof("setting up test environment with kind cluster %q", envCfg.KindCluster)
	testEnv.Setup(
		envfuncs.CreateKindCluster(envCfg.KindCluster),
	)

	testEnv.Finish(
	// envfuncs.DestroyKindCluster(envCfg.KindCluster),
	)

	// create/delete namespace per feature
	testEnv.BeforeEachFeature(func(ctx context.Context, cfg *envconf.Config, _ *testing.T, f features.Feature) (context.Context, error) {
		return createNSForFeature(ctx, cfg, f.Name())
	})
	testEnv.AfterEachFeature(func(ctx context.Context, cfg *envconf.Config, t *testing.T, f features.Feature) (context.Context, error) {
		return deleteNSForFeature(ctx, cfg, t, f.Name())
	})

	os.Exit(testEnv.Run(m))
}
