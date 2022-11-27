package webui

import (
	"embed"
	"github.com/gin-gonic/gin"
	"github.com/shenyisyn/goft-gin/goft"
	"html/template"
	"io/fs"
	"net/http"
)

//go:embed templates static
var FS embed.FS

func initFuncMap(server *gin.Engine,tpl *template.Template)  {

}
// 启动内置 GIN
func StartWeb()  {

	//创建 GInSERVER
	server:=goft.Ignite().Mount("",NewWebCtl())
	gin.SetMode(gin.ReleaseMode)
	// 模板配置
	tpls := template.Must(template.New("").
		ParseFS(FS, "templates/*.html"))
	server.SetHTMLTemplate(tpls)
	initFuncMap(server.Engine,tpls)
	//静态文件配置
	fe, _ := fs.Sub(FS, "static") //静态资源处理 ，放图片 等
	server.StaticFS("/static", http.FS(fe))

	server.Launch()//默认是 8080

}

//本地测试用。 因为用了embed 每次修改html 就要重启 。很麻烦
func StartWebLocal()  {

	//创建 GInSERVER
	server:=goft.Ignite().Mount("",NewWebCtl())

	// 模板配置
	tpls := template.Must(template.New("main").
		Funcs(server.FuncMap).ParseGlob("pkg/webui/templates/*"))
	initFuncMap(server.Engine,tpls)
	server.SetHTMLTemplate(tpls)


	//静态文件配置
	server.Static("/static", "./pkg/webui/static")

	server.Launch()//默认是 8080

}