package http

import (
	"context"
	"errors"
	"fmt"
	"github.com/applike/gosoline/pkg/cfg"
	"github.com/applike/gosoline/pkg/mon"
	"gopkg.in/resty.v1"
	"net/http"
	netUrl "net/url"
	"time"
)

const (
	DeleteRequest         = "DELETE"
	GetRequest            = "GET"
	PostRequest           = "POST"
	PutRequest            = "PUT"
	PatchRequest          = "PATCH"
	metricRequest         = "HttpClientRequest"
	metricError           = "HttpClientError"
	metricResponseCode    = "HttpClientResponseCode"
	metricRequestDuration = "HttpRequestDuration"
)

//go:generate mockery -name Client
type Client interface {
	Delete(ctx context.Context, request *Request) (*Response, error)
	Get(ctx context.Context, request *Request) (*Response, error)
	Patch(ctx context.Context, request *Request) (*Response, error)
	Post(ctx context.Context, request *Request) (*Response, error)
	Put(ctx context.Context, request *Request) (*Response, error)
	SetTimeout(timeout time.Duration)
	SetUserAgent(ua string)
	SetProxyUrl(p string)
	SetCookies(cs []*http.Cookie)
	SetCookie(c *http.Cookie)
	SetRedirectValidator(allowRequest func(request *http.Request) bool)
	AddRetryCondition(f RetryConditionFunc)
	NewRequest() *Request
	NewJsonRequest() *Request
	NewXmlRequest() *Request
}

type RetryConditionFunc func(*Response) (bool, error)

type Response struct {
	Body            []byte
	Header          http.Header
	Cookies         []*http.Cookie
	StatusCode      int
	RequestDuration time.Duration
}

type headers map[string]string

type client struct {
	logger         mon.Logger
	defaultHeaders headers
	http           restyClient
	mo             mon.MetricWriter
}

type Settings struct {
	RetryCount       int
	Timeout          time.Duration
	RetryWaitTime    time.Duration `cfg:"retry_wait_time" default:"100ms"`
	RetryMaxWaitTime time.Duration `cfg:"retry_max_wait_time" default:"2000ms"`
	FollowRedirect   bool          `cfg:"follow_redirects"`
}

func NewHttpClient(config cfg.Config, logger mon.Logger) Client {
	mo := mon.NewMetricDaemonWriter()

	settings := &Settings{}
	config.UnmarshalKey("http_client", settings)

	settings.RetryCount = config.GetInt("http_client_retry_count")
	settings.Timeout = config.GetDuration("http_client_request_timeout")

	httpClient := resty.New()
	httpClient.SetRedirectPolicy(resty.FlexibleRedirectPolicy(10))
	httpClient.SetRetryCount(settings.RetryCount)
	httpClient.SetTimeout(settings.Timeout)
	httpClient.SetRetryWaitTime(settings.RetryWaitTime)
	httpClient.SetRetryMaxWaitTime(settings.RetryMaxWaitTime)

	return NewHttpClientWithInterfaces(logger, mo, httpClient)
}

func NewHttpClientWithInterfaces(logger mon.Logger, mo mon.MetricWriter, httpClient restyClient) Client {
	return &client{
		logger:         logger,
		defaultHeaders: make(headers),
		http:           httpClient,
		mo:             mo,
	}
}

func (c *client) NewRequest() *Request {
	return &Request{
		queryParams:  netUrl.Values{},
		restyRequest: c.http.NewRequest(),
		url:          &netUrl.URL{},
	}
}

func (c *client) NewJsonRequest() *Request {
	return c.NewRequest().WithHeader(HdrAccept, ContentTypeApplicationJson)
}

func (c *client) NewXmlRequest() *Request {
	return c.NewRequest().WithHeader(HdrAccept, ContentTypeApplicationXml)
}

func (c *client) SetCookie(hc *http.Cookie) {
	c.http.SetCookie(hc)
}

func (c *client) SetCookies(cs []*http.Cookie) {
	c.http.SetCookies(cs)
}

func (c *client) SetTimeout(timeout time.Duration) {
	c.http.SetTimeout(timeout)
}

func (c *client) SetUserAgent(ua string) {
	c.defaultHeaders[HdrUserAgent] = ua
}

func (c *client) SetProxyUrl(p string) {
	c.http.SetProxy(p)
}

func (c *client) AddRetryCondition(f RetryConditionFunc) {
	c.http.AddRetryCondition(func(r *resty.Response) (bool, error) {
		return f(buildResponse(r))
	})
}

func (c *client) SetRedirectValidator(allowRequest func(request *http.Request) bool) {
	c.http.SetRedirectPolicy(
		resty.FlexibleRedirectPolicy(10),
		resty.RedirectPolicyFunc(func(request *http.Request, _ []*http.Request) error {
			if !allowRequest(request) {
				return http.ErrUseLastResponse
			}

			return nil
		}),
	)
}

func (c *client) Delete(ctx context.Context, request *Request) (*Response, error) {
	return c.do(ctx, DeleteRequest, request)
}

func (c *client) Get(ctx context.Context, request *Request) (*Response, error) {
	return c.do(ctx, GetRequest, request)
}

func (c *client) Patch(ctx context.Context, request *Request) (*Response, error) {
	return c.do(ctx, PatchRequest, request)
}

func (c *client) Post(ctx context.Context, request *Request) (*Response, error) {
	return c.do(ctx, PostRequest, request)
}

func (c *client) Put(ctx context.Context, request *Request) (*Response, error) {
	return c.do(ctx, PutRequest, request)
}

func (c *client) do(ctx context.Context, method string, request *Request) (*Response, error) {
	req, url, err := request.build()
	logger := c.logger.WithContext(ctx).WithFields(mon.Fields{
		"url":    url,
		"method": method,
	})

	if err != nil {
		logger.Error(err, "failed to assemble request")
		return nil, fmt.Errorf("failed to assemble request: %w", err)
	}

	req.SetContext(ctx)
	req.SetHeaders(c.defaultHeaders)

	if request.outputFile != nil {
		req.SetOutput(*request.outputFile)
	}

	c.writeMetric(metricRequest, method, mon.UnitCount, 1.0)
	resp, err := req.Execute(method, url)

	if errors.Is(err, context.Canceled) {
		return nil, err
	}

	// Only log an error if the error was not caused by a canceled context
	// Otherwise a user might spam our error logs by just canceling a lot of requests
	// (or many users spam us because sometimes they cancel requests)
	if err != nil {
		c.writeMetric(metricError, method, mon.UnitCount, 1.0)
		return nil, fmt.Errorf("failed to perform %s request to %s: %w", request.restyRequest.Method, request.url.String(), err)
	}

	metricName := fmt.Sprintf("%s%dXX", metricResponseCode, resp.StatusCode()/100)
	c.writeMetric(metricName, method, mon.UnitCount, 1.0)

	response := buildResponse(resp)

	// Only log the duration if we did not get an error.
	// If we get an error, we might not actually have send anything,
	// so the duration will be very low. If we get back an error (e.g., status 500),
	// we log the duration as this is just a valid http response.
	requestDurationMs := float64(resp.Time() / time.Millisecond)
	c.writeMetric(metricRequestDuration, method, mon.UnitMillisecondsAverage, requestDurationMs)

	return response, nil
}

func (c *client) writeMetric(metricName string, method string, unit string, value float64) {
	c.mo.WriteOne(&mon.MetricDatum{
		Priority:   mon.PriorityHigh,
		Timestamp:  time.Now(),
		MetricName: metricName,
		Dimensions: mon.MetricDimensions{
			"Method": method,
		},
		Unit:  unit,
		Value: value,
	})
}

func buildResponse(resp *resty.Response) *Response {
	return &Response{
		Body:            resp.Body(),
		Cookies:         resp.Cookies(),
		Header:          resp.Header(),
		StatusCode:      resp.StatusCode(),
		RequestDuration: resp.Time(),
	}
}
