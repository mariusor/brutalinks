package main

import (
	"context"
	"flag"
	"fmt"
	"os"
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
	s.brutalinksStopFn()
}
func initSuite() (suite, error) {
	sOpts := []selenium.ServiceOption{
		selenium.StartFrameBuffer(),       // Start an X frame buffer for the browser to run in.
		selenium.ChromeDriver(driverPath), // Specify the path to GeckoDriver in order to use Firefox.
		selenium.Output(opts.Output),      // Output debug information to godog's writer.
	}
	selenium.SetDebug(false)

	var err error
	s.sl, err = selenium.NewSeleniumService(seleniumPath, seleniumPort, sOpts...)
	s.brutalinksStartFn, s.brutalinksStopFn = initBrutalinks()
	return s, err
}

func (s *suite) InitializeTestSuite(t *testing.T) func(ctx *godog.TestSuiteContext) {
	return func(ctx *godog.TestSuiteContext) {
		//ctx.BeforeSuite(func() { })
		//ctx.AfterSuite(func() { })
	}
}

var caps = selenium.Capabilities{"browserName": "chrome"}

func (s *suite) InitializeScenario(t *testing.T) func(ctx *godog.ScenarioContext) {
	return func(ctx *godog.ScenarioContext) {
		ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
			var err error
			s.wd, err = selenium.NewRemote(caps, fmt.Sprintf("http://localhost:%d/wd/hub", seleniumPort))
			t.Errorf("Failed to start web driver: %s", err)
			return ctx, err
		})
		ctx.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
			return ctx, s.wd.Quit()
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

func initBrutalinks() (func() error, func() error) {
	c := config.Load(config.TEST, 10)
	c.APIURL = apiURL
	errors.IncludeBacktrace = true
	l := log.Dev(log.TraceLevel)

	a, err := app.New(c, l, "localhost", 5443, "-test-head")
	if err != nil {
		return func() error {
				return err
			},
			func() error {
				return nil
			}
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
	if status := cucumber.Run(); status != 0 {
		t.Errorf("Invalid cucumber return value %d, expected 0", status)
	}
}

func TestMain(m *testing.M) {
	flag.Parse()
	opts.Paths = flag.Args()

	var err error
	s, err = initSuite()
	if err != nil {
		panic(err)
	}
	defer s.stop()
	status := m.Run()
	os.Exit(status)
}
