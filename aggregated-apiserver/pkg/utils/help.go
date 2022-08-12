package utils

import (
	"github.com/shenyisyn/aapi/pkg/apis/myingress/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	Table_ListColumns = []string{"Name", "Namespace", "Path", "Host"}
)

// 把列表 或单资源 变成表格化
func ConvertToTable(obj interface{}) *metav1.Table {
	t := &metav1.Table{}
	t.Kind = "Table"
	t.APIVersion = "meta.k8s.io/v1"

	//设置表头
	th := make([]metav1.TableColumnDefinition, len(Table_ListColumns))
	for i, h := range Table_ListColumns {
		th[i] = metav1.TableColumnDefinition{Name: h, Type: "string"}
	}
	t.ColumnDefinitions = th                       //设置表头
	if v, ok := obj.(*v1beta1.MyIngressList); ok { //代表取列表
		//设置 行  数据
		rows := make([]metav1.TableRow, len(v.Items))
		for i, item := range v.Items {
			rows[i] = metav1.TableRow{
				Cells: []interface{}{item.Name, item.Namespace, item.Spec.Path, item.Spec.Host},
			}
		}
		t.Rows = rows
	}
	if v, ok := obj.(*v1beta1.MyIngress); ok {
		rows := make([]metav1.TableRow, 1)
		rows[0] = metav1.TableRow{
			Cells: []interface{}{v.Name, v.Namespace, v.Spec.Path, v.Spec.Host},
		}
		t.Rows = rows
	}
	return t
}

func SuccessStatus(message string) *metav1.Status {
	status := &metav1.Status{
		Code:    200,
		Message: message,
		Status:  "Success",
	}
	status.Kind = "Status"
	status.APIVersion = "v1"
	return status
}

func ErrorStatus(code int32, message string, reason metav1.StatusReason) *metav1.Status {
	status := &metav1.Status{
		Code:    code,
		Message: message,
		Reason:  reason,
		Status:  "Failure",
	}
	status.Kind = "Status"
	status.APIVersion = "v1"
	return status
}

func NotFoundStatus(message string) *metav1.Status {
	status := &metav1.Status{
		Code:    404,
		Message: message,
		Reason:  metav1.StatusReasonNotFound,
		Status:  "Failure",
	}
	status.Kind = "Status"
	status.APIVersion = "v1"
	return status
}
