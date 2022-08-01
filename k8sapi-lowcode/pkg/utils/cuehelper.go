package utils

import (
	"bytes"
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/encoding/openapi"
	"encoding/json"
	"fmt"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gin-gonic/gin"
	"github.com/shenyisyn/goft-gin/goft"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	"strings"
	"text/template"
)

func extractParameterDefinitionNodeFromInstance(inst *cue.Instance, rule string) ast.Node {
	opts := []cue.Option{cue.All(), cue.DisallowCycles(true),
		cue.ResolveReferences(true), cue.Docs(true)}
	node := inst.Value().Syntax(opts...)
	if fileNode, ok := node.(*ast.File); ok {
		for _, decl := range fileNode.Decls {
			if field, ok := decl.(*ast.Field); ok {
				if label, ok := field.Label.(*ast.Ident); ok && label.Name == "#"+rule {
					return decl.(*ast.Field).Value
				}
			}
		}
	}
	paramVal := inst.LookupDef(rule)
	return paramVal.Syntax(opts...)
}

func RefineParameterInstance(inst *cue.Instance, rule string) (*cue.Instance, error) {
	r := cue.Runtime{}
	paramVal := inst.LookupDef(rule)
	var paramOnlyStr string
	switch k := paramVal.IncompleteKind(); k {
	case cue.StructKind, cue.ListKind:
		paramSyntax, _ := format.Node(extractParameterDefinitionNodeFromInstance(inst, rule))
		paramOnlyStr = fmt.Sprintf("#%s: %s\n", rule, string(paramSyntax))
	case cue.IntKind, cue.StringKind, cue.FloatKind, cue.BoolKind:
		paramOnlyStr = fmt.Sprintf("#%s: %v", rule, paramVal)
	case cue.BottomKind:
		paramOnlyStr = fmt.Sprintf("#%s: {}", rule)
	default:
		return nil, fmt.Errorf("unsupport  kind: %s", k.String())
	}
	paramOnlyIns, err := r.Compile("-", paramOnlyStr)
	if err != nil {
		return nil, err
	}
	return paramOnlyIns, nil
}

func GenOpenAPI(inst *cue.Instance, rule string, opts ...FillOption) ([]byte, error) {
	if inst.Err != nil {
		return nil, inst.Err
	}
	paramOnlyIns, err := RefineParameterInstance(inst, rule) // 返回的就是一个Instance
	if err != nil {
		return nil, err
	}
	//fmt.Println(paramOnlyIns.Value().LookupPath(cue.ParsePath("#input")))

	for _, fill := range opts {
		paramOnlyIns = fill(paramOnlyIns)
	}
	//fmt.Println(paramOnlyIns.Value().LookupPath(cue.ParsePath("#input")))
	//return []byte{}, nil
	b, err := openapi.Gen(paramOnlyIns, &openapi.Config{})
	if err != nil {
		return nil, err
	}
	var out = &bytes.Buffer{}
	_ = json.Indent(out, b, "", "   ")
	return out.Bytes(), nil
}

//新增的函数
type FillOption func(inst *cue.Instance) *cue.Instance

// 把文件解析为 cue.value
func MustParseFileToCueValue(path string) cue.Value {
	cc := cuecontext.New()
	cv := cc.CompileBytes(MustLoadFile(path))
	if err := cv.Err(); err != nil {
		panic(err)
	}
	return cv
}

// 渲染cue的指定规则节点
func PareseCue(file, param string, opts ...FillOption) gin.H {
	binst := load.Instances([]string{file}, nil)
	inst := cue.Build(binst)[0] //取第一个 。   暂时先这样

	b, err := GenOpenAPI(inst, param, opts...)
	goft.Error(err)
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(b)
	goft.Error(err)
	//这里注意：要重新取一次 实例，因为上面已经丢失了 其他规则，譬如 uiSchema 是不存在的
	uiSchema := make(map[string]interface{})
	inst = cue.Build(binst)[0]
	goft.Error(inst.Value().LookupPath(cue.ParsePath("uiSchema")).Decode(&uiSchema))
	return gin.H{
		"code": 20000,
		"data": gin.H{
			"schemas":  doc.Components.Schemas,
			"uiSchema": uiSchema,
		},
	}
}

// 吧字符串  譬如 k8s.jtthink.com_v1_fastnginxs
// 转为k8s gvr 。没啥特别的 就是切割字符串， 毫无技术含量
func ConvertToGvr(gvr string) schema.GroupVersionResource {
	gvrList := strings.Split(gvr, "_")
	if len(gvrList) != 3 {
		panic("error gvr")
	}
	return schema.GroupVersionResource{
		Group: gvrList[0], Version: gvrList[1], Resource: gvrList[2],
	}
}

// 预先写好的填充 命名空间的函数
func WithNameSpaceInject(fact informers.SharedInformerFactory, lookuppath string, fillpath string) FillOption {
	return func(inst *cue.Instance) *cue.Instance {

		nsList, err := fact.Core().V1().Namespaces().Lister().List(labels.Everything())
		goft.Error(err)

		nsTpl := `
        #namespaces: "" {{ range . }} | "{{ .Name }}"  {{ end }} 
`
		tpl := template.New("ns")
		var tplResult bytes.Buffer
		goft.Error(template.Must(tpl.Parse(nsTpl)).Execute(&tplResult, nsList))
		//首先查找母节点  ,自动拼凑了 #  因此外面传的时候 不要加 #
		source := inst.Value().LookupPath(cue.ParsePath("#" + lookuppath))

		//编译 动态数据
		v := inst.Value().Context().CompileString(tplResult.String())
		v = v.LookupPath(cue.ParsePath("#namespaces"))
		//灌入数据
		newV := source.FillPath(cue.ParsePath(fillpath), v)

		//  由于cue.Value 转 cue.Instance 比较麻烦 。所以采用了
		// 先把cue.Value 变成 string  . 再使用 cue.Compile 变成Instance
		toStr, err := CueValueToString(newV)
		goft.Error(err)
		//重新组装
		newVstr := fmt.Sprintf("{#%s: { %s } }", lookuppath, toStr)

		// 这里是直接 用字符串模式生成  cue实例
		var r cue.Runtime
		newInst, err := r.Compile("-", newVstr)
		goft.Error(err)
		return newInst
	}
}
