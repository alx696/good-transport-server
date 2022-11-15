package http_server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"good-transport-server/qc"
	"html/template"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alx696/go-less/lilu_net"
	"github.com/fasthttp/websocket"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
)

// Callback 回调
type Callback interface {
	// Log 日志
	Log(txt string)
	// Ready 就绪
	Ready(addr string)
	// Error 错误
	Error(txt string)
	// UploadStart 文件上传开始
	UploadStart(fileId, fileName string, fileSize int64)
	// UploadProgress 文件上传进度
	UploadProgress(fileId string, finishSize int64)
	// Text 文本
	Text(id string, text string)
}

type templateData struct {
}

type ServerInfo struct {
	HttpAddress   string `json:"http_address"`
	RootDirectory string `json:"root_directory"`
}

var sm sync.RWMutex
var wsUpgrader = websocket.FastHTTPUpgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	//跨域
	CheckOrigin: func(ctx *fasthttp.RequestCtx) bool {
		return true
	},
}
var wsConnMap = make(map[string]*websocket.Conn)
var rootDir, templateDir, fileDir string
var clientCallback Callback
var httpServer *fasthttp.Server
var httpAddress string

// 更新WebSocket连接
//
// conn 设为nil表示删除并关闭连接
func wsConnUpdate(id string, conn *websocket.Conn) {
	sm.Lock()
	defer sm.Unlock()
	oldConn, connExists := wsConnMap[id]
	if connExists {
		log.Println("WebSocket关闭连接", id)
		_ = oldConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "服务主动关闭"))
		_ = oldConn.Close()
		delete(wsConnMap, id)
	}
	if conn != nil {
		log.Println("WebSocket更新连接", id)
		wsConnMap[id] = conn
	}
}

// 首页
func rootHandler(ctx *fasthttp.RequestCtx) {
	// 获取当前文件夹
	currentDir, e := filepath.Abs(filepath.Dir(os.Args[0]))
	if e != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		_, _ = ctx.Write([]byte(fmt.Sprintf(`获取当前文件夹出错: %s`, e.Error())))
		return
	}
	log.Println("当前文件夹", currentDir)

	// 加载模板
	t, e := template.ParseFiles(filepath.Join(templateDir, "index.html"))
	if e != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		_, _ = ctx.Write([]byte(fmt.Sprintf(`加载模板失败: %s`, e.Error())))
		return
	}
	ctx.Response.Header.Set("Content-Type", "text/html; charset=utf-8")
	_ = t.Execute(ctx.Response.BodyWriter(), &templateData{})
}

// 服务信息
func serverInfoHandler(ctx *fasthttp.RequestCtx) {
	method := string(ctx.Method())

	if method != "GET" {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.SetBodyString("只能GET")
		return
	}

	data, _ := json.Marshal(ServerInfo{HttpAddress: httpAddress, RootDirectory: rootDir})

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody(data)
}

// 生成二维码
func qrcodeHandler(ctx *fasthttp.RequestCtx) {
	method := string(ctx.Method())

	if method != "GET" {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.SetBodyString("只能GET")
		return
	}

	text := string(ctx.QueryArgs().Peek("text"))
	name := string(ctx.QueryArgs().Peek("name"))
	if text == "" || name == "" {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.SetBodyString("缺少参数")
		return
	}

	filePath := path.Join(fileDir, name)
	e := qc.Encode(filePath, text, 256, 256)
	if e != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		ctx.SetBodyString(fmt.Sprintf("生成二维码错误: %s", e.Error()))
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBodyString(filePath)
}

// 订阅
func feedHandler(ctx *fasthttp.RequestCtx) {
	e := wsUpgrader.Upgrade(ctx, func(conn *websocket.Conn) {
		defer conn.Close()

		// 读取请求
		messageType, messageBytes, e := conn.ReadMessage()
		if e != nil || messageType != websocket.TextMessage {
			log.Println("订阅请求必须是文本: ", e)
			return
		}
		requestText := string(messageBytes)
		log.Println("订阅请求: ", requestText)
		clientID := uuid.New().String()

		// 保持连接, 检测连接断开
		var closeChan = make(chan error, 1)
		ticker := time.NewTicker(time.Second)
		go func() {
			for {
				<-ticker.C

				sm.RLock()
				e := conn.WriteMessage(websocket.PingMessage, nil)
				sm.RUnlock()
				if e != nil {
					closeChan <- e
					return
				}
			}
		}()

		// 缓存连接, 用于批量发送回调
		wsConnUpdate(clientID, conn)

		closeError := <-closeChan
		log.Println("订阅连接断开(ping报错)", clientID, closeError)

		// 移除连接
		wsConnUpdate(clientID, nil)
	})
	if e != nil {
		log.Println(e)
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
	}
}

// 发送文本
func textHandler(ctx *fasthttp.RequestCtx) {
	method := string(ctx.Method())

	if method != "POST" {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.SetBodyString("只能POST")
		return
	}

	text := string(ctx.FormValue("text"))
	if text == "" {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.SetBodyString("没有文本内容")
		return
	}

	id := uuid.New().String()
	log.Println("收到文本", id, text)

	clientCallback.Text(id, text)

	ctx.SetStatusCode(fasthttp.StatusOK)
}

// 开始上传文件
func uploadStartHandler(ctx *fasthttp.RequestCtx) {
	method := string(ctx.Method())

	if method != "POST" {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.SetBodyString("只能POST")
		return
	}

	fileName := string(ctx.FormValue("file_name"))
	if fileName == "" {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.SetBodyString("没有文件名称")
		return
	}

	fileSizeText := string(ctx.FormValue("file_size"))
	if fileSizeText == "" {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.SetBodyString("没有文件大小")
		return
	}
	fileSize, e := strconv.ParseInt(fileSizeText, 10, 64)
	if e != nil {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.SetBodyString("文件大小错误")
		return
	}
	log.Println("开始上传文件", fileName, fileSize)

	// 生成文件ID
	fileId := fmt.Sprint(uuid.New().String(), filepath.Ext(fileName))

	// 回调
	clientCallback.UploadStart(fileId, fileName, fileSize)

	// 返回
	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBodyString(fileId)
}

// 继续上传文件
func uploadBlockHandler(ctx *fasthttp.RequestCtx) {
	method := string(ctx.Method())

	if method != "POST" {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.SetBodyString("只能POST")
		return
	}

	fileId := string(ctx.FormValue("file_id"))
	if fileId == "" {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.SetBodyString("没有文件名称")
		return
	}
	filePath := filepath.Join(fileDir, fileId)

	fileBlockHeader, e := ctx.FormFile("file_block")
	if e != nil {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.SetBodyString(fmt.Sprintf(`获取文件块失败: %s`, e.Error()))
		return
	}

	fileBlockData, e := fileBlockHeader.Open()
	if e != nil {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.SetBodyString(fmt.Sprintf(`打开文件块失败: %s`, e.Error()))
		return
	}
	defer func() {
		_ = fileBlockData.Close()
	}()
	buffer := bytes.NewBuffer(nil)
	if _, e := io.Copy(buffer, fileBlockData); e != nil {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.SetBodyString(fmt.Sprintf(`读取文件块失败: %s`, e.Error()))
		return
	}

	//保存文件块
	finalFile, e := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if e != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		ctx.SetBodyString(fmt.Sprintf(`创建文件失败: %s`, e.Error()))
		return
	}
	defer func() {
		_ = finalFile.Close()
	}()

	_, e = finalFile.Write(buffer.Bytes())
	if e != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		ctx.SetBodyString(fmt.Sprintf(`保存文件块失败: %s`, e.Error()))
		return
	}

	finalFileInfo, _ := finalFile.Stat()
	clientCallback.UploadProgress(fileId, finalFileInfo.Size())

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBodyString("上传完成")
}

// Start 启动
//
// templateDirArg 模板文件夹, 需要把模板放到这个文件夹中
//
// fileDirArg 文件存放目录, 用来存放上传的文件
func Start(rootDirArg string, callbackArg Callback) {
	log.Println("启动HTTP", rootDirArg)
	rootDir = rootDirArg
	clientCallback = callbackArg
	clientCallback.Log(fmt.Sprintf("开始启动服务 根目录:%s", rootDirArg))

	// 检查模板文件夹
	templateDir = path.Join(rootDirArg, "template")
	_, e := os.Stat(templateDir)
	if e != nil {
		log.Println("模板文件夹不存在")
		clientCallback.Error("模板文件夹不存在")
		return
	}

	// 创建文件存放文件夹
	fileDir = path.Join(rootDirArg, "file")
	_, e = os.Stat(fileDir)
	if e != nil {
		e = os.MkdirAll(fileDir, os.ModePerm)
		if e != nil {
			log.Println("无法创建文件存放文件夹")
			clientCallback.Error("无法创建文件存放文件夹")
			return
		}
	}

	// 获取IP
	ip, e := lilu_net.GetIp()
	if e != nil {
		log.Println(e)
		clientCallback.Error(e.Error())
		return
	}

	// 获取端口
	port, e := getPort(rootDir)
	if e != nil {
		log.Println(e)
		clientCallback.Error(e.Error())
		return
	}

	requestHandler := func(ctx *fasthttp.RequestCtx) {
		//CORS
		ctx.Response.Header.Set("Access-Control-Allow-Origin", "*")
		ctx.Response.Header.Set("Access-Control-Allow-Methods", "GET, OPTIONS, POST, PUT, DELETE")
		ctx.Response.Header.Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization")
		ctx.Response.Header.Set("Access-Control-Expose-Headers", "x-name, x-size")
		//防止客户端发送OPTIONS时报错
		if string(ctx.Method()) == "OPTIONS" {
			return
		}

		//PATH
		switch string(ctx.Path()) {
		case "/":
			rootHandler(ctx)
		case "/server/info":
			serverInfoHandler(ctx)
		case "/qrcode":
			qrcodeHandler(ctx)
		case "/feed":
			feedHandler(ctx)
		case "/text":
			textHandler(ctx)
		case "/upload/start":
			uploadStartHandler(ctx)
		case "/upload/block":
			uploadBlockHandler(ctx)
		default:
			ctx.Error("路径无效", fasthttp.StatusNotImplemented)
		}
	}
	httpServer = &fasthttp.Server{
		Name: "File Service",
		// Other Server settings may be set here.
		MaxRequestBodySize: 1024 * 1024 * 16,
		Handler:            requestHandler,
	}
	go func() {
		e := httpServer.ListenAndServe(fmt.Sprintf(`:%d`, port))
		if e != nil {
			log.Println("服务启动错误", e.Error())
			clientCallback.Error(e.Error())
			return
		}
	}()

	httpAddress = fmt.Sprintf("%s:%d", ip, port)
	if strings.Contains(ip, ":") {
		httpAddress = fmt.Sprintf("[%s]:%d", ip, port)
	}

	clientCallback.Ready(httpAddress)
	clientCallback.Log(fmt.Sprintf("已经启动服务 %s", httpAddress))
}

// Stop 停止
func Stop() {
	clientCallback.Log(`开始关闭服务`)

	_ = httpServer.Shutdown()

	clientCallback.Log(`已经关闭服务`)
}

// WebSocketPush WebSocket推送
func WebSocketPush(c, t string) {
	m := map[string]interface{}{"c": c, "t": t}
	jsonBytes, _ := json.Marshal(m)
	log.Println("WebSocket推送", string(jsonBytes))

	sm.Lock()
	defer sm.Unlock()
	for id, conn := range wsConnMap {
		log.Println("WebSocket推送", id, string(jsonBytes))

		_ = conn.WriteMessage(websocket.TextMessage, jsonBytes)
	}
}
