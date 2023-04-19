package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/kelseyhightower/envconfig"
	"github.com/nats-io/nats.go"
	"go.uber.org/automaxprocs/maxprocs"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

type loggerCtxKey string

const (
	loggerKey loggerCtxKey = "logger"
)

type config struct {
	NatsURL     string `envconfig:"NATS_SERVER" required:"true"`
	Topic       string `envconfig:"NATS_TOPIC" required:"true"`
	HealthZ     string `envconfig:"HEALTHZ_ADDRESS" default:":8080"`
	HealthZPath string `envconfig:"HEALTHZ_PATH" default:"/healthz"`
}

var ready atomic.Bool

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logger, err := zap.NewDevelopment()
	if err != nil {
		panic("could not create logger: " + err.Error())
	}
	logger = logger.Named("subscriber")
	ctx = context.WithValue(ctx, loggerKey, logger)

	_, err = maxprocs.Set()
	if err != nil {
		logger.Fatal("could not set maxprocs goroutine limit", zap.Error(err))
	}

	var cfg config
	err = envconfig.Process("", &cfg)
	if err != nil {
		logger.Fatal("could not create configuration", zap.Error(err))
	}

	if err = run(ctx, cfg); err != nil && !(errors.Is(err, context.Canceled) || errors.Is(err, http.ErrServerClosed)) {
		logger.Fatal("could not run subscriber", zap.Error(err))
	}
	logger.Info("shutdown complete")
}

func run(ctx context.Context, cfg config) error {
	logger := ctx.Value(loggerKey).(*zap.Logger)
	eg, egCtx := errgroup.WithContext(ctx)

	logger.Info(
		"starting healthz handler",
		zap.String("address", cfg.HealthZ),
		zap.String("path", cfg.HealthZPath),
	)
	eg.Go(func() error {
		return runHealthZ(egCtx, cfg.HealthZ, cfg.HealthZPath)
	})

	logger.Info("starting nats jetstream message producer", zap.String("natsURL", cfg.NatsURL))
	eg.Go(func() error {
		return runSubscriber(egCtx, cfg.NatsURL, cfg.Topic)
	})

	return eg.Wait()
}

func runSubscriber(ctx context.Context, natsURL, topic string) error {
	logger := ctx.Value(loggerKey).(*zap.Logger)

	nc, err := nats.Connect(natsURL)
	if err != nil {
		return fmt.Errorf("could not connect to nats: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		return fmt.Errorf("could not create nats jetstream context: %w", err)
	}
	handler := func(msg *nats.Msg) {
		md, err := msg.Metadata()
		if err != nil {
			ready.Store(false)
			logger.Error("unexpected nats message without metadata", zap.Error(err))
			return
		}

		logger.Info(
			"received nats message",
			zap.String("data", string(msg.Data)),
			zap.Any("sequence", md.Sequence),
		)
		ready.Store(true)
	}

	_, err = js.Subscribe(topic, handler)
	if err != nil {
		return fmt.Errorf("could not subscribe to nats stream: %w", err)
	}

	<-ctx.Done()
	logger.Info("shutting down subscriber", zap.Any("cause", ctx.Err()))
	return nil
}

func runHealthZ(ctx context.Context, address, path string) error {
	logger := ctx.Value(loggerKey).(*zap.Logger)

	router := httprouter.New()
	router.GET(path, func(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
		if !ready.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	})

	srv := http.Server{
		Addr:         address,
		Handler:      router,
		ReadTimeout:  time.Second * 5,
		WriteTimeout: time.Second * 5,
	}

	go func() {
		<-ctx.Done()
		logger.Info("shutting down http server", zap.Any("cause", ctx.Err()))

		timeoutCtx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		defer cancel()

		if err := srv.Shutdown(timeoutCtx); err != nil {
			logger.Error("could not shut down http server", zap.Error(err))
		}
	}()

	return srv.ListenAndServe()
}
