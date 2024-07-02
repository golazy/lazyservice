// Package lazyapp provides a framework for building lazy applications in Go.
// A lazy application is an application that starts and stops services on demand.
// It allows you to define services as functions and run them within the application.
// The lazyapp package provides an interface for defining services, adding values and types to the application, and running the application and its services.
// It also provides a default logger implementation and supports colored debug messages and JSON logs.
// The application uses trace regions for the app and each of the services.
package lazyapp

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"runtime/trace"

	"github.com/lmittmann/tint"
	"golang.org/x/sync/errgroup"
	"golang.org/x/term"
)

func ServiceFunc(name string, f func(context.Context, *slog.Logger) error) Service {
	return &serviceFunc{
		name: name,
		f:    f,
	}
}

type serviceFunc struct {
	name string
	f    func(context.Context, *slog.Logger) error
}

func (f *serviceFunc) Run(ctx context.Context, l *slog.Logger) error {
	if l == nil {
		l = slog.Default()
	}

	return f.f(ctx, l)
}

func (f *serviceFunc) Desc() ServiceDescription {
	return serviceFuncDesc{name: f.name}

}

type serviceFuncDesc struct {
	name string
}

func (d serviceFuncDesc) Name() string {
	return d.name
}

type ServiceDescription interface {
	Name() string
}

type Service interface {
	Desc() ServiceDescription
	Run(context.Context, *slog.Logger) error
}

type LazyApp interface {
	context.Context

	Name() string
	Version() string

	AddService(Service)
	AddValue(key any, value any)

	Run() error
}

type lazyApp struct {
	name     string
	version  string
	ctx      context.Context
	logger   *slog.Logger
	services []Service
	done     chan struct{}
	cancel   context.CancelFunc
}

func (a *lazyApp) Name() string {
	return a.name
}

func (a *lazyApp) Version() string {
	return a.version
}

func (a *lazyApp) AddService(s Service) {
	a.services = append(a.services, s)
}

func (a *lazyApp) AddValue(key any, value any) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	a.ctx = context.WithValue(ctx, key, value)
}

func AppGet[T any](app context.Context) T {
	t, ok := app.Value(reflect.TypeOf(*new(T))).(T)
	if ok {
		return t
	}
	return *new(T)
}
func AppSet[T any](app LazyApp, value T) {
	app.AddValue(reflect.TypeOf(*new(T)), value)
}

// Run runs the app and all its services.
// If the app does not have a context, it creates a new one. This new context will be canceled when the app receives an interrupt signal.
// The app sets up a default slog Logger unless one is set through AppSet.
// The default logger outputs colored debug messages if the output is a terminal, otherwise it outputs JSON logs.
// The logger will have the app name and version as attributes for the json output.
// Each service will also have the service attribute set to the service name.
// If any service returns, all the contexts will be canceled and the app will wait for all services to stop.
// The application uses trace regions for the app and for each of the services.
func (a *lazyApp) Run() error {
	if a.ctx == nil {
		a.ctx, a.cancel = signal.NotifyContext(context.Background(), os.Interrupt)
		defer a.cancel()
	}

	// Logger
	a.logger = AppGet[*slog.Logger](a)
	if a.logger == nil {
		if term.IsTerminal(int(os.Stdout.Fd())) {
			a.logger = slog.New(tint.NewHandler(os.Stdout,
				&tint.Options{
					AddSource: true,
					Level:     slog.LevelDebug,
					ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
						switch string(attr.Key) {
						case "app", "version":
							return attr
						}
						return attr
					},
				}))
		} else {
			a.logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
				AddSource: true,
				Level:     slog.LevelWarn,
			}))

		}
	}
	a.logger = a.logger.With("app", a.name, "version", a.version)

	AppSet(a, a.logger)

	appRegion := trace.StartRegion(a, "lazyapp.Run")
	defer appRegion.End()

	a.logger.Info("starting app")

	grp, grpCtx := errgroup.WithContext(a)

	for _, s := range a.services {
		s := s // create a local variable and assign the value of s to it
		grp.Go(func() error {
			l := a.logger.With("service", s.Desc().Name())
			ctx := context.WithValue(grpCtx, reflect.TypeOf(a.logger), l)

			srvReg := trace.StartRegion(ctx, "service:"+s.Desc().Name())
			defer srvReg.End()

			l.InfoContext(ctx, "starting service")
			err := s.Run(ctx, l)
			if errors.Is(err, context.Canceled) ||
				errors.Is(err, context.DeadlineExceeded) {
				l.InfoContext(ctx, "stopped")
				return nil
			}
			if err != nil {
				l.ErrorContext(ctx, err.Error())
			}
			l.InfoContext(ctx, "app gracefully stoped")
			return err
		})
	}
	return grp.Wait()
}

// New creates a new app setting the name and version of the app.
// If the name is empty, it tries to use the base name of the executable.
// If the version is empty, it tries to use the modification time of the executable.
func New(name, version string) LazyApp {
	return (&lazyApp{
		name:    name,
		version: version,
	}).init()
}

// NewWithContext creates a new app setting the name and version of the app and the context.
func NewWithContext(ctx context.Context, name, version string) LazyApp {
	return (&lazyApp{
		ctx:     ctx,
		name:    name,
		version: version,
	}).init()
}

func (a *lazyApp) init() *lazyApp {
	if a.done == nil {
		a.done = make(chan struct{})
	}

	if a.name == "" {
		a.name = filepath.Base(os.Args[0])
	}
	if a.version == "" {
		a.version = getVersion()
	}
	return a

}

func getVersion() string {
	s, err := os.Stat(os.Args[0])
	if err != nil {
		return ""
	}
	return s.ModTime().Format("20060102-150405")

}
