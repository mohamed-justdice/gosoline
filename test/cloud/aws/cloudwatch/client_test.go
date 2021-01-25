//+build integration

package cloudwatch_test

import (
	"context"
	"fmt"
	"github.com/applike/gosoline/pkg/cloud/aws/cloudwatch"
	"github.com/applike/gosoline/pkg/test/suite"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsCw "github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"testing"
	"time"
)

type ClientTestSuite struct {
	suite.Suite
}

func (s *ClientTestSuite) SetupSuite() []suite.SuiteOption {
	return []suite.SuiteOption{
		suite.WithConfigFile("client_test_cfg.yml"),
		suite.WithLogLevel("debug"),
	}
}

func (s *ClientTestSuite) TestNewDefault() {
	client, err := cloudwatch.NewClient(context.Background(), s.Env().Config(), s.Env().Logger(), "default")
	s.NoError(err)

	out, err := client.GetMetricStatistics(context.Background(), &awsCw.GetMetricStatisticsInput{
		StartTime:  aws.Time(time.Now().Add(time.Hour * -1)),
		EndTime:    aws.Time(time.Now()),
		Namespace:  aws.String("gosoline"),
		MetricName: aws.String("test"),
		Period:     aws.Int32(60),
		Statistics: []types.Statistic{
			types.StatisticSum,
		},
	})
	s.NoError(err)

	fmt.Println(out)
}

func TestClientTestSuite(t *testing.T) {
	suite.Run(t, new(ClientTestSuite))
}
