package kernel_test

import (
	"context"
	"fmt"
	"github.com/applike/gosoline/pkg/cfg"
	cfgMocks "github.com/applike/gosoline/pkg/cfg/mocks"
	"github.com/applike/gosoline/pkg/coffin"
	"github.com/applike/gosoline/pkg/conc"
	"github.com/applike/gosoline/pkg/kernel"
	kernelMocks "github.com/applike/gosoline/pkg/kernel/mocks"
	"github.com/applike/gosoline/pkg/mon"
	monMocks "github.com/applike/gosoline/pkg/mon/mocks"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/sys/unix"
	"strings"
	"sync"
	"testing"
	"time"
)

func createMocks() (*cfgMocks.Config, *monMocks.Logger, *kernelMocks.FullModule) {
	config := new(cfgMocks.Config)
	config.On("AllSettings").Return(map[string]interface{}{})
	config.On("UnmarshalKey", "kernel", mock.AnythingOfType("*kernel.Settings")).Return(map[string]interface{}{})

	logger := new(monMocks.Logger)
	logger.On("WithChannel", mock.Anything).Return(logger)
	logger.On("WithFields", mock.Anything).Return(logger)
	logger.On("Info", mock.Anything)
	logger.On("Infof", mock.Anything, mock.Anything, mock.Anything, mock.Anything)

	module := new(kernelMocks.FullModule)
	module.On("GetType").Return(kernel.TypeForeground)

	return config, logger, module
}

func TestNoModules(t *testing.T) {
	config, logger, _ := createMocks()
	logger.On("Warn", "nothing to run")

	k := kernel.New(config, logger, kernel.KillTimeout(time.Second))
	k.Run()
}

func TestRunFactoriesError(t *testing.T) {
	config, logger, _ := createMocks()

	err := fmt.Errorf("error in module factory")
	logger.On("Error", err, "error building additional modules by multiFactories")

	assert.NotPanics(t, func() {
		k := kernel.New(config, logger, kernel.KillTimeout(time.Second))
		k.AddFactory(func(cfg.Config, mon.Logger) (map[string]kernel.ModuleFactory, error) {
			return nil, err
		})
		k.Run()
	})
}

func TestRunFactoriesPanic(t *testing.T) {
	config, logger, _ := createMocks()

	logger.On("Error", mock.Anything, "error running module factories").Run(func(args mock.Arguments) {
		err := args.Get(0).(error)
		assert.True(t, strings.Contains(err.Error(), "panic in module factory"))
	})
	logger.On("Error", mock.Anything, "error building additional modules by multiFactories").Run(func(args mock.Arguments) {
		err := args.Get(0).(error)
		assert.True(t, strings.Contains(err.Error(), "panic in module factory"))
	})

	assert.NotPanics(t, func() {
		k := kernel.New(config, logger, kernel.KillTimeout(time.Second))
		k.AddFactory(func(cfg.Config, mon.Logger) (map[string]kernel.ModuleFactory, error) {
			panic("panic in module factory")
		})
		k.Run()
	})
}

func TestBootFailure(t *testing.T) {
	config, logger, _ := createMocks()

	logger.On("Info", mock.Anything)
	logger.On("Error", mock.AnythingOfType("*errors.withStack"), "error building modules")

	assert.NotPanics(t, func() {
		k := kernel.New(config, logger, kernel.KillTimeout(time.Second))
		k.Add("module", func(ctx context.Context, config cfg.Config, logger mon.Logger) (kernel.Module, error) {
			panic(errors.New("panic"))
		})
		k.Run()
	})
}

func TestRunSuccess(t *testing.T) {
	config, logger, module := createMocks()

	module.On("GetStage").Return(kernel.StageApplication)
	module.On("Run", mock.Anything).Return(nil)

	assert.NotPanics(t, func() {
		k := kernel.New(config, logger, kernel.KillTimeout(time.Second))
		k.Add("module", func(ctx context.Context, config cfg.Config, logger mon.Logger) (kernel.Module, error) {
			return module, nil
		})
		k.Run()
	})

	module.AssertCalled(t, "Run", mock.Anything)
}

func TestRunFailure(t *testing.T) {
	config, logger, module := createMocks()

	logger.On("Errorf", mock.Anything, "error during the execution of stage %d", kernel.StageApplication)

	module.On("GetStage").Return(kernel.StageApplication)
	module.On("Run", mock.Anything).Run(func(args mock.Arguments) {
		panic("panic")
	})

	assert.NotPanics(t, func() {
		k := kernel.New(config, logger, kernel.KillTimeout(time.Second))
		k.Add("module", func(ctx context.Context, config cfg.Config, logger mon.Logger) (kernel.Module, error) {
			return module, nil
		})
		k.Run()
	})

	module.AssertCalled(t, "Run", mock.Anything)
}

func TestStop(t *testing.T) {
	config, logger, module := createMocks()
	k := kernel.New(config, logger, kernel.KillTimeout(time.Second))

	module.On("GetType").Return(kernel.TypeForeground)
	module.On("GetStage").Return(kernel.StageApplication)
	module.On("Run", mock.Anything).Run(func(args mock.Arguments) {
		ctx := args.Get(0).(context.Context)

		k.Stop("test done")
		<-ctx.Done()
	}).Return(nil)

	k.Add("module", func(ctx context.Context, config cfg.Config, logger mon.Logger) (kernel.Module, error) {
		return module, nil
	})
	k.Run()

	module.AssertCalled(t, "Run", mock.Anything)
}

func TestRunningType(t *testing.T) {
	config, logger, _ := createMocks()
	k := kernel.New(config, logger, kernel.KillTimeout(time.Second))

	mf := new(kernelMocks.Module)
	// type = foreground & stage = application are the defaults for a module
	mf.On("Run", mock.Anything).Run(func(args mock.Arguments) {}).Return(nil)

	mb := new(kernelMocks.FullModule)
	mb.On("GetType").Return(kernel.TypeBackground)
	mb.On("GetStage").Return(kernel.StageApplication)
	mb.On("Run", mock.Anything).Run(func(args mock.Arguments) {
		ctx := args.Get(0).(context.Context)

		<-ctx.Done()
	}).Return(nil)

	k.Add("foreground", func(ctx context.Context, config cfg.Config, logger mon.Logger) (kernel.Module, error) {
		return mf, nil
	})
	k.Add("background", func(ctx context.Context, config cfg.Config, logger mon.Logger) (kernel.Module, error) {
		return mb, nil
	})
	k.Run()

	mf.AssertExpectations(t)
	mb.AssertExpectations(t)
}

func TestMultipleStages(t *testing.T) {
	config, logger, _ := createMocks()

	k := kernel.New(config, logger, kernel.KillTimeout(time.Second))
	var allMocks []*kernelMocks.FullModule
	var stageStatus []int

	maxStage := 5
	wg := &sync.WaitGroup{}
	wg.Add(maxStage)

	for stage := 0; stage < maxStage; stage++ {
		thisStage := stage

		m := new(kernelMocks.FullModule)
		m.On("GetType").Return(kernel.TypeEssential)
		m.On("GetStage").Return(thisStage)
		m.On("Run", mock.Anything).Run(func(args mock.Arguments) {
			ctx := args.Get(0).(context.Context)

			stageStatus[thisStage] = 1

			wg.Done()
			wg.Wait()
			<-ctx.Done()

			logger.Infof("stage %d: ctx done", thisStage)

			for i := 0; i <= thisStage; i++ {
				assert.GreaterOrEqual(t, stageStatus[i], 1, fmt.Sprintf("stage %d: expected stage %d to be at least running", thisStage, i))
			}
			for i := thisStage + 1; i < maxStage; i++ {
				assert.Equal(t, 2, stageStatus[i], fmt.Sprintf("stage %d: expected stage %d to be done", thisStage, i))
			}

			stageStatus[thisStage] = 2
		}).Return(nil)

		allMocks = append(allMocks, m)
		stageStatus = append(stageStatus, 0)

		k.Add("m", func(ctx context.Context, config cfg.Config, logger mon.Logger) (kernel.Module, error) {
			return m, nil
		})
	}

	go func() {
		time.Sleep(time.Millisecond * 300)
		k.Stop("we are done testing")
	}()
	k.Run()

	for _, m := range allMocks {
		m.AssertExpectations(t)
	}
}

func TestKernelForcedExit(t *testing.T) {
	config, logger, _ := createMocks()
	logger.On("Errorf", mock.Anything, mock.Anything)

	mayStop := conc.NewSignalOnce()
	appStopped := conc.NewSignalOnce()

	k := kernel.New(config, logger, kernel.KillTimeout(200*time.Millisecond), kernel.ForceExit(func(code int) {
		assert.Equal(t, 1, code)

		mayStop.Signal()
	}))

	app := new(kernelMocks.FullModule)
	app.On("GetType").Return(kernel.TypeBackground)
	app.On("GetStage").Return(kernel.StageApplication)
	app.On("Run", mock.Anything).Run(func(args mock.Arguments) {
		ctx := args.Get(0).(context.Context)

		<-ctx.Done()
		appStopped.Signal()
	}).Return(nil)

	m := new(kernelMocks.Module)
	m.On("Run", mock.Anything).Run(func(args mock.Arguments) {
		<-mayStop.Channel()
		assert.True(t, appStopped.Signaled())
	}).Return(nil)

	k.Add("m", func(ctx context.Context, config cfg.Config, logger mon.Logger) (kernel.Module, error) {
		return m, nil
	}, kernel.ModuleStage(kernel.StageService), kernel.ModuleType(kernel.TypeForeground))
	k.Add("app", func(ctx context.Context, config cfg.Config, logger mon.Logger) (kernel.Module, error) {
		return app, nil
	})

	go func() {
		time.Sleep(time.Millisecond * 300)
		k.Stop("we are done testing")
	}()

	k.Run()

	app.AssertExpectations(t)
	m.AssertExpectations(t)
	assert.True(t, mayStop.Signaled())
}

func TestKernelStageStopped(t *testing.T) {
	config, logger, _ := createMocks()
	logger.On("Errorf", mock.Anything, mock.Anything)

	success := false
	appStopped := conc.NewSignalOnce()

	k := kernel.New(config, logger, kernel.KillTimeout(200*time.Millisecond))

	app := new(kernelMocks.FullModule)
	app.On("GetType").Return(kernel.TypeForeground)
	app.On("GetStage").Return(kernel.StageApplication)
	app.On("Run", mock.Anything).Run(func(args mock.Arguments) {
		ctx := args.Get(0).(context.Context)

		ticker := time.NewTicker(time.Millisecond * 300)
		defer ticker.Stop()

		select {
		case <-ctx.Done():
			t.Fatal("kernel stopped before 300ms")
		case <-ticker.C:
			success = true
		}

		appStopped.Signal()
	}).Return(nil)

	m := new(kernelMocks.FullModule)
	m.On("GetType").Return(kernel.TypeBackground)
	m.On("GetStage").Return(777)
	m.On("Run", mock.Anything).Return(nil)

	k.Add("m", func(ctx context.Context, config cfg.Config, logger mon.Logger) (kernel.Module, error) {
		return m, nil
	})
	k.Add("app", func(ctx context.Context, config cfg.Config, logger mon.Logger) (kernel.Module, error) {
		return app, nil
	})
	k.Run()

	assert.True(t, success)

	app.AssertExpectations(t)
	m.AssertExpectations(t)
}

type fakeModule struct {
}

func (m *fakeModule) Boot(_ cfg.Config, _ mon.Logger) error {
	return nil
}

func (m *fakeModule) Run(_ context.Context) error {
	return nil
}

type realModule struct {
	t *testing.T
}

func (m *realModule) Boot(_ cfg.Config, _ mon.Logger) error {
	return nil
}

func (m *realModule) Run(ctx context.Context) error {
	cfn, cfnCtx := coffin.WithContext(ctx)

	cfn.GoWithContext(cfnCtx, func(ctx context.Context) error {
		ticker := time.NewTicker(time.Millisecond * 2)
		defer ticker.Stop()

		counter := 0

		for {
			select {
			case <-ticker.C:
				counter++
				if counter == 3 {
					err := unix.Kill(unix.Getpid(), unix.SIGTERM)
					assert.NoError(m.t, err)
				}
			case <-ctx.Done():
				return nil
			}
		}
	})

	err := cfn.Wait()
	if !errors.Is(err, context.Canceled) {
		assert.NoError(m.t, err)
	}
	return err
}

func TestKernel_RunRealModule(t *testing.T) {
	// test that we can run the kernel multiple times
	// if this does not work, the next test does not make sense
	for i := 0; i < 10; i++ {
		t.Run(fmt.Sprintf("fake iteration %d", i), func(t *testing.T) {
			config, logger, _ := createMocks()
			assert.NotPanics(t, func() {
				k := kernel.New(config, logger)
				k.Add("main", func(ctx context.Context, config cfg.Config, logger mon.Logger) (kernel.Module, error) {
					return &fakeModule{}, nil
				})
				k.Run()
			})
		})
	}
	// test for a race condition on kernel shutdown
	// in the past, this would panic in a close on closed channel in the tomb module
	for i := 0; i < 10; i++ {
		t.Run(fmt.Sprintf("real iteration %d", i), func(t *testing.T) {
			config, logger, _ := createMocks()
			assert.NotPanics(t, func() {
				k := kernel.New(config, logger)
				k.Add("main", func(ctx context.Context, config cfg.Config, logger mon.Logger) (kernel.Module, error) {
					return &realModule{
						t: t,
					}, nil
				})
				k.Run()
			})
		})
	}
}

type fastExitModule struct {
	kernel.BackgroundModule
}

func (f *fastExitModule) Run(_ context.Context) error {
	return nil
}

type slowExitModule struct {
	fastExitModule
	kernel.ForegroundModule
	kernel kernel.Kernel
}

func (s *slowExitModule) Run(_ context.Context) error {
	s.kernel.Stop("slowly")

	return nil
}

func TestModuleFastShutdown(t *testing.T) {
	config, logger, _ := createMocks()
	assert.NotPanics(t, func() {
		k := kernel.New(config, logger)
		for s := 5; s < 10; s++ {
			k.Add("exist-fast", func(ctx context.Context, config cfg.Config, logger mon.Logger) (kernel.Module, error) {
				return &fastExitModule{}, nil
			}, kernel.ModuleStage(s))
			k.Add("exist-slow", func(ctx context.Context, config cfg.Config, logger mon.Logger) (kernel.Module, error) {
				return &slowExitModule{
					kernel: k,
				}, nil
			}, kernel.ModuleStage(s))
		}
		k.Run()
	})
}
