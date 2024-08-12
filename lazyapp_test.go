package lazyservice

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"testing"
	"time"
)

func TestAppNameAndVersion(t *testing.T) {

	app := New("", "")
	if app.Name() != "lazyapp.test" {
		t.Error("app name is:", app.Name())
	}
	if !regexp.MustCompile(`\d{8}-\d{6}`).MatchString(app.Version()) {
		t.Error("app version is:", app.Version())
	}
}

func TestServiceFunc(t *testing.T) {

	service := func(ctx context.Context, l *slog.Logger) error {
		l.Info("hi")
		return fmt.Errorf("hi")
	}
	srv := serviceFunc("basic", service)

	if srv.Desc().Name() != "basic" {
		t.Error(srv.Desc().Name())
	}

	err := srv.Run(context.Background())
	if err.Error() != "hi" {
		t.Error("error didn't said hi")
	}
}

func TestAddType(t *testing.T) {

	app := New("test", "1.0.0")

	type testStruct struct {
		Name string
	}

	s := &testStruct{Name: "test"}
	AppSet(app, s)

	s2 := AppGet[*testStruct](app)
	if s2 == nil {
		t.Fatal("s2 is nil")
	}
	if s2.Name != "test" {
		t.Error("Name is not test")
	}
}

type testStruct string

func TestValues(t *testing.T) {

	app := New("test", "1.0.0")
	AppSet(app, testStruct("test"))

	ts := AppGet[testStruct](app)

	if ts != "test" {
		t.Error("ts is not test")
	}

}

func TestLazyApp(t *testing.T) {

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	app := NewWithContext(
		ctx,
		"test", "1.0.0")
	app.AddValue("key", "value")
	app.AddService(serviceFunc("http", func(ctx context.Context, l *slog.Logger) error {

		s := &http.Server{
			Addr: ":8083",
		}

		idleConnsClosed := make(chan struct{})
		go func() {
			defer close(idleConnsClosed)
			<-ctx.Done()
			l.InfoContext(app, "shutting down")
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()
			err := s.Shutdown(ctx)
			if err == nil || err == context.Canceled || err == context.DeadlineExceeded {
				return
			}
			l.ErrorContext(app, err.Error(), "err", err)
		}()

		l.InfoContext(app, "listening on 8083")
		err := s.ListenAndServe()
		if err != http.ErrServerClosed {
			return err
		}
		<-idleConnsClosed
		return nil

	}))

	err := app.Run()
	if err != nil {
		t.Fatal(err)
	}

}
