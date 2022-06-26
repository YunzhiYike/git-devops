package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis"
	"github.com/spf13/viper"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"
)

type Server struct {
	redis *redis.Client
}

func (server *Server) initConfig(app *gin.Engine) {
	viper.AddConfigPath("./")
	viper.SetConfigName("config.ini")
	viper.SetConfigType("ini")
	err := viper.ReadInConfig()
	if err != nil {
		panic(err)
	}
	// redis初始化
	server.redis = redis.NewClient(&redis.Options{
		Addr:     viper.GetString("redis.host"), // redis地址
		Password: viper.GetString("redis.auth"), // redis密码，没有则留空
		DB:       viper.GetInt("redis.db"),      // 默认数据库，默认是0
	})

	f, _ := os.Create("gin.log")
	gin.DefaultWriter = io.MultiWriter(f)
	// 路由初始化
	app.GET("/api/git/command", func(context *gin.Context) {
		cx := context.Query("zl")
		fmt.Println(cx)
		res := viper.Get("cmd." + cx)
		if res != nil {
			server.redis.Publish("gitpush", res)
			context.String(http.StatusOK, "ok")
			return
		}
		context.String(http.StatusOK, "error")

	})
}

func Subscribe(redis *redis.Client) {
	log.Output(0, "启动redis订阅....")
	sub := redis.Subscribe("gitpush")
	for msg := range sub.Channel() {
		timeStr := time.Now().Format("2006-01-02 15:04:05")
		execStr := msg.Payload
		//execStr := "cd " + msg.Payload + " && git pull"
		res, err := exec.Command("/bin/sh", "-c", execStr).Output()
		if err != nil {
			log.Println("error: " + err.Error())
		}
		GitNoticeToWx(execStr, string(res), timeStr, err)
	}
}

func WxNotice(text, errs string) {
	host := viper.GetString("wx.webhook")
	cx := make(map[string]interface{})
	content := make(map[string]interface{})
	cx["msgtype"] = "markdown"
	cx["markdown"] = content
	timeStr := time.Now().Format("2006-01-02 15:04:05")
	errMsg := "<font color=\"info\">服务运行成功 [ok]</font>"
	if errs != "" {
		errMsg = "<font color=\"warning\">服务运行异常 [" + errs + "]</font>"
	}
	content["content"] = "【Git自动化部署服务】\n" +
		">执行结果: <font color=\"comment\">" + text + "</font>\n" +
		">执行时间：" + timeStr + "\n" +
		">状态：" + errMsg + "\n"
	res, _ := json.Marshal(cx)
	resp, err := http.Post(host, "application/json", bytes.NewBuffer([]byte(res)))
	if err != nil {
		log.Println(err.Error())
	}
	defer resp.Body.Close()
	resulst, _ := ioutil.ReadAll(resp.Body)
	log.Println(string(resulst))
}

func GitNoticeToWx(command, context, time string, errs error) {
	host := viper.GetString("wx.webhook")
	cx := make(map[string]interface{})
	content := make(map[string]interface{})
	cx["msgtype"] = "markdown"
	errMsg := "<font color=\"info\">执行成功 [ok]</font>"
	if errs != nil {
		errMsg = "<font color=\"warning\">执行失败 [" + errs.Error() + "]</font>"
	}
	content["content"] = "【Git自动化部署】\n```shell \n" + command + "\n```\n" +
		">执行结果: <font color=\"comment\">" + context + "</font>\n" +
		">执行时间：" + time + "\n" +
		">状态：" + errMsg + "\n"
	cx["markdown"] = content
	res, _ := json.Marshal(cx)
	resp, err := http.Post(host, "application/json", bytes.NewBuffer([]byte(res)))
	if err != nil {
		log.Println(err.Error())
	}
	defer resp.Body.Close()
	resulst, _ := ioutil.ReadAll(resp.Body)
	log.Println(string(resulst))

}

func (server *Server) servStart(app *gin.Engine) {
	server.initConfig(app)
	defer func() {
		err := recover()
		if err != nil {
			WxNotice("服务运行时失败", string(err.([]byte)))
		}
	}()
	go func() {
		Subscribe(server.redis)
	}()
	WxNotice("SERVER RUNTIME SUCCESS", "")
	log.Fatal(app.Run("0.0.0.0:7799"))

}

func main() {
	r := gin.Default()
	serv := new(Server)
	serv.servStart(r)
}
