package http_server

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/alx696/go-less/lilu_net"
)

// 获取可用端口并在保存后返回
func getNew(confPath string) (int, error) {
	// 获取端口
	freePort, e := lilu_net.GetFreePort()
	if e != nil {
		return 0, fmt.Errorf("获取HTTP端口出错: %s", e.Error())
	}

	// 保存端口
	confFile, e := os.OpenFile(confPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if e != nil {
		return 0, fmt.Errorf("创建HTTP配置文件出错: %s", e.Error())
	}
	_, e = confFile.Write([]byte(strconv.FormatInt(int64(freePort), 10)))
	if e != nil {
		return 0, fmt.Errorf("保存HTTP配置文件出错: %s", e.Error())
	}

	return freePort, nil
}

// 获取可用端口(如果之前端口可用则返回之前端口)
func getPort(dir string) (int, error) {
	confPath := filepath.Join(dir, "port.txt")
	_, e := os.Stat(confPath)
	if e != nil {
		return getNew(confPath)
	}

	// 读取端口
	portData, e := os.ReadFile(confPath)
	if e != nil {
		return 0, fmt.Errorf("读取HTTP配置文件出错: %s", e.Error())
	}
	confPort, e := strconv.ParseInt(string(portData), 10, 0)
	if e != nil {
		return 0, fmt.Errorf("HTTP配置文件端口不是数字: %s", e.Error())
	}
	port := int(confPort)

	// 检查端口是否已被使用
	if lilu_net.CheckPortFree(port) {
		return port, nil
	}
	return getNew(confPath)
}
