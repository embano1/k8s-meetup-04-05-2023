package e2e

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	ebv1alpha "github.com/aws-controllers-k8s/eventbridge-controller/apis/v1alpha1"
	ackcore "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	"github.com/aws/aws-sdk-go/aws"
	ecrsvcsdk "github.com/aws/aws-sdk-go/service/ecrpublic"
	ebsvcsdk "github.com/aws/aws-sdk-go/service/eventbridge"
	"github.com/kelseyhightower/envconfig"
	"github.com/vladimirvivien/gexe"
	"gotest.tools/v3/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
	"sigs.k8s.io/e2e-framework/third_party/helm"
)

const (
	// helm
	eventbridgeRegistry     = "public.ecr.aws"
	eventbridgeChart        = "oci://public.ecr.aws/aws-controllers-k8s/eventbridge-chart"
	eventbridgeChartVersion = "v1.0.0"
	eventbridgeDeployment   = "ack-eventbridge-controller"
	controllerNamespace     = "ack-system"
	eventbridgeConfig       = "./testdata/eventbridge.config"

	// aws credentials
	secretName = "eventbridge-credentials"
	secretKey  = "credentials"

	// 	test variables
	testbusCtxKey = "testbus"
)

var awscfg awsConfig

type awsConfig struct {
	Region       string `envconfig:"AWS_DEFAULT_REGION" required:"true"`
	AccessKey    string `envconfig:"AWS_ACCESS_KEY_ID" required:"true"`
	SecretKey    string `envconfig:"AWS_SECRET_ACCESS_KEY" required:"true"`
	SessionToken string `envconfig:"AWS_SESSION_TOKEN" required:"true"`
}

func setupEventBridge() features.Func {
	steps := []features.Func{
		createCredentials(),
		setupController(),
	}

	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		for _, step := range steps {
			ctx = step(ctx, t, cfg)
		}

		return ctx
	}
}

func createCredentials() features.Func {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		err := envconfig.Process("", &awscfg)
		assert.NilError(t, err)

		ns := getTestNamespaceFromContext(ctx, t)

		secret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: ns,
			},
			Immutable: pointer.Bool(true),
			Data: map[string][]byte{
				secretKey: awsCredentials(awscfg.AccessKey, awscfg.SecretKey, awscfg.SessionToken),
			},
		}

		klog.Infof("creating aws credentials secret %q in namespace %q", secretName, ns)
		err = cfg.Client().Resources().Create(ctx, &secret)
		assert.NilError(t, err)

		return ctx
	}
}

func awsCredentials(id, key, token string) []byte {
	return []byte(fmt.Sprintf(`
[default]
aws_access_key_id = %s
aws_secret_access_key = %s
aws_session_token = %s`, id, key, token))
}

func setupController() features.Func {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		ecr := ecrSDKClient(t)
		klog.Infof("retrieving ecr authorization token")
		resp, err := ecr.GetAuthorizationTokenWithContext(ctx, &ecrsvcsdk.GetAuthorizationTokenInput{})
		assert.NilError(t, err)

		b, err := base64.StdEncoding.DecodeString(*resp.AuthorizationData.AuthorizationToken)
		assert.NilError(t, err, "decode ecr authorization token")
		// e.g. AWS:eyJwYXlsb2...
		token := strings.SplitN(string(b), ":", 2)
		assert.Equal(t, len(token), 2, "ecr authorization token validation")

		klog.Infof("logging in to ecr registry %q", eventbridgeRegistry)
		result := gexe.Pipe(fmt.Sprintf("echo -n %s", token[1]), "docker login --username AWS --password-stdin public.ecr.aws")
		assert.NilError(t, result.LastProc().Err(), "docker public.ecr.aws login")

		hm := helm.New(cfg.KubeconfigFile())
		ns := getTestNamespaceFromContext(ctx, t)
		klog.Infof("installing eventbridge controller %q in namespace %q with version %q", eventbridgeDeployment, ns, eventbridgeChartVersion)
		opts := []helm.Option{
			helm.WithName(eventbridgeDeployment),
			helm.WithNamespace(ns),
			helm.WithChart(eventbridgeChart),
			helm.WithVersion(eventbridgeChartVersion),
			helm.WithArgs(
				"--create-namespace",
				"-f", eventbridgeConfig,
				"--set", fmt.Sprintf("aws.region=%s", awscfg.Region),
				"--wait",
			),
		}

		err = hm.RunInstall(opts...)
		assert.NilError(t, err)

		return ctx
	}
}

func eventbusCreated() features.Func {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		r, err := resources.New(cfg.Client().RESTConfig())
		assert.NilError(t, err)

		err = ebv1alpha.AddToScheme(r.GetScheme())
		assert.NilError(t, err)

		ns := getTestNamespaceFromContext(ctx, t)

		busname := envconf.RandomName("e2e-feature", 15)
		ctx = context.WithValue(ctx, testbusCtxKey, busname)

		bus := eventBusFor(busname, ns)

		klog.Infof("creating event bus %q in namespace %q", busname, ns)
		err = r.Create(ctx, &bus)
		assert.NilError(t, err)

		// check if ack resource is synchronized
		klog.Infof("waiting for event bus %q in namespace %q to become ready", busname, ns)
		syncedCondition := conditions.New(r).ResourceMatch(&bus, func(bus k8s.Object) bool {
			for _, cond := range bus.(*ebv1alpha.EventBus).Status.Conditions {
				if cond.Type == ackcore.ConditionTypeResourceSynced && cond.Status == corev1.ConditionTrue {
					return true
				}
			}
			return false
		})

		err = wait.For(syncedCondition, wait.WithTimeout(3*time.Minute)) // controller takes a while on initial start even after reporting available
		assert.NilError(t, err)

		// check if it exists in aws service control plane
		eb := ebSDKClient(t)
		klog.Infof("asserting event bus %q in namespace %q exists in aws service control plane", busname, ns)
		input := ebsvcsdk.DescribeEventBusInput{Name: aws.String(busname)}
		resp, err := eb.DescribeEventBusWithContext(ctx, &input)
		assert.NilError(t, err)
		assert.Equal(t, *resp.Name, busname)

		return ctx
	}
}

func eventbusDeleted() features.Func {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		r, err := resources.New(cfg.Client().RESTConfig())
		assert.NilError(t, err)

		err = ebv1alpha.AddToScheme(r.GetScheme())
		assert.NilError(t, err)

		ns := getTestNamespaceFromContext(ctx, t)

		busname := ctx.Value(testbusCtxKey).(string)
		bus := eventBusFor(busname, ns)
		klog.Infof("deleting event bus %q in namespace %q", busname, ns)
		err = r.Delete(ctx, &bus)
		assert.NilError(t, err)

		// check if it is deleted in aws service control plane
		eb := ebSDKClient(t)
		klog.Infof("asserting event bus %q in namespace %q is deleted in aws service control plane", busname, ns)
		busDeleted := func(ctx context.Context) (bool, error) {
			resp, err := eb.ListEventBusesWithContext(ctx, &ebsvcsdk.ListEventBusesInput{
				NamePrefix: aws.String(busname), // ignore "default" bus
			})
			if err != nil {
				return false, fmt.Errorf("list event buses: %w", err)
			}

			return len(resp.EventBuses) == 0, nil
		}
		err = wait.For(busDeleted, wait.WithTimeout(time.Minute))
		assert.NilError(t, err)

		return ctx
	}
}

func teardownEventBridge() features.Func {
	steps := []features.Func{
		uninstallController(),
	}

	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		for _, step := range steps {
			ctx = step(ctx, t, cfg)
		}

		return ctx
	}
}

func uninstallController() features.Func {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		hm := helm.New(cfg.KubeconfigFile())
		ns := getTestNamespaceFromContext(ctx, t)
		klog.Infof("uninstalling eventbridge controller %q in namespace %q", eventbridgeDeployment, ns)
		opts := []helm.Option{
			helm.WithName(eventbridgeDeployment),
			helm.WithNamespace(ns),
		}
		err := hm.RunUninstall(opts...)
		assert.NilError(t, err)

		return ctx
	}
}
