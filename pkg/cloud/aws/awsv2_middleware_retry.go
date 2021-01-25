package aws

import (
	"context"
	"fmt"
	"github.com/applike/gosoline/pkg/clock"
	"github.com/applike/gosoline/pkg/mon"
	awsMiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	smithyMiddleware "github.com/aws/smithy-go/middleware"
	"time"
)

type attemptInfoKey struct{}

type attemptInfo struct {
	resouceName string
	start       time.Time
	count       int
	lastErr     error
}

func getAttemptInfo(ctx context.Context) *attemptInfo {
	info := smithyMiddleware.GetStackValue(ctx, attemptInfoKey{})

	if info == nil {
		return nil
	}

	return info.(*attemptInfo)
}

func setAttemptInfo(ctx context.Context, info *attemptInfo) context.Context {
	return smithyMiddleware.WithStackValue(ctx, attemptInfoKey{}, info)
}

func increaseAttemptCount(ctx context.Context) (*attemptInfo, context.Context) {
	stats := getAttemptInfo(ctx)

	if stats == nil {
		stats = &attemptInfo{
			start: time.Now(),
		}
	}

	stats.count++
	ctx = smithyMiddleware.WithStackValue(ctx, attemptInfoKey{}, stats)

	return stats, ctx
}

func AttemptLoggerInitMiddleware(logger mon.Logger, clock clock.Clock) smithyMiddleware.InitializeMiddleware {
	return smithyMiddleware.InitializeMiddlewareFunc("AttemptLoggerInit", func(ctx context.Context, input smithyMiddleware.InitializeInput, handler smithyMiddleware.InitializeHandler) (smithyMiddleware.InitializeOutput, smithyMiddleware.Metadata, error) {
		var err error
		var metadata smithyMiddleware.Metadata
		var output smithyMiddleware.InitializeOutput

		serviceId := awsMiddleware.GetServiceID(ctx)
		operation := awsMiddleware.GetOperationName(ctx)
		resourceName := fmt.Sprintf("%s/%s", serviceId, operation)

		info := &attemptInfo{
			start:       clock.Now(),
			resouceName: resourceName,
		}
		ctx = setAttemptInfo(ctx, info)

		output, metadata, err = handler.HandleInitialize(ctx, input)

		if info.count > 1 && err == nil {
			duration := clock.Now().Sub(info.start)
			logger.WithContext(ctx).Infof("sent request to resource %s successful after %d retries in %s", info.resouceName, info.count, duration)
		}

		return output, metadata, err
	})
}

func AttemptLoggerRetryMiddleware(logger mon.Logger, clock clock.Clock) smithyMiddleware.FinalizeMiddleware {
	return smithyMiddleware.FinalizeMiddlewareFunc("AttemptLoggerRetry", func(ctx context.Context, input smithyMiddleware.FinalizeInput, next smithyMiddleware.FinalizeHandler) (smithyMiddleware.FinalizeOutput, smithyMiddleware.Metadata, error) {
		var info *attemptInfo
		var metadata smithyMiddleware.Metadata
		var output smithyMiddleware.FinalizeOutput

		info, ctx = increaseAttemptCount(ctx)

		if info.count > 1 {
			duration := clock.Now().Sub(info.start)
			logger.WithContext(ctx).Warnf("retrying action on resource %s after %s cause of error %s", info.resouceName, duration, info.lastErr)
		}

		output, metadata, info.lastErr = next.HandleFinalize(ctx, input)

		return output, metadata, info.lastErr
	})
}
