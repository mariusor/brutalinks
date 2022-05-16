package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/cucumber/godog"
	"github.com/cucumber/godog/colors"
	"github.com/tebeka/selenium"
)

const (
	// These paths will be different on your system.
	seleniumPath    = "/usr/share/selenium-server/selenium-server-standalone.jar"
	geckoDriverPath = "vendor/geckodriver-v0.18.0-linux64"
	seleniumPort    = 4666
)

type suite struct {
	sl *selenium.Service
	wd selenium.WebDriver
}

var service *selenium.Service

func initSuite() (suite, error) {
	sOpts := []selenium.ServiceOption{
		selenium.StartFrameBuffer(),           // Start an X frame buffer for the browser to run in.
		selenium.GeckoDriver(geckoDriverPath), // Specify the path to GeckoDriver in order to use Firefox.
		selenium.Output(opts.Output),          // Output debug information to godog's writer.
	}
	selenium.SetDebug(true)

	var err error
	s.sl, err = selenium.NewSeleniumService(seleniumPath, seleniumPort, sOpts...)
	return s, err
}

func (s *suite) InitializeTestSuite(t *testing.T) func(ctx *godog.TestSuiteContext) {
	return func(ctx *godog.TestSuiteContext) {
		ctx.BeforeSuite(func() {
			t.Logf("starting suite")
		})
		ctx.AfterSuite(func() {
			t.Logf("ending suite")
		})
	}
}

var caps = selenium.Capabilities{"browserName": "firefox"}
var wd selenium.WebDriver

func (s *suite) InitializeScenario(t *testing.T) func(ctx *godog.ScenarioContext) {
	return func(ctx *godog.ScenarioContext) {
		ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
			// Connect to the WebDriver instance running locally.
			var err error
			t.Logf("initializing web-driver")
			s.wd, err = selenium.NewRemote(caps, fmt.Sprintf("http://localhost:%d/wd/hub", seleniumPort))
			return ctx, err
		})
		ctx.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
			t.Logf("shutting-down web-driver")
			return ctx, wd.Quit()
		})
		ctx.Step(`^I visit (\w+)$`, iVisit)
		ctx.Step(`^site is up$`, func() {})
		ctx.Step(`^I should get "(\w+)"$`, func(status string) {})
	}
}

var opts = godog.Options{
	StopOnFailure: true,
	Format:        "pretty",
	Paths:         []string{"features"},
	Output:        colors.Colored(os.Stdout),
}

var executorURL = "https://brutalinks.tech"

func iVisit(url string) error {
	// Navigate to the simple playground interface.
	if err := wd.Get(fmt.Sprintf("%s%s", executorURL, url)); err != nil {
		return err
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
	defer s.sl.Stop()
	status := m.Run()
	os.Exit(status)
}
