package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"testing"

	w "git.sr.ht/~mariusor/wrapper"
	"github.com/cucumber/godog"
	"github.com/cucumber/godog/colors"
	"github.com/go-ap/errors"
	"github.com/mariusor/go-littr/app"
	"github.com/mariusor/go-littr/internal/config"
	"github.com/mariusor/go-littr/internal/log"
	"github.com/tebeka/selenium"
)

const (
	// These paths will be different on your system.
	seleniumPath = "/usr/share/selenium-server/selenium-server-standalone.jar"
	driverPath   = "/usr/bin/chromedriver"
	seleniumPort = 4666
)

type suite struct {
	brutalinksStartFn func() error
	brutalinksStopFn  func() error
	sl                *selenium.Service
	wd                selenium.WebDriver
}

func (s suite) stop() {
	s.sl.Stop()
}

func initSuite() (suite, error) {
	sOpts := []selenium.ServiceOption{
		selenium.StartFrameBuffer(),       // Start an X frame buffer for the browser to run in.
		selenium.ChromeDriver(driverPath), // Specify the path to GeckoDriver in order to use Firefox.
		//selenium.Output(opts.Output),      // Output debug information to godog's writer.
	}
	selenium.SetDebug(true)

	var err error
	s.sl, err = selenium.NewSeleniumService(seleniumPath, seleniumPort, sOpts...)
	return s, err
}

func (s *suite) InitializeTestSuite(t *testing.T) func(ctx *godog.TestSuiteContext) {
	s.brutalinksStartFn, s.brutalinksStopFn = initBrutalinks(t)
	return func(ctx *godog.TestSuiteContext) {
		ctx.BeforeSuite(func() {
			if err := s.brutalinksStartFn(); err != nil {
				t.Errorf("unable to start brutalinks: %s", err)
				os.Exit(1)
			}
		})
		ctx.AfterSuite(func() {
			if err := s.brutalinksStopFn(); err != nil {
				t.Errorf("unable to stop brutalinks: %s", err)
				os.Exit(1)
			}
		})
	}
}

var caps = selenium.Capabilities{"browserName": "chrome"}

func (s *suite) InitializeScenario(t *testing.T) func(ctx *godog.ScenarioContext) {
	return func(ctx *godog.ScenarioContext) {
		ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
			var err error
			if s.wd, err = selenium.NewRemote(caps, fmt.Sprintf("http://localhost:%d/wd/hub", seleniumPort)); err != nil {
				t.Errorf("Failed to start web driver: %s", err)
			}
			return ctx, err
		})
		ctx.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
			if err != nil {
				t.Errorf("Error after: %s", err)
			}
			if err = s.wd.Quit(); err != nil {
				t.Errorf("Failed to stop web driver: %s", err)
			}
			return ctx, err
		})
		ctx.Step(`^site is up$`, s.siteIsUp)
		ctx.Step(`^I visit "([^"]*)"$`, s.iVisit)
		ctx.Step(`^I should get the logo of "([^"]*)"$`, s.iShouldGetTheLogo)
	}
}

var opts = godog.Options{
	StopOnFailure: true,
	Format:        "pretty",
	Paths:         []string{"features"},
	Output:        colors.Colored(os.Stdout),
}

var executorURL = "https://brutalinks.local"
var apiURL = "https://fedbox.local"

func initBrutalinks(t *testing.T) (func() error, func() error) {
	// NOTE(marius): needs to match the actor that we use in fedboxmock
	os.Setenv(config.KeyFedBOXOAuthKey, "mock-app-1")
	os.Setenv(config.KeyFedBOXOAuthSecret, "mockpw")

	c := config.Load(config.TEST, 10)
	c.CachingEnabled = false
	// NOTE(marius): we need to mock FedBOX to return just some expected values
	// Should I look into having brutalinks support connecting over a socket?
	c.APIURL = apiMockURL()
	c.Secure = false
	c.KeyPath = ""
	c.CertPath = ""

	errors.IncludeBacktrace = true
	l := log.Dev(log.TraceLevel)

	a, err := app.New(c, l, "localhost", 5443, runtime.Version())
	if err != nil {
		return func() error { return err }, func() error { return nil }
	}
	ctx, cancelFn := context.WithCancel(context.TODO())
	srvRun, srvStop := w.HttpServer(w.Handler(a.Mux), w.HTTP(a.Conf.Listen()))

	stopFn := func() error {
		defer cancelFn()
		return srvStop(ctx)
	}

	return srvRun, stopFn
}

func (s *suite) siteIsUp() error {
	return nil
}

func (s *suite) iShouldGetTheLogo(expected string) error {
	logo, err := s.wd.FindElement(selenium.ByCSSSelector, "body > header > h1")
	if err != nil {
		return err
	}
	content, err := logo.Text()
	if err != nil {
		return err
	}
	expected = "trash-o" + expected
	if content != expected {
		return fmt.Errorf("logo content is not equal: %q, expected %q", content, expected)
	}
	return nil
}

func (s *suite) iVisit(url string) error {
	url = fmt.Sprintf("%s%s", executorURL, url)
	if err := s.wd.Get(url); err != nil {
		return err
	}
	curl, err := s.wd.CurrentURL()
	if err != nil {
		return err
	}
	if curl != url {
		return fmt.Errorf("invalid url %q, expected %q", curl, url)
	}
	return nil
}

var s suite

func Test_Features(t *testing.T) {
	cucumber := godog.TestSuite{
		Name:                 "brutalinks",
		Options:              &opts,
		TestSuiteInitializer: s.InitializeTestSuite(t),
		ScenarioInitializer:  s.InitializeScenario(t),
	}

	var err error
	s, err = initSuite()
	if err != nil {
		t.Errorf("unable to initialize suite: %s", err)
	}
	defer s.stop()

	if status := cucumber.Run(); status != 0 {
		t.Errorf("Invalid cucumber return value %d, expected 0", status)
	}
}

func TestMain(m *testing.M) {
	flag.Parse()
	opts.Paths = flag.Args()

	status := m.Run()
	os.Exit(status)
}
