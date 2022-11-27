package cmds

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/spf13/cobra"
	"strings"
)


//用于保存 各个组件的内容
type globalComp struct {
	App *tview.Application
	POD  *tview.List
	Detail *tview.TextView
	List *tview.List
	Header *tview.TextView
	Flex *tview.Flex
}
var global =&globalComp{}
func GetGlobalComp() *globalComp{
	return global
}
//全局显示的标题
const Title="程序员在囧途(www.jtthink.com)沉浸式k8s插件(deployment)"


var viewCmd= &cobra.Command{
	Use:          "view",
	Short:        "view deploys ",
	Example:      "kubectl deploys view",
	SilenceUsage: true,
	RunE: func(c *cobra.Command, args []string) error {
		app:=tview.NewApplication()
		newPrimitive := func(text string) tview.Primitive {
			return tview.NewTextView().
				SetTextAlign(tview.AlignCenter).
				SetText(text)
		}



		detail:=DetailText(app) //中间部分
		detail.SetTitle("yaml详情")
		list:=RenderTViewDeploy(c,app) //渲染的列表   在最左边

		 pods :=RenderTViewPod(app)
		 pods.SetTitle("POD列表")
		 pods.SetSecondaryTextColor(tcell.Color18)


		//这里很重要, 必须要注册进去。
		global.Detail=detail
		global.List=list
		global.POD=pods
		global.App=app

		//设置头
		header:=tview.NewTextView().SetTextAlign(tview.AlignCenter).SetTitle(Title).
			SetBorder(true)

		flex:=tview.NewFlex().
			AddItem(list, 20, 1, true).
			AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
				AddItem(header, 0, 1, false).
				AddItem(detail, 0, 3, false).
				AddItem(newPrimitive("事件和说明"), 5, 1, false), 0, 2, false).
			AddItem(pods, 20, 1, false)

		global.Flex=flex//这句话很重要 ,其他地方要调用的话 必须要保存起来

		app.SetRoot(flex, true)
		app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			// key是字母
			if event.Key() == tcell.KeyRune {
				if l,ok:=app.GetFocus().(*tview.List);ok{
					if strings.Index(l.GetTitle(),"POD")>=0{
						if event.Rune() == 'd' {
							DeletePOD(c,app)
						}
					}
					//焦点在deploy列表
					if strings.Index(l.GetTitle(),"deploys")>=0{
						if event.Rune() == 'r' {
							ScaleDeployForView(c,app)
						}
					}

				}

			}
			return event
		})
		if err := app.EnableMouse(true).Run(); err != nil {
			panic(err)
		}
		return nil
	},

}