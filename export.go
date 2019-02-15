package invoke

import (
	"context"
	"net/http"
	"time"

	"github.com/lvhuat/code"
)

var (
	doLogger      = true
	doLoggerParam = false
)

// DiscoveryFunc 服务发现的函数
type DiscoveryFunc func(name string) ([]string, []string, error)

// Option 用于初始化引擎的参数
type (
	Option struct {
		Discovery       DiscoveryFunc // 服务发现函数
		LoadBalanceMode string        // 保留
		UseTracing      bool          // 保留

		// 日志配置
		DoLogger      bool // 开关日志
		DoLoggerParam bool // 打印请求参数，包括query,route,header

		// 容错配置
		UseCircuit                   bool          // 开关容错
		DefaultTimeout               time.Duration // 通用请求超时
		DefaultMaxConcurrentRequests int           // 通用请求并发度
		DefaultErrorPercentThreshold int           // 通用错误容限（%）

		// 结果解析
		ParseRsp ParseRspFunc
	}

	// Engine 引擎
	Engine interface {
		Service(string) Service // 获取一个服务
		Addr(string) Service    // 创建一个匿名服务
		Init(*Option) error     // 初始化
	}

	// Service 服务
	Service interface {
		Get(string) Client            // GET
		Post(string) Client           // POST
		Put(string) Client            // PUT
		Delete(string) Client         // DELETE
		Method(string, string) Client // 自定义方法

		Name() string // 服务名称
		UseTracing() bool
		UseCircuit() bool
		Remote() (string, string, error) // 获取一个服务地址和ID
	}

	// Client 客户端
	Client interface {
		// 请求内容
		Headers(map[string]string) Client    // 添加头部
		Header(string, string) Client        // 添加头部
		Query(string, string) Client         // 添加查询参数
		QueryArray(string, ...string) Client // 添加查询参数
		Queries(map[string][]string) Client  // 添加查询参数
		Route(string, string) Client         // 添加路径参数
		Routes(map[string]string) Client     // 添加路径参数
		Json(interface{}) Client             // 添加Json消息体
		Body(string, []byte) Client          // 添加byte消息体
		Tls() Client                         // 使用HTTPS

		// 容错
		Hystrix(timeOutMillisecond, maxConn, thresholdPercent int) Client // 添加熔断参数（废弃）
		Timeout(time.Duration) Client                                     // 超时
		MaxConcurrent(int) Client                                         // 最大并发
		ErrorThreshold(int) Client                                        // 容错门限（%）
		Fallback(func(error) error) Client                                // 失败触发器

		// 获取结果
		Exec(interface{}) (int, error)                                // 执行请求
		Response() (*http.Response, error)                            // 执行请求，返回标准的http.Response
		Result(interface{}) code.Error                                // 执行请求，并且读取其中的数据，获得返回对象
		ResultByParse(parse ParseRspFunc, out interface{}) code.Error // 执行请求，并且使用自定义解析器，获得返回对象

		// 其他
		Context(context.Context) Client // 上下文（预留）
	}
)

// Name 返回服务器实例
func Name(name string) Service {
	return eng.Service(name)
}

// Addr 返回一个临时服务
func Addr(addr string) Service {
	return eng.Addr(addr)
}
