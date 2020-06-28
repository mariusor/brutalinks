package config

import "strings"

// EnvType type alias
type EnvType string

const (
	// DEV environment
	DEV EnvType = "dev"
	// PROD environment
	PROD EnvType = "prod"
	// QA environment
	QA EnvType = "qa"
	// testing environment
	TEST EnvType = "test"
)

var (
	Default = Configuration{Env: DEV}

	validEnvTypes = []EnvType{
		DEV,
		PROD,
		QA,
		TEST,
	}
)

func ValidEnv(env EnvType) bool {
	if len(env) == 0 {
		return false
	}
	s := strings.ToLower(string(env))
	for _, k := range validEnvTypes {
		if strings.Contains(s, string(k)) {
			return true
		}
	}
	return false
}

func (e EnvType) IsProd() bool {
	return strings.Contains(string(e), string(PROD))
}
func (e EnvType) IsQA() bool {
	return strings.Contains(string(e), string(QA))
}
func (e EnvType) IsTest() bool {
	return strings.Contains(string(e), string(TEST))
}
func (e EnvType) IsDev() bool {
	return strings.Contains(string(e), string(DEV))
}

