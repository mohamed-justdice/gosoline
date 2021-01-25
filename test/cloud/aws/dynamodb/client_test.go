//+build integration

package dynamodb_test

import (
	"context"
	"fmt"
	toxiproxy "github.com/Shopify/toxiproxy/client"
	"github.com/applike/gosoline/pkg/clock"
	gosoAws "github.com/applike/gosoline/pkg/cloud/aws"
	"github.com/applike/gosoline/pkg/cloud/aws/dynamodb"
	"github.com/applike/gosoline/pkg/test/env"
	"github.com/applike/gosoline/pkg/test/suite"
	"github.com/aws/aws-sdk-go-v2/aws"
	http2 "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	awsCfg "github.com/aws/aws-sdk-go-v2/config"
	awsDdb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go/middleware"
	"net/http"
	"testing"
	"time"
)

type MockHttpClient struct {
	attempts int
	base     awsCfg.HTTPClient
	clock    clock.FakeClock
}

func (m *MockHttpClient) Do(request *http.Request) (*http.Response, error) {
	defer func() {
		m.attempts++
		m.clock.Advance(time.Second)
	}()

	if m.attempts == 0 {
		return nil, fmt.Errorf("connection reset")
	}

	return m.base.Do(request)
}

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
	ctx := context.Background()
	config := s.Env().Config()
	logger := s.Env().Logger()
	clock := clock.NewFakeClock()

	//baseHttpClient := awsHttp.NewBuildableClient()
	//mockHttpClient := &MockHttpClient{
	//	base:  baseHttpClient,
	//	clock: clock,
	//}

	httpClient := http2.NewBuildableClient().WithTimeout(time.Second)

	ddbAddress := s.Env().DynamoDb("default").Address()

	tComp := s.Env().Component("toxiproxy", "default").(*env.ToxiproxyComponent)
	toxi := s.Env().Component("toxiproxy", "default").(*env.ToxiproxyComponent).Client()

	proxy, err := toxi.CreateProxy("redis", ":56248", ddbAddress)
	s.NoError(err)

	proxy.AddToxic("latency_down", "latency", "downstream", 1.0, toxiproxy.Attributes{
		"latency": 3000,
	})

	endpoint := fmt.Sprintf("http://%s", tComp.Bla)
	client, err := dynamodb.NewClientWithInterfaces(ctx, config, logger, clock, "default", gosoAws.WithEndpoint(endpoint))
	s.NoError(err)

	_, err = client.CreateTable(ctx, &awsDdb.CreateTableInput{
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("id"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("id"),
				KeyType:       types.KeyTypeHash,
			},
		},
		TableName: aws.String("gosoline-cloud-dynamodb-test"),
		ProvisionedThroughput: &types.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(1),
			WriteCapacityUnits: aws.Int64(1),
		},
	})
	s.NoError(err)

	ctx, _ = context.WithDeadline(ctx, time.Now().Add(time.Second*10))
	i := 0

	_, err = client.PutItem(ctx, &awsDdb.PutItemInput{
		Item: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{
				Value: "goso-id",
			},
		},
		TableName: aws.String("gosoline-cloud-dynamodb-test"),
	}, func(options *awsDdb.Options) {
		// Register the defaultBucketMiddleware for this operation only
		options.APIOptions = append(options.APIOptions, func(stack *middleware.Stack) error {
			return stack.Finalize.Add(middleware.FinalizeMiddlewareFunc("bla", func(ctx context.Context, input middleware.FinalizeInput, handler middleware.FinalizeHandler) (middleware.FinalizeOutput, middleware.Metadata, error) {
				i++
				if i == 3 {
					proxy.RemoveToxic("latency_down")
				}

				return handler.HandleFinalize(ctx, input)
			}), middleware.After)
		})
		options.HTTPClient = httpClient
	})
	//}, func(opt *awsDdb.Options) {
	//	opt.HTTPClient = mockHttpClient
	//})
	s.NoError(err)
}

func TestClientTestSuite(t *testing.T) {
	suite.Run(t, new(ClientTestSuite))
}
