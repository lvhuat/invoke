package invoke

import (
	"errors"
	"sync"
	"time"

	"github.com/afex/hystrix-go/hystrix"
)

var eng Engine = newEngine()
var defaultParseRsp = extractHttpResponse

// Init 初始化
func Init(option *Option) error {
	doLogger = option.DoLogger
	doLoggerParam = option.DoLoggerParam
	if option.ParseRsp != nil {
		defaultParseRsp = extractHttpResponse
	}

	if option.UseCircuit {
		switch {
		case option.DefaultTimeout == 0:
			option.DefaultTimeout = time.Minute
		case option.DefaultTimeout < 0:
			panic("bad timeout")
		}

		switch {
		case option.DefaultMaxConcurrentRequests == 0:
			option.DefaultMaxConcurrentRequests = 2000
		case option.DefaultMaxConcurrentRequests < 0:
			panic("bad max concurrent")
		}

		switch {
		case option.DefaultErrorPercentThreshold == 0:
			option.DefaultErrorPercentThreshold = 20
		case option.DefaultErrorPercentThreshold < 0 || option.DefaultErrorPercentThreshold > 100:
			panic("bad error threshold")
		}
	}

	return eng.Init(option)
}

// ErrDiscoveryNotConfig 出现在没有设置服务发现函数
var ErrDiscoveryNotConfig = errors.New("discovery not config")

// Engine 提供了向服务发送请求的入口
type engine struct {
	dv            DiscoveryFunc
	serviceMap    map[string]Service
	mutex         sync.RWMutex
	lbMode        string
	useTracing    bool
	useCircuit    bool
	circuitConfig hystrix.CommandConfig
}

// Init 初始化引擎
func (engine *engine) Init(option *Option) error {
	engine.dv = option.Discovery
	engine.lbMode = option.LoadBalanceMode
	engine.useTracing = option.UseTracing
	engine.useCircuit = option.UseCircuit

	engine.circuitConfig.ErrorPercentThreshold = option.DefaultErrorPercentThreshold
	engine.circuitConfig.MaxConcurrentRequests = option.DefaultMaxConcurrentRequests
	engine.circuitConfig.Timeout = int(option.DefaultTimeout / time.Millisecond)

	return nil
}

// Service 获取一个服务
func (engine *engine) Service(name string) Service {
	engine.mutex.RLock()
	service, exsit := engine.serviceMap[name]
	engine.mutex.RUnlock()

	if !exsit {
		service = engine.newService(name, engine.dv)
		engine.mutex.Lock()
		engine.serviceMap[name] = service
		engine.mutex.Unlock()
	}

	return service
}

// Addr 获取一个匿名服务
func (engine *engine) Addr(addr string) Service {
	return engine.newAddr(addr)
}

// newAddr 创建一个服务
func (engine *engine) newService(serviceName string, discovery DiscoveryFunc) Service {
	return &service{
		discovery:     discovery,
		name:          serviceName,
		useTracing:    engine.useTracing,
		useCircuit:    engine.useCircuit,
		circuitConfig: engine.circuitConfig,
	}
}

// newAddr 创建固定IP的匿名服务
func (engine *engine) newAddr(addr string) Service {
	discovery := func(string) ([]string, []string, error) {
		return []string{addr}, []string{addr}, nil
	}
	return &service{
		discovery:     discovery,
		name:          addr,
		useTracing:    engine.useTracing,
		useCircuit:    engine.useCircuit,
		circuitConfig: engine.circuitConfig,
	}
}

func newEngine() *engine {
	return &engine{
		dv: func(name string) ([]string, []string, error) {
			return nil, nil, ErrDiscoveryNotConfig
		},
		serviceMap: make(map[string]Service, 10),
	}
}
