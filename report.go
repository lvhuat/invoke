package invoke

import (
	"net/http"
	"strconv"
	"time"

	"github.com/lvhuat/monitor"
)

func (client *client) reportErrorToMonitor(code string, beginTime time.Time) {
	infc := "ACTIVE_" + client.method + "_" + client.path //ACTIVE表示主调
	//请求失败，上报失败计数和失败平均耗时
	timeNow := time.Now()
	var failedCountReport monitor.ReqFailedCountDimension
	failedCountReport.SName = monitor.GetCurrentServerName()
	failedCountReport.TName = client.service.Name()
	failedCountReport.TIP = client.host
	failedCountReport.Code = code
	failedCountReport.Infc = infc
	monitor.ReportReqFailed(&failedCountReport)

	var failedAvgTimeReport monitor.ReqFailedAvgTimeDimension
	failedAvgTimeReport.SName = monitor.GetCurrentServerName()
	failedAvgTimeReport.SIP = monitor.GetCurrentServerIP()
	failedAvgTimeReport.TName = client.service.Name()
	failedAvgTimeReport.TIP = client.host
	failedAvgTimeReport.Infc = infc
	monitor.ReportFailedAvgTime(&failedAvgTimeReport, (timeNow.UnixNano()-beginTime.UnixNano())/1e3) //耗时单位为微秒
}

func (client *client) reportSuccessToMonitor(beginTime time.Time) {
	infc := "ACTIVE_" + client.method + "_" + client.path //ACTIVE表示主调
	//请求失败，上报失败计数和失败平均耗时
	timeNow := time.Now()
	var succCountReport monitor.ReqSuccessCountDimension
	succCountReport.SName = monitor.GetCurrentServerName()
	succCountReport.SIP = monitor.GetCurrentServerIP()
	succCountReport.TName = client.service.Name()
	succCountReport.TIP = client.host
	succCountReport.Infc = infc
	monitor.ReportReqSuccess(&succCountReport)

	var succAvgTimeReport monitor.ReqSuccessAvgTimeDimension
	succAvgTimeReport.SName = monitor.GetCurrentServerName()
	succAvgTimeReport.SIP = monitor.GetCurrentServerIP()
	succAvgTimeReport.TName = client.service.Name()
	succAvgTimeReport.TIP = client.host
	succAvgTimeReport.Infc = infc
	monitor.ReportSuccessAvgTime(&succAvgTimeReport, (timeNow.UnixNano()-beginTime.UnixNano())/1e3) //耗时单位为微秒
}

//处理Response函数的http请求结果monitor上报
func (client *client) processResponseMonitorReport(resp *http.Response, beginTime time.Time) {
	if monitor.EnableReportMonitor() == false {
		return
	}

	if nil == resp {
		//请求失败，上报失败计数和失败平均耗时
		client.reportErrorToMonitor("-1", beginTime) //code暂时取"-1"
	} else {
		//把beginTime，infc，TName放入resp的header中，由ExtractHttpResponse取上报失败或成功
		infc := "ACTIVE_" + client.method + "_" + client.path //ACTIVE表示主调
		resp.Header.Set("Infc", infc)
		resp.Header.Set("TName", client.service.Name())
		resp.Header.Set("Endpoint", client.host) //请求的IP:Port，或者一个domain:Port/domain
		resp.Header.Set("BeginTime", strconv.FormatInt(beginTime.UnixNano()/1e3, 10))
	}
}

//处理Exec函数http请求结果的monitor上报
func (client *client) processExecMonitorReport(code int, err error, beginTime time.Time) {
	if monitor.EnableReportMonitor() == false {
		return
	}

	if nil != err {
		//请求失败，上报失败计数和失败平均耗时
		client.reportErrorToMonitor(strconv.FormatInt(int64(code), 10), beginTime)
	} else {
		//请求成功，上报成功计数和成功平均耗时
		client.reportSuccessToMonitor(beginTime)
	}
}
