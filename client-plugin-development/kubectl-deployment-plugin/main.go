package main

import (
	"depplugin/pkg/cmds"
	"depplugin/pkg/utils"

)

//本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
func main() {
	 // 把上节课代码做了封装，防止显示太乱
	defer utils.ResetSTTY()
    cmds.RunCmd()



	//本课程来自 程序员在囧途(www.jtthink.com) 咨询群：98514334
}
