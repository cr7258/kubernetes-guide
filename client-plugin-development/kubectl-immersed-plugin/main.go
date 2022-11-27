package main

import (
	"depplugin/pkg/cmds"
	"depplugin/pkg/utils"
	"log"
	"os"
	"os/signal"
)

//本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
func main() {
	 // 把上节课代码做了封装，防止显示太乱
	quit := make(chan os.Signal)
	go func() {
		defer func() {
			if e:=recover();e!=nil{
				log.Println(e)
				quit<-os.Interrupt
			}
		}()
	//	 webui.StartWeb()    不要管这边
	}()
	go func() {
		defer func() {
			utils.ResetSTTY()
			quit<-os.Interrupt
		}()
		cmds.RunCmd()
	}()
	signal.Notify(quit, os.Interrupt)
	<- quit
	os.Exit(0)




	//本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
}
