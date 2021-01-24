package main

import (
	"flag"
	"os"
	"testing"

	"github.com/cucumber/godog"
	"github.com/cucumber/godog/colors"
)

func InitializeTestSuite(ctx *godog.TestSuiteContext) {
	ctx.BeforeSuite(func() {
		// initialize fedbox and go-littr
	})
}

func InitializeScenario(ctx *godog.ScenarioContext) {
	ctx.BeforeScenario(func(*godog.Scenario) {})
	ctx.Step(`^I visit (\w+)$`, func(url string) {})
	ctx.Step(`^site is up$`, func() {})
	ctx.Step(`^I should get "(\w+)"$`, func(status string) {})
}

var opts = godog.Options{Output: colors.Colored(os.Stdout), Format: "pretty"}

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
