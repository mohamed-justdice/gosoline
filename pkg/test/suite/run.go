package suite

import (
	"github.com/applike/gosoline/pkg/clock"
	"github.com/applike/gosoline/pkg/test/env"
	"github.com/stretchr/testify/assert"
	"reflect"
	"regexp"
	"testing"
)

func Run(t *testing.T, suite TestingSuite) {
	suite.SetT(t)

	methodFinder := reflect.TypeOf(suite)

	for i := 0; i < methodFinder.NumMethod(); i++ {
		method := methodFinder.Method(i)

		if ok := filterTestMethod(t, method); !ok {
			continue
		}

		runTestCase(t, suite, func() {
			method.Func.Call([]reflect.Value{reflect.ValueOf(suite)})
		})
	}
}

func filterTestMethod(t *testing.T, method reflect.Method) bool {
	if ok, _ := regexp.MatchString("^Test", method.Name); !ok {
		return false
	}

	if method.Func.Type().NumIn() != 1 {
		assert.FailNow(t, "invalid test func signature", "test func %s has to have the signature func()", method.Name)
	}

	return true
}

func runTestCase(t *testing.T, suite TestingSuite, testCase func(), extraOptions ...SuiteOption) {
	suiteOptions := &suiteOptions{}

	setupOptions := []SuiteOption{
		WithClockProvider(clock.NewFakeClock()),
	}
	setupOptions = append(setupOptions, suite.SetupSuite()...)
	setupOptions = append(setupOptions, extraOptions...)

	for _, opt := range setupOptions {
		opt(suiteOptions)
	}

	envOptions := []env.Option{
		env.WithLoggerSettingsFromConfig,
	}
	envOptions = append(envOptions, suiteOptions.envOptions...)
	envOptions = append(envOptions, env.WithConfigMap(map[string]interface{}{
		"env": "test",
	}))

	environment, err := env.NewEnvironment(t, envOptions...)

	defer func() {
		if err = environment.Stop(); err != nil {
			assert.FailNow(t, "failed to stop test environment", err.Error())
		}
	}()

	if err != nil {
		assert.FailNow(t, "failed to create test environment", err.Error())
	}

	suite.SetEnv(environment)
	for _, envSetup := range suiteOptions.envSetup {
		if err = envSetup(); err != nil {
			assert.FailNow(t, "failed to execute additional environment setup", err.Error())
		}
	}

	testCase()
}
