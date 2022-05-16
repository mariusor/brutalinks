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
	seleniumPort    = 8666
)

var service *selenium.Service

func InitializeSelenium() {
	sOpts := []selenium.ServiceOption{
		selenium.StartFrameBuffer(),           // Start an X frame buffer for the browser to run in.
		selenium.GeckoDriver(geckoDriverPath), // Specify the path to GeckoDriver in order to use Firefox.
		selenium.Output(opts.Output),          // Output debug information to godog's writer.
	}
	selenium.SetDebug(true)

	var err error
	service, err = selenium.NewSeleniumService(seleniumPath, seleniumPort, sOpts...)
	if err != nil {
		panic(err)
	}
}

func InitializeTestSuite(ctx *godog.TestSuiteContext) {
	ctx.BeforeSuite(func() {
		InitializeSelenium()
		// initialize fedbox and go-littr
	})
	ctx.AfterSuite(func() {
		service.Stop()
	})
}

var caps = selenium.Capabilities{"browserName": "firefox"}
var wd selenium.WebDriver

func InitializeScenario(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		// Connect to the WebDriver instance running locally.
		var err error
		wd, err = selenium.NewRemote(caps, fmt.Sprintf("http://localhost:%d/wd/hub", seleniumPort))
		return ctx, err
	})
	ctx.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		return ctx, wd.Quit()
	})
	ctx.Step(`^I visit (\w+)$`, iVisit)
	ctx.Step(`^site is up$`, func() {})
	ctx.Step(`^I should get "(\w+)"$`, func(status string) {})
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

func TestMain(m *testing.M) {
	flag.Parse()
	opts.Paths = flag.Args()

	status := godog.TestSuite{
		Name:                 "littr",
		TestSuiteInitializer: InitializeTestSuite,
		ScenarioInitializer:  InitializeScenario,
		Options:              &opts,
	}.Run()
	os.Exit(status)
}
