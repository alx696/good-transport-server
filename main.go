package main

import (
	"encoding/json"
	"flag"
	"good-transport-server/http_server"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

type CallbackImpl struct {
}

type FileStart struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type FileProgress struct {
	Id   string `json:"id"`
	Size int64  `json:"size"`
}

type Text struct {
	Id   string `json:"id"`
	Text string `json:"text"`
}

func (impl CallbackImpl) Log(txt string) {
	log.Println("日志", txt)
	http_server.WebSocketPush("日志", txt)
}

func (impl CallbackImpl) Ready() {
	log.Println("就绪")
	http_server.WebSocketPush("就绪", "就绪")
}

func (impl CallbackImpl) Error(txt string) {
	log.Println("错误", txt)
	http_server.WebSocketPush("错误", txt)
}

func (impl CallbackImpl) UploadStart(fileId, fileName string, fileSize int64) {
	log.Println("上传开始", fileId, fileName, fileSize)
	info := FileStart{Id: fileId, Name: fileName, Size: fileSize}
	infoData, _ := json.Marshal(info)
	http_server.WebSocketPush("上传开始", string(infoData))
}

func (impl CallbackImpl) UploadProgress(fileId string, finishSize int64) {
	log.Println("上传进度", fileId, finishSize)
	info := FileProgress{Id: fileId, Size: finishSize}
	infoData, _ := json.Marshal(info)
	http_server.WebSocketPush("上传进度", string(infoData))
}

func (impl CallbackImpl) Text(id, text string) {
	log.Println("文本", id, text)
	info := Text{Id: id, Text: text}
	infoData, _ := json.Marshal(info)
	http_server.WebSocketPush("文本", string(infoData))
}

func main() {
	// 获取当前文件夹
	currentDir, e := filepath.Abs(filepath.Dir(os.Args[0]))
	if e != nil {
		log.Fatalln(e)
	}
	log.Println("当前文件夹", currentDir)

	// 根文件夹
	rootDirectory := flag.String("d", currentDir, "root directory")
	httpPort := flag.Int64("p", 1000, "http port")
	flag.Parse()
	log.Println("文件夹", *rootDirectory)
	log.Println("http端口", *httpPort)

	// 启动HTTP服务
	http_server.Start(
		*rootDirectory,
		*httpPort,
		CallbackImpl{},
	)

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGTERM, syscall.SIGINT)
	<-signalChan
	log.Println("收到关闭信号")

	// 停止HTTP服务
	http_server.Stop()
}
