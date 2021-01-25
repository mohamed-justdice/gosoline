package aws

import (
	"context"
	"fmt"
	"github.com/applike/gosoline/pkg/clock"
	"github.com/applike/gosoline/pkg/mon"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	awsCfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/smithy-go/logging"
	"github.com/aws/smithy-go/middleware"
)

type ClientSettings struct {
	Region   string `cfg:"region" default:"eu-central-1"`
	Endpoint string `cfg:"endpoint" default:"http://localhost:4566"`
}

func DefaultClientOptions(logger mon.Logger, settings ClientSettings, optFns ...func(options *awsCfg.LoadOptions) error) []func(options *awsCfg.LoadOptions) error {
	options := []func(options *awsCfg.LoadOptions) error{
		awsCfg.WithRegion(settings.Region),
		awsCfg.WithEndpointResolver(EndpointResolver(settings.Endpoint)),
		awsCfg.WithLogger(NewLogger(logger)),
		awsCfg.WithClientLogMode(aws.ClientLogMode(0)),
		awsCfg.WithRetryer(func() aws.Retryer {
			return retry.AddWithMaxAttempts(retry.NewStandard(), 10)
		}),
	}
	options = append(options, optFns...)

	return options
}

func DefaultClientConfig(ctx context.Context, logger mon.Logger, clock clock.Clock, settings ClientSettings, optFns ...func(options *awsCfg.LoadOptions) error) (aws.Config, error) {
	var err error
	var awsConfig aws.Config
	var options = DefaultClientOptions(logger, settings, optFns...)

	if awsConfig, err = awsCfg.LoadDefaultConfig(ctx, options...); err != nil {
		return awsConfig, fmt.Errorf("can not initialize config: %w", err)
	}

	awsConfig.APIOptions = append(awsConfig.APIOptions, func(stack *middleware.Stack) error {
		return stack.Initialize.Add(AttemptLoggerInitMiddleware(logger, clock), middleware.After)
	})
	awsConfig.APIOptions = append(awsConfig.APIOptions, func(stack *middleware.Stack) error {
		return stack.Finalize.Insert(AttemptLoggerRetryMiddleware(logger, clock), "Retry", middleware.After)
	})

	return awsConfig, nil
}

func WithEndpoint(url string) func(options *awsCfg.LoadOptions) error {
	return func(o *awsCfg.LoadOptions) error {
		o.EndpointResolver = EndpointResolver(url)
		return nil
	}
}

func EndpointResolver(url string) aws.EndpointResolverFunc {
	return func(service, region string) (aws.Endpoint, error) {
		if url == "" {
			return aws.Endpoint{}, &aws.EndpointNotFoundError{}
		}

		return aws.Endpoint{
			PartitionID:   "aws",
			URL:           url,
			SigningRegion: region,
		}, nil
	}
}

type Logger struct {
	base mon.Logger
}

func NewLogger(base mon.Logger) *Logger {
	return &Logger{
		base: base,
	}
}

func (l Logger) Logf(classification logging.Classification, format string, v ...interface{}) {
	switch classification {
	case logging.Warn:
		l.base.Warnf(format, v...)
	default:
		l.base.Infof(format, v...)
	}
}

func (l Logger) WithContext(ctx context.Context) logging.Logger {
	return &Logger{
		base: l.base.WithContext(ctx),
	}
}
