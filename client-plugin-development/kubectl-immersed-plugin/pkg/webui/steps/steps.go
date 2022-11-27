package steps

var CurrentUser string
var StepsData Steps
//临时代码
func init() {
	CurrentUser="guest"
	StepsData=make(Steps)
}
type Steps map[string]string
func(s Steps) SetStep(step string){
	s[CurrentUser]=step
}
const (
	StepDeployCreate="deploy_create"
	StepDeployList="deploy_list"
)
func(s Steps) GetStep() string {
	if v,ok:=s[CurrentUser];ok{
		return v
	}
	return ""
}
