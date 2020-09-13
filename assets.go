//+build !dev
//go:generate go run -tags $(ENV) aletheia.icu/broccoli -build "prod qa" -src ./templates,./assets,./README.md -var assets -o internal/assets/assets.gen.go

package assets
