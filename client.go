package invoke

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/afex/hystrix-go/hystrix"
)

const (
	HttpHeaderContentType     = "Content-Type"
	HttpHeaderContentTypeJSON = "application/json"
)

type client struct {
	service      Service
	path         string
	createTime   time.Time
	errInProcess error

	method        string
	host          string
	scheme        string
	serverid      string
	circuitConfig hystrix.CommandConfig

	headers map[string]string
	queries map[string][]string
	routes  map[string]string
	payload func() ([]byte, error)

	logFields  map[string]interface{}
	ctx        context.Context
	useTracing bool
	useCircuit bool
	fallback   func(error) error
}

func (client *client) circuitName() string {
	return client.serverid
}

//未设置hytrix参数，或者参数不合理，使用默认熔断策略
func (client *client) hytrixCommand() string {
	return client.serverid + client.method + client.path
}

func (client *client) tracingName() string {
	return client.serverid
}

func (client *client) clear() {
	client.service = nil
	client.path = ""
	client.createTime = time.Unix(0, 0)
	client.errInProcess = nil
	client.headers = nil
	client.routes = nil
	client.queries = nil
	client.method = "GET"
	client.host = ""
	client.scheme = "http"
	client.payload = nil
	client.logFields = make(map[string]interface{}, 10)
	client.ctx = nil
}

func (client *client) Tls() Client {
	if client.errInProcess != nil {
		return client
	}

	client.scheme = "https"

	return client
}

func (client *client) Fallback(func(error) error) Client {
	if client.errInProcess != nil {
		return client
	}
	return client
}

func (client *client) Header(headerName, headerValue string) Client {
	if client.errInProcess != nil {
		return client
	}

	if client.headers == nil {
		client.headers = map[string]string{headerName: headerValue}
		return client
	}

	client.headers[headerName] = headerValue

	return client
}

func (client *client) Headers(headers map[string]string) Client {
	if client.errInProcess != nil {
		return client
	}

	if client.headers == nil {
		client.headers = make(map[string]string, len(headers))
	}

	for key, value := range headers {
		client.headers[key] = value
	}

	return client
}

func (client *client) Query(queryName, queryValue string) Client {
	if client.errInProcess != nil {
		return client
	}

	if client.queries == nil {
		client.queries = map[string][]string{
			queryName: []string{queryValue},
		}
		return client
	}

	queries := client.queries[queryName]
	queries = append(queries, queryValue)
	client.queries[queryName] = queries

	return client
}

func (client *client) QueryArray(queryName string, queryValues ...string) Client {
	if client.errInProcess != nil {
		return client
	}

	if client.queries == nil {
		client.queries = map[string][]string{
			queryName: queryValues,
		}
		return client
	}

	queries := client.queries[queryName]
	queries = append(queries, queryValues...)
	client.queries[queryName] = queries

	return client
}

func (client *client) Queries(queryValues map[string][]string) Client {
	if client.errInProcess != nil {
		return client
	}

	if client.queries == nil {
		client.queries = make(map[string][]string, len(queryValues))
		return client
	}

	for key, queryValueSlice := range queryValues {
		queries := client.queries[key]
		queries = append(queries, queryValueSlice...)
		client.queries[key] = queries
	}

	return client
}

func (client *client) Route(routeName, routeTo string) Client {
	if client.errInProcess != nil {
		return client
	}

	if client.routes == nil {
		client.routes = map[string]string{routeName: routeTo}
		return client
	}

	client.routes[routeName] = routeTo

	return client
}

func (client *client) Routes(routes map[string]string) Client {
	if client.errInProcess != nil {
		return client
	}

	if client.routes == nil {
		client.routes = make(map[string]string, len(routes))
	}

	for routeName, route := range routes {
		client.routes[routeName] = route
	}

	return client
}

func (client *client) Json(payload interface{}) Client {
	if client.errInProcess != nil {
		return client
	}

	client.Header(HttpHeaderContentType, HttpHeaderContentTypeJSON)

	client.payload = func() ([]byte, error) {
		return json.Marshal(payload)
	}

	return client
}

func (client *client) Body(contentType string, payload []byte) Client {
	if client.errInProcess != nil {
		return client
	}

	client.Header(HttpHeaderContentType, contentType)

	client.payload = func() ([]byte, error) {
		return payload, nil
	}

	return client
}

func (client *client) Context(ctx context.Context) Client {
	if client.errInProcess != nil {
		return client
	}

	client.ctx = ctx

	return client
}

func (client *client) Hystrix(timeOutMillisecond, maxConn, thresholdPercent int) Client {
	if client.errInProcess != nil {
		return client
	}

	client.circuitConfig.Timeout = timeOutMillisecond
	client.circuitConfig.MaxConcurrentRequests = maxConn
	client.circuitConfig.ErrorPercentThreshold = thresholdPercent

	return client
}

func (client *client) Timeout(timeout time.Duration) Client {
	if client.errInProcess != nil {
		return client
	}

	if timeout <= 0 {
		panic("bad timeout")
	}

	client.circuitConfig.Timeout = int(timeout / time.Millisecond)

	return client
}

func (client *client) MaxConcurrent(concurrenct int) Client {
	if client.errInProcess != nil {
		return client
	}

	if concurrenct <= 0 {
		panic("bad concurrenct")
	}

	client.circuitConfig.MaxConcurrentRequests = concurrenct

	return client
}

func (client *client) ErrorThreshold(threshold int) Client {
	if client.errInProcess != nil {
		return client
	}

	if threshold <= 0 {
		panic("bad threshold")
	}

	client.circuitConfig.ErrorPercentThreshold = threshold

	return client
}

func (client *client) Exec(out interface{}) (int, error) {
	beginTime := time.Now()
	var err error
	var status int
	if !client.useCircuit {
		status, err = client.exec(out, nil)
	} else {
		client.updateHystrix()

		var cancel context.CancelFunc
		err = hystrix.Do(client.hytrixCommand(), func() error {
			s, err := client.exec(out, &cancel)
			status = s
			return err
		}, client.fallback)
		if nil != err && nil != cancel {
			cancel()
		}
	}
	client.processExecMonitorReport(status, err, beginTime)

	if doLogger {
		fields := map[string]interface{}{
			"service":    client.service.Name(),
			"service_id": client.serverid,
			"method":     client.method,
			"path":       client.path,
			"endpoint":   client.host,
			"cost":       time.Since(client.createTime).String(),
		}

		if doLoggerParam {
			fields["headers"] = client.headers
			fields["queries"] = client.queries
			fields["routes"] = client.routes
			fields["payload"] = client.payload
		}

		if err != nil {
			fields["error"] = err.Error()
			log.Println("Invoke service failed", fields)
		} else {
			log.Println("Invoke service done", fields)
		}
	}

	return status, err
}

func (client *client) build() (*http.Request, error) {
	host, serviceNode, err := client.service.Remote()
	if err != nil {
		return nil, fmt.Errorf("discovery failed,%v", err)
	}

	client.host, client.serverid = host, serviceNode

	path, err := parsePath(client.path, client.routes)
	if err != nil {
		client.logFields["error"] = "routes parameter invalid"
		client.logFields["routes"] = client.routes
		return nil, err
	}

	if client.host == "" {
		client.logFields["error"] = "no avaliable remote"
		return nil, fmt.Errorf("remote is emtpy")
	}

	url, err := makeUrl(client.scheme, client.host, path, client.queries)

	if err != nil {
		client.logFields["scheme"] = client.scheme
		client.logFields["remote"] = client.host
		client.logFields["path"] = url
		client.logFields["queries"] = client.queries
		return nil, err
	}

	reader := &bytes.Reader{}
	if client.payload != nil {
		b, err := client.payload()
		if err != nil {
			return nil, err
		}
		client.logFields["payload"] = string(b)
		reader = bytes.NewReader(b)
	}

	request, err := http.NewRequest(client.method, url, reader)
	if err != nil {
		client.logFields["error"] = err
		return nil, fmt.Errorf("create http request failed,%v", err)
	}

	if client.ctx != nil {
		request = request.WithContext(client.ctx)
	}

	for headerKey, headerValue := range client.headers {
		request.Header.Add(headerKey, headerValue)
	}

	return request, nil
}

func (client *client) exec(out interface{}, cancel *context.CancelFunc) (int, error) {
	if client.errInProcess != nil {
		return 0, client.errInProcess
	}

	if nil != cancel && nil != client.ctx {
		client.ctx, *cancel = context.WithCancel(client.ctx)
	}
	request, err := client.build()
	if err != nil {
		return 0, err
	}

	cli := &http.Client{}
	resp, err := cli.Do(request)
	if err != nil {
		client.logFields["error"] = err
		return 0, err
	}

	client.logFields["status"] = resp.StatusCode
	client.logFields["status_code"] = resp.Status

	if resp.StatusCode < http.StatusOK ||
		resp.StatusCode >= http.StatusMultipleChoices {
		client.logFields["error"] = "response status error"
		return resp.StatusCode, fmt.Errorf("reponse with bad status,%d", resp.StatusCode)
	}

	rsp, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		client.logFields["error"] = err
		return resp.StatusCode, fmt.Errorf("Read response body failed")
	}
	defer resp.Body.Close()

	client.logFields["response_payload_len"] = len(rsp)

	err = json.Unmarshal(rsp, out)
	if err != nil {
		client.logFields["error"] = err
		client.logFields["content"] = string(cutBytes(rsp, 4096))
		return 0, fmt.Errorf("marshal result body failed")
	}

	return resp.StatusCode, nil
}

func (client *client) getResp(cancel *context.CancelFunc) (*http.Response, error) {
	if client.errInProcess != nil {
		return nil, client.errInProcess
	}

	if nil != cancel && nil != client.ctx {
		client.ctx, *cancel = context.WithCancel(client.ctx)
	}
	request, err := client.build()
	if err != nil {
		return nil, err
	}

	cli := &http.Client{}
	resp, err := cli.Do(request)
	if err != nil {
		client.logFields["error"] = err
		return nil, err
	}

	return resp, nil
}

func (client *client) updateHystrix() {
	hytrixCmd := client.hytrixCommand()
	//if _, exist, _ := hystrix.GetCircuit(hytrixCmd); exist {
	//	return
	//}

	hystrix.ConfigureCommand(hytrixCmd, client.circuitConfig)
}

func (client *client) Response() (*http.Response, error) {
	return client.response()
}

func (client *client) response() (*http.Response, error) {
	beginTime := time.Now()
	var err error
	var resp *http.Response
	if !client.useCircuit {
		resp, err = client.getResp(nil)
	} else {
		client.updateHystrix()

		var cancel context.CancelFunc
		err = hystrix.Do(client.hytrixCommand(), func() error {
			s, err := client.getResp(&cancel)
			resp = s
			return err
		}, client.fallback)
		if nil != err && nil != cancel {
			cancel() //cancel run client.getResp
		}
	}
	client.processResponseMonitorReport(resp, beginTime) //若resp为nil则上报错误，否则添加请求信息到header待进一步上报monitor数据

	if doLogger {
		fields := map[string]interface{}{
			"service":  client.service.Name(),
			"method":   client.method,
			"path":     client.path,
			"endpoint": client.host,
			"scheme":   client.scheme,
			"cost":     time.Since(client.createTime),
		}
		if client.service.Name() != client.serverid {
			fields["service_id"] = client.serverid
		}

		if doLoggerParam {
			if len(client.headers) != 0 {
				fields["headers"] = fmt.Sprintln(client.headers)
			}
			if len(client.queries) != 0 {
				fields["queries"] = fmt.Sprintln(client.queries)
			}
			if len(client.routes) != 0 {
				fields["routes"] = fmt.Sprintln(client.routes)
			}
			if client.payload != nil {
				pl := func() string {
					b, _ := client.payload()
					return string(b)
				}()

				if pl != "" && pl != "{}" {
					fields["payload"] = pl
				}
			}
		}

		if err != nil {
			fields["error"] = err.Error()
			log.Println("Invoke service failed", fields)
		} else {
			log.Println("Invoke service done", fields)
		}
	}

	return resp, err
}
