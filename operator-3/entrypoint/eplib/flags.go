package eplib

var waitFile string        //等待 这个文件存在才能运行
var out string             //执行结果输出到哪  默认输出到当前目录下的out文件
var command string         //入口
var args string            //参数
var waitFileContent string // 当 等待文件 存在时 ，还要同时判断内容
var quitContent string     //如果waitFileContent设置了值， 当waitFile内容==quitContent时，则退出程序

var encodefile string //这代表是加密文件 ,有这个参数 则无视 command 和args
